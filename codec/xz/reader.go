// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xz

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"

	"github.com/lemon4ksan/foundation/codec/compress/lzma"
	"github.com/lemon4ksan/foundation/codec/filter/bcj"
)

// Reader implements an XZ stream decompressor.
type Reader struct {
	r          io.ByteReader
	check      CheckType
	headerDone bool
	eof        bool
	outBuf     []byte
}

// NewReader constructs an [io.ReadCloser] decompressing an XZ format stream from r.
func NewReader(r io.Reader) (*Reader, error) {
	var br io.ByteReader
	if b, ok := r.(io.ByteReader); ok {
		br = b
	} else {
		br = &byteReaderWrapper{r: r}
	}

	zr := &Reader{r: br}
	if err := zr.readHeader(); err != nil {
		return nil, err
	}
	return zr, nil
}

func (zr *Reader) readHeader() error {
	if zr.headerDone {
		return nil
	}
	zr.headerDone = true

	var magic [6]byte
	for i := range 6 {
		b, err := zr.r.ReadByte()
		if err != nil {
			return fmt.Errorf("read xz magic: %w", err)
		}
		magic[i] = b
	}
	if magic != HeaderMagic {
		return ErrInvalidHeaderMagic
	}

	var flags [2]byte
	for i := range 2 {
		b, err := zr.r.ReadByte()
		if err != nil {
			return err
		}
		flags[i] = b
	}

	var crcBuf [4]byte
	for i := range 4 {
		b, err := zr.r.ReadByte()
		if err != nil {
			return err
		}
		crcBuf[i] = b
	}
	expectedCRC := binary.LittleEndian.Uint32(crcBuf[:])
	actualCRC := crc32.ChecksumIEEE(flags[:])
	if expectedCRC != actualCRC {
		return ErrHeaderCRCMismatch
	}

	zr.check = CheckType(flags[1] & 0x0F)
	return nil
}

func (zr *Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	for len(zr.outBuf) == 0 && !zr.eof {
		if err := zr.readNextBlock(); err != nil {
			if errors.Is(err, io.EOF) {
				zr.eof = true
				break
			}
			return 0, err
		}
	}

	if len(zr.outBuf) == 0 && zr.eof {
		return 0, io.EOF
	}

	n := copy(p, zr.outBuf)
	zr.outBuf = zr.outBuf[n:]
	return n, nil
}

func (zr *Reader) readNextBlock() error {
	b0, err := zr.r.ReadByte()
	if err != nil {
		return err
	}

	// 1. Index indicator check (0x00 means we reached the XZ Index!)
	if b0 == 0x00 {
		return zr.readIndexAndFooter()
	}

	// 2. Block Header
	encodedHeaderSize := b0
	realHeaderSize := int(encodedHeaderSize+1) * 4

	headerBuf := make([]byte, realHeaderSize)
	headerBuf[0] = encodedHeaderSize
	for i := 1; i < realHeaderSize; i++ {
		b, err := zr.r.ReadByte()
		if err != nil {
			return err
		}
		headerBuf[i] = b
	}

	// Verify Header CRC32
	storedCRC := binary.LittleEndian.Uint32(headerBuf[realHeaderSize-4:])
	computedCRC := crc32.ChecksumIEEE(headerBuf[:realHeaderSize-4])
	if storedCRC != computedCRC {
		return ErrBlockCRCMismatch
	}

	// Parse Block Flags
	blockFlags := headerBuf[1]
	numFilters := int(blockFlags&0x03) + 1
	hasCompressedSize := (blockFlags & 0x40) != 0
	hasUncompressedSize := (blockFlags & 0x80) != 0

	pos := 2
	hr := bytes.NewReader(headerBuf[pos:])
	if hasCompressedSize {
		_, _ = ReadVLI(hr)
	}
	if hasUncompressedSize {
		_, _ = ReadVLI(hr)
	}

	var (
		useBCJ   bool
		dictSize uint32 = 8 * 1024 * 1024
	)

	for range numFilters {
		filterID, err := ReadVLI(hr)
		if err != nil {
			return err
		}
		propsSize, err := ReadVLI(hr)
		if err != nil {
			return err
		}
		props := make([]byte, propsSize)
		_, _ = hr.Read(props)

		switch filterID {
		case FilterX86:
			useBCJ = true
		case FilterLZMA2:
			if len(props) >= 1 {
				if ds, err := lzma.DictSizeFromProp(props[0]); err == nil {
					dictSize = ds
				}
			}
		default:
			return fmt.Errorf("%w: ID 0x%x", ErrUnsupportedFilter, filterID)
		}
	}

	// 3. Decompress LZMA2 block payload
	dec := lzma.NewDecompressor2(dictSize)
	decompData, compRead, err := dec.DecompressStream(zr.r.(io.Reader))
	if err != nil {
		return fmt.Errorf("decompress lzma2 block: %w", err)
	}

	// 3.1 Skip Block Padding (align compressed payload size to 4 bytes)
	padLen := (4 - (compRead % 4)) % 4
	for range padLen {
		if _, err := zr.r.ReadByte(); err != nil {
			return err
		}
	}

	// 4. Reverse Filter (BCJ)
	if useBCJ {
		bcj.Filter(bcj.X86, decompData, 0, false)
	}

	// 5. Verify Block Check
	checkSize := zr.check.CheckSize()
	if checkSize > 0 {
		var checkBuf [32]byte
		for i := range checkSize {
			b, err := zr.r.ReadByte()
			if err != nil {
				return err
			}
			checkBuf[i] = b
		}

		if zr.check == CheckCRC32 {
			expectedCheck := binary.LittleEndian.Uint32(checkBuf[:4])
			actualCheck := crc32.ChecksumIEEE(decompData)
			if expectedCheck != actualCheck {
				return ErrDataCheckMismatch
			}
		}
	}

	zr.outBuf = append(zr.outBuf, decompData...)
	return nil
}

func (zr *Reader) readIndexAndFooter() error {
	// Read Index (Number of blocks)
	numBlocks, err := ReadVLI(zr.r)
	if err != nil {
		return err
	}

	idxBytes := make([]byte, 0, 64)
	idxBytes = append(idxBytes, 0x00)
	var vliBuf [16]byte
	n, _ := WriteVLI(bytes.NewBuffer(vliBuf[:0]), numBlocks)
	idxBytes = append(idxBytes, vliBuf[:n]...)

	for range numBlocks {
		unpadded, err := ReadVLI(zr.r)
		if err != nil {
			return err
		}
		uncomp, err := ReadVLI(zr.r)
		if err != nil {
			return err
		}
		n, _ = WriteVLI(bytes.NewBuffer(vliBuf[:0]), unpadded)
		idxBytes = append(idxBytes, vliBuf[:n]...)
		n, _ = WriteVLI(bytes.NewBuffer(vliBuf[:0]), uncomp)
		idxBytes = append(idxBytes, vliBuf[:n]...)
	}

	// Read Index Padding to 4 bytes
	for (len(idxBytes)+4)%4 != 0 {
		b, err := zr.r.ReadByte()
		if err != nil {
			return err
		}
		idxBytes = append(idxBytes, b)
	}

	// Read Index CRC32
	var crcBuf [4]byte
	for i := range 4 {
		b, err := zr.r.ReadByte()
		if err != nil {
			return err
		}
		crcBuf[i] = b
	}
	expectedIdxCRC := binary.LittleEndian.Uint32(crcBuf[:])
	actualIdxCRC := crc32.ChecksumIEEE(idxBytes)
	if expectedIdxCRC != actualIdxCRC {
		return ErrIndexCRCMismatch
	}

	// Read 12-byte Stream Footer
	var footer [12]byte
	for i := range 12 {
		b, err := zr.r.ReadByte()
		if err != nil {
			return err
		}
		footer[i] = b
	}

	expectedFooterCRC := binary.LittleEndian.Uint32(footer[0:4])
	actualFooterCRC := crc32.ChecksumIEEE(footer[4:10])
	if expectedFooterCRC != actualFooterCRC {
		return ErrFooterCRCMismatch
	}

	if footer[10] != FooterMagic[0] || footer[11] != FooterMagic[1] {
		return ErrInvalidFooterMagic
	}

	return io.EOF
}

// Close closes the reader.
func (zr *Reader) Close() error {
	return nil
}

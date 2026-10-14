// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package xz

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"

	"github.com/lemon4ksan/seal/codec/compress/lzma"
	"github.com/lemon4ksan/seal/codec/filter/bcj"
)

// Writer implements an XZ format stream encoder.
type Writer struct {
	w           io.Writer
	opts        Options
	buf         bytes.Buffer
	closed      bool
	headerDone  bool
	indexBlocks []indexRecord
}

type indexRecord struct {
	unpaddedSize     uint64
	uncompressedSize uint64
}

// Options configures the XZ compressor.
type Options struct {
	DictSize uint32
	Level    lzma.Level
	Check    CheckType
	UseBCJ   bool
}

// DefaultOptions returns standard options for XZ encoding.
func DefaultOptions() Options {
	return Options{
		DictSize: 8 * 1024 * 1024, // 8MB dictionary
		Level:    lzma.LevelNormal,
		Check:    CheckCRC32,
		UseBCJ:   false,
	}
}

// NewWriter constructs a new XZ encoder streaming to w with default options.
func NewWriter(w io.Writer) (*Writer, error) {
	return NewWriterOptions(w, DefaultOptions())
}

// NewWriterOptions constructs an XZ encoder configured with custom Options.
func NewWriterOptions(w io.Writer, opts Options) (*Writer, error) {
	if w == nil {
		return nil, errors.New("xz: nil writer")
	}
	if opts.DictSize == 0 {
		opts.DictSize = 8 * 1024 * 1024
	}
	if opts.Check != CheckNone && opts.Check != CheckCRC32 {
		opts.Check = CheckCRC32
	}

	zw := &Writer{
		w:    w,
		opts: opts,
	}
	return zw, nil
}

func (zw *Writer) writeHeader() error {
	if zw.headerDone {
		return nil
	}
	zw.headerDone = true

	// Stream Header (12 bytes): [Magic:6B][StreamFlags:2B][CRC32:4B]
	var hdr [12]byte
	copy(hdr[:6], HeaderMagic[:])
	hdr[6] = 0x00                // Reserved
	hdr[7] = byte(zw.opts.Check) // Check type

	flagsCRC := crc32.ChecksumIEEE(hdr[6:8])
	binary.LittleEndian.PutUint32(hdr[8:12], flagsCRC)

	_, err := zw.w.Write(hdr[:])
	return err
}

func (zw *Writer) Write(p []byte) (int, error) {
	if zw.closed {
		return 0, errors.New("xz: write to closed writer")
	}
	return zw.buf.Write(p)
}

// Close flushes the pending block, writes the XZ index and the stream footer.
func (zw *Writer) Close() error {
	if zw.closed {
		return nil
	}
	zw.closed = true

	if err := zw.writeHeader(); err != nil {
		return err
	}

	data := zw.buf.Bytes()
	uncompressedLen := uint64(len(data))

	// 1. Process data through filters (BCJ if enabled)
	processed := data
	if zw.opts.UseBCJ {
		processed = make([]byte, len(data))
		copy(processed, data)
		bcj.Filter(bcj.X86, processed, 0, true)
	}

	// 2. Compress via LZMA2
	var compBuf bytes.Buffer
	comp := lzma.NewCompressor2(zw.opts.DictSize)
	comp.Options = lzma.OptionsForLevel(zw.opts.Level)
	if _, err := comp.Compress(bytes.NewReader(processed), &compBuf); err != nil {
		return err
	}
	compBytes := compBuf.Bytes()

	// 3. Compute Data Check (CRC32 of uncompressed original data)
	var checkVal uint32
	if zw.opts.Check == CheckCRC32 {
		checkVal = crc32.ChecksumIEEE(data)
	}

	// 4. Construct Block Header
	var bhBuf bytes.Buffer
	numFilters := 1
	if zw.opts.UseBCJ {
		numFilters = 2
	}
	blockFlags := byte(numFilters - 1)
	bhBuf.WriteByte(blockFlags)

	if zw.opts.UseBCJ {
		_, _ = WriteVLI(&bhBuf, FilterX86)
		_, _ = WriteVLI(&bhBuf, 0) // PropsSize = 0
	}

	_, _ = WriteVLI(&bhBuf, FilterLZMA2)
	_, _ = WriteVLI(&bhBuf, 1) // PropsSize = 1
	propByte := lzma.PropFromDictSize(zw.opts.DictSize)
	bhBuf.WriteByte(propByte)

	// Pad Block Header to 4-byte boundary (including 1-byte HeaderSize and 4-byte CRC32)
	for (bhBuf.Len()+1+4)%4 != 0 {
		bhBuf.WriteByte(0x00)
	}

	realHeaderSize := bhBuf.Len() + 1 + 4
	encodedHeaderSize := byte((realHeaderSize / 4) - 1)

	var fullHeader bytes.Buffer
	fullHeader.WriteByte(encodedHeaderSize)
	fullHeader.Write(bhBuf.Bytes())
	headerCRC := crc32.ChecksumIEEE(fullHeader.Bytes())
	var crcB [4]byte
	binary.LittleEndian.PutUint32(crcB[:], headerCRC)
	fullHeader.Write(crcB[:])

	// 5. Write Block Header
	if _, err := zw.w.Write(fullHeader.Bytes()); err != nil {
		return err
	}

	// 6. Write Compressed Data
	if _, err := zw.w.Write(compBytes); err != nil {
		return err
	}

	// 7. Write Block Padding to 4 bytes
	padLen := (4 - (len(compBytes) % 4)) % 4
	if padLen > 0 {
		var pad [4]byte
		if _, err := zw.w.Write(pad[:padLen]); err != nil {
			return err
		}
	}

	// 8. Write Block Check
	if zw.opts.Check == CheckCRC32 {
		binary.LittleEndian.PutUint32(crcB[:], checkVal)
		if _, err := zw.w.Write(crcB[:]); err != nil {
			return err
		}
	}

	unpaddedBlockSize := uint64(realHeaderSize) + uint64(len(compBytes)) + uint64(zw.opts.Check.CheckSize())
	zw.indexBlocks = append(zw.indexBlocks, indexRecord{
		unpaddedSize:     unpaddedBlockSize,
		uncompressedSize: uncompressedLen,
	})

	// 9. Write Index
	var idxBuf bytes.Buffer
	idxBuf.WriteByte(0x00) // Index indicator
	_, _ = WriteVLI(&idxBuf, uint64(len(zw.indexBlocks)))
	for _, rec := range zw.indexBlocks {
		_, _ = WriteVLI(&idxBuf, rec.unpaddedSize)
		_, _ = WriteVLI(&idxBuf, rec.uncompressedSize)
	}
	for (idxBuf.Len()+4)%4 != 0 {
		idxBuf.WriteByte(0x00)
	}
	indexCRC := crc32.ChecksumIEEE(idxBuf.Bytes())
	binary.LittleEndian.PutUint32(crcB[:], indexCRC)
	idxBuf.Write(crcB[:])

	if _, err := zw.w.Write(idxBuf.Bytes()); err != nil {
		return err
	}

	// 10. Write Stream Footer (12 bytes)
	var footer [12]byte
	backwardSize := uint32((idxBuf.Len() / 4) - 1)
	binary.LittleEndian.PutUint32(footer[4:8], backwardSize)
	footer[8] = 0x00                // Reserved
	footer[9] = byte(zw.opts.Check) // Check type
	footerCRC := crc32.ChecksumIEEE(footer[4:10])
	binary.LittleEndian.PutUint32(footer[0:4], footerCRC)
	copy(footer[10:12], FooterMagic[:])

	_, err := zw.w.Write(footer[:])
	return err
}

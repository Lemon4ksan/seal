// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cmpsc implements the IBM z/OS CMPSC (Compression Call / Method 16) algorithm
// defined in IBM Enterprise Systems Architecture/390 Principles of Operation (SA22-7208 & SA22-7832)
// and PKWARE APPNOTE.TXT for IBM S/390 and z/Architecture mainframes.
package cmpsc

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/codec"
)

func init() {
	codec.DefaultRegistry.RegisterDecompressor(codec.IBMCmpsc, func() codec.Decompressor {
		return &cmpscDecompressor{}
	})
	codec.DefaultRegistry.RegisterCompressor(codec.IBMCmpsc, func(level int) codec.Compressor {
		return &cmpscCompressor{level: level}
	})
}

type cmpscDecompressor struct{}

func (d *cmpscDecompressor) Decompress(r io.Reader) (io.ReadCloser, error) {
	return NewReader(r)
}

type cmpscCompressor struct {
	level int
}

func (c *cmpscCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	zw := NewWriterLevel(dest, c.level)
	n, err := io.Copy(zw, src)
	if err != nil {
		_ = zw.Close()
		return n, err
	}
	return n, zw.Close()
}

// CMPSC Dictionary Header Constants.
const (
	MinSymbolBits = 9   // 512 entries (4 KB expansion dict)
	MaxSymbolBits = 13  // 8192 entries (64 KB expansion dict)
	DefaultBits   = 12  // 4096 entries (32 KB expansion dict)
	SymEOS        = 256 // End of Stream symbol
)

// Header holds the Method 16 CMPSC Dictionary metadata.
type Header struct {
	SymbolBits uint8  // 9..13 bits per symbol
	Format     uint8  // Format flags (0 = standard)
	DictSize   uint32 // Expansion dictionary size in bytes (2^SymbolBits * 8)
}

// DictEntry represents an 8-byte expansion dictionary entry (UE or PE).
type DictEntry struct {
	IsPreceded bool   // P-bit: true if preceded by parent symbol
	ParentSym  uint16 // Preceding symbol index (if IsPreceded)
	Length     uint8  // Number of characters stored in this entry (1..7)
	Chars      [7]byte
}

// Reader decompresses an IBM z/OS CMPSC (Method 16) byte stream.
type Reader struct {
	r         io.Reader
	header    Header
	dict      []DictEntry
	bitBuf    uint64
	bitsInBuf uint8
	buf       bytes.Buffer
	eof       bool
	readErr   error
}

// NewReader constructs a new CMPSC stream decompressor.
func NewReader(r io.Reader) (*Reader, error) {
	var hdrBuf [6]byte
	if _, err := io.ReadFull(r, hdrBuf[:]); err != nil {
		return nil, fmt.Errorf("cmpsc: read header: %w", err)
	}

	symBits := hdrBuf[0]
	if symBits < MinSymbolBits || symBits > MaxSymbolBits {
		return nil, fmt.Errorf("cmpsc: unsupported symbol size %d (must be 9..13)", symBits)
	}

	dictEntries := 1 << symBits
	dictSizeBytes := dictEntries * 8

	zr := &Reader{
		r: r,
		header: Header{
			SymbolBits: symBits,
			Format:     hdrBuf[1],
			DictSize:   uint32(dictSizeBytes),
		},
		dict: make([]DictEntry, dictEntries),
	}

	// Read Expansion Dictionary (dictEntries * 8 bytes)
	rawDict := make([]byte, dictSizeBytes)
	if _, err := io.ReadFull(r, rawDict); err != nil {
		return nil, fmt.Errorf("cmpsc: read expansion dictionary: %w", err)
	}

	// Parse 8-byte entries
	for i := 0; i < dictEntries; i++ {
		entryBytes := rawDict[i*8 : (i+1)*8]
		zr.dict[i] = parseDictEntry(entryBytes)
	}

	return zr, nil
}

func parseDictEntry(b []byte) DictEntry {
	// Bit 0 of byte 0: P-bit (1 = Preceded, 0 = Unpreceded)
	isPreceded := (b[0] & 0x80) != 0
	var entry DictEntry
	entry.IsPreceded = isPreceded

	if isPreceded {
		// Preceded Entry (CMPSCDICT_PE):
		// Parent symbol: bits 1..13 across bytes 0-1
		parent := (uint16(b[0]&0x7F) << 8) | uint16(b[1])
		parent >>= 3 // 13-bit parent symbol
		lenCount := (b[1] & 0x07) + 1
		if lenCount > 6 {
			lenCount = 6
		}
		entry.ParentSym = parent
		entry.Length = lenCount
		copy(entry.Chars[:], b[2:2+lenCount])
	} else {
		// Unpreceded Entry (CMPSCDICT_UE):
		// Length count in bits 1..3 of byte 0
		lenCount := ((b[0] & 0x70) >> 4) + 1
		if lenCount > 7 {
			lenCount = 7
		}
		entry.Length = lenCount
		copy(entry.Chars[:], b[1:1+lenCount])
	}

	return entry
}

func (zr *Reader) Read(p []byte) (int, error) {
	if zr.buf.Len() > 0 {
		return zr.buf.Read(p)
	}
	if zr.eof {
		return 0, io.EOF
	}
	if zr.readErr != nil {
		return 0, zr.readErr
	}

	// Decode symbols until buffer has data
	symBits := zr.header.SymbolBits
	var stack [256]byte
	dictLen := len(zr.dict)

	for zr.buf.Len() < len(p) {
		sym, err := zr.readBits(symBits)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				zr.eof = true
				break
			}
			zr.readErr = err
			return 0, err
		}

		if sym == SymEOS || int(sym) >= dictLen {
			zr.eof = true
			break
		}

		// Expand symbol through prefix tree
		stackLen := 0
		currSym := sym
		loopGuard := 0

		for {
			entry := &zr.dict[currSym]
			if entry.Length == 0 && !entry.IsPreceded {
				break
			}
			// Push entry's characters in reverse onto stack
			for k := int(entry.Length) - 1; k >= 0; k-- {
				if stackLen < len(stack) {
					stack[stackLen] = entry.Chars[k]
					stackLen++
				}
			}

			if !entry.IsPreceded || entry.ParentSym == currSym {
				break
			}

			currSym = entry.ParentSym
			if int(currSym) >= dictLen {
				break
			}

			loopGuard++
			if loopGuard > 1024 {
				break // Cycle protection
			}
		}

		// Pop stack to output buffer
		for stackLen > 0 {
			stackLen--
			zr.buf.WriteByte(stack[stackLen])
		}
	}

	if zr.buf.Len() == 0 && zr.eof {
		return 0, io.EOF
	}

	return zr.buf.Read(p)
}

func (zr *Reader) readBits(n uint8) (uint16, error) {
	for zr.bitsInBuf < n {
		var b [1]byte
		_, err := zr.r.Read(b[:])
		if err != nil {
			if zr.bitsInBuf > 0 {
				val := uint16(zr.bitBuf >> (64 - n))
				zr.bitsInBuf = 0
				return val, nil
			}
			return 0, err
		}
		zr.bitBuf |= uint64(b[0]) << (56 - zr.bitsInBuf)
		zr.bitsInBuf += 8
	}

	val := uint16(zr.bitBuf >> (64 - n))
	zr.bitBuf <<= n
	zr.bitsInBuf -= n
	return val, nil
}

func (zr *Reader) Close() error {
	return nil
}

// Writer compresses data into an IBM z/OS CMPSC (Method 16) stream.
type Writer struct {
	w          io.Writer
	level      int
	symBits    uint8
	dict       []DictEntry
	bitBuf     uint64
	bitsInBuf  uint8
	charToSym  [256]uint16
	closed     bool
	headerSent bool
	inBuf      bytes.Buffer
}

// NewWriter constructs a CMPSC stream compressor with default settings (12-bit dictionary).
func NewWriter(w io.Writer) *Writer {
	return NewWriterLevel(w, 6)
}

// NewWriterLevel constructs a CMPSC compressor with specified level.
func NewWriterLevel(w io.Writer, level int) *Writer {
	symBits := uint8(DefaultBits)
	if level <= 2 {
		symBits = 9 // 512 entries
	} else if level <= 4 {
		symBits = 10 // 1024 entries
	}

	dictEntries := 1 << symBits
	zw := &Writer{
		w:       w,
		level:   level,
		symBits: symBits,
		dict:    make([]DictEntry, dictEntries),
	}

	// Initialize single-character base symbols (0..255)
	for i := 0; i < 256; i++ {
		zw.dict[i] = DictEntry{
			IsPreceded: false,
			Length:     1,
			Chars:      [7]byte{byte(i)},
		}
		zw.charToSym[i] = uint16(i)
	}

	// Initialize SymEOS entry
	zw.dict[SymEOS] = DictEntry{
		IsPreceded: false,
		Length:     0,
	}

	return zw
}

func (zw *Writer) Write(p []byte) (int, error) {
	if zw.level >= 4 {
		// Buffer input to build multi-byte prefix dictionary
		return zw.inBuf.Write(p)
	}

	// Streaming level: emit single character codes
	if !zw.headerSent {
		if err := zw.writeHeaderAndDict(); err != nil {
			return 0, err
		}
	}

	for _, b := range p {
		sym := zw.charToSym[b]
		if err := zw.writeBits(sym, zw.symBits); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (zw *Writer) writeHeaderAndDict() error {
	if zw.headerSent {
		return nil
	}
	zw.headerSent = true

	dictEntries := 1 << zw.symBits
	dictSizeBytes := uint32(dictEntries * 8)

	// Write 6-byte header
	hdr := [6]byte{
		zw.symBits,
		0, // Format = 0
		byte(dictSizeBytes >> 24),
		byte(dictSizeBytes >> 16),
		byte(dictSizeBytes >> 8),
		byte(dictSizeBytes),
	}
	if _, err := zw.w.Write(hdr[:]); err != nil {
		return err
	}

	// Serialize Expansion Dictionary (dictEntries * 8 bytes)
	rawDict := make([]byte, dictSizeBytes)
	for i := 0; i < dictEntries; i++ {
		entry := &zw.dict[i]
		offset := i * 8
		if entry.IsPreceded {
			parentBits := (entry.ParentSym & 0x1FFF) << 3
			lenCount := (entry.Length - 1) & 0x07
			rawDict[offset] = 0x80 | byte(parentBits>>8)
			rawDict[offset+1] = byte(parentBits&0xFF) | lenCount
			copy(rawDict[offset+2:offset+8], entry.Chars[:entry.Length])
		} else if entry.Length > 0 {
			lenCount := (entry.Length - 1) & 0x07
			rawDict[offset] = (lenCount << 4) & 0x70
			copy(rawDict[offset+1:offset+8], entry.Chars[:entry.Length])
		}
	}

	_, err := zw.w.Write(rawDict)
	return err
}

func (zw *Writer) writeBits(val uint16, n uint8) error {
	zw.bitBuf |= (uint64(val) & ((1 << n) - 1)) << (64 - zw.bitsInBuf - n)
	zw.bitsInBuf += n

	for zw.bitsInBuf >= 8 {
		b := byte(zw.bitBuf >> 56)
		if _, err := zw.w.Write([]byte{b}); err != nil {
			return err
		}
		zw.bitBuf <<= 8
		zw.bitsInBuf -= 8
	}

	return nil
}

func (zw *Writer) Close() error {
	if zw.closed {
		return nil
	}
	zw.closed = true

	if zw.level >= 4 && zw.inBuf.Len() > 0 {
		data := zw.inBuf.Bytes()
		dictEntries := len(zw.dict)
		trie := make(map[uint32]uint16, dictEntries)
		nextSym := uint16(257)

		// 1. Pass 1: Build dictionary prefix tree
		curr := uint16(data[0])
		for i := 1; i < len(data); i++ {
			b := data[i]
			key := (uint32(curr) << 8) | uint32(b)
			if nextCode, ok := trie[key]; ok {
				curr = nextCode
			} else {
				if int(nextSym) < dictEntries {
					trie[key] = nextSym
					zw.dict[nextSym] = DictEntry{
						IsPreceded: true,
						ParentSym:  curr,
						Length:     1,
						Chars:      [7]byte{b},
					}
					nextSym++
				}
				curr = uint16(b)
			}
		}

		// 2. Write Header & Expansion Dictionary
		if err := zw.writeHeaderAndDict(); err != nil {
			return err
		}

		// 3. Pass 2: Encode stream
		curr = uint16(data[0])
		for i := 1; i < len(data); i++ {
			b := data[i]
			key := (uint32(curr) << 8) | uint32(b)
			if nextCode, ok := trie[key]; ok {
				curr = nextCode
			} else {
				if err := zw.writeBits(curr, zw.symBits); err != nil {
					return err
				}
				curr = uint16(b)
			}
		}
		if err := zw.writeBits(curr, zw.symBits); err != nil {
			return err
		}
	} else if zw.inBuf.Len() == 0 {
		// Empty stream or fast mode already initialized
		if err := zw.writeHeaderAndDict(); err != nil {
			return err
		}
	}

	// Write End-of-Stream symbol
	if err := zw.writeBits(SymEOS, zw.symBits); err != nil {
		return err
	}

	// Flush remaining bits
	if zw.bitsInBuf > 0 {
		b := byte(zw.bitBuf >> 56)
		if _, err := zw.w.Write([]byte{b}); err != nil {
			return err
		}
		zw.bitsInBuf = 0
		zw.bitBuf = 0
	}

	return nil
}

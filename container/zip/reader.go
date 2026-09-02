// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"iter"
	"strings"
	"time"

	"github.com/lemon4ksan/foundation/silicon/endian"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/internal/sys"
	"github.com/lemon4ksan/seal/vfs"
)

// OpenFromArchive returns an [io.ReadCloser] providing decompressed file data.
func OpenFromArchive(f *file.File, rAt io.ReaderAt, size int64, reg *codec.Registry) (io.ReadCloser, error) {
	if f.IsDir() {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	offset := f.LocalHeaderOffset()
	sr := io.NewSectionReader(rAt, offset, size-offset)
	var sig [4]byte
	if _, err := io.ReadFull(sr, sig[:]); err != nil {
		return nil, fmt.Errorf("read local signature: %w", err)
	}
	if endian.Load32(sig[:], 0) != LocalFileHeaderSignature {
		return nil, errors.New("zip: invalid local file header signature")
	}

	lfh, err := ReadLocalFileHeader(sr)
	if err != nil {
		return nil, fmt.Errorf("read local header: %w", err)
	}

	payloadOffset := offset + 30 + int64(lfh.FilenameLength) + int64(lfh.ExtraFieldLength)
	payloadReader := io.NewSectionReader(rAt, payloadOffset, f.CompressedSize())

	var inReader io.Reader = payloadReader
	if cipher := ResolveCipher(f.Encryption()); cipher != nil {
		pwd := f.Password()
		_, dosTime := timeToMsDos(f.ModTime())
		plainR, err := cipher.Decrypt(
			payloadReader,
			pwd,
			f.CompressedSize(),
			lfh.ExtraField,
			uint16(f.Flags()),
			f.CRC(),
			dosTime,
		)
		if err != nil {
			return nil, err
		}
		inReader = plainR
	}

	if reg == nil {
		reg = codec.DefaultRegistry
	}

	decompressor, err := reg.Decompressor(f.Method())
	if err != nil {
		return nil, err
	}

	reader, err := decompressor.Decompress(inReader)
	if err != nil {
		return nil, err
	}

	return &crcVerifyingReader{
		r:        reader,
		expected: f.CRC(),
		size:     f.UncompressedSize(),
	}, nil
}

// Reader provides random-access read and inspection over a ZIP archive.
type Reader struct {
	rAt      io.ReaderAt
	size     int64
	entries  []*file.File
	byName   map[string]*file.File
	comment  string
	password string
	registry *codec.Registry
}

// NewReader initializes a random-access ZIP archive reader.
func NewReader(rAt io.ReaderAt, size int64, password string, reg *codec.Registry) (*Reader, error) {
	if reg == nil {
		reg = codec.DefaultRegistry
	}

	zr := &Reader{
		rAt:      rAt,
		size:     size,
		byName:   make(map[string]*file.File),
		password: password,
		registry: reg,
	}

	if err := zr.init(); err != nil {
		return nil, err
	}
	return zr, nil
}

func (zr *Reader) init() error {
	end, endOffset, err := findEOCD(zr.rAt, zr.size)
	if err != nil {
		return err
	}
	zr.comment = end.Comment

	entriesCount := uint64(end.EntriesNum)
	cdSize := int64(end.CentralDirSize)
	cdOffset := int64(end.CentralDirOffset)

	// Check for Zip64 EOCD Locator preceding standard EOCD
	if endOffset >= 20 {
		var locBuf [20]byte
		if _, err := zr.rAt.ReadAt(locBuf[:], endOffset-20); err == nil {
			if endian.Load32(locBuf[:], 0) == Zip64EOCDLocatorSignature {
				z64Offset := int64(endian.Load64(locBuf[:], 8))
				z64Rec := io.NewSectionReader(zr.rAt, z64Offset, 56)
				if z64EOCD, err := ReadZip64EOCD(z64Rec); err == nil {
					entriesCount = z64EOCD.EntriesNum
					cdSize = int64(z64EOCD.CentralDirSize)
					cdOffset = int64(z64EOCD.CentralDirOffset)
				}
			}
		}
	}

	// SFX base offset detection:
	// In an SFX (Self-Extracting) archive or file with a prepended executable stub (PE32/ELF),
	// the Central Directory and local file headers are shifted forward by the stub size.
	var sfxOffset int64
	if actualCD := endOffset - cdSize; actualCD > cdOffset {
		sfxOffset = actualCD - cdOffset
		cdOffset = actualCD
	}

	if cdOffset < 0 || cdOffset+cdSize > zr.size {
		return errors.New("zip: invalid central directory offset")
	}

	cdReader := io.NewSectionReader(zr.rAt, cdOffset, cdSize)
	zr.entries = make([]*file.File, 0, entriesCount)

	for i := uint64(0); i < entriesCount; i++ {
		var sig [4]byte
		if _, err := io.ReadFull(cdReader, sig[:]); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read CD signature: %w", err)
		}
		if endian.Load32(sig[:], 0) != CentralDirectorySignature {
			return errors.New("zip: corrupted central directory entry signature")
		}

		cd, err := ReadCentralDirEntry(cdReader)
		if err != nil {
			return err
		}

		entry := zr.createEntry(cd, sfxOffset)
		zr.entries = append(zr.entries, entry)
		zr.byName[entry.Name()] = entry
	}

	return nil
}

// Entries returns a copy of all entries in the archive.
func (zr *Reader) Entries() []*file.File {
	res := make([]*file.File, len(zr.entries))
	copy(res, zr.entries)
	return res
}

// Entry returns a named entry if present.
func (zr *Reader) Entry(name string) (*file.File, bool) {
	e, ok := zr.byName[name]
	return e, ok
}

// Comment returns the archive comment.
func (zr *Reader) Comment() string {
	return zr.comment
}

// ExtractEntry opens and returns the decompressed data payload for a single entry.
func (zr *Reader) ExtractEntry(entry *file.File) (io.ReadCloser, error) {
	return OpenFromArchive(entry, zr.rAt, zr.size, zr.registry)
}

func (zr *Reader) createEntry(cd CentralDirectory, sfxOffset int64) *file.File {
	cleanName := vfs.CleanPath(cd.Filename)
	mode := ParseFileMode(cd)
	modTime := dosDateTimeToTime(cd.LastModFileDate, cd.LastModFileTime)
	method := codec.Method(cd.CompressionMethod)
	hostSystem := sys.HostSystem(cd.VersionMadeBy >> 8)
	isDir := mode.IsDir() || strings.HasSuffix(cleanName, "/")

	enc := file.NotEncrypted
	if cd.GeneralPurposeBitFlag&0x0001 != 0 {
		enc = file.ZipCrypto
	}

	uncompSize := int64(cd.UncompressedSize)
	compSize := int64(cd.CompressedSize)
	localOffset := int64(cd.LocalHeaderOffset) + sfxOffset

	extraMap := ParseExtraFields(cd.ExtraField)

	_, detectedEnc, realMethod, ok := DetectCipher(cd.GeneralPurposeBitFlag, cd.CompressionMethod, cd.ExtraField)
	if ok {
		enc = detectedEnc
		method = realMethod
	}

	entry := file.NewArchiveFile(
		cleanName,
		cd.Comment,
		uncompSize,
		compSize,
		localOffset,
		cd.CRC32,
		cd.GeneralPurposeBitFlag,
		method,
		enc,
		zr.password,
		mode,
		modTime,
		hostSystem,
		extraMap,
		cd.ExtraField,
		sys.Metadata{},
		isDir,
	)

	// Procedural Extra Field parsing via modular plugin registry
	ctx := HeaderContext{
		IsLocalHeader: false,
		RawName:       cd.Filename,
		RawComment:    cd.Comment,
		RawFlags:      cd.GeneralPurposeBitFlag,
		Uncompressed:  &uncompSize,
		Compressed:    &compSize,
		LocalOffset:   &localOffset,
	}
	DefaultExtraRegistry.ParseAll(cd.ExtraField, entry, &ctx)

	// Update sizes if modified by Zip64 plugin
	entry.SetSizes(uncompSize, compSize, cd.CRC32)
	entry.SetLocalHeaderOffset(localOffset)

	entry.SetOpenFunc(func() (io.ReadCloser, error) {
		return OpenFromArchive(entry, zr.rAt, zr.size, zr.registry)
	})
	entry.SetRawFunc(func() (*io.SectionReader, error) {
		dataOffset, err := zr.findDataOffset(entry.LocalHeaderOffset())
		if err != nil {
			return nil, err
		}
		return io.NewSectionReader(zr.rAt, dataOffset, entry.CompressedSize()), nil
	})

	return entry
}

func (zr *Reader) findDataOffset(localOffset int64) (int64, error) {
	var sig [4]byte
	if _, err := zr.rAt.ReadAt(sig[:], localOffset); err != nil {
		return 0, fmt.Errorf("read local header signature: %w", err)
	}
	if endian.Load32(sig[:], 0) != LocalFileHeaderSignature {
		return 0, errors.New("zip: corrupted local file header signature")
	}

	sr := io.NewSectionReader(zr.rAt, localOffset+4, zr.size-(localOffset+4))
	lfh, err := ReadLocalFileHeader(sr)
	if err != nil {
		return 0, err
	}

	dataOffset := localOffset + 30 + int64(lfh.FilenameLength) + int64(lfh.ExtraFieldLength)
	return dataOffset, nil
}

// StreamReader enables sequential streaming parsing of ZIP streams without seek capability.
type StreamReader struct {
	br       *bufio.Reader
	registry *codec.Registry
	password string
	curr     *streamEntryReader
	eof      bool
}

// NewStreamReader constructs a sequential streaming ZIP parser.
func NewStreamReader(r io.Reader, password string, reg *codec.Registry) *StreamReader {
	if reg == nil {
		reg = codec.DefaultRegistry
	}
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &StreamReader{
		br:       br,
		registry: reg,
		password: password,
	}
}

// Next advances to the next entry in the streaming archive.
func (sr *StreamReader) Next() (*file.File, io.Reader, error) {
	if sr.eof {
		return nil, nil, io.EOF
	}

	// Drain previous unread payload and parse Data Descriptor if needed
	if sr.curr != nil {
		if err := sr.curr.discard(); err != nil {
			return nil, nil, err
		}
		sr.curr = nil
	}

	var sig [4]byte
	if _, err := io.ReadFull(sr.br, sig[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			sr.eof = true
			return nil, nil, io.EOF
		}
		return nil, nil, err
	}

	s := endian.Load32(sig[:], 0)
	if s != LocalFileHeaderSignature {
		if s == CentralDirectorySignature || s == EOCDSignature || s == Zip64EOCDSignature {
			sr.eof = true
			return nil, nil, io.EOF // Reached central directory
		}
		return nil, nil, fmt.Errorf("zip stream: invalid local header signature 0x%08x", s)
	}

	lfh, err := ReadLocalFileHeader(sr.br)
	if err != nil {
		return nil, nil, err
	}

	method := codec.Method(lfh.CompressionMethod)
	enc := file.NotEncrypted

	if _, detectedEnc, realMethod, ok := DetectCipher(
		lfh.GeneralPurposeBitFlag,
		lfh.CompressionMethod,
		lfh.ExtraField,
	); ok {
		enc = detectedEnc
		method = realMethod
	}

	uncompSize := int64(lfh.UncompressedSize)
	compSize := int64(lfh.CompressedSize)
	localOffset := int64(0)

	modTime := dosDateTimeToTime(lfh.LastModFileDate, lfh.LastModFileTime)
	cleanName := vfs.CleanPath(lfh.Filename)
	isDir := strings.HasSuffix(cleanName, "/")

	entry := file.NewArchiveFile(
		cleanName,
		"",
		uncompSize,
		compSize,
		localOffset,
		lfh.CRC32,
		lfh.GeneralPurposeBitFlag,
		method,
		enc,
		sr.password,
		0o644,
		modTime,
		sys.DefaultHostSystem,
		nil,
		lfh.ExtraField,
		sys.Metadata{},
		isDir,
	)

	// Execute procedural Extra Field scan through plugin registry
	ctx := HeaderContext{
		IsLocalHeader: true,
		RawName:       lfh.Filename,
		RawFlags:      lfh.GeneralPurposeBitFlag,
		Uncompressed:  &uncompSize,
		Compressed:    &compSize,
		LocalOffset:   &localOffset,
	}
	DefaultExtraRegistry.ParseAll(lfh.ExtraField, entry, &ctx)

	entry.SetSizes(uncompSize, compSize, lfh.CRC32)

	hasDD := (lfh.GeneralPurposeBitFlag & FlagDataDescriptor) != 0
	isZip64 := uncompSize >= int64(MaxUint32) || compSize >= int64(MaxUint32)

	var limitedReader io.Reader
	if compSize > 0 && !hasDD {
		limitedReader = io.LimitReader(sr.br, compSize)
	} else {
		limitedReader = sr.br
	}

	if cipher := ResolveCipher(entry.Encryption()); cipher != nil {
		pwd := entry.Password()
		aesR, err := cipher.Decrypt(
			limitedReader,
			pwd,
			compSize,
			lfh.ExtraField,
			lfh.GeneralPurposeBitFlag,
			lfh.CRC32,
			lfh.LastModFileTime,
		)
		if err != nil {
			return nil, nil, err
		}
		limitedReader = aesR
	}

	var payloadReader io.Reader
	if entry.Method() == codec.Store {
		if !hasDD && compSize > 0 {
			payloadReader = limitedReader
		} else {
			payloadReader = io.LimitReader(limitedReader, uncompSize)
		}
	} else {
		decompressor, err := sr.registry.Decompressor(entry.Method())
		if err != nil {
			return nil, nil, err
		}
		cr, err := decompressor.Decompress(limitedReader)
		if err != nil {
			return nil, nil, err
		}
		payloadReader = cr
	}

	entryReader := &streamEntryReader{
		sr:      sr,
		entry:   entry,
		rawR:    payloadReader,
		hasDD:   hasDD,
		isZip64: isZip64,
	}
	sr.curr = entryReader

	return entry, entryReader, nil
}

// All returns a standard Go range-over-func iterator yielding all (*file.File, io.Reader) pairs.
func (sr *StreamReader) All() iter.Seq2[*file.File, io.Reader] {
	return func(yield func(*file.File, io.Reader) bool) {
		for {
			fe, r, err := sr.Next()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				return
			}
			if !yield(fe, r) {
				return
			}
		}
	}
}

type streamEntryReader struct {
	sr        *StreamReader
	entry     *file.File
	rawR      io.Reader
	hasDD     bool
	isZip64   bool
	readCount int64
	closed    bool
}

func (s *streamEntryReader) Read(p []byte) (int, error) {
	if s.closed {
		return 0, io.EOF
	}
	n, err := s.rawR.Read(p)
	if n > 0 {
		s.readCount += int64(n)
	}
	if errors.Is(err, io.EOF) {
		if !s.closed {
			_ = s.finalize()
		}
	}
	return n, err
}

func (s *streamEntryReader) finalize() error {
	if s.closed {
		return nil
	}
	s.closed = true

	if s.hasDD {
		dd, err := ReadDataDescriptor(s.sr.br, s.isZip64)
		if err != nil {
			return err
		}
		s.entry.SetSizes(dd.UncompressedSize, dd.CompressedSize, dd.CRC32)
	}
	return nil
}

func (s *streamEntryReader) discard() error {
	if s.closed {
		return nil
	}
	_, err := io.Copy(io.Discard, s)
	return err
}

func findEOCD(rAt io.ReaderAt, size int64) (EOCD, int64, error) {
	if size < 22 {
		return EOCD{}, 0, errors.New("zip: file size too small for ZIP archive")
	}

	bufSize := min(size, 65536+22)
	buf := make([]byte, bufSize)
	readOffset := size - bufSize

	if _, err := rAt.ReadAt(buf, readOffset); err != nil {
		return EOCD{}, 0, fmt.Errorf("read end of central directory search buffer: %w", err)
	}

	for i := len(buf) - 22; i >= 0; i-- {
		if endian.Load32(buf[i:], 0) == EOCDSignature {
			eocdOffset := readOffset + int64(i)
			sr := io.NewSectionReader(rAt, eocdOffset+4, size-(eocdOffset+4))
			eocd, err := ReadEOCD(sr)
			if err != nil {
				return EOCD{}, 0, err
			}
			return eocd, eocdOffset, nil
		}
	}

	return EOCD{}, 0, errors.New("zip: End of Central Directory signature not found")
}

func dosDateTimeToTime(date, timeVal uint16) time.Time {
	year := int((date>>9)&0x7F) + 1980
	month := time.Month((date >> 5) & 0x0F)
	day := int(date & 0x1F)

	hour := int((timeVal >> 11) & 0x1F)
	min := int((timeVal >> 5) & 0x3F)
	sec := int((timeVal & 0x1F) * 2)

	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Unix(0, 0).UTC()
	}

	return time.Date(year, month, day, hour, min, sec, 0, time.UTC)
}

func timeToMsDos(t time.Time) (date, timeVal uint16) {
	t = t.UTC()
	year := max(t.Year(), 1980)
	date = uint16((year-1980)<<9 | int(t.Month())<<5 | t.Day())
	timeVal = uint16(t.Hour()<<11 | t.Minute()<<5 | t.Second()/2)
	return date, timeVal
}

func winFiletimeToTime(ft uint64) time.Time {
	// FILETIME is 100-nanosecond intervals since January 1, 1601 UTC
	const epochDiff = 116444736000000000
	if ft < epochDiff {
		return time.Unix(0, 0).UTC()
	}
	nsec := int64((ft - epochDiff) * 100)
	return time.Unix(0, nsec).UTC()
}

type crcVerifyingReader struct {
	r        io.ReadCloser
	expected uint32
	size     int64
	read     int64
	crc      uint32
}

func (c *crcVerifyingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.read += int64(n)
		c.crc = crc32.Update(c.crc, crc32.IEEETable, p[:n])
	}
	if err == io.EOF {
		if c.read != c.size {
			return n, fmt.Errorf("zip: uncompressed size mismatch: got %d, want %d", c.read, c.size)
		}
		if c.crc != c.expected && c.expected != 0 {
			return n, fmt.Errorf("zip: CRC-32 checksum mismatch: got 0x%08x, want 0x%08x", c.crc, c.expected)
		}
	}
	return n, err
}

func (c *crcVerifyingReader) Close() error {
	return c.r.Close()
}

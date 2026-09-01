// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/internal/pool"
	"github.com/lemon4ksan/seal/internal/sys"
)

// Writer serializes file entries and data streams into a specification-compliant ZIP archive.
type Writer struct {
	mu             sync.Mutex
	dest           io.Writer
	seeker         io.WriteSeeker
	offset         int64
	entriesNum     int
	cdSize         int64
	centralDir     *bytes.Buffer
	registry       *codec.Registry
	comment        string
	defaultMethod  codec.Method
	defaultLevel   int
	globalPassword string
}

// NewWriter constructs a new ZIP archive serializer.
func NewWriter(w io.Writer, reg *codec.Registry) *Writer {
	if reg == nil {
		reg = codec.DefaultRegistry
	}

	var ws io.WriteSeeker
	if s, ok := w.(io.WriteSeeker); ok {
		ws = s
	}

	return &Writer{
		dest:          w,
		seeker:        ws,
		centralDir:    bytes.NewBuffer(make([]byte, 0, 64*1024)),
		registry:      reg,
		defaultMethod: codec.Deflate,
		defaultLevel:  codec.DefaultCompression,
	}
}

// SetComment sets the global archive comment.
func (w *Writer) SetComment(comment string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.comment = comment
}

// SetPassword sets the default encryption password for entries without per-file overrides.
func (w *Writer) SetPassword(pwd string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.globalPassword = pwd
}

// WriteEntry processes and writes an individual file entry to the archive.
func (w *Writer) WriteEntry(entry *file.File, src io.Reader) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if entry.IsDir() {
		return w.writeDirectory(entry)
	}

	if entry.Method() == 0 && !entry.IsDir() {
		entry.SetMethod(w.defaultMethod)
	}

	headerOffset := w.offset
	entry.SetLocalHeaderOffset(headerOffset)

	// Check if we can stream directly via Data Descriptor (Bit 3) or if seeker is available
	if w.seeker == nil {
		return w.writeStreaming(entry, src)
	}

	return w.writeSeekable(entry, src)
}

// WriteEntryRaw copies raw pre-compressed bytes directly into the archive without recompression.
func (w *Writer) WriteEntryRaw(entry *file.File, rawSrc io.Reader) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry.SetLocalHeaderOffset(w.offset)

	// Prepare LFH
	dosDate, dosTime := timeToMsDos(entry.ModTime())
	name := entry.Name()
	compSize := entry.CompressedSize()
	uncompSize := entry.UncompressedSize()

	lfh := LocalFileHeader{
		VersionNeededToExtract: 20,
		GeneralPurposeBitFlag:  entry.Flags(),
		CompressionMethod:      uint16(entry.Method()),
		LastModFileTime:        dosTime,
		LastModFileDate:        dosDate,
		CRC32:                  entry.CRC(),
		CompressedSize:         uint32(min(uint64(compSize), MaxUint32)),
		UncompressedSize:       uint32(min(uint64(uncompSize), MaxUint32)),
		FilenameLength:         uint16(len(name)),
		Filename:               name,
	}

	if uint64(uncompSize) > MaxUint32 || uint64(compSize) > MaxUint32 {
		lfh.VersionNeededToExtract = 45
		lfh.ExtraField = EncodeZip64LocalExtraField(uncompSize, compSize)
		lfh.ExtraFieldLength = uint16(len(lfh.ExtraField))
	}

	hdrBytes := lfh.AppendBytes(nil)
	if _, err := w.dest.Write(hdrBytes); err != nil {
		return err
	}
	w.offset += int64(len(hdrBytes))

	bufPtr := pool.Get64K()
	defer pool.Put64K(bufPtr)

	n, err := io.CopyBuffer(w.dest, rawSrc, *bufPtr)
	if err != nil {
		return fmt.Errorf("copy raw entry: %w", err)
	}
	w.offset += n

	return w.addCentralDirectoryEntry(entry)
}

func (w *Writer) writeDirectory(entry *file.File) error {
	dosDate, dosTime := timeToMsDos(entry.ModTime())
	entry.SetLocalHeaderOffset(w.offset)
	name := entry.Name()

	lfh := LocalFileHeader{
		VersionNeededToExtract: 20,
		LastModFileTime:        dosTime,
		LastModFileDate:        dosDate,
		FilenameLength:         uint16(len(name)),
		Filename:               name,
	}

	hdrBytes := lfh.AppendBytes(nil)
	if _, err := w.dest.Write(hdrBytes); err != nil {
		return err
	}
	w.offset += int64(len(hdrBytes))

	return w.addCentralDirectoryEntry(entry)
}

func (w *Writer) writeStreaming(entry *file.File, src io.Reader) error {
	// Enable Bit 3 (Data Descriptor)
	entry.SetFlags(entry.Flags() | 0x08)
	dosDate, dosTime := timeToMsDos(entry.ModTime())
	name := entry.Name()

	lfh := LocalFileHeader{
		VersionNeededToExtract: 20,
		GeneralPurposeBitFlag:  entry.Flags(),
		CompressionMethod:      uint16(entry.Method()),
		LastModFileTime:        dosTime,
		LastModFileDate:        dosDate,
		FilenameLength:         uint16(len(name)),
		Filename:               name,
	}

	if entry.Encryption() == file.AES256 {
		lfh.CompressionMethod = 99 // WinZip AES marker
		lfh.ExtraField = EncodeAESExtraField(uint16(entry.Method()))
		lfh.ExtraFieldLength = uint16(len(lfh.ExtraField))
	}

	hdrBytes := lfh.AppendBytes(nil)
	if _, err := w.dest.Write(hdrBytes); err != nil {
		return err
	}
	w.offset += int64(len(hdrBytes))

	// Compression + CRC Pipeline
	crcR := pool.GetCRCReader(src)
	defer pool.PutCRCReader(crcR)

	byteCounter := &countWriter{w: w.dest}
	var compWriter io.Writer = byteCounter

	// Handle encryption wrapping
	var encCloser io.Closer
	if entry.Encryption() == file.AES256 {
		pwd := entry.Password()
		if pwd == "" {
			pwd = w.globalPassword
		}
		if pwd == "" {
			return ErrPasswordMismatch
		}
		aesW, err := NewAES256Writer(compWriter, pwd)
		if err != nil {
			return err
		}
		compWriter = aesW
		encCloser = aesW
	}

	level := entry.Level()
	if level == 0 {
		level = w.defaultLevel
	}

	compressor, err := w.registry.Compressor(entry.Method(), level)
	if err != nil {
		return err
	}

	uncompBytes, err := compressor.Compress(crcR, compWriter)
	if err != nil {
		return err
	}

	if encCloser != nil {
		if err := encCloser.Close(); err != nil {
			return err
		}
	}

	entry.SetSizes(uncompBytes, byteCounter.count, crcR.CRC())
	w.offset += entry.CompressedSize()

	// Write Data Descriptor
	dd := EncodeDataDescriptor(entry.CRC(), entry.CompressedSize(), entry.UncompressedSize())
	if _, err := w.dest.Write(dd); err != nil {
		return fmt.Errorf("write data descriptor: %w", err)
	}
	w.offset += int64(len(dd))

	return w.addCentralDirectoryEntry(entry)
}

func (w *Writer) writeSeekable(entry *file.File, src io.Reader) error {
	headerOffset := w.offset
	dosDate, dosTime := timeToMsDos(entry.ModTime())
	name := entry.Name()

	lfh := LocalFileHeader{
		VersionNeededToExtract: 20,
		GeneralPurposeBitFlag:  entry.Flags(),
		CompressionMethod:      uint16(entry.Method()),
		LastModFileTime:        dosTime,
		LastModFileDate:        dosDate,
		FilenameLength:         uint16(len(name)),
		Filename:               name,
	}

	if entry.Encryption() == file.AES256 {
		lfh.CompressionMethod = 99
		lfh.ExtraField = EncodeAESExtraField(uint16(entry.Method()))
		lfh.ExtraFieldLength = uint16(len(lfh.ExtraField))
	}

	hdrBytes := lfh.AppendBytes(nil)
	if _, err := w.dest.Write(hdrBytes); err != nil {
		return err
	}
	w.offset += int64(len(hdrBytes))

	crcR := pool.GetCRCReader(src)
	defer pool.PutCRCReader(crcR)

	byteCounter := &countWriter{w: w.dest}
	var compWriter io.Writer = byteCounter

	var encCloser io.Closer
	if entry.Encryption() == file.AES256 {
		pwd := entry.Password()
		if pwd == "" {
			pwd = w.globalPassword
		}
		if pwd == "" {
			return ErrPasswordMismatch
		}
		aesW, err := NewAES256Writer(compWriter, pwd)
		if err != nil {
			return err
		}
		compWriter = aesW
		encCloser = aesW
	}

	level := entry.Level()
	if level == 0 {
		level = w.defaultLevel
	}

	compressor, err := w.registry.Compressor(entry.Method(), level)
	if err != nil {
		return err
	}

	uncompBytes, err := compressor.Compress(crcR, compWriter)
	if err != nil {
		return err
	}

	if encCloser != nil {
		if err := encCloser.Close(); err != nil {
			return err
		}
	}

	entry.SetSizes(uncompBytes, byteCounter.count, crcR.CRC())
	w.offset += entry.CompressedSize()

	// Seek back and patch Local File Header with calculated sizes and CRC
	currentPos := w.offset
	if _, err := w.seeker.Seek(headerOffset, io.SeekStart); err != nil {
		return fmt.Errorf("seek to patch local header: %w", err)
	}

	lfh.CRC32 = entry.CRC()
	lfh.CompressedSize = uint32(min(uint64(entry.CompressedSize()), MaxUint32))
	lfh.UncompressedSize = uint32(min(uint64(entry.UncompressedSize()), MaxUint32))
	patchedHdr := lfh.AppendBytes(nil)

	if _, err := w.seeker.Write(patchedHdr); err != nil {
		return fmt.Errorf("write patched header: %w", err)
	}

	// Restore write position
	if _, err := w.seeker.Seek(currentPos, io.SeekStart); err != nil {
		return fmt.Errorf("restore position: %w", err)
	}

	return w.addCentralDirectoryEntry(entry)
}

func (w *Writer) addCentralDirectoryEntry(entry *file.File) error {
	dosDate, dosTime := timeToMsDos(entry.ModTime())
	versionMadeBy := uint16(sys.DefaultHostSystem)<<8 | 20
	versionNeeded := uint16(20)

	extAttrs := uint32(entry.Mode().Perm()) << 16
	if entry.IsDir() {
		extAttrs |= 0x10 | (sys.S_IFDIR << 16)
	} else {
		extAttrs |= sys.S_IFREG << 16
	}

	name := entry.Name()
	comment := entry.Comment()
	uncompSize := entry.UncompressedSize()
	compSize := entry.CompressedSize()
	localOffset := entry.LocalHeaderOffset()

	cd := CentralDirectory{
		VersionMadeBy:          versionMadeBy,
		VersionNeededToExtract: versionNeeded,
		GeneralPurposeBitFlag:  entry.Flags(),
		CompressionMethod:      uint16(entry.Method()),
		LastModFileTime:        dosTime,
		LastModFileDate:        dosDate,
		CRC32:                  entry.CRC(),
		CompressedSize:         uint32(min(uint64(compSize), MaxUint32)),
		UncompressedSize:       uint32(min(uint64(uncompSize), MaxUint32)),
		FilenameLength:         uint16(len(name)),
		FileCommentLength:      uint16(len(comment)),
		ExternalFileAttributes: extAttrs,
		LocalHeaderOffset:      uint32(min(uint64(localOffset), MaxUint32)),
		Filename:               name,
		Comment:                comment,
	}

	if entry.Encryption() == file.AES256 {
		cd.CompressionMethod = 99
		cd.ExtraField = EncodeAESExtraField(uint16(entry.Method()))
	}

	// Check if Zip64 is required
	if uint64(uncompSize) > MaxUint32 || uint64(compSize) > MaxUint32 || uint64(localOffset) > MaxUint32 {
		cd.VersionNeededToExtract = 45
		cd.ExtraField = append(cd.ExtraField, EncodeZip64ExtraField(nil, uncompSize, compSize, localOffset)...)
	}

	cd.ExtraFieldLength = uint16(len(cd.ExtraField))
	cdBytes := cd.AppendBytes(nil)

	w.centralDir.Write(cdBytes)
	w.cdSize += int64(len(cdBytes))
	w.entriesNum++

	return nil
}

// Close flushes the Central Directory index and finalizes the archive.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	cdOffset := w.offset

	// Flush Central Directory
	if _, err := io.Copy(w.dest, w.centralDir); err != nil {
		return fmt.Errorf("flush central directory: %w", err)
	}
	w.offset += w.cdSize

	// Check if ZIP64 end records are required
	requiresZip64 := uint64(w.entriesNum) > MaxUint16 || uint64(w.cdSize) > MaxUint32 || uint64(cdOffset) > MaxUint32

	if requiresZip64 {
		z64EOCD := EncodeZip64EOCDRecord(w.entriesNum, w.cdSize, cdOffset)
		if _, err := w.dest.Write(z64EOCD); err != nil {
			return err
		}
		w.offset += int64(len(z64EOCD))

		z64Loc := EncodeZip64EOCDLocator(cdOffset + w.cdSize)
		if _, err := w.dest.Write(z64Loc); err != nil {
			return err
		}
		w.offset += int64(len(z64Loc))
	}

	// Write standard EOCD record
	eocd := EncodeEOCD(w.entriesNum, w.cdSize, cdOffset, w.comment)
	if _, err := w.dest.Write(eocd); err != nil {
		return fmt.Errorf("write EOCD: %w", err)
	}
	w.offset += int64(len(eocd))

	return nil
}

type countWriter struct {
	w     io.Writer
	count int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if n > 0 {
		c.count += int64(n)
	}
	return n, err
}

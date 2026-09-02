// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/lemon4ksan/foundation/generic"

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
	var ws io.WriteSeeker
	if s, ok := w.(io.WriteSeeker); ok {
		ws = s
	}

	return &Writer{
		dest:          w,
		seeker:        ws,
		centralDir:    bytes.NewBuffer(make([]byte, 0, 64*1024)),
		registry:      generic.CoalesceNil(reg, codec.DefaultRegistry),
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

	if entry.Method() == 0 {
		entry.SetMethod(w.defaultMethod)
	}

	entry.SetLocalHeaderOffset(w.offset)

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
	if err := w.writeLocalFileHeader(entry, entry.CompressedSize(), entry.UncompressedSize(), false); err != nil {
		return err
	}

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
	entry.SetLocalHeaderOffset(w.offset)
	if err := w.writeLocalFileHeader(entry, 0, 0, false); err != nil {
		return err
	}
	return w.addCentralDirectoryEntry(entry)
}

func (w *Writer) writeStreaming(entry *file.File, src io.Reader) error {
	// Enable Bit 3 (Data Descriptor)
	entry.SetFlags(entry.Flags() | FlagDataDescriptor)

	if err := w.writeLocalFileHeader(entry, 0, 0, true); err != nil {
		return err
	}

	// Compression + CRC Pipeline
	crcR := pool.GetCRCReader(src)
	defer pool.PutCRCReader(crcR)

	byteCounter := &countWriter{w: w.dest}
	var compWriter io.Writer = byteCounter

	// Handle encryption wrapping
	var encCloser io.Closer
	if cipher := ResolveCipher(entry.Encryption()); cipher != nil {
		pwd := generic.Coalesce(entry.Password(), w.globalPassword)
		_, dosTime := timeToMsDos(entry.ModTime())
		checkByte := byte(dosTime >> 8)
		res, err := cipher.Encrypt(compWriter, pwd, entry.Method(), getAESStrength(entry.Encryption()), checkByte)
		if err != nil {
			return err
		}
		compWriter = res.Writer
		encCloser = res.Writer
	}

	level := generic.Coalesce(entry.Level(), w.defaultLevel)
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
	if err := w.writeLocalFileHeader(entry, 0, 0, false); err != nil {
		return err
	}

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

	patchedLFH := w.createLocalFileHeader(entry, entry.CompressedSize(), entry.UncompressedSize(), false)
	patchedHdr := patchedLFH.AppendBytes(nil)

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
	versionMadeBy := uint16(sys.DefaultHostSystem)<<8 | VersionDefault
	versionNeeded := VersionDefault

	extAttrs := uint32(entry.Mode().Perm()) << 16
	if entry.IsDir() {
		extAttrs |= 0x10 | (sys.S_IFDIR << 16)
	} else {
		extAttrs |= sys.S_IFREG << 16
	}

	name := entry.Name()
	if len(name) > int(MaxUint16) {
		return fmt.Errorf("zip: entry name length %d exceeds max uint16 limit (%d)", len(name), MaxUint16)
	}
	comment := entry.Comment()
	if len(comment) > int(MaxUint16) {
		return fmt.Errorf("zip: entry comment length %d exceeds max uint16 limit (%d)", len(comment), MaxUint16)
	}
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

	if cipher := ResolveCipher(entry.Encryption()); cipher != nil {
		res, _ := cipher.Encrypt(nil, "", entry.Method(), getAESStrength(entry.Encryption()), 0)
		cd.CompressionMethod = res.WireMethod
		cd.GeneralPurposeBitFlag |= res.BitFlags
		cd.ExtraField = append(cd.ExtraField, res.ExtraField...)
	}

	// Check if Zip64 is required
	if uint64(uncompSize) > MaxUint32 || uint64(compSize) > MaxUint32 || uint64(localOffset) > MaxUint32 {
		cd.VersionNeededToExtract = VersionZip64
		cd.ExtraField = append(cd.ExtraField, EncodeZip64ExtraField(nil, uncompSize, compSize, localOffset)...)
	}

	if len(cd.ExtraField) > int(MaxUint16) {
		return fmt.Errorf(
			"zip: central directory extra field length %d exceeds max uint16 limit (%d)",
			len(cd.ExtraField),
			MaxUint16,
		)
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

	if len(w.comment) > int(MaxUint16) {
		return fmt.Errorf("zip: archive comment length %d exceeds max uint16 limit (%d)", len(w.comment), MaxUint16)
	}

	cdOffset := w.offset

	// Flush Central Directory
	if _, err := io.Copy(w.dest, w.centralDir); err != nil {
		return fmt.Errorf("flush central directory: %w", err)
	}
	w.offset += w.cdSize

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

// createLocalFileHeader constructs a specification-compliant LocalFileHeader record.
func (w *Writer) createLocalFileHeader(entry *file.File, compSize, uncompSize int64, streaming bool) LocalFileHeader {
	dosDate, dosTime := timeToMsDos(entry.ModTime())
	name := entry.Name()
	flags := entry.Flags()

	if streaming {
		flags |= FlagDataDescriptor
	}

	lfh := LocalFileHeader{
		VersionNeededToExtract: VersionDefault,
		GeneralPurposeBitFlag:  flags,
		CompressionMethod:      generic.Ternary(entry.IsDir(), MethodStore, uint16(entry.Method())),
		LastModFileTime:        dosTime,
		LastModFileDate:        dosDate,
		CRC32:                  entry.CRC(),
		CompressedSize:         uint32(min(uint64(compSize), MaxUint32)),
		UncompressedSize:       uint32(min(uint64(uncompSize), MaxUint32)),
		FilenameLength:         uint16(len(name)),
		Filename:               name,
	}

	if cipher := ResolveCipher(entry.Encryption()); cipher != nil {
		res, _ := cipher.Encrypt(nil, "", entry.Method(), getAESStrength(entry.Encryption()), 0)
		lfh.CompressionMethod = res.WireMethod
		lfh.GeneralPurposeBitFlag |= res.BitFlags
		lfh.ExtraField = append(lfh.ExtraField, res.ExtraField...)
		lfh.ExtraFieldLength = uint16(len(lfh.ExtraField))
	}

	if uint64(uncompSize) > MaxUint32 || uint64(compSize) > MaxUint32 {
		lfh.VersionNeededToExtract = VersionZip64
		lfh.ExtraField = append(lfh.ExtraField, EncodeZip64LocalExtraField(uncompSize, compSize)...)
		lfh.ExtraFieldLength = uint16(len(lfh.ExtraField))
	}

	return lfh
}

func (w *Writer) writeLocalFileHeader(entry *file.File, compSize, uncompSize int64, streaming bool) error {
	if len(entry.Name()) > int(MaxUint16) {
		return fmt.Errorf("zip: entry name length %d exceeds max uint16 limit (%d)", len(entry.Name()), MaxUint16)
	}
	if uncompSize < 0 || compSize < 0 {
		return fmt.Errorf("zip: invalid negative entry size (uncomp=%d, comp=%d)", uncompSize, compSize)
	}
	lfh := w.createLocalFileHeader(entry, compSize, uncompSize, streaming)
	if len(lfh.ExtraField) > int(MaxUint16) {
		return fmt.Errorf("zip: extra field length %d exceeds max uint16 limit (%d)", len(lfh.ExtraField), MaxUint16)
	}
	hdrBytes := lfh.AppendBytes(nil)
	if _, err := w.dest.Write(hdrBytes); err != nil {
		return err
	}
	w.offset += int64(len(hdrBytes))
	return nil
}

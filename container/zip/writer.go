// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/lemon4ksan/foundation/generic"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/internal/pool"
	"github.com/lemon4ksan/seal/internal/sys"
)

// WriterOptions allows customizing serialization strictness and defaults.
type WriterOptions struct {
	Registry       *codec.Registry
	DefaultMethod  codec.Method
	DefaultLevel   int
	GlobalPassword string
	// AllowEmpty disables APPNOTE 4.1.2 enforcement (which specifies that ZIP files SHOULD contain at least one file).
	AllowEmpty bool
}

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
	checkpointData []byte
	defaultMethod  codec.Method
	defaultLevel   int
	globalPassword string
	allowEmpty     bool
}

// NewWriter constructs a new ZIP archive serializer.
func NewWriter(w io.Writer, reg *codec.Registry) *Writer {
	return NewWriterOptions(w, WriterOptions{Registry: reg})
}

// NewWriterOptions constructs a ZIP archive serializer with granular options.
func NewWriterOptions(w io.Writer, opts WriterOptions) *Writer {
	var ws io.WriteSeeker
	if s, ok := w.(io.WriteSeeker); ok {
		ws = s
	}

	method := generic.Coalesce(opts.DefaultMethod, codec.Deflate)
	level := generic.Coalesce(opts.DefaultLevel, codec.DefaultCompression)

	return &Writer{
		dest:           w,
		seeker:         ws,
		centralDir:     bytes.NewBuffer(make([]byte, 0, 64*1024)),
		registry:       generic.CoalesceNil(opts.Registry, codec.DefaultRegistry),
		defaultMethod:  method,
		defaultLevel:   level,
		globalPassword: opts.GlobalPassword,
		allowEmpty:     opts.AllowEmpty,
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

// SetCheckpointData sets the transient checkpoint payload to be stored in the archive metadata.
func (w *Writer) SetCheckpointData(data []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.checkpointData = slices.Clone(data)
}

// WithComment sets the global archive comment fluently.
func (w *Writer) WithComment(comment string) *Writer {
	w.SetComment(comment)
	return w
}

// WithPassword sets the default encryption password fluently.
func (w *Writer) WithPassword(pwd string) *Writer {
	w.SetPassword(pwd)
	return w
}

// WithCheckpointData sets the transient checkpoint payload fluently.
func (w *Writer) WithCheckpointData(data []byte) *Writer {
	w.SetCheckpointData(data)
	return w
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

	if !entry.IsDir() && entry.Encryption() == file.NotEncrypted && (entry.Password() != "" || w.globalPassword != "") {
		pwd := entry.Password()
		if pwd == "" {
			pwd = w.globalPassword
		}
		entry.SetEncryption(file.AES256, pwd)
	}

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
		if pwd == "" {
			return ErrPasswordMismatch
		}
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
	initialUncomp := entry.UncompressedSize()
	if err := w.writeLocalFileHeader(entry, 0, initialUncomp, false); err != nil {
		return err
	}
	initialHdrLen := w.offset - headerOffset

	crcR := pool.GetCRCReader(src)
	defer pool.PutCRCReader(crcR)

	byteCounter := &countWriter{w: w.dest}
	var compWriter io.Writer = byteCounter

	var encCloser io.Closer
	if cipher := ResolveCipher(entry.Encryption()); cipher != nil {
		pwd := generic.Coalesce(entry.Password(), w.globalPassword)
		if pwd == "" {
			return ErrPasswordMismatch
		}
		_, dosTime := timeToMsDos(entry.ModTime())
		checkByte := byte(dosTime >> 8)
		res, err := cipher.Encrypt(compWriter, pwd, entry.Method(), getAESStrength(entry.Encryption()), checkByte)
		if err != nil {
			return err
		}
		compWriter = res.Writer
		encCloser = res.Writer
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

	if int64(len(patchedHdr)) != initialHdrLen {
		return fmt.Errorf(
			"zip: seekable entry header size changed from %d to %d bytes (Zip64 transition requires known uncompressed size > 4GB or streaming writer)",
			initialHdrLen,
			len(patchedHdr),
		)
	}

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

	name := entry.Name()
	if len(name) > int(MaxUint16) {
		return fmt.Errorf(
			"zip: 4.4.3.2 entry name length %d exceeds max uint16 limit (%d): %w",
			len(name),
			MaxUint16,
			ErrFileNameTooLong,
		)
	}
	comment := entry.Comment()
	if len(comment) > int(MaxUint16) {
		return fmt.Errorf(
			"zip: 4.4.3.2 entry comment length %d exceeds max uint16 limit (%d): %w",
			len(comment),
			MaxUint16,
			ErrCommentTooLong,
		)
	}

	uncompSize := entry.UncompressedSize()
	compSize := entry.CompressedSize()
	localOffset := entry.LocalHeaderOffset()
	isZip64 := uint64(uncompSize) > MaxUint32 || uint64(compSize) > MaxUint32 || uint64(localOffset) > MaxUint32

	// APPNOTE 4.4.1.1 & 4.4.3.2: Minimum Feature Versions
	versionNeeded := ComputeVersionNeeded(
		entry.Method(),
		entry.IsDir(),
		entry.Encryption() != file.NotEncrypted,
		isZip64,
	)
	versionMadeBy := uint16(entry.HostSystem())<<8 | versionNeeded

	extAttrs := uint32(entry.Mode().Perm()) << 16
	if entry.IsDir() {
		extAttrs |= 0x10 | (sys.S_IFDIR << 16)
	} else {
		extAttrs |= sys.S_IFREG << 16
	}

	flags := entry.Flags()
	// APPNOTE 4.4.4: Bit 11 (Language encoding flag / EFS)
	if !isASCII(name) || !isASCII(comment) {
		flags |= FlagLanguageEncodingUTF8
	}

	cd := CentralDirectory{
		VersionMadeBy:          versionMadeBy,
		VersionNeededToExtract: versionNeeded,
		GeneralPurposeBitFlag:  flags,
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

	// APPNOTE 4.5.3: ZIP64 Extended Information Extra Field
	if isZip64 {
		cd.VersionNeededToExtract = VersionZip64
		cd.ExtraField = append(cd.ExtraField, EncodeZip64ExtraField(nil, uncompSize, compSize, localOffset)...)
	}

	if len(cd.ExtraField) > int(MaxUint16) {
		return fmt.Errorf(
			"zip: 4.4.3.2 central directory extra field length %d exceeds max uint16 limit (%d): %w",
			len(cd.ExtraField),
			MaxUint16,
			ErrExtraFieldTooLong,
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

	// APPNOTE 4.1.2: ZIP files SHOULD contain at least one file and MAY contain multiple files.
	// In accordance with RFC 2119, "SHOULD" signifies a strong recommendation rather than a mandatory
	// constraint ("MUST"). An empty archive containing only the 22-byte EOCD record is syntactically
	// valid, fully specification-compliant, and permitted by default.

	if len(w.checkpointData) > 0 {
		if w.comment == "" {
			w.comment = string(w.checkpointData)
		} else {
			w.comment += "\n" + string(w.checkpointData)
		}
	}

	if len(w.comment) > int(MaxUint16) {
		return fmt.Errorf(
			"zip: 4.4.14 archive comment length %d exceeds max uint16 limit (%d): %w",
			len(w.comment),
			MaxUint16,
			ErrCommentTooLong,
		)
	}

	cdOffset := w.offset

	// APPNOTE 4.1.5: The Central Directory MUST be placed after the last file.
	if _, err := io.Copy(w.dest, w.centralDir); err != nil {
		return fmt.Errorf("flush central directory: %w", err)
	}
	w.offset += w.cdSize

	// APPNOTE 4.4.14 / 4.4.15: Check if Zip64 EOCD is required
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

	// APPNOTE 4.1.6 & 4.4.14: End of Central Directory Record (EOCD)
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

// createLocalFileHeader constructs a specification-compliant LocalFileHeader record (APPNOTE 4.4.1).
func (w *Writer) createLocalFileHeader(entry *file.File, compSize, uncompSize int64, streaming bool) LocalFileHeader {
	dosDate, dosTime := timeToMsDos(entry.ModTime())
	name := entry.Name()
	flags := entry.Flags()

	// APPNOTE 4.4.4: Bit 11 (Language encoding flag / EFS)
	if !isASCII(name) || !isASCII(entry.Comment()) {
		flags |= FlagLanguageEncodingUTF8
	}

	var (
		crcVal        uint32
		compSizeVal   uint32
		uncompSizeVal uint32
	)

	// APPNOTE 4.4.4: Bit 3 (Data Descriptor).
	// If bit 3 is set, CRC-32, compressed size and uncompressed size MUST be set to zero in the local header.
	if streaming {
		flags |= FlagDataDescriptor
		crcVal = 0
		compSizeVal = 0
		uncompSizeVal = 0
	} else {
		crcVal = entry.CRC()
		compSizeVal = uint32(min(uint64(compSize), MaxUint32))
		uncompSizeVal = uint32(min(uint64(uncompSize), MaxUint32))
	}

	isZip64 := uint64(uncompSize) > MaxUint32 || uint64(compSize) > MaxUint32
	versionNeeded := ComputeVersionNeeded(
		entry.Method(),
		entry.IsDir(),
		entry.Encryption() != file.NotEncrypted,
		isZip64,
	)

	lfh := LocalFileHeader{
		VersionNeededToExtract: versionNeeded,
		GeneralPurposeBitFlag:  flags,
		CompressionMethod:      generic.Ternary(entry.IsDir(), MethodStore, uint16(entry.Method())),
		LastModFileTime:        dosTime,
		LastModFileDate:        dosDate,
		CRC32:                  crcVal,
		CompressedSize:         compSizeVal,
		UncompressedSize:       uncompSizeVal,
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

	if isZip64 {
		lfh.VersionNeededToExtract = VersionZip64
		lfh.ExtraField = append(lfh.ExtraField, EncodeZip64LocalExtraField(uncompSize, compSize)...)
		lfh.ExtraFieldLength = uint16(len(lfh.ExtraField))
	}

	return lfh
}

func (w *Writer) writeLocalFileHeader(entry *file.File, compSize, uncompSize int64, streaming bool) error {
	if len(entry.Name()) > int(MaxUint16) {
		return fmt.Errorf(
			"zip: 4.4.1.2 entry name length %d exceeds max uint16 limit (%d): %w",
			len(entry.Name()),
			MaxUint16,
			ErrFileNameTooLong,
		)
	}
	if uncompSize < 0 || compSize < 0 {
		return fmt.Errorf("zip: invalid negative entry size (uncomp=%d, comp=%d)", uncompSize, compSize)
	}
	lfh := w.createLocalFileHeader(entry, compSize, uncompSize, streaming)
	if len(lfh.ExtraField) > int(MaxUint16) {
		return fmt.Errorf(
			"zip: 4.4.1.3 extra field length %d exceeds max uint16 limit (%d): %w",
			len(lfh.ExtraField),
			MaxUint16,
			ErrExtraFieldTooLong,
		)
	}
	hdrBytes := lfh.AppendBytes(nil)
	if _, err := w.dest.Write(hdrBytes); err != nil {
		return err
	}
	w.offset += int64(len(hdrBytes))
	return nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

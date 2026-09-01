// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/internal/pool"
	"github.com/lemon4ksan/seal/internal/sys"
	"github.com/lemon4ksan/seal/vfs"
)

// EncryptionMethod identifies the payload encryption algorithm.
type EncryptionMethod uint16

// Universal encryption algorithm identifiers.
const (
	NotEncrypted EncryptionMethod = 0 // Plaintext storage
	ZipCrypto    EncryptionMethod = 1 // Legacy PKWARE ZipCrypto stream cipher
	AES256       EncryptionMethod = 2 // WinZip AES-256 standard encryption
)

type fileMeta struct {
	name        string
	comment     string
	password    string
	mode        fs.FileMode
	modTime     time.Time
	hostSystem  sys.HostSystem
	metadata    sys.Metadata
	extraFields map[uint16][]byte
	rawExtra    []byte
}

// File represents a single canonical file or directory record in an archive.
// It is completely format-agnostic, 100% lock-free, and safe for concurrent access across goroutines.
type File struct {
	meta atomic.Pointer[fileMeta]

	uncompressedSize  atomic.Int64
	compressedSize    atomic.Int64
	localHeaderOffset atomic.Int64
	crc32             atomic.Uint32
	flags             atomic.Uint32
	method            atomic.Uint32
	level             atomic.Int32
	encryption        atomic.Uint32
	isDir             atomic.Bool
	isImplicit        atomic.Bool

	openFunc atomic.Pointer[func() (io.ReadCloser, error)]
	rawFunc  atomic.Pointer[func() (*io.SectionReader, error)]
}

// New constructs a standalone [File] entry with initialized defaults.
func New(name string, isDir bool) (*File, error) {
	cleanName := vfs.CleanPath(name)
	if cleanName == "" {
		return nil, errors.New("file: filename cannot be empty")
	}

	if isDir && !strings.HasSuffix(cleanName, "/") {
		cleanName += "/"
	}

	f := &File{}
	f.isDir.Store(isDir)
	f.method.Store(uint32(codec.Deflate))
	f.level.Store(int32(codec.DefaultCompression))

	initialMeta := &fileMeta{
		name:       cleanName,
		mode:       0o644,
		modTime:    time.Now(),
		hostSystem: sys.DefaultHostSystem,
	}

	if isDir {
		initialMeta.mode = 0o755 | fs.ModeDir
		f.method.Store(uint32(codec.Store))
		openFn := func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(nil)), nil
		}
		f.openFunc.Store(&openFn)
	}

	f.meta.Store(initialMeta)
	return f, nil
}

// NewArchiveFile constructs a [File] populated from container metadata.
func NewArchiveFile(
	name string,
	comment string,
	uncompressedSize int64,
	compressedSize int64,
	localHeaderOffset int64,
	crc32 uint32,
	flags uint16,
	method codec.Method,
	encryption EncryptionMethod,
	password string,
	mode fs.FileMode,
	modTime time.Time,
	hostSystem sys.HostSystem,
	extraFields map[uint16][]byte,
	rawExtra []byte,
	metadata sys.Metadata,
	isDir bool,
) *File {
	f := &File{}
	f.uncompressedSize.Store(uncompressedSize)
	f.compressedSize.Store(compressedSize)
	f.localHeaderOffset.Store(localHeaderOffset)
	f.crc32.Store(crc32)
	f.flags.Store(uint32(flags))
	f.method.Store(uint32(method))
	f.encryption.Store(uint32(encryption))
	f.isDir.Store(isDir)

	f.meta.Store(&fileMeta{
		name:        name,
		comment:     comment,
		password:    password,
		mode:        mode,
		modTime:     modTime,
		hostSystem:  hostSystem,
		extraFields: maps.Clone(extraFields),
		rawExtra:    slices.Clone(rawExtra),
		metadata:    metadata,
	})

	return f
}

func (f *File) updateMeta(fn func(m *fileMeta)) {
	for {
		curr := f.meta.Load()
		var next fileMeta
		if curr != nil {
			next = *curr
			if curr.extraFields != nil {
				next.extraFields = maps.Clone(curr.extraFields)
			}
			if curr.rawExtra != nil {
				next.rawExtra = slices.Clone(curr.rawExtra)
			}
		}
		fn(&next)
		if f.meta.CompareAndSwap(curr, &next) {
			return
		}
	}
}

// Name returns the archive-relative normalized forward-slash path.
func (f *File) Name() string {
	if m := f.meta.Load(); m != nil {
		return m.name
	}
	return ""
}

// Size returns the uncompressed byte length of the file entry.
func (f *File) Size() int64 {
	return f.uncompressedSize.Load()
}

// UncompressedSize returns the uncompressed byte length of the file entry.
func (f *File) UncompressedSize() int64 {
	return f.uncompressedSize.Load()
}

// CompressedSize returns the compressed byte length of the file entry.
func (f *File) CompressedSize() int64 {
	return f.compressedSize.Load()
}

// CRC returns the CRC-32 checksum of the uncompressed data.
func (f *File) CRC() uint32 {
	return f.crc32.Load()
}

// Mode returns the file system permission and mode bits.
func (f *File) Mode() fs.FileMode {
	if m := f.meta.Load(); m != nil {
		return m.mode
	}
	return 0o644
}

// ModTime returns the modification time of the file entry.
func (f *File) ModTime() time.Time {
	if m := f.meta.Load(); m != nil {
		return m.modTime
	}
	return time.Time{}
}

// IsDir reports whether the entry represents a directory.
func (f *File) IsDir() bool {
	return f.isDir.Load()
}

// Method returns the compression method applied to this entry.
func (f *File) Method() codec.Method {
	return codec.Method(f.method.Load())
}

// Level returns the compression level applied to this entry.
func (f *File) Level() int {
	return int(f.level.Load())
}

// Encryption returns the encryption method applied to this entry.
func (f *File) Encryption() EncryptionMethod {
	return EncryptionMethod(f.encryption.Load())
}

// Password returns the per-entry encryption password.
func (f *File) Password() string {
	if m := f.meta.Load(); m != nil {
		return m.password
	}
	return ""
}

// HostSystem returns the creator host operating system identifier.
func (f *File) HostSystem() sys.HostSystem {
	if m := f.meta.Load(); m != nil {
		return m.hostSystem
	}
	return sys.DefaultHostSystem
}

// Metadata returns the NTFS/UNIX timestamp metadata.
func (f *File) Metadata() sys.Metadata {
	if m := f.meta.Load(); m != nil {
		return m.metadata
	}
	return sys.Metadata{}
}

// ExtraFields returns parsed extra fields indexed by 16-bit header ID.
func (f *File) ExtraFields() map[uint16][]byte {
	if m := f.meta.Load(); m != nil {
		return maps.Clone(m.extraFields)
	}
	return nil
}

// RawExtra returns the unparsed raw extra field byte buffer.
func (f *File) RawExtra() []byte {
	if m := f.meta.Load(); m != nil {
		return slices.Clone(m.rawExtra)
	}
	return nil
}

// LocalHeaderOffset returns the byte offset in the archive where the Local File Header begins.
func (f *File) LocalHeaderOffset() int64 {
	return f.localHeaderOffset.Load()
}

// Flags returns the GPBF (General Purpose Bit Flag) integer.
func (f *File) Flags() uint16 {
	return uint16(f.flags.Load())
}

// Comment returns the per-entry comment string.
func (f *File) Comment() string {
	if m := f.meta.Load(); m != nil {
		return m.comment
	}
	return ""
}

// IsImplicit reports whether the directory entry was synthesized implicitly.
func (f *File) IsImplicit() bool {
	return f.isImplicit.Load()
}

// SetName updates the entry path.
func (f *File) SetName(name string) {
	clean := vfs.CleanPath(name)
	f.updateMeta(func(m *fileMeta) {
		m.name = clean
	})
}

// SetLocalHeaderOffset updates the local header byte offset.
func (f *File) SetLocalHeaderOffset(offset int64) {
	f.localHeaderOffset.Store(offset)
}

// SetFlags updates the GPBF integer.
func (f *File) SetFlags(flags uint16) {
	f.flags.Store(uint32(flags))
}

// SetIsImplicit marks the entry as implicitly generated.
func (f *File) SetIsImplicit(implicit bool) {
	f.isImplicit.Store(implicit)
}

// SetSizes updates the uncompressed size, compressed size, and CRC-32 checksum.
func (f *File) SetSizes(uncompressed, compressed int64, crc uint32) {
	f.uncompressedSize.Store(uncompressed)
	f.compressedSize.Store(compressed)
	f.crc32.Store(crc)
}

// SetMethod updates the compression algorithm.
func (f *File) SetMethod(m codec.Method) {
	f.method.Store(uint32(m))
}

// SetOpenFunc assigns the lazy decompression stream opener.
func (f *File) SetOpenFunc(fn func() (io.ReadCloser, error)) {
	if fn == nil {
		f.openFunc.Store(nil)
	} else {
		f.openFunc.Store(&fn)
	}
}

// SetRawFunc assigns the raw compressed stream provider.
func (f *File) SetRawFunc(fn func() (*io.SectionReader, error)) {
	if fn == nil {
		f.rawFunc.Store(nil)
	} else {
		f.rawFunc.Store(&fn)
	}
}

// Open opens the decompressed (and decrypted) content stream for reading.
func (f *File) Open() (io.ReadCloser, error) {
	fnPtr := f.openFunc.Load()
	if fnPtr == nil || *fnPtr == nil {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return (*fnPtr)()
}

// OpenRaw opens the raw compressed byte stream if backed by a random-access archive.
func (f *File) OpenRaw() (*io.SectionReader, error) {
	fnPtr := f.rawFunc.Load()
	if fnPtr == nil || *fnPtr == nil {
		return nil, errors.New("file: raw stream not available for dynamic entry")
	}
	return (*fnPtr)()
}

// SaveTo extracts the uncompressed file directly to dstPath on the local filesystem.
func (f *File) SaveTo(dstPath string) error {
	targetPath, err := vfs.SafePath(filepath.Dir(dstPath), filepath.Base(dstPath))
	if err != nil {
		return err
	}

	if f.IsDir() {
		return os.MkdirAll(targetPath, 0o755)
	}

	if dir := filepath.Dir(targetPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", f.Name(), err)
		}
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open %s: %w", f.Name(), err)
	}
	defer rc.Close()

	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create %s: %w", f.Name(), err)
	}
	defer out.Close()

	bufPtr := pool.Get64K()
	defer pool.Put64K(bufPtr)

	if _, err := io.CopyBuffer(out, rc, *bufPtr); err != nil {
		return fmt.Errorf("write %s: %w", f.Name(), err)
	}

	return nil
}

// Bytes reads and returns the entire uncompressed contents of the file.
func (f *File) Bytes() ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// String reads and returns the uncompressed contents of the file as a string.
func (f *File) String() (string, error) {
	b, err := f.Bytes()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WithCompression configures the compression algorithm and level for the file entry.
func (f *File) WithCompression(m codec.Method, level int) *File {
	if !f.IsDir() {
		f.method.Store(uint32(m))
		f.level.Store(int32(level))
	}
	return f
}

// WithEncryption configures encryption for the file entry.
func (f *File) WithEncryption(enc EncryptionMethod, password string) *File {
	if !f.IsDir() {
		f.encryption.Store(uint32(enc))
		f.updateMeta(func(m *fileMeta) {
			m.password = password
		})
	}
	return f
}

// WithPassword sets the encryption password for the file entry.
func (f *File) WithPassword(password string) *File {
	if !f.IsDir() {
		f.updateMeta(func(m *fileMeta) {
			m.password = password
		})
		if f.Encryption() == NotEncrypted && password != "" {
			f.encryption.Store(uint32(AES256))
		}
	}
	return f
}

// WithComment sets the comment for the file entry.
func (f *File) WithComment(comment string) *File {
	f.updateMeta(func(m *fileMeta) {
		m.comment = comment
	})
	return f
}

// WithMode sets the permission and mode bits.
func (f *File) WithMode(mode fs.FileMode) *File {
	f.updateMeta(func(m *fileMeta) {
		m.mode = mode
	})
	return f
}

// WithModTime sets the modification time.
func (f *File) WithModTime(modTime time.Time) *File {
	f.updateMeta(func(m *fileMeta) {
		m.modTime = modTime
	})
	return f
}

// VFS Adapter methods to implement [vfs.VFSEntry].

func (f *File) EntryName() string                  { return f.Name() }
func (f *File) EntrySize() int64                   { return f.Size() }
func (f *File) EntryMode() fs.FileMode             { return f.Mode() }
func (f *File) EntryModTime() time.Time            { return f.ModTime() }
func (f *File) EntryIsDir() bool                   { return f.IsDir() }
func (f *File) OpenStream() (io.ReadCloser, error) { return f.Open() }

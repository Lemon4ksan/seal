// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package container defines archive container specifications, multiplexers, and framing engines.
package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lemon4ksan/foundation/generic"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
)

// Type identifies an archive container or compression format.
type Type string

const (
	// Unknown represents an unassigned or unrecognized archive container.
	Unknown Type = "unknown"

	// Zip represents the standard PKWARE ZIP archive container (.zip).
	Zip Type = "zip"

	// Tar represents the standard POSIX / GNU / USTAR TAR tape archive container (.tar).
	Tar Type = "tar"

	// TarGz represents a GZIP-compressed TAR archive container (.tar.gz, .tgz).
	TarGz Type = "tar.gz"

	// TarBz2 represents a BZIP2-compressed TAR archive container (.tar.bz2, .tbz2).
	TarBz2 Type = "tar.bz2"

	// TarXz represents an XZ-compressed TAR archive container (.tar.xz, .txz).
	TarXz Type = "tar.xz"

	// TarZst represents a Zstandard-compressed TAR archive container (.tar.zst, .tzst).
	TarZst Type = "tar.zst"

	// Gz represents a standalone GZIP compressed container stream (.gz).
	Gz Type = "gz"

	// Bz2 represents a standalone BZIP2 compressed container stream (.bz2).
	Bz2 Type = "bz2"

	// SevenZip represents the 7-Zip archive container (.7z).
	SevenZip Type = "7z"

	// Wim represents the Windows Imaging Format container (.wim).
	Wim Type = "wim"

	// Rar represents the RAR archive container (.rar).
	Rar Type = "rar"

	// Cab represents the Microsoft Cabinet archive container (.cab).
	Cab Type = "cab"

	// Iso represents the ISO 9660 / Joliet optical disc image container (.iso, .img).
	Iso Type = "iso"

	// Cpio represents the CPIO archive container (.cpio).
	Cpio Type = "cpio"

	// Ar represents the Unix AR archive and Debian package container (.ar, .deb, .a).
	Ar Type = "ar"

	// Eon represents the EON container format (.eon).
	Eon Type = "eon"
)

// String returns the string representation of the container type.
func (t Type) String() string {
	return string(t)
}

// IsTarFamily reports whether the container belongs to the TAR family or its compressed variants.
func (t Type) IsTarFamily() bool {
	switch t {
	case Tar, TarGz, TarBz2, TarXz, TarZst:
		return true
	default:
		return false
	}
}

// ErrArchiveFrozen indicates that archive creation was gracefully frozen on context cancellation,
// producing a valid partial archive with an embedded stealth checkpoint.
var ErrArchiveFrozen = errors.New("container: archive creation frozen; stealth checkpoint persisted")

// Config encapsulates parameterization options for container serialization.
type Config struct {
	Comment        string
	Password       string
	DefaultMethod  codec.Method
	DefaultLevel   int
	Registry       *codec.Registry
	CheckpointData []byte
	ExtensionData  []byte
	Context        context.Context
	Resumable      bool
	SessionID      string
	SourceDir      string
	OnFileDone     func(*file.File, error)
	OnProgress     func(delta int64, currentFile string)
	BytecodeFilter string
}

// Container defines the bidirectional serialization interface for archive containers.
type Container interface {
	// Type returns the container format identifier.
	Type() Type

	// OpenCatalog parses the table of contents and file entries from a random-access source (e.g. *os.File, *bytes.Reader, *io.SectionReader).
	OpenCatalog(rAt io.ReaderAt, size int64, password string, reg *codec.Registry) ([]*file.File, string, error)

	// ScanStream sequentially scans an incoming stream (e.g. stdin, HTTP body, pipe) to extract entries.
	ScanStream(r io.Reader, password string, reg *codec.Registry) ([]*file.File, string, error)

	// WriteArchive serializes the slice of files into dest and returns the total bytes written.
	WriteArchive(dest io.Writer, files []*file.File, cfg Config) (int64, error)
}

// Factory constructs a new [Container] instance.
type Factory func() Container

var registry generic.ConcurrentMap[Type, Factory]

// Register registers a container factory for the specified container type.
func Register(t Type, factory Factory) {
	registry.Store(t, factory)
}

// Get constructs a registered [Container] implementation for the requested type.
func Get(t Type) (Container, error) {
	factory, ok := registry.Load(t)
	if !ok {
		return nil, fmt.Errorf("container: unsupported container type %q", t)
	}
	return factory(), nil
}

// DetectExtension inspects a filename extension and infers the container type.
func DetectExtension(filename string) Type {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return TarGz
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return TarBz2
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return TarXz
	case strings.HasSuffix(lower, ".tar.zst"), strings.HasSuffix(lower, ".tzst"):
		return TarZst
	case strings.HasSuffix(lower, ".zip"):
		return Zip
	case strings.HasSuffix(lower, ".tar"):
		return Tar
	case strings.HasSuffix(lower, ".gz"):
		return Gz
	case strings.HasSuffix(lower, ".bz2"):
		return Bz2
	case strings.HasSuffix(lower, ".7z"):
		return SevenZip
	case strings.HasSuffix(lower, ".wim"), strings.HasSuffix(lower, ".swm"), strings.HasSuffix(lower, ".esd"):
		return Wim
	case strings.HasSuffix(lower, ".rar"):
		return Rar
	case strings.HasSuffix(lower, ".cab"):
		return Cab
	case strings.HasSuffix(lower, ".iso"), strings.HasSuffix(lower, ".img"):
		return Iso
	case strings.HasSuffix(lower, ".cpio"):
		return Cpio
	case strings.HasSuffix(lower, ".ar"), strings.HasSuffix(lower, ".deb"), strings.HasSuffix(lower, ".a"):
		return Ar
	case strings.HasSuffix(lower, ".eon"):
		return Eon
	default:
		ext := filepath.Ext(lower)
		if ext != "" {
			return Type(strings.TrimPrefix(ext, "."))
		}
		return Unknown
	}
}

// SniffHeader analyzes leading magic bytes from a data stream to identify the container type.
func SniffHeader(header []byte) Type {
	if len(header) < 2 {
		return Unknown
	}

	// EON container (EON!)
	if len(header) >= 4 && header[0] == 'E' && header[1] == 'O' && header[2] == 'N' && header[3] == '!' {
		return Eon
	}

	// GZIP container (.tar.gz, .tgz, .gz)
	if header[0] == 0x1F && header[1] == 0x8B {
		return TarGz
	}

	// ZIP container
	if len(header) >= 4 && header[0] == 0x50 && header[1] == 0x4B {
		if (header[2] == 0x03 && header[3] == 0x04) ||
			(header[2] == 0x05 && header[3] == 0x06) ||
			(header[2] == 0x07 && header[3] == 0x08) {
			return Zip
		}
	}

	// 7-Zip container
	if len(header) >= 6 && header[0] == '7' && header[1] == 'z' && header[2] == 0xBC && header[3] == 0xAF &&
		header[4] == 0x27 &&
		header[5] == 0x1C {
		return SevenZip
	}

	// WIM container (MSWIM\0\0\0)
	if len(header) >= 8 && header[0] == 'M' && header[1] == 'S' && header[2] == 'W' && header[3] == 'I' &&
		header[4] == 'M' && header[5] == 0 && header[6] == 0 && header[7] == 0 {
		return Wim
	}

	// Zstandard container
	if len(header) >= 4 && header[0] == 0x28 && header[1] == 0xB5 && header[2] == 0x2F && header[3] == 0xFD {
		return TarZst
	}

	// BZIP2 container
	if len(header) >= 3 && header[0] == 'B' && header[1] == 'Z' && header[2] == 'h' {
		return TarBz2
	}

	// XZ container
	if len(header) >= 6 && header[0] == 0xFD && header[1] == '7' && header[2] == 'z' && header[3] == 'X' &&
		header[4] == 'Z' &&
		header[5] == 0x00 {
		return TarXz
	}

	// RAR container (RAR 1.5 - 4.x: "Rar!\x1A\x07\x00", RAR 5.0+: "Rar!\x1A\x07\x01\x00")
	if len(header) >= 7 && header[0] == 'R' && header[1] == 'a' && header[2] == 'r' && header[3] == '!' &&
		header[4] == 0x1A && header[5] == 0x07 {
		if header[6] == 0x00 || (len(header) >= 8 && header[6] == 0x01 && header[7] == 0x00) {
			return Rar
		}
	}

	// TAR container (check "ustar" at offset 257)
	if len(header) >= 262 && string(header[257:262]) == "ustar" {
		return Tar
	}

	// CAB container (MSCF)
	if len(header) >= 4 && header[0] == 'M' && header[1] == 'S' && header[2] == 'C' && header[3] == 'F' {
		return Cab
	}

	// ISO 9660 container (CD001 at offset 32769)
	if len(header) >= 32774 && string(header[32769:32774]) == "CD001" {
		return Iso
	}

	// CPIO container (070701, 070702, 070707 or binary)
	if len(header) >= 6 {
		magic := string(header[:6])
		if magic == "070701" || magic == "070702" || magic == "070707" {
			return Cpio
		}
		if (header[0] == 0xC7 && header[1] == 0x71) || (header[0] == 0x71 && header[1] == 0xC7) {
			return Cpio
		}
	}

	// AR container (!<arch>\n)
	if len(header) >= 8 && string(header[:8]) == "!<arch>\n" {
		return Ar
	}

	return Unknown
}

// Sentinel container errors.
var (
	ErrUnsupportedContainer = errors.New("container: unsupported archive container format")
	ErrCorruptedContainer   = errors.New("container: corrupted archive container headers")
)

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package container defines archive container specifications, multiplexers, and framing engines.
package container

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

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

	// Rar represents the RAR archive container (.rar).
	Rar Type = "rar"
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

// Config encapsulates parameterization options for container serialization.
type Config struct {
	Comment       string
	Password      string
	DefaultMethod codec.Method
	DefaultLevel  int
	Registry      *codec.Registry
	OnFileDone    func(*file.File, error)
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

var (
	registryMu sync.RWMutex
	registry   = make(map[Type]Factory)
)

// Register registers a container factory for the specified container type.
func Register(t Type, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[t] = factory
}

// Get constructs a registered [Container] implementation for the requested type.
func Get(t Type) (Container, error) {
	registryMu.RLock()
	factory, ok := registry[t]
	registryMu.RUnlock()

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
	case strings.HasSuffix(lower, ".rar"):
		return Rar
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

	// RAR container
	if len(header) >= 7 && string(header[:7]) == "Rar!\x1A\x07\x00" {
		return Rar
	}

	// TAR container (check "ustar" at offset 257)
	if len(header) >= 262 && string(header[257:262]) == "ustar" {
		return Tar
	}

	return Unknown
}

// Sentinel container errors.
var (
	ErrUnsupportedContainer = errors.New("container: unsupported archive container format")
	ErrCorruptedContainer   = errors.New("container: corrupted archive container headers")
)

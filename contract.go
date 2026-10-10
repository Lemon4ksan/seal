// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"errors"
	"io"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/option"
	"github.com/lemon4ksan/seal/vfs"
)

// Universal Archive & Container Atoms.
//
// These core aliases bridge the modular subsystems (archive, container, file, codec, vfs, option)
// into a unified, zero-friction developer experience matching modern Go standards.
type (
	// Archive represents an in-memory mutable archive builder and manager.
	Archive = archive.Archive

	// File represents a single canonical, lock-free file or directory record across all containers.
	File = file.File

	// Container defines the serialization backend interface.
	Container = container.Container

	// ContainerType identifies the archive container format (ZIP, TAR, TAR.GZ, 7Z, etc.).
	ContainerType = container.Type

	// ContainerConfig encapsulates parameters passed to container serialization engines.
	ContainerConfig = container.Config

	// FileSortStrategy defines the ordering of entries when serializing an archive.
	FileSortStrategy = archive.FileSortStrategy

	// ConflictAction represents a name collision policy.
	ConflictAction = archive.ConflictAction

	// ConflictHandler resolves name collisions during archive merging and loading.
	ConflictHandler = archive.ConflictHandler

	// EncryptionMethod identifies the payload encryption algorithm.
	EncryptionMethod = file.EncryptionMethod

	// CompressionMethod aliases codec algorithm identifiers.
	CompressionMethod = codec.Method

	// Option represents a composable configuration option.
	Option = option.Option

	// Iterator provides cursor-based traversal over archive files.
	Iterator = archive.Iterator

	// Filter evaluates inclusion or exclusion of archive entries.
	Filter = option.Filter

	// ProgressFunc reports real-time transfer progress for archive I/O.
	ProgressFunc = option.ProgressFunc

	// ProgressStats delivers real-time throughput telemetry during archive operations.
	ProgressStats = option.ProgressStats

	// SecuritySettings configures decompression bomb defenses and traversal guards.
	SecuritySettings = option.SecuritySettings

	// ResourceLimits defines limits on uncompressed sizes and expansion ratios.
	ResourceLimits = vfs.ResourceLimits

	// Source provides an abstraction for reading archive payloads from disk, memory, or network streams.
	Source interface {
		Open() (io.Reader, int64, error)
		Close() error
	}

	// Sink provides an abstraction for writing generated archive payloads to disk, memory, or network streams.
	Sink interface {
		Create() (io.Writer, error)
		Close() error
	}
)

// Encryption algorithm constants.
const (
	NotEncrypted = file.NotEncrypted
	ZipCrypto    = file.ZipCrypto
	AES256       = file.AES256
)

// Compression method constants.
const (
	Store     = codec.Store
	Deflate   = codec.Deflate
	Deflate64 = codec.Deflate64
	BZIP2     = codec.BZIP2
	LZMA      = codec.LZMA
	ZStandard = codec.ZStandard
	Gzip      = codec.Gzip
	Zlib      = codec.Zlib
)

// Collision resolution action constants.
const (
	ActionReplace = archive.ActionReplace
	ActionSkip    = archive.ActionSkip
	ActionError   = archive.ActionError
	ActionRename  = archive.ActionRename
)

// Serialization sorting strategy constants.
const (
	SortDefault      = archive.SortDefault
	SortNameAsc      = archive.SortNameAsc
	SortNameDesc     = archive.SortNameDesc
	SortSizeAsc      = archive.SortSizeAsc
	SortSizeDesc     = archive.SortSizeDesc
	SortExtensionAsc = archive.SortExtensionAsc
)

// Exported sentinel domain errors.
var (
	ErrSignatureInvalid   = errors.New("seal: cryptographic signature verification failed")
	ErrSignatureNotFound  = errors.New("seal: no embedded signature found in archive")
	ErrProvenanceNotFound = errors.New("seal: no build provenance found in archive")
	ErrInvalidPassword    = errors.New("seal: incorrect or missing archive password")
	ErrEntryNotFound      = errors.New("seal: file entry not found in archive")
)

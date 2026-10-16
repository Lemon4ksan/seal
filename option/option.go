// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package option provides zero-allocation stack-allocated modifier payloads for configuring archives, codecs, and I/O pipelines.
package option

import (
	"io/fs"
	"time"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/extension"
)

// OptionKind identifies the discrete mutation payload of an [Option] value.
type OptionKind uint8

const (
	OptNone OptionKind = iota
	OptContainer
	OptCompression
	OptPassword
	OptComment
	OptWorkers
	OptSortStrategy
	OptConflictPolicy
	OptFilter
	OptSecurity
	OptProgress
	OptFileDone
	OptName
	OptPathPrefix
	OptMode
	OptModTime
	OptCustom
	OptResumable
	OptSessionID
	OptSourceDir
	OptCheckpoint
	OptExtension
	OptBytecodeFilter
	OptSolid
	OptSolidExtension
	OptEncoding
)

// ProgressStats delivers real-time throughput and status telemetry during archive operations.
type ProgressStats struct {
	CurrentFile    string // Name of the file currently being processed
	CurrentRead    int64  // Bytes read from current entry
	CurrentWritten int64  // Bytes written for current entry
	ExpectedRead   int64  // Cumulative uncompressed bytes expected
	TotalRead      int64  // Total uncompressed bytes processed
	TotalWritten   int64  // Total compressed bytes produced
	ArchiveWritten int64  // Total bytes flushed to destination
	TotalFiles     int64  // Total count of entries to process
	ProcessedFiles int64  // Count of completed entries (including errors)
	Errors         int64  // Count of failed entries
}

// ProgressFunc defines a progress callback signature.
type ProgressFunc func(stats ProgressStats)

// Option represents a zero-allocation, value-based modifier payload residing on the stack.
type Option struct {
	Kind           OptionKind
	Container      container.Type
	Method         codec.Method
	Level          int
	Workers        int
	Text           string
	Mode           fs.FileMode
	ModTime        time.Time
	Security       SecuritySettings
	Filter         Filter
	Progress       ProgressFunc
	Custom         func(any)
	OnDone         func(entryName string, err error)
	Resumable      bool
	SourceDir      string
	SessionID      string
	Bytes          []byte
	Extensions     []extension.Extension
	Solid          bool
	SolidSize      int64
	SolidExtension bool
	Encoding       string
	TextDecoder    func(string) string
}

// IsZero reports whether the option is an uninitialized zero value.
func (o Option) IsZero() bool {
	return o.Kind == OptNone
}

// Functional option constructors returning stack value structs.

// WithContainer sets the target archive container (ZIP, TAR, TAR.GZ, etc.).
func WithContainer(containerType container.Type) Option {
	return Option{Kind: OptContainer, Container: containerType}
}

// WithCompression configures the compression algorithm and level.
func WithCompression(method codec.Method, level int) Option {
	return Option{Kind: OptCompression, Method: method, Level: level}
}

// WithCompressionLevel sets the compression effort level (1 = Fastest, 5 = Normal, 9 = Ultra).
func WithCompressionLevel(level int) Option {
	return Option{Kind: OptCompression, Level: level}
}

// WithPassword sets the encryption password.
func WithPassword(pwd string) Option {
	return Option{Kind: OptPassword, Text: pwd}
}

// WithComment sets the archive or entry comment.
func WithComment(comment string) Option {
	return Option{Kind: OptComment, Text: comment}
}

// WithWorkers sets multi-worker concurrency.
func WithWorkers(workers int) Option {
	if workers <= 0 {
		workers = 1
	}
	return Option{Kind: OptWorkers, Workers: workers}
}

// WithFilter attaches an entry filter predicate.
func WithFilter(f Filter) Option {
	return Option{Kind: OptFilter, Filter: f}
}

// WithOnly retains only entries matching the specified paths.
func WithOnly(names ...string) Option {
	return Option{Kind: OptFilter, Filter: FilterOnly(names...)}
}

// WithFromDir retains only entries residing under dirPath.
func WithFromDir(dirPath string) Option {
	return Option{Kind: OptFilter, Filter: FilterFromDir(dirPath)}
}

// WithExcludeDir excludes dirPath and all its descendants.
func WithExcludeDir(dirPath string) Option {
	return Option{Kind: OptFilter, Filter: FilterExcludeDir(dirPath)}
}

// WithSecurity establishes extraction containment and resource limits.
func WithSecurity(sec SecuritySettings) Option {
	return Option{Kind: OptSecurity, Security: sec}
}

// WithProgress attaches a progress monitoring callback.
func WithProgress(fn ProgressFunc) Option {
	return Option{Kind: OptProgress, Progress: fn}
}

// WithOnFileDone attaches an entry completion callback.
func WithOnFileDone(fn func(entryName string, err error)) Option {
	return Option{Kind: OptFileDone, OnDone: fn}
}

// WithName overrides the relative destination filename of the entry inside the archive.
func WithName(name string) Option {
	return Option{Kind: OptName, Text: name}
}

// WithPath prepends a directory prefix to the entry's archive path.
func WithPath(prefix string) Option {
	return Option{Kind: OptPathPrefix, Text: prefix}
}

// WithMode sets custom permission and mode bits on the file entry.
func WithMode(mode fs.FileMode) Option {
	return Option{Kind: OptMode, Mode: mode}
}

// WithModTime sets a custom modification timestamp on the file entry.
func WithModTime(t time.Time) Option {
	return Option{Kind: OptModTime, ModTime: t}
}

// WithCustom attaches a custom closure callback for arbitrary runtime manipulation.
func WithCustom(fn func(any)) Option {
	return Option{Kind: OptCustom, Custom: fn}
}

// Custom attaches a custom closure callback for arbitrary runtime manipulation.
//
// Deprecated: Use WithCustom instead for consistency with the With... option convention.
func Custom(fn func(any)) Option {
	return WithCustom(fn)
}

// WithResumable enables graceful freeze on cancellation and stealth checkpoint persistence.
func WithResumable(enable ...bool) Option {
	val := true
	if len(enable) > 0 {
		val = enable[0]
	}
	return Option{Kind: OptResumable, Resumable: val}
}

// WithSessionID assigns a task or job session UUID for the archive operation.
func WithSessionID(id string) Option {
	return Option{Kind: OptSessionID, Text: id, SessionID: id}
}

// WithSourceDir sets the base source directory path for resumption.
func WithSourceDir(dir string) Option {
	return Option{Kind: OptSourceDir, Text: dir, SourceDir: dir}
}

// WithCheckpointData injects raw transient checkpoint bytes into container options.
func WithCheckpointData(data []byte) Option {
	return Option{Kind: OptCheckpoint, Bytes: data}
}

// WithExtension registers one or more metadata extensions with the archive operation.
func WithExtension(exts ...extension.Extension) Option {
	return Option{Kind: OptExtension, Extensions: exts}
}

// WithBytecodeFilter configures the bytecode preprocessing filter (e.g. "bcj", "bcj2", "arm").
func WithBytecodeFilter(filter string) Option {
	return Option{Kind: OptBytecodeFilter, Text: filter}
}

// WithSolid enables or disables solid archiving, with an optional maximum solid block size in bytes.
// If size is omitted or <= 0, the container default solid size (e.g. 64 MiB for EON) is used.
func WithSolid(solid bool, size ...int64) Option {
	var sz int64
	if len(size) > 0 {
		sz = size[0]
	}
	return Option{Kind: OptSolid, Solid: solid, SolidSize: sz}
}

// WithSolidExtension configures whether solid blocks should be split/grouped by file extension.
func WithSolidExtension(enabled bool) Option {
	return Option{Kind: OptSolidExtension, SolidExtension: enabled}
}

// WithEncoding sets the character encoding/codepage for non-UTF8 archive metadata.
func WithEncoding(enc string) Option {
	return Option{Kind: OptEncoding, Encoding: enc, Text: enc}
}

// WithTextDecoder sets an explicit fallback TextDecoder for non-UTF8 archive metadata.
func WithTextDecoder(dec func(string) string) Option {
	return Option{Kind: OptEncoding, TextDecoder: dec}
}

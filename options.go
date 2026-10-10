// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"io/fs"
	"time"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/option"
)

// Forwarded functional option constructors returning zero-allocation stack structs.

// WithContainer sets the target archive container (ZIP, TAR, TAR.GZ, etc.).
func WithContainer(containerType container.Type) Option {
	eng, err := DefaultContainer(containerType)
	if err == nil {
		return archive.WithContainer(eng)
	}
	return option.WithContainer(containerType)
}

// WithCompression configures the compression algorithm and level.
func WithCompression(method codec.Method, level int) Option {
	return option.WithCompression(method, level)
}

// WithPassword sets the encryption password.
func WithPassword(pwd string) Option {
	return option.WithPassword(pwd)
}

// WithComment sets the archive or entry comment.
func WithComment(comment string) Option {
	return option.WithComment(comment)
}

// WithWorkers sets multi-worker concurrency.
func WithWorkers(workers int) Option {
	return option.WithWorkers(workers)
}

// WithFilter attaches an entry filter predicate.
func WithFilter(f option.Filter) Option {
	return option.WithFilter(f)
}

// WithOnly retains only entries matching the specified paths.
func WithOnly(names ...string) Option {
	return option.WithOnly(names...)
}

// WithFromDir retains only entries residing under dirPath.
func WithFromDir(dirPath string) Option {
	return option.WithFromDir(dirPath)
}

// WithExcludeDir excludes dirPath and all its descendants.
func WithExcludeDir(dirPath string) Option {
	return option.WithExcludeDir(dirPath)
}

// WithSecurity establishes extraction containment and resource limits.
func WithSecurity(sec option.SecuritySettings) Option {
	return option.WithSecurity(sec)
}

// WithProgress attaches a progress monitoring callback.
func WithProgress(fn option.ProgressFunc) Option {
	return option.WithProgress(fn)
}

// WithOnFileDone attaches an entry completion callback.
func WithOnFileDone(fn func(entryName string, err error)) Option {
	return option.WithOnFileDone(fn)
}

// WithName overrides the relative destination filename of the entry inside the archive.
func WithName(name string) Option {
	return option.WithName(name)
}

// WithPath prepends a directory prefix to the entry's archive path.
func WithPath(prefix string) Option {
	return option.WithPath(prefix)
}

// WithMode sets custom permission and mode bits on the file entry.
func WithMode(mode fs.FileMode) Option {
	return option.WithMode(mode)
}

// WithModTime sets a custom modification timestamp on the file entry.
func WithModTime(t time.Time) Option {
	return option.WithModTime(t)
}

// WithSortStrategy configures the entry sorting strategy during archive writing.
func WithSortStrategy(s FileSortStrategy) Option {
	return archive.WithSortStrategy(s)
}

// WithConflictHandler configures the name collision resolution policy.
func WithConflictHandler(h ConflictHandler) Option {
	return archive.WithConflictHandler(h)
}

// WithResumable enables graceful freeze on cancellation and stealth checkpoint persistence.
func WithResumable(enable ...bool) Option {
	return option.WithResumable(enable...)
}

// WithSessionID assigns a task or job session UUID for the archive operation.
func WithSessionID(id string) Option {
	return option.WithSessionID(id)
}

// WithSourceDir sets the base source directory path for resumption.
func WithSourceDir(dir string) Option {
	return option.WithSourceDir(dir)
}

// WithCheckpointData injects raw transient checkpoint bytes into container options.
func WithCheckpointData(data []byte) Option {
	return option.WithCheckpointData(data)
}

// WithExtension registers one or more metadata extensions with the archive operation.
func WithExtension(exts ...extension.Extension) Option {
	return option.WithExtension(exts...)
}

// WithBytecodeFilter configures the bytecode preprocessing filter (e.g. "bcj", "bcj2", "arm").
func WithBytecodeFilter(filter string) Option {
	return option.WithBytecodeFilter(filter)
}

// WithSolid enables or disables solid archiving, with an optional maximum solid block size in bytes.
// If size is omitted or <= 0, container-default solid block sizes (e.g. 64 MiB for EON) are used.
func WithSolid(solid bool, size ...int64) Option {
	return option.WithSolid(solid, size...)
}

// WithSolidExtension configures whether solid blocks should be split/grouped by file extension.
func WithSolidExtension(enabled bool) Option {
	return option.WithSolidExtension(enabled)
}

// WithEncoding specifies the character encoding/codepage for non-UTF-8 archive metadata (e.g. "cp866", "shift_jis", "windows-1251").
func WithEncoding(name string) Option {
	opt := option.WithEncoding(name)
	if dec, err := DecoderByName(name); err == nil {
		opt.TextDecoder = dec
	}
	return opt
}

// WithTextDecoder attaches an explicit fallback text decoder function for non-UTF-8 metadata.
func WithTextDecoder(dec TextDecoder) Option {
	return option.WithTextDecoder(dec)
}


// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/internal/pool"
	"github.com/lemon4ksan/seal/option"
	"github.com/lemon4ksan/seal/vfs"
)

type processConfig struct {
	workers    int
	password   string
	filters    []option.Filter
	security   option.SecuritySettings
	onProgress option.ProgressFunc
	onDone     func(string, error)
}

// Archiver provides a configurable multi-container archiving environment with context cancellation and error aggregation.
//
// # Architectural Philosophy
//
// Archiver operates at the orchestrator layer. It coordinates filesystem traversal, streaming,
// security boundary validation, resource limits, and progress telemetry while delegating in-memory mutations
// and serialization to [archive.Archive] and [container.Container]. It never manipulates raw byte headers directly.
type Archiver struct {
	engineOptions []Option
}

// NewArchiver constructs a reusable [Archiver] engine configured with the specified options.
func NewArchiver(opts ...Option) *Archiver {
	return &Archiver{engineOptions: opts}
}

func resolveSourceEngine(r io.Reader, rAt io.ReaderAt) (container.Container, io.Reader) {
	var header [512]byte
	if rAt != nil {
		n, _ := rAt.ReadAt(header[:], 0)
		t := container.SniffHeader(header[:n])
		if eng, err := DefaultEngine(t); err == nil {
			return eng, r
		}
	} else if r != nil {
		br, isBR := r.(*bufio.Reader)
		if !isBR {
			br = bufio.NewReader(r)
			r = br
		}
		peeked, _ := br.Peek(512)
		t := container.SniffHeader(peeked)
		if eng, err := DefaultEngine(t); err == nil {
			return eng, r
		}
	}
	eng, _ := DefaultEngine(container.Zip)
	return eng, r
}

// ArchiveDir creates an archive containing the specified directory tree and writes it to dest.
func (a *Archiver) ArchiveDir(dirPath string, dest Sink, opts ...Option) error {
	return a.ArchiveDirWithContext(context.Background(), dirPath, dest, opts...)
}

// ArchiveDirWithContext creates an archive containing the directory tree with cancellation support.
func (a *Archiver) ArchiveDirWithContext(ctx context.Context, dirPath string, dest Sink, opts ...Option) error {
	allOpts := slices.Concat(a.engineOptions, opts)
	arc := NewArchive(allOpts...)
	if _, err := arc.AddDir(dirPath); err != nil {
		return err
	}

	return UseSink(dest, func(w io.Writer) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := arc.WriteTo(w, opts...)
		return err
	})
}

// Unarchive extracts an archive from src into destDir.
func (a *Archiver) Unarchive(src Source, destDir string, opts ...Option) error {
	return a.UnarchiveWithContext(context.Background(), src, destDir, opts...)
}

// UnarchiveWithContext extracts an archive with context awareness, format auto-detection, and resource limit enforcement.
func (a *Archiver) UnarchiveWithContext(ctx context.Context, src Source, destDir string, opts ...Option) error {
	allOpts := slices.Concat(a.engineOptions, opts)
	procCfg := processConfig{workers: 1}
	for _, opt := range allOpts {
		switch opt.Kind {
		case option.OptWorkers:
			procCfg.workers = opt.Workers
		case option.OptPassword:
			procCfg.password = opt.Text
		case option.OptFilter:
			procCfg.filters = append(procCfg.filters, opt.Filter)
		case option.OptSecurity:
			procCfg.security = opt.Security
		case option.OptProgress:
			procCfg.onProgress = opt.Progress
		case option.OptFileDone:
			procCfg.onDone = opt.OnDone
		case option.OptCustom:
			if opt.Custom != nil {
				opt.Custom(&procCfg)
			}
		}
	}

	return UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		engine, r := resolveSourceEngine(r, rAt)
		arc := NewArchive(append([]Option{archive.WithEngine(engine)}, allOpts...)...)
		var files []*File
		var err error
		if rAt != nil && size > 0 {
			files, err = arc.Load(rAt, size, engine)
		} else {
			files, err = arc.LoadFromReader(r, engine)
		}
		if err != nil {
			return err
		}

		var errs []error
		var globalExtracted int64

		for _, f := range files {
			if err := ctx.Err(); err != nil {
				return err
			}

			// Evaluate inclusion filters
			if len(procCfg.filters) > 0 {
				include := true
				for _, filter := range procCfg.filters {
					if !filter(f.Name(), f.IsDir()) {
						include = false
						break
					}
				}
				if !include {
					continue
				}
			}

			targetPath, err := vfs.SafePath(destDir, f.Name())
			if err != nil {
				errs = append(errs, wrapErr("extract", f.Name(), err))
				continue
			}

			if f.IsDir() {
				if err := os.MkdirAll(targetPath, 0o755); err != nil {
					errs = append(errs, wrapErr("mkdir", f.Name(), err))
				}
				continue
			}

			if dir := filepath.Dir(targetPath); dir != "" && dir != "." {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					errs = append(errs, wrapErr("mkdir", f.Name(), err))
					continue
				}
			}

			rc, err := f.Open()
			if err != nil {
				errs = append(errs, wrapErr("open", f.Name(), err))
				if procCfg.onDone != nil {
					procCfg.onDone(f.Name(), err)
				}
				continue
			}

			mode := f.Mode()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
			if err != nil {
				_ = rc.Close()
				errs = append(errs, wrapErr("create", f.Name(), err))
				if procCfg.onDone != nil {
					procCfg.onDone(f.Name(), err)
				}
				continue
			}

			var w io.Writer = out
			if procCfg.security.ResourceLimits.MaxFileSize > 0 || procCfg.security.ResourceLimits.MaxTotalSize > 0 ||
				procCfg.security.ResourceLimits.MaxRatio > 0 {
				w = vfs.NewSecureWriter(out, procCfg.security.ResourceLimits, f.CompressedSize(), &globalExtracted)
			}

			bufPtr := pool.Get64K()
			_, copyErr := io.CopyBuffer(w, rc, *bufPtr)
			pool.Put64K(bufPtr)
			_ = rc.Close()
			_ = out.Close()

			if copyErr != nil {
				errs = append(errs, wrapErr("write", f.Name(), copyErr))
			}
			if procCfg.onDone != nil {
				procCfg.onDone(f.Name(), copyErr)
			}
		}

		return errors.Join(errs...)
	})
}

// Extract extracts an archive from src into destDir using automatic format detection.
func Extract(src Source, destDir string, opts ...Option) error {
	return NewArchiver().Unarchive(src, destDir, opts...)
}

// ExtractFile extracts an individual named file from an archive into destPath.
func ExtractFile(src Source, targetFile, destPath string, opts ...Option) error {
	cleanTarget := vfs.CleanPath(targetFile)

	return UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		engine, r := resolveSourceEngine(r, rAt)
		arc := NewArchive(append([]Option{archive.WithEngine(engine)}, opts...)...)
		if rAt != nil && size > 0 {
			if _, err := arc.Load(rAt, size, engine); err != nil {
				return err
			}
		} else {
			if _, err := arc.LoadFromReader(r, engine); err != nil {
				return err
			}
		}

		f, ok := arc.File(cleanTarget)
		if !ok {
			return ErrFileNotFound
		}
		return f.SaveTo(destPath)
	})
}

// ReadFile reads an individual named file from an archive into memory.
func ReadFile(src Source, targetFile string, opts ...Option) ([]byte, error) {
	cleanTarget := vfs.CleanPath(targetFile)

	var data []byte
	err := UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		engine, r := resolveSourceEngine(r, rAt)
		arc := NewArchive(append([]Option{archive.WithEngine(engine)}, opts...)...)
		if rAt != nil && size > 0 {
			if _, err := arc.Load(rAt, size, engine); err != nil {
				return err
			}
		} else {
			if _, err := arc.LoadFromReader(r, engine); err != nil {
				return err
			}
		}

		f, ok := arc.File(cleanTarget)
		if !ok {
			return ErrFileNotFound
		}

		var err error
		data, err = f.Bytes()
		return err
	})

	return data, err
}

// ReadFile reads an individual named file from an archive into memory.
func (a *Archiver) ReadFile(src Source, targetFile string, opts ...Option) ([]byte, error) {
	allOpts := slices.Concat(a.engineOptions, opts)
	return ReadFile(src, targetFile, allOpts...)
}

// Verify checks the integrity of all file entries and checksums within an archive.
func (a *Archiver) Verify(src Source, opts ...Option) error {
	allOpts := slices.Concat(a.engineOptions, opts)
	return Verify(src, allOpts...)
}

// GetEntries returns a metadata snapshot of all file entries in the archive.
func (a *Archiver) GetEntries(src Source) ([]*File, error) {
	var files []*File
	err := UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		engine, r := resolveSourceEngine(r, rAt)
		arc := NewArchive(append([]Option{archive.WithEngine(engine)}, a.engineOptions...)...)
		var err error
		if rAt != nil && size > 0 {
			files, err = arc.Load(rAt, size, engine)
		} else {
			files, err = arc.LoadFromReader(r, engine)
		}
		return err
	})

	return files, err
}

// List returns a list of archive entries from src.
func List(src Source, opts ...Option) ([]*File, error) {
	return NewArchiver(opts...).GetEntries(src)
}

// Verify checks the integrity of all file entries and checksums within an archive.
func Verify(src Source, opts ...Option) error {
	return UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		engine, r := resolveSourceEngine(r, rAt)
		arc := NewArchive(append([]Option{archive.WithEngine(engine)}, opts...)...)
		var files []*File
		var err error
		if rAt != nil && size > 0 {
			files, err = arc.Load(rAt, size, engine)
		} else {
			files, err = arc.LoadFromReader(r, engine)
		}
		if err != nil {
			return err
		}

		var errs []error
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				errs = append(errs, wrapErr("verify", f.Name(), err))
				continue
			}
			bufPtr := pool.Get64K()
			_, copyErr := io.CopyBuffer(io.Discard, rc, *bufPtr)
			pool.Put64K(bufPtr)
			_ = rc.Close()

			if copyErr != nil {
				errs = append(errs, wrapErr("verify", f.Name(), copyErr))
			}
		}
		return errors.Join(errs...)
	})
}

// Top-level 1-line zero-config convenience helpers.

// ArchiveDir compresses a physical directory into a destination Sink.
func ArchiveDir(dirPath string, dest Sink, opts ...Option) error {
	return NewArchiver(opts...).ArchiveDir(dirPath, dest)
}

// Unarchive extracts an archive from Source into destDir.
func Unarchive(src Source, destDir string, opts ...Option) error {
	return NewArchiver().Unarchive(src, destDir, opts...)
}

// FileCount returns the number of entries in the archive.
func FileCount(src Source) (int, error) {
	entries, err := List(src)
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// ExistsIn reports whether targetFile exists inside the archive.
func ExistsIn(src Source, targetFile string) (bool, error) {
	clean := vfs.CleanPath(targetFile)
	entries, err := List(src)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(entries, func(e *File) bool {
		return e.Name() == clean
	}), nil
}

// Tree returns an ASCII visual hierarchy string from an archive source.
func Tree(src Source) (string, error) {
	entries, err := List(src)
	if err != nil {
		return "", err
	}

	treeEntries := make([]struct {
		Name  string
		IsDir bool
	}, len(entries))

	for i, e := range entries {
		treeEntries[i].Name = e.Name()
		treeEntries[i].IsDir = e.IsDir()
	}

	root := vfs.BuildTree(treeEntries)
	return vfs.RenderTree(root), nil
}

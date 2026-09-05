// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/vfs"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/checkpoint"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/sevenzip"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/extension/provenance"
	"github.com/lemon4ksan/seal/extension/signature"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/internal/pool"
	"github.com/lemon4ksan/seal/option"
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
	containerOptions []Option
}

// NewArchiver constructs a reusable [Archiver] engine configured with the specified options.
func NewArchiver(opts ...Option) *Archiver {
	return &Archiver{containerOptions: opts}
}

func resolveSourceContainer(r io.Reader, rAt io.ReaderAt) (container.Container, io.Reader) {
	var header [512]byte
	if rAt != nil {
		n, _ := rAt.ReadAt(header[:], 0)
		t := container.SniffHeader(header[:n])
		if eng, err := DefaultContainer(t); err == nil {
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
		if eng, err := DefaultContainer(t); err == nil {
			return eng, r
		}
	}
	eng, _ := DefaultContainer(container.Zip)
	return eng, r
}

// ArchiveDir creates an archive containing the specified directory tree and writes it to dest with cancellation support.
func (a *Archiver) ArchiveDir(ctx context.Context, dirPath string, dest Sink, opts ...Option) error {
	allOpts := slices.Concat(a.containerOptions, opts)
	allOpts = append(allOpts, WithSourceDir(dirPath))
	arc := NewArchive(allOpts...)
	if _, err := arc.AddDir(dirPath); err != nil {
		return err
	}

	return UseSink(dest, func(w io.Writer) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		writeOpts := append(slices.Clone(allOpts), WithSourceDir(dirPath))
		_, err := arc.WriteTo(ctx, w, writeOpts...)
		return err
	})
}

// Unarchive extracts an archive from src into destDir with context awareness, format auto-detection, and resource limit enforcement.
func (a *Archiver) Unarchive(ctx context.Context, src Source, destDir string, opts ...Option) error {
	allOpts := slices.Concat(a.containerOptions, opts)
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

		container, r := resolveSourceContainer(r, rAt)
		arc := NewArchive(append([]Option{archive.WithContainer(container)}, allOpts...)...)
		var files []*File
		var err error
		if rAt != nil && size > 0 {
			files, err = arc.Load(rAt, size, container)
		} else {
			files, err = arc.LoadFromReader(r, container)
		}
		if err != nil {
			return err
		}

		var errs []error
		var errMu sync.Mutex
		var globalExtracted int64

		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return err
		}
		var createdDirs sync.Map
		createdDirs.Store(destDir, struct{}{})
		createdDirs.Store(filepath.Clean(destDir), struct{}{})

		// Ensure directory hierarchy exists prior to writing leaf files
		for _, f := range files {
			if f.IsDir() {
				targetPath, err := vfs.SafePath(destDir, f.Name())
				if err != nil {
					errs = append(errs, wrapErr("extract", f.Name(), err))
					continue
				}
				if err := os.MkdirAll(targetPath, 0o755); err != nil {
					errs = append(errs, wrapErr("mkdir", f.Name(), err))
				} else {
					createdDirs.Store(targetPath, struct{}{})
				}
			}
		}

		// Dispatch regular files and symlinks across worker pool for parallel disk writes
		nonDirFiles := generic.Filter(files, func(f *File) bool {
			return !f.IsDir()
		})

		numWorkers := procCfg.workers
		if numWorkers <= 0 {
			numWorkers = min(runtime.GOMAXPROCS(0), len(nonDirFiles))
		}
		if numWorkers <= 0 {
			numWorkers = 1
		}

		var wg sync.WaitGroup
		var taskCounter atomic.Int64

		for w := 0; w < numWorkers; w++ {
			wg.Go(func() {
				for {
					if err := ctx.Err(); err != nil {
						errMu.Lock()
						errs = append(errs, err)
						errMu.Unlock()
						return
					}

					idx := int(taskCounter.Add(1) - 1)
					if idx >= len(nonDirFiles) {
						return
					}
					f := nonDirFiles[idx]

					// Evaluate inclusion filters
					if len(procCfg.filters) > 0 && !generic.All(procCfg.filters, func(filter option.Filter) bool {
						return filter(f.Name(), f.IsDir())
					}) {
						continue
					}

					targetPath, err := vfs.SafePath(destDir, f.Name())
					if err != nil {
						errMu.Lock()
						errs = append(errs, wrapErr("extract", f.Name(), err))
						errMu.Unlock()
						continue
					}

					if dir := filepath.Dir(targetPath); dir != "" && dir != "." {
						if _, ok := createdDirs.Load(dir); !ok {
							_ = os.MkdirAll(dir, 0o755)
							createdDirs.Store(dir, struct{}{})
						}
					}

					rc, err := f.Open()
					if err != nil {
						errMu.Lock()
						errs = append(errs, wrapErr("open", f.Name(), err))
						errMu.Unlock()
						if procCfg.onDone != nil {
							procCfg.onDone(f.Name(), err)
						}
						continue
					}

					// Handle symlinks
					if f.Mode()&os.ModeSymlink != 0 {
						targetBytes, err := io.ReadAll(rc)
						_ = rc.Close()
						if err == nil {
							_ = os.Remove(targetPath)
							if err := os.Symlink(string(targetBytes), targetPath); err != nil {
								errMu.Lock()
								errs = append(errs, wrapErr("symlink", f.Name(), err))
								errMu.Unlock()
							}
						}
						continue
					}

					mode := generic.Coalesce(f.Mode(), 0o644)
					out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
					if err != nil {
						_ = rc.Close()
						errMu.Lock()
						errs = append(errs, wrapErr("create", f.Name(), err))
						errMu.Unlock()
						if procCfg.onDone != nil {
							procCfg.onDone(f.Name(), err)
						}
						continue
					}

					var w io.Writer = out
					if procCfg.security.ResourceLimits.MaxFileSize > 0 ||
						procCfg.security.ResourceLimits.MaxTotalSize > 0 ||
						procCfg.security.ResourceLimits.MaxRatio > 0 {
						w = vfs.NewSecureWriter(
							out,
							procCfg.security.ResourceLimits,
							f.CompressedSize(),
							&globalExtracted,
						)
					}

					bufPtr := pool.Get64K()
					_, copyErr := io.CopyBuffer(w, rc, *bufPtr)
					pool.Put64K(bufPtr)
					_ = rc.Close()
					_ = out.Close()

					// Restore file modification time (mtime)
					if !f.ModTime().IsZero() {
						_ = os.Chtimes(targetPath, f.ModTime(), f.ModTime())
					}

					if copyErr != nil {
						errMu.Lock()
						errs = append(errs, wrapErr("write", f.Name(), copyErr))
						errMu.Unlock()
					}
					if procCfg.onDone != nil {
						procCfg.onDone(f.Name(), copyErr)
					}
				}
			})
		}

		wg.Wait()
		return errors.Join(errs...)
	})
}

// Extract extracts an archive from src into destDir using automatic format detection.
func Extract(ctx context.Context, src Source, destDir string, opts ...Option) error {
	return NewArchiver().Unarchive(ctx, src, destDir, opts...)
}

// ExtractFile extracts an individual named file from an archive into destPath.
func ExtractFile(ctx context.Context, src Source, targetFile, destPath string, opts ...Option) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cleanTarget := vfs.CleanPath(targetFile)

	return UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		container, r := resolveSourceContainer(r, rAt)
		arc := NewArchive(append([]Option{archive.WithContainer(container)}, opts...)...)
		if rAt != nil && size > 0 {
			if _, err := arc.Load(rAt, size, container); err != nil {
				return err
			}
		} else {
			if _, err := arc.LoadFromReader(r, container); err != nil {
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
func ReadFile(ctx context.Context, src Source, targetFile string, opts ...Option) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cleanTarget := vfs.CleanPath(targetFile)

	var data []byte
	err := UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		container, r := resolveSourceContainer(r, rAt)
		arc := NewArchive(append([]Option{archive.WithContainer(container)}, opts...)...)
		if rAt != nil && size > 0 {
			if _, err := arc.Load(rAt, size, container); err != nil {
				return err
			}
		} else {
			if _, err := arc.LoadFromReader(r, container); err != nil {
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
func (a *Archiver) ReadFile(ctx context.Context, src Source, targetFile string, opts ...Option) ([]byte, error) {
	allOpts := slices.Concat(a.containerOptions, opts)
	return ReadFile(ctx, src, targetFile, allOpts...)
}

// ReadTo reads an individual named file from an archive and unmarshals its content into *T.
// If unmarshaler is omitted, json.Unmarshal is used by default.
func ReadTo[T any](ctx context.Context, src Source, targetFile string, unmarshaler ...func([]byte, any) error) (*T, error) {
	data, err := ReadFile(ctx, src, targetFile)
	if err != nil {
		return nil, err
	}
	var res T
	unmarshal := json.Unmarshal
	if len(unmarshaler) > 0 && unmarshaler[0] != nil {
		unmarshal = unmarshaler[0]
	}
	if err := unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("seal: unmarshal %s: %w", targetFile, err)
	}
	return &res, nil
}

// ReadInto reads an individual named file from an archive and unmarshals its content directly into target.
func ReadInto[T any](ctx context.Context, src Source, targetFile string, target *T, unmarshaler ...func([]byte, any) error) error {
	data, err := ReadFile(ctx, src, targetFile)
	if err != nil {
		return err
	}
	unmarshal := json.Unmarshal
	if len(unmarshaler) > 0 && unmarshaler[0] != nil {
		unmarshal = unmarshaler[0]
	}
	if err := unmarshal(data, target); err != nil {
		return fmt.Errorf("seal: unmarshal %s: %w", targetFile, err)
	}
	return nil
}

// ReadTo reads an individual named file from an archive and unmarshals its content into *T.
func (a *Archiver) ReadTo[T any](ctx context.Context, src Source, targetFile string, unmarshaler ...func([]byte, any) error) (*T, error) {
	data, err := a.ReadFile(ctx, src, targetFile)
	if err != nil {
		return nil, err
	}
	var res T
	unmarshal := json.Unmarshal
	if len(unmarshaler) > 0 && unmarshaler[0] != nil {
		unmarshal = unmarshaler[0]
	}
	if err := unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("seal: unmarshal %s: %w", targetFile, err)
	}
	return &res, nil
}

// ReadInto reads an individual named file from an archive and unmarshals its content directly into target.
func (a *Archiver) ReadInto[T any](ctx context.Context, src Source, targetFile string, target *T, unmarshaler ...func([]byte, any) error) error {
	data, err := a.ReadFile(ctx, src, targetFile)
	if err != nil {
		return err
	}
	unmarshal := json.Unmarshal
	if len(unmarshaler) > 0 && unmarshaler[0] != nil {
		unmarshal = unmarshaler[0]
	}
	if err := unmarshal(data, target); err != nil {
		return fmt.Errorf("seal: unmarshal %s: %w", targetFile, err)
	}
	return nil
}

// Verify checks the integrity of all file entries and checksums within an archive.
func (a *Archiver) Verify(ctx context.Context, src Source, opts ...Option) error {
	allOpts := slices.Concat(a.containerOptions, opts)
	return Verify(ctx, src, allOpts...)
}

// GetEntries returns a metadata snapshot of all file entries in the archive.
func (a *Archiver) GetEntries(ctx context.Context, src Source) ([]*File, error) {
	var files []*File
	err := UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		container, r := resolveSourceContainer(r, rAt)
		arc := NewArchive(append([]Option{archive.WithContainer(container)}, a.containerOptions...)...)
		var err error
		if rAt != nil && size > 0 {
			files, err = arc.Load(rAt, size, container)
		} else {
			files, err = arc.LoadFromReader(r, container)
		}
		return err
	})

	return files, err
}

// List returns a list of archive entries from src.
func List(ctx context.Context, src Source, opts ...Option) ([]*File, error) {
	return NewArchiver(opts...).GetEntries(ctx, src)
}

// Verify checks the integrity of all file entries and checksums within an archive.
func Verify(ctx context.Context, src Source, opts ...Option) error {
	return UseSource(src, func(r io.Reader, rAt io.ReaderAt, size int64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		container, r := resolveSourceContainer(r, rAt)
		arc := NewArchive(append([]Option{archive.WithContainer(container)}, opts...)...)
		var files []*File
		var err error
		if rAt != nil && size > 0 {
			files, err = arc.Load(rAt, size, container)
		} else {
			files, err = arc.LoadFromReader(r, container)
		}
		if err != nil {
			return err
		}

		var errs []error
		for _, f := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
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
func ArchiveDir(ctx context.Context, dirPath string, dest Sink, opts ...Option) error {
	return NewArchiver(opts...).ArchiveDir(ctx, dirPath, dest)
}

// Unarchive extracts an archive from Source into destDir.
func Unarchive(ctx context.Context, src Source, destDir string, opts ...Option) error {
	return NewArchiver().Unarchive(ctx, src, destDir, opts...)
}

// FileCount returns the number of entries in the archive.
func FileCount(ctx context.Context, src Source) (int, error) {
	entries, err := List(ctx, src)
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// ExistsIn reports whether targetFile exists inside the archive.
func ExistsIn(ctx context.Context, src Source, targetFile string) (bool, error) {
	clean := vfs.CleanPath(targetFile)
	entries, err := List(ctx, src)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(entries, func(e *File) bool {
		return e.Name() == clean
	}), nil
}

// Tree returns an ASCII visual hierarchy string from an archive source.
func Tree(ctx context.Context, src Source) (string, error) {
	entries, err := List(ctx, src)
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

// ReadExtensionBundle extracts all embedded stealth extensions from an archive file.
func ReadExtensionBundle(ctx context.Context, archivePath string) (map[extension.ID][]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := fi.Size()
	if size < 22 {
		return nil, nil
	}

	var header [512]byte
	n, _ := f.ReadAt(header[:], 0)
	cType := container.SniffHeader(header[:n])

	var rawData []byte
	switch cType {
	case container.SevenZip:
		zr, err := sevenzip.NewReader(f, size, "")
		if err != nil {
			return nil, err
		}
		if data, ok := zr.ExtensionData(); ok {
			rawData = data
		}
	case container.Zip:
		zr, err := zip.NewReader(f, size, "", nil)
		if err != nil {
			return nil, err
		}
		if data, ok := zr.ExtensionData(); ok {
			rawData = data
		}
	default:
		// Fallback to checking comment or pax records via DefaultContainer
		if eng, err := DefaultContainer(cType); err == nil {
			_, comment, err := eng.OpenCatalog(f, size, "", nil)
			if err == nil && comment != "" {
				rawData = []byte(comment)
			}
		}
	}

	if len(rawData) == 0 {
		return nil, nil
	}

	return extension.DecodeBundle(rawData)
}

// InspectExtensions populates the provided extension instances with metadata found in the archive.
func InspectExtensions(ctx context.Context, archivePath string, exts ...extension.Extension) error {
	bundle, err := ReadExtensionBundle(ctx, archivePath)
	if err != nil {
		return err
	}
	if len(bundle) == 0 {
		return nil
	}
	for _, ext := range exts {
		if data, ok := bundle[ext.ID()]; ok {
			info := extension.ArchiveInfo{}
			if err := ext.OnRead(ctx, data, info); err != nil {
				return fmt.Errorf("seal: extension %q read error: %w", ext.ID(), err)
			}
		}
	}
	return nil
}

// GetExtension extracts a single extension payload from the archive by its ID.
func GetExtension(ctx context.Context, archivePath string, id extension.ID) ([]byte, error) {
	bundle, err := ReadExtensionBundle(ctx, archivePath)
	if err != nil {
		return nil, err
	}
	if data, ok := bundle[id]; ok {
		return data, nil
	}
	return nil, extension.ErrExtensionNotFound
}

// GetExtensionTo extracts a single extension payload from the archive and unmarshals it into *T.
// If unmarshaler is omitted, json.Unmarshal is used by default.
func GetExtensionTo[T any](ctx context.Context, archivePath string, id extension.ID, unmarshaler ...func([]byte, any) error) (*T, error) {
	data, err := GetExtension(ctx, archivePath, id)
	if err != nil {
		return nil, err
	}
	var res T
	unmarshal := json.Unmarshal
	if len(unmarshaler) > 0 && unmarshaler[0] != nil {
		unmarshal = unmarshaler[0]
	}
	if err := unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("seal: unmarshal extension %s: %w", id, err)
	}
	return &res, nil
}

// CanResume inspects an archive file and returns whether an embedded stealth checkpoint is present.
func CanResume(ctx context.Context, archivePath string) (*checkpoint.State, bool, error) {
	data, err := GetExtension(ctx, archivePath, "seal.checkpoint")
	if err != nil {
		if errors.Is(err, extension.ErrExtensionNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	st, err := checkpoint.Decode(data)
	if err != nil {
		return nil, false, err
	}
	return st, true, nil
}

// ResumeArchive resumes an interrupted archive operation from its stealth checkpoint with cancellation support.
func ResumeArchive(ctx context.Context, archivePath string, opts ...Option) error {
	st, can, err := CanResume(ctx, archivePath)
	if err != nil {
		return fmt.Errorf("seal: inspect resume checkpoint: %w", err)
	}
	if !can || st == nil {
		return errors.New("seal: no valid resume checkpoint found in archive")
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	size := fi.Size()

	var header [512]byte
	n, _ := f.ReadAt(header[:], 0)
	cType := container.SniffHeader(header[:n])
	cnt, err := DefaultContainer(cType)
	if err != nil {
		_ = f.Close()
		return err
	}

	arc := NewArchive(append([]Option{archive.WithContainer(cnt)}, opts...)...)
	_, err = arc.Load(f, size, cnt)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("seal: load existing files: %w", err)
	}

	// Add remaining files from sourceDir
	for _, relPath := range st.RemainingFiles {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			return err
		}
		fullPath := filepath.Join(st.SourceDir, relPath)
		fileInfo, statErr := os.Stat(fullPath)
		if statErr != nil {
			continue
		}
		if fileInfo.IsDir() {
			continue
		}
		entry, newErr := file.New(relPath, false)
		if newErr != nil {
			continue
		}
		entry.WithMode(fileInfo.Mode()).WithModTime(fileInfo.ModTime()).WithSize(fileInfo.Size())
		currPath := fullPath
		entry.SetOpenFunc(func() (io.ReadCloser, error) {
			return os.Open(currPath)
		})
		arc.Add(entry)
	}

	tmpOut := archivePath + ".tmp.seal"
	outF, err := os.Create(tmpOut)
	if err != nil {
		_ = f.Close()
		return err
	}

	writeOpts := append(slices.Clone(opts),
		WithResumable(true),
		WithSessionID(st.SessionID),
		WithSourceDir(st.SourceDir),
	)

	_, writeErr := arc.WriteTo(ctx, outF, writeOpts...)
	_ = outF.Close()
	_ = f.Close() // Close source handle so Windows allows replacing archivePath

	if writeErr != nil {
		if errors.Is(writeErr, ErrArchiveFrozen) {
			_ = os.Remove(archivePath)
			_ = os.Rename(tmpOut, archivePath)
			return ErrArchiveFrozen
		}
		_ = os.Remove(tmpOut)
		return writeErr
	}

	_ = os.Remove(archivePath)
	return os.Rename(tmpOut, archivePath)
}

// VerifySignature inspects an archive file, extracts its embedded Ed25519 signature,
// and cryptographically verifies its authenticity.
func VerifySignature(ctx context.Context, archivePath string, pubKey ed25519.PublicKey) (*signature.State, error) {
	data, err := GetExtension(ctx, archivePath, signature.ExtensionID)
	if err != nil {
		if errors.Is(err, extension.ErrExtensionNotFound) {
			return nil, ErrSignatureNotFound
		}
		return nil, err
	}

	st, err := signature.DecodeState(data)
	if err != nil {
		return nil, fmt.Errorf("seal: corrupted signature metadata: %w", err)
	}

	// Verify against archive entry metadata
	entries, err := List(ctx, FromFilePath(archivePath))
	if err != nil {
		return nil, err
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var header [512]byte
	n, _ := f.ReadAt(header[:], 0)
	cType := container.SniffHeader(header[:n])

	info := extension.ArchiveInfo{
		Format:     cType,
		TotalFiles: len(entries),
	}

	if err := st.Verify(pubKey, info); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSignatureInvalid, err)
	}

	return st, nil
}

// InspectProvenance extracts embedded build passport and VCS supply-chain metadata from an archive.
func InspectProvenance(ctx context.Context, archivePath string) (*provenance.Info, error) {
	info, err := GetExtensionTo[provenance.Info](ctx, archivePath, provenance.ExtensionID)
	if err != nil {
		if errors.Is(err, extension.ErrExtensionNotFound) {
			return nil, ErrProvenanceNotFound
		}
		return nil, err
	}
	return info, nil
}

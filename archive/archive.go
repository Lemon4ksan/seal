// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lemon4ksan/foundation/fskit"
	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/pathkit"

	"github.com/lemon4ksan/seal/checkpoint"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/option"
	"github.com/lemon4ksan/seal/vfs"
)

// FileSortStrategy defines the ordering of entries when serializing an archive.
type FileSortStrategy int

const (
	// SortDefault maintains the insertion order of entries.
	SortDefault FileSortStrategy = iota

	// SortNameAsc sorts entries alphabetically by normalized path (A-Z).
	SortNameAsc

	// SortNameDesc sorts entries reverse-alphabetically (Z-A).
	SortNameDesc

	// SortSizeAsc sorts entries by uncompressed size (smallest first).
	SortSizeAsc

	// SortSizeDesc sorts entries by uncompressed size (largest first).
	SortSizeDesc
)

// ConflictAction represents a name collision resolution policy.
type ConflictAction int

const (
	// ActionReplace overwrites the existing entry with the incoming one.
	ActionReplace ConflictAction = iota

	// ActionSkip ignores the incoming entry, preserving the existing one.
	ActionSkip

	// ActionError rejects the incoming entry, returning an error.
	ActionError

	// ActionRename adds the incoming entry under an adjusted path provided by the handler.
	ActionRename
)

// ConflictHandler resolves name collisions during archive merging and entry attachment.
type ConflictHandler func(existing, incoming *file.File) (ConflictAction, string)

// ErrFileNotFound indicates that the requested file entry was not found in the archive.
var ErrFileNotFound = fs.ErrNotExist

// DefaultConflictHandler replaces previous entries by default.
func DefaultConflictHandler(existing, incoming *file.File) (ConflictAction, string) {
	return ActionReplace, ""
}

// Archive represents an in-memory mutable archive manager holding virtual paths,
// enforcing path containment, and dispatching wire serialization to the active container engine.
type Archive struct {
	mu             sync.RWMutex
	container      container.Container
	files          []*file.File
	byName         map[string]*file.File
	comment        string
	password       string
	registry       *codec.Registry
	defaultMethod  codec.Method
	defaultLevel   int
	sortStrategy   FileSortStrategy
	conflictPolicy ConflictHandler
	vfsFS          *vfs.FS
	bytecodeFilter string
}

type archiverConfig struct {
	container      container.Container
	format         container.Type
	method         codec.Method
	level          int
	password       string
	comment        string
	sortStrategy   FileSortStrategy
	conflictPolicy ConflictHandler
	registry       *codec.Registry
	workers        int
	bytecodeFilter string
}

type processConfig struct {
	workers        int
	password       string
	filters        []option.Filter
	security       option.SecuritySettings
	onProgress     option.ProgressFunc
	onDone         func(string, error)
	checkpointData []byte
	resumable      bool
	sessionID      string
	sourceDir      string
	extensions     []extension.Extension
	bytecodeFilter string
}

// New constructs a new universal [Archive] instance with the specified options.
func New(opts ...option.Option) *Archive {
	cfg := archiverConfig{
		format:         container.Zip,
		method:         codec.Deflate,
		level:          codec.DefaultCompression,
		sortStrategy:   SortDefault,
		conflictPolicy: DefaultConflictHandler,
		registry:       codec.DefaultRegistry,
		workers:        1,
	}

	for _, opt := range opts {
		switch opt.Kind {
		case option.OptContainer:
			cfg.format = opt.Container
		case option.OptCompression:
			cfg.method = opt.Method
			cfg.level = opt.Level
		case option.OptPassword:
			cfg.password = opt.Text
		case option.OptComment:
			cfg.comment = opt.Text
		case option.OptWorkers:
			cfg.workers = opt.Workers
		case option.OptBytecodeFilter:
			cfg.bytecodeFilter = opt.Text
		case option.OptCustom:
			if opt.Custom != nil {
				opt.Custom(&cfg)
			}
		}
	}

	a := &Archive{
		container:      cfg.container,
		byName:         make(map[string]*file.File),
		comment:        cfg.comment,
		password:       cfg.password,
		registry:       cfg.registry,
		defaultMethod:  cfg.method,
		defaultLevel:   cfg.level,
		sortStrategy:   cfg.sortStrategy,
		conflictPolicy: cfg.conflictPolicy,
		bytecodeFilter: cfg.bytecodeFilter,
	}

	a.vfsFS = vfs.NewFS(a)
	return a
}

// NewZip creates a new [Archive] configured with the specified options.
func NewZip(opts ...option.Option) *Archive {
	return New(opts...)
}

// Engine returns the active serialization container engine.
func (a *Archive) Engine() container.Container {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.container
}

// SetEngine assigns an explicit serialization container engine.
func (a *Archive) SetEngine(c container.Container) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.container = c
}

// Container returns the container type of the active engine, or Unknown if none is configured.
func (a *Archive) Container() container.Type {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.container != nil {
		return a.container.Type()
	}
	return container.Unknown
}

// Comment returns the archive comment.
func (a *Archive) Comment() string {
	return generic.WithRGuard(&a.mu, func() string {
		return a.comment
	})
}

// SetComment updates the archive comment.
func (a *Archive) SetComment(c string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.comment = c
}

// SetPassword sets the default encryption password for entries in the archive.
func (a *Archive) SetPassword(pwd string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.password = pwd
}

// AddFile reads an OS file from disk, normalizes its path, and attaches it as an archive entry.
func (a *Archive) AddFile(diskPath string, opts ...option.Option) (*file.File, error) {
	p := pathkit.FromFilePath(diskPath)
	stat, err := os.Stat(p.FilePath())
	if err != nil {
		return nil, fmt.Errorf("archive: stat %s: %w", diskPath, err)
	}

	entryName := p.Base()
	if stat.IsDir() {
		return a.addDirEntry(entryName, stat.Mode(), stat.ModTime(), opts...)
	}

	entry, err := file.New(entryName, false)
	if err != nil {
		return nil, err
	}
	entry.SetSizes(stat.Size(), 0, 0)
	entry.WithMode(stat.Mode())
	entry.WithModTime(stat.ModTime())
	entry.WithCompression(a.defaultMethod, a.defaultLevel)
	if a.password != "" {
		entry.WithPassword(a.password)
	}
	entry.SetOpenFunc(func() (io.ReadCloser, error) {
		return os.Open(p.FilePath())
	})

	for _, opt := range opts {
		applyOption(entry, opt)
	}

	a.attachEntry(entry)
	return entry, nil
}

// AddDir recursively attaches a physical directory and all nested entries to the archive.
func (a *Archive) AddDir(dirPath string, opts ...option.Option) ([]*file.File, error) {
	var added []*file.File
	cleanRoot := pathkit.FromFilePath(dirPath).FilePath()

	err := fskit.FastWalk(cleanRoot, func(currentPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(cleanRoot, currentPath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		entryName := filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}

		if d.IsDir() {
			f, err := a.addDirEntry(entryName, info.Mode(), info.ModTime(), opts...)
			if err != nil {
				return err
			}
			added = append(added, f)
			return nil
		}

		f, err := file.New(entryName, false)
		if err != nil {
			return err
		}
		f.SetSizes(info.Size(), 0, 0)
		f.WithMode(info.Mode())
		f.WithModTime(info.ModTime())
		f.WithCompression(a.defaultMethod, a.defaultLevel)
		if a.password != "" {
			f.WithPassword(a.password)
		}
		f.SetOpenFunc(func() (io.ReadCloser, error) {
			return os.Open(currentPath)
		})

		for _, opt := range opts {
			applyOption(f, opt)
		}

		a.attachEntry(f)
		added = append(added, f)
		return nil
	})

	return added, err
}

func (a *Archive) addDirEntry(
	name string,
	mode fs.FileMode,
	modTime time.Time,
	opts ...option.Option,
) (*file.File, error) {
	entry, err := file.NewDir(name)
	if err != nil {
		return nil, err
	}
	entry.WithMode(mode | fs.ModeDir)
	entry.WithModTime(modTime)

	for _, opt := range opts {
		applyOption(entry, opt)
	}

	a.attachEntry(entry)
	return entry, nil
}

// Add attaches one or more pre-configured [*file.File] entries to the archive.
func (a *Archive) Add(files ...*file.File) {
	for _, f := range files {
		a.attachEntry(f)
	}
}

// AddBytes attaches an in-memory byte slice as an archive entry.
func (a *Archive) AddBytes(data []byte, name string, opts ...option.Option) *file.File {
	entry, _ := file.New(name, false)
	entry.SetSizes(int64(len(data)), 0, 0)
	entry.WithCompression(a.defaultMethod, a.defaultLevel)
	if a.password != "" {
		entry.WithPassword(a.password)
	}
	entry.SetOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})

	for _, opt := range opts {
		applyOption(entry, opt)
	}

	a.attachEntry(entry)
	return entry
}

// AddString attaches an in-memory string as an archive entry.
func (a *Archive) AddString(content, name string, opts ...option.Option) *file.File {
	return a.AddBytes([]byte(content), name, opts...)
}

// AddLazy registers a dynamically evaluated stream factory as an archive entry.
func (a *Archive) AddLazy(
	name string,
	factory func() (io.ReadCloser, error),
	opts ...option.Option,
) (*file.File, error) {
	entry, err := file.New(name, false)
	if err != nil {
		return nil, err
	}
	entry.WithCompression(a.defaultMethod, a.defaultLevel)
	if a.password != "" {
		entry.WithPassword(a.password)
	}
	entry.SetOpenFunc(factory)

	for _, opt := range opts {
		applyOption(entry, opt)
	}

	a.attachEntry(entry)
	return entry, nil
}

// AddFS recursively imports entries from an [io/fs.FS] virtual filesystem.
func (a *Archive) AddFS(fsys fs.FS, opts ...option.Option) ([]*file.File, error) {
	var added []*file.File

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		entryName := filepath.ToSlash(p)
		if d.IsDir() {
			f, err := a.addDirEntry(entryName, info.Mode(), info.ModTime(), opts...)
			if err != nil {
				return err
			}
			added = append(added, f)
			return nil
		}

		f, err := file.New(entryName, false)
		if err != nil {
			return err
		}
		f.SetSizes(info.Size(), 0, 0)
		f.WithMode(info.Mode())
		f.WithModTime(info.ModTime())
		f.WithCompression(a.defaultMethod, a.defaultLevel)
		if a.password != "" {
			f.WithPassword(a.password)
		}
		f.SetOpenFunc(func() (io.ReadCloser, error) {
			return fsys.Open(p)
		})

		for _, opt := range opts {
			applyOption(f, opt)
		}

		a.attachEntry(f)
		added = append(added, f)
		return nil
	})

	return added, err
}

func (a *Archive) attachEntry(incoming *file.File) {
	a.mu.Lock()
	defer a.mu.Unlock()

	clean := incoming.Name()
	if existing, ok := a.byName[clean]; ok {
		action, newName := a.conflictPolicy(existing, incoming)
		switch action {
		case ActionSkip, ActionError:
			return
		case ActionRename:
			if newName != "" {
				incoming.SetName(newName)
				clean = incoming.Name()
			}
		case ActionReplace:
			idx := slices.Index(a.files, existing)
			if idx >= 0 {
				a.files[idx] = incoming
				a.byName[clean] = incoming
				return
			}
		}
	}

	a.files = append(a.files, incoming)
	a.byName[clean] = incoming
}

// Remove deletes an entry by relative path.
func (a *Archive) Remove(name string) bool {
	clean := vfs.CleanPath(name)
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.byName[clean]; !ok {
		return false
	}
	delete(a.byName, clean)

	a.files = slices.DeleteFunc(a.files, func(f *file.File) bool {
		return f.Name() == clean
	})
	return true
}

// Rename renames an existing entry within the archive.
func (a *Archive) Rename(oldName, newName string) error {
	cleanOld := vfs.CleanPath(oldName)
	cleanNew := vfs.CleanPath(newName)

	a.mu.Lock()
	defer a.mu.Unlock()

	entry, ok := a.byName[cleanOld]
	if !ok {
		return errors.New("archive: file not found")
	}

	delete(a.byName, cleanOld)
	entry.SetName(cleanNew)
	a.byName[cleanNew] = entry
	return nil
}

// Move renames or moves an existing entry within the archive.
func (a *Archive) Move(oldName, newName string) error {
	return a.Rename(oldName, newName)
}

// Exists reports whether an entry exists in the archive.
func (a *Archive) Exists(name string) bool {
	clean := vfs.CleanPath(name)
	a.mu.RLock()
	defer a.mu.RUnlock()

	_, ok := a.byName[clean]
	if !ok && strings.HasSuffix(name, "/") {
		_, ok = a.byName[clean+"/"]
	}
	return ok
}

// File retrieves an individual file entry by relative path.
func (a *Archive) File(name string) (*file.File, bool) {
	clean := vfs.CleanPath(name)
	a.mu.RLock()
	defer a.mu.RUnlock()

	f, ok := a.byName[clean]
	if !ok && strings.HasSuffix(name, "/") {
		f, ok = a.byName[clean+"/"]
	}
	return f, ok
}

// Files returns a snapshot slice of all file entries in the archive.
func (a *Archive) Files() []*file.File {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return slices.Clone(a.files)
}

// Filter mutates the archive in place, retaining only entries matching the predicate.
func (a *Archive) Filter(pred func(f *file.File) bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.files = slices.DeleteFunc(a.files, func(f *file.File) bool {
		if !pred(f) {
			delete(a.byName, f.Name())
			return true
		}
		return false
	})
}

// Find searches for the first entry in the archive satisfying predicate.
func (a *Archive) Find(predicate func(*file.File) bool) (*file.File, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return generic.Find(a.files, predicate)
}

// FindAll returns a snapshot of all entries in the archive satisfying predicate.
func (a *Archive) FindAll(predicate func(*file.File) bool) []*file.File {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return generic.Filter(a.files, predicate)
}

// ReadTo reads a named entry and unmarshals its content into *T.
// If unmarshaler is omitted, json.Unmarshal is used by default.
func ReadTo[T any](a *Archive, name string, unmarshaler ...func([]byte, any) error) (*T, error) {
	f, ok := a.File(name)
	if !ok {
		return nil, ErrFileNotFound
	}
	return f.ReadTo[T](unmarshaler...)
}

// ReadTo reads a named entry and unmarshals its content into *T.
func (a *Archive) ReadTo[T any](name string, unmarshaler ...func([]byte, any) error) (*T, error) {
	return ReadTo[T](a, name, unmarshaler...)
}

// ReadInto reads a named entry and unmarshals its content directly into target.
func (a *Archive) ReadInto[T any](name string, target *T, unmarshaler ...func([]byte, any) error) error {
	f, ok := a.File(name)
	if !ok {
		return ErrFileNotFound
	}
	return f.ReadInto(target, unmarshaler...)
}

// Tree returns an ASCII visual representation of the archive hierarchy.
func (a *Archive) Tree() string {
	files := a.Files()
	entries := generic.Map(files, func(f *file.File) struct {
		Name  string
		IsDir bool
	} {
		return struct {
			Name  string
			IsDir bool
		}{
			Name:  f.Name(),
			IsDir: f.IsDir(),
		}
	})

	root := vfs.BuildTree(entries)
	return vfs.RenderTree(root)
}

// Load loads entries from an [io.ReaderAt] or [io.Reader] source using an explicit container engine.
func (a *Archive) Load(rAt io.ReaderAt, size int64, engines ...container.Container) ([]*file.File, error) {
	var c container.Container
	if len(engines) > 0 && engines[0] != nil {
		c = engines[0]
	} else {
		a.mu.RLock()
		c = a.container
		a.mu.RUnlock()
	}

	if c == nil {
		return nil, errors.New(
			"archive: no container engine specified; pass an engine or configure WithEngine(zip.New())",
		)
	}

	files, comment, err := c.OpenCatalog(rAt, size, a.password, a.registry)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.container = c
	if comment != "" {
		a.comment = checkpoint.StripCheckpoint(comment)
	}

	for _, f := range files {
		a.files = append(a.files, f)
		a.byName[f.Name()] = f
	}

	return files, nil
}

// LoadFromReader loads entries from a sequential [io.Reader] stream using an explicit container engine.
func (a *Archive) LoadFromReader(r io.Reader, engines ...container.Container) ([]*file.File, error) {
	var c container.Container
	if len(engines) > 0 && engines[0] != nil {
		c = engines[0]
	} else {
		a.mu.RLock()
		c = a.container
		a.mu.RUnlock()
	}

	if c == nil {
		return nil, errors.New(
			"archive: no container engine specified; pass an engine or configure WithEngine(zip.New())",
		)
	}

	files, comment, err := c.ScanStream(r, a.password, a.registry)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.container = c
	if comment != "" {
		a.comment = comment
	}

	for _, f := range files {
		a.files = append(a.files, f)
		a.byName[f.Name()] = f
	}

	return files, nil
}

// WriteTo serializes all entries in the archive into dest using the active container engine.
func (a *Archive) WriteTo(ctx context.Context, dest io.Writer, opts ...option.Option) (int64, error) {
	procCfg := processConfig{workers: 1}
	for _, opt := range opts {
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
		case option.OptResumable:
			procCfg.resumable = opt.Resumable
		case option.OptSessionID:
			procCfg.sessionID = opt.SessionID
		case option.OptSourceDir:
			procCfg.sourceDir = opt.SourceDir
		case option.OptCheckpoint:
			procCfg.checkpointData = opt.Bytes
		case option.OptExtension:
			procCfg.extensions = append(procCfg.extensions, opt.Extensions...)
		case option.OptBytecodeFilter:
			procCfg.bytecodeFilter = opt.Text
		case option.OptCustom:
			if opt.Custom != nil {
				opt.Custom(&procCfg)
			}
		}
	}

	a.mu.RLock()
	filesCopy := slices.Clone(a.files)
	comment := a.comment
	password := a.password
	method := a.defaultMethod
	level := a.defaultLevel
	sortStrat := a.sortStrategy
	c := a.container
	reg := a.registry
	a.mu.RUnlock()

	if c == nil {
		return 0, errors.New(
			"archive: no container engine attached; configure WithEngine(zip.New()) or WithContainer(tar.New())",
		)
	}

	if procCfg.password != "" {
		password = procCfg.password
	}

	// Apply filtering
	if len(procCfg.filters) > 0 {
		filesCopy = slices.DeleteFunc(filesCopy, func(f *file.File) bool {
			for _, filter := range procCfg.filters {
				if !filter(f.Name(), f.IsDir()) {
					return true
				}
			}
			return false
		})
	}

	// Apply sorting strategy
	switch sortStrat {
	case SortNameAsc:
		slices.SortFunc(filesCopy, func(a, b *file.File) int { return strings.Compare(a.Name(), b.Name()) })
	case SortNameDesc:
		slices.SortFunc(filesCopy, func(a, b *file.File) int { return strings.Compare(b.Name(), a.Name()) })
	case SortSizeAsc:
		slices.SortFunc(
			filesCopy,
			func(a, b *file.File) int { return cmp.Compare(a.UncompressedSize(), b.UncompressedSize()) },
		)
	case SortSizeDesc:
		slices.SortFunc(
			filesCopy,
			func(a, b *file.File) int { return cmp.Compare(b.UncompressedSize(), a.UncompressedSize()) },
		)
	}

	var onDoneWrapper func(*file.File, error)
	if procCfg.onDone != nil {
		onDoneWrapper = func(f *file.File, err error) {
			procCfg.onDone(f.Name(), err)
		}
	}

	// Pre-validation: ensure all files are valid and no overflows occur before starting write stream
	for _, f := range filesCopy {
		if err := f.Validate(); err != nil {
			return 0, fmt.Errorf("archive: invalid entry %q: %w", f.Name(), err)
		}
	}

	var extEntries map[extension.ID][]byte
	if len(procCfg.extensions) > 0 {
		info := extension.ArchiveInfo{
			Format:     c.Type(),
			SourceDir:  procCfg.sourceDir,
			TotalFiles: len(filesCopy),
			Comment:    comment,
		}
		for _, ext := range procCfg.extensions {
			payload, err := ext.OnWrite(ctx, info)
			if err != nil {
				return 0, fmt.Errorf("archive: extension %q write failed: %w", ext.ID(), err)
			}
			if len(payload) > 0 {
				if extEntries == nil {
					extEntries = make(map[extension.ID][]byte)
				}
				extEntries[ext.ID()] = payload
			}
		}
	}
	if len(procCfg.checkpointData) > 0 {
		if extEntries == nil {
			extEntries = make(map[extension.ID][]byte)
		}
		extEntries["seal.checkpoint"] = procCfg.checkpointData
	}
	var extBundle []byte
	if len(extEntries) > 0 {
		extBundle = extension.EncodeBundle(extEntries)
	}

	var onProgressWrapper func(int64, string)
	if procCfg.onProgress != nil {
		var processedBytes atomic.Int64
		onProgressWrapper = func(delta int64, curFile string) {
			newTotal := processedBytes.Add(delta)
			procCfg.onProgress(option.ProgressStats{
				CurrentFile: curFile,
				TotalRead:   newTotal,
				TotalFiles:  int64(len(filesCopy)),
			})
		}
	}

	bFilter := a.bytecodeFilter
	if procCfg.bytecodeFilter != "" {
		bFilter = procCfg.bytecodeFilter
	}

	containerCfg := container.Config{
		Comment:        comment,
		Password:       password,
		DefaultMethod:  method,
		DefaultLevel:   level,
		Registry:       reg,
		CheckpointData: procCfg.checkpointData,
		ExtensionData:  extBundle,
		Context:        ctx,
		Resumable:      procCfg.resumable,
		SessionID:      procCfg.sessionID,
		SourceDir:      procCfg.sourceDir,
		OnFileDone:     onDoneWrapper,
		OnProgress:     onProgressWrapper,
		BytecodeFilter: bFilter,
	}

	return c.WriteArchive(dest, filesCopy, containerCfg)
}

// VFS implementation helpers forwarding to [vfs.FS].

func (a *Archive) Open(name string) (fs.File, error) {
	return a.vfsFS.Open(name)
}

func (a *Archive) Stat(name string) (fs.FileInfo, error) {
	return a.vfsFS.Stat(name)
}

func (a *Archive) ReadFile(name string) ([]byte, error) {
	return a.vfsFS.ReadFile(name)
}

func (a *Archive) ReadDir(name string) ([]fs.DirEntry, error) {
	return a.vfsFS.ReadDir(name)
}

func (a *Archive) GetEntries() []vfs.VFSEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	entries := make([]vfs.VFSEntry, len(a.files))
	for i, f := range a.files {
		entries[i] = f
	}
	return entries
}

func (a *Archive) FindEntry(name string) (vfs.VFSEntry, bool) {
	return a.File(name)
}

func (a *Archive) Entries() []vfs.VFSEntry {
	return a.GetEntries()
}

func applyOption(entry *file.File, opt option.Option) {
	switch opt.Kind {
	case option.OptCompression:
		entry.WithCompression(opt.Method, opt.Level)
	case option.OptPassword:
		entry.WithPassword(opt.Text)
	case option.OptComment:
		entry.WithComment(opt.Text)
	case option.OptMode:
		entry.WithMode(opt.Mode)
	case option.OptModTime:
		entry.WithModTime(opt.ModTime)
	case option.OptName:
		entry.SetName(opt.Text)
	case option.OptPathPrefix:
		entry.SetName(vfs.CleanPath(opt.Text + "/" + entry.Name()))
	}
}

// WithContainer assigns an explicit serialization [container.Container] engine to the archive.
func WithContainer(c container.Container) option.Option {
	return option.Option{
		Kind: option.OptCustom,
		Custom: func(target any) {
			if cfg, ok := target.(*archiverConfig); ok {
				cfg.container = c
				if c != nil {
					cfg.format = c.Type()
				}
			}
		},
	}
}

// WithSortStrategy configures the entry sorting strategy during archive writing.
func WithSortStrategy(s FileSortStrategy) option.Option {
	return option.Option{
		Kind: option.OptSortStrategy,
		Custom: func(target any) {
			if cfg, ok := target.(*archiverConfig); ok {
				cfg.sortStrategy = s
			}
		},
	}
}

// WithConflictHandler configures the name collision resolution policy.
func WithConflictHandler(h ConflictHandler) option.Option {
	return option.Option{
		Kind: option.OptConflictPolicy,
		Custom: func(target any) {
			if cfg, ok := target.(*archiverConfig); ok {
				cfg.conflictPolicy = h
			}
		},
	}
}

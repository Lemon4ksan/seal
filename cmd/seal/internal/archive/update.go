// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lemon4ksan/ctrl/argkit"
	"github.com/lemon4ksan/foundation/fskit"

	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// UpdateCmd implements in-place file updating inside containers.
type UpdateCmd struct{}

func (c *UpdateCmd) Name() string      { return "update" }
func (c *UpdateCmd) Aliases() []string { return []string{"u"} }
func (c *UpdateCmd) Synopsis() string {
	return "Add or update files inside an existing container in-place"
}

func (c *UpdateCmd) Usage() string {
	return "aeon update [flags] <archive.eon> <target1> [target2 ...]"
}

func (c *UpdateCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("update", flag.ContinueOnError)
	fset.SetOutput(stderr)

	var passwords util.MultiStringFlag
	fset.Var(&passwords, "p", "Passphrase(s) for encrypted container / vaults")
	fset.Var(&passwords, "password", "Passphrase(s) for encrypted container / vaults")
	codecName := fset.String("c", "zstd", "Compression codec: zstd, lzma2, brotli, store")
	levelStr := fset.String("l", "default", "Compression level: fast, default, max, ultra, or 1..9")

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 2 {
		return errors.New("usage: aeon update [-p password] <archive.eon> <target1> [target2 ...]")
	}

	archivePath := remaining[0]
	targets := remaining[1:]

	var rOpts []aeon.ReaderOption
	if len(passwords) > 0 {
		rOpts = append(rOpts, aeon.WithReaderPasswords(passwords...))
	}

	r, origFile, err := aeon.OpenFile(archivePath, rOpts...)
	if err != nil {
		return fmt.Errorf("opening container to update: %w", err)
	}
	defer origFile.Close()

	// Prepare updated temporary container
	tmpPath := archivePath + ".update.tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp archive: %w", err)
	}

	codecID := spec.CodecZSTD
	switch strings.ToLower(*codecName) {
	case "lz4":
		codecID = spec.CodecLZ4
	case "lzma2", "lzma":
		codecID = spec.CodecLZMA2
	case "brotli", "br":
		codecID = spec.CodecBrotli
	case "przm", "prism":
		codecID = spec.CodecPRZM
	case "store", "copy", "raw":
		codecID = spec.CodecStore
	}

	var codecLevel uint8 = 3
	switch strings.ToLower(*levelStr) {
	case "fast", "fastest", "1":
		codecLevel = 1
	case "default", "normal", "3":
		codecLevel = 3
	case "max", "maximum", "7":
		codecLevel = 7
	case "ultra", "best", "9":
		codecLevel = 9
	}

	wOpts := []aeon.WriterOption{
		aeon.WithCodec(codecID),
		aeon.WithCodecLevel(codecLevel),
	}
	if len(passwords) > 0 {
		wOpts = append(wOpts, aeon.WithPassword(passwords[0]))
	}

	w := aeon.NewWriter(tmpFile, wOpts...)

	// Scan incoming targets to update/add
	type updateFile struct {
		name    string
		size    int64
		modTime time.Time
		path    string
	}
	incoming := make(map[string]updateFile)

	for _, target := range targets {
		select {
		case <-ctx.Done():
			tmpFile.Close()
			_ = os.Remove(tmpPath)
			return ctx.Err()
		default:
		}

		info, err := os.Stat(target)
		if err != nil {
			fmt.Fprintf(stderr, "error stating target %s: %v\n", target, err)
			continue
		}
		if info.IsDir() {
			ch := make(chan fskit.Entry, 2048)
			go fskit.Walk(target, ch)
			for entry := range ch {
				if entry.IsDir {
					continue
				}
				incoming[entry.RelPath] = updateFile{
					name:    entry.RelPath,
					size:    entry.Size,
					modTime: entry.ModTime,
					path:    entry.Path,
				}
			}
		} else {
			base := filepath.Base(target)
			incoming[base] = updateFile{
				name:    base,
				size:    info.Size(),
				modTime: info.ModTime(),
				path:    target,
			}
		}
	}

	// 1. Copy over all existing extents NOT superseded by incoming files
	copiedCount := 0
	for _, ext := range r.Extents() {
		if _, superseded := incoming[ext.Name]; superseded {
			continue
		}
		rc, err := r.Open(ext.Name)
		if err != nil {
			continue
		}
		modTime := time.Unix(0, ext.Timestamp)
		_ = w.AddFileStream(ext.Name, int64(ext.PayloadSize), modTime, rc)
		rc.Close()
		copiedCount++
	}

	// 2. Add new/updated incoming files
	updatedCount := 0
	for _, inc := range incoming {
		f, err := os.Open(inc.path)
		if err != nil {
			fmt.Fprintf(stderr, "error opening %s: %v\n", inc.path, err)
			continue
		}
		err = w.AddFileStream(inc.name, inc.size, inc.modTime, f)
		f.Close()
		if err == nil {
			updatedCount++
		}
	}

	if err := w.Close(); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing updated container: %w", err)
	}
	tmpFile.Close()
	origFile.Close()

	// Atomically replace original archive
	if err := os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(archivePath)
		if err := os.Rename(tmpPath, archivePath); err != nil {
			return fmt.Errorf("replacing container: %w", err)
		}
	}

	fmt.Fprintf(stdout, "✓ Successfully updated %s: %d unchanged item(s) preserved, %d item(s) added/updated\n",
		filepath.Base(archivePath), copiedCount, updatedCount)
	return nil
}

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prolly

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/lemon4ksan/ctrl/argkit"
	"github.com/lemon4ksan/foundation/fskit"

	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	aeon "github.com/lemon4ksan/seal/container/eon/engine"
)

// SnapshotCmd creates an immutable snapshot inside an existing archive.
type SnapshotCmd struct{}

func (c *SnapshotCmd) Name() string      { return "snapshot" }
func (c *SnapshotCmd) Aliases() []string { return nil }
func (c *SnapshotCmd) Synopsis() string {
	return "Create an immutable snapshot inside an existing archive (-m message)"
}

func (c *SnapshotCmd) Usage() string {
	return "aeon snapshot -m \"message\" [flags] <archive.eon> <target1> [target2 ...]"
}

func (c *SnapshotCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fset.SetOutput(stderr)

	msg := fset.String("m", "", "Commit message for the snapshot (required)")
	fset.StringVar(msg, "message", "", "Commit message for the snapshot (required)")
	cdc := fset.Bool("cdc", true, "Enable FastCDC deduplication against prior snapshot chunks")
	var passwords util.MultiStringFlag
	fset.Var(&passwords, "p", "Passphrase(s) for encrypted container")
	fset.Var(&passwords, "password", "Passphrase(s) for encrypted container")

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 2 || *msg == "" {
		return errors.New("usage: aeon snapshot -m \"message\" <archive.eon> <target1> [target2 ...]")
	}

	archivePath := remaining[0]
	targets := remaining[1:]

	var rOpts []aeon.ReaderOption
	if len(passwords) > 0 {
		rOpts = append(rOpts, aeon.WithReaderPasswords(passwords...))
	}

	r, origFile, err := aeon.OpenFile(archivePath, rOpts...)
	if err != nil {
		return fmt.Errorf("opening container to snapshot: %w", err)
	}
	defer origFile.Close()

	parentSnap := r.LatestSnapshot()
	parentID := ""
	if parentSnap != nil {
		parentID = parentSnap.ID
	}

	tmpPath := archivePath + ".snap.tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp archive: %w", err)
	}

	wOpts := []aeon.WriterOption{
		aeon.WithSnapshotsFrom(r),
		aeon.WithSnapshot(*msg, parentID),
	}
	if *cdc {
		wOpts = append(wOpts, aeon.WithCDC(true, 0))
	}
	if len(passwords) > 0 {
		wOpts = append(wOpts, aeon.WithPassword(passwords[0]))
	}

	w := aeon.NewWriter(tmpFile, wOpts...)

	type fileItem struct {
		name    string
		size    int64
		modTime time.Time
		path    string
	}
	incoming := make(map[string]fileItem)

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
				incoming[entry.RelPath] = fileItem{
					name:    entry.RelPath,
					size:    entry.Size,
					modTime: entry.ModTime,
					path:    entry.Path,
				}
			}
		} else {
			base := filepath.Base(target)
			incoming[base] = fileItem{
				name:    base,
				size:    info.Size(),
				modTime: info.ModTime(),
				path:    target,
			}
		}
	}

	// 1. Copy over existing extents NOT superseded by incoming files
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

	// 2. Add incoming files
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
		return fmt.Errorf("closing snapshot container: %w", err)
	}
	tmpFile.Close()
	origFile.Close()

	if err := os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(archivePath)
		if err := os.Rename(tmpPath, archivePath); err != nil {
			return fmt.Errorf("replacing container: %w", err)
		}
	}

	fmt.Fprintf(stdout, "✓ Created snapshot %q: %d preserved, %d added/updated\n",
		*msg, copiedCount, updatedCount)
	return nil
}

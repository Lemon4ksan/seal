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
	"time"

	"github.com/lemon4ksan/ctrl/argkit"

	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// VacuumCmd implements container defragmentation and repacking.
type VacuumCmd struct{}

func (c *VacuumCmd) Name() string      { return "vacuum" }
func (c *VacuumCmd) Aliases() []string { return []string{"compact"} }
func (c *VacuumCmd) Synopsis() string {
	return "Defragment and repack container, purging unreferenced chunks"
}
func (c *VacuumCmd) Usage() string {
	return "aeon vacuum [flags] [-o compacted.eon] <archive.eon>"
}

func (c *VacuumCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("vacuum", flag.ContinueOnError)
	fset.SetOutput(stderr)

	var passwords util.MultiStringFlag
	fset.Var(&passwords, "p", "Passphrase(s) for encrypted container / vaults")
	fset.Var(&passwords, "password", "Passphrase(s) for encrypted container / vaults")
	outPath := fset.String("o", "", "Output defragmented container path (default: in-place)")

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 1 {
		return errors.New("usage: aeon vacuum [-p password] [-o compacted.eon] <archive.eon>")
	}

	archivePath := remaining[0]
	statOrig, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("stating container: %w", err)
	}

	var rOpts []aeon.ReaderOption
	if len(passwords) > 0 {
		rOpts = append(rOpts, aeon.WithReaderPasswords(passwords...))
	}

	r, origFile, err := aeon.OpenFile(archivePath, rOpts...)
	if err != nil {
		return fmt.Errorf("opening container: %w", err)
	}
	defer origFile.Close()

	destPath := *outPath
	inPlace := false
	if destPath == "" {
		inPlace = true
		destPath = archivePath + ".vacuum.tmp"
	}

	dstFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating compacted container: %w", err)
	}

	wOpts := []aeon.WriterOption{
		aeon.WithCodec(spec.CodecZSTD),
		aeon.WithCodecLevel(3),
	}
	if len(passwords) > 0 {
		wOpts = append(wOpts, aeon.WithPassword(passwords[0]))
	}

	w := aeon.NewWriter(dstFile, wOpts...)
	extents := r.Extents()
	compactedCount := 0

	for _, ext := range extents {
		select {
		case <-ctx.Done():
			dstFile.Close()
			_ = os.Remove(destPath)
			return ctx.Err()
		default:
		}

		rc, err := r.Open(ext.Name)
		if err != nil {
			continue
		}
		modTime := time.Unix(0, ext.Timestamp)
		_ = w.AddFileStream(ext.Name, int64(ext.PayloadSize), modTime, rc)
		rc.Close()
		compactedCount++
	}

	if err := w.Close(); err != nil {
		dstFile.Close()
		_ = os.Remove(destPath)
		return fmt.Errorf("closing compacted container: %w", err)
	}
	dstFile.Close()
	origFile.Close()

	statNew, _ := os.Stat(destPath)

	if inPlace {
		_ = os.Remove(archivePath)
		if err := os.Rename(destPath, archivePath); err != nil {
			return fmt.Errorf("replacing container: %w", err)
		}
	}

	origSize := uint64(statOrig.Size())
	newSize := uint64(statNew.Size())
	diff := int64(origSize) - int64(newSize)
	diffStr := util.FormatHumanBytes(uint64(diff))
	if diff < 0 {
		diffStr = "-" + util.FormatHumanBytes(uint64(-diff))
	}

	fmt.Fprintf(stdout, "✓ Vacuumed %s: %d live extents packed (Original: %s ➔ Compacted: %s, Saved: %s)\n",
		filepath.Base(archivePath), compactedCount,
		util.FormatHumanBytes(origSize), util.FormatHumanBytes(newSize), diffStr)
	return nil
}

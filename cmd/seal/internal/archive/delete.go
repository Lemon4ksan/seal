// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lemon4ksan/cli/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container"
)

// DeleteCmd implements deleting files from an archive (command 'd').
type DeleteCmd struct{}

func (c *DeleteCmd) Name() string      { return "delete" }
func (c *DeleteCmd) Aliases() []string { return []string{"d", "del"} }
func (c *DeleteCmd) Synopsis() string  { return "Delete files from an existing archive" }
func (c *DeleteCmd) Usage() string     { return "seal d [flags] <archive-path> <patterns...>" }

func (c *DeleteCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	sz, err := util.ParseSevenZipArgs("d", args)
	if err != nil {
		return err
	}

	if sz.Archive == "" {
		return errors.New("archive file not specified")
	}

	deletePatterns := sz.Items
	if len(sz.IncludePatterns) > 0 {
		deletePatterns = append(deletePatterns, sz.IncludePatterns...)
	}
	if len(deletePatterns) == 0 {
		return errors.New("no files or patterns specified for deletion")
	}

	f, err := os.Open(sz.Archive)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}
	size := stat.Size()

	var header [512]byte
	n, _ := f.ReadAt(header[:], 0)
	cType := container.SniffHeader(header[:n])
	if cType == container.Unknown {
		cType = util.DetectContainerType(sz.Archive)
	}

	engine, err := seal.DefaultContainer(cType)
	if err != nil {
		return fmt.Errorf("unsupported archive format %s: %w", cType, err)
	}

	arc := seal.NewArchive(
		seal.WithContainer(cType),
		seal.WithPassword(sz.Password),
	)

	files, err := arc.Load(f, size, engine)
	if err != nil {
		return fmt.Errorf("loading archive catalog: %w", err)
	}

	start := time.Now()
	var deletedCount int
	for _, entry := range files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if util.MatchFilters(entry.Name(), entry.IsDir(), deletePatterns, nil, sz.Recurse) {
			if arc.Remove(entry.Name()) {
				deletedCount++
			}
		}
	}

	if deletedCount == 0 {
		fmt.Fprintf(stdout, "\n%s No matching entries found in %s to delete\n\n",
			tuikit.BadgeWarn(), tuikit.Bold(sz.Archive))
		return nil
	}

	fmt.Fprintf(stdout, "\nDeleting %d entries from %s (patterns: %s)...\n",
		deletedCount, tuikit.Bold(sz.Archive), tuikit.Yellow(strings.Join(deletePatterns, ", ")))

	tmpOut := sz.Archive + ".del.tmp"
	outF, err := os.OpenFile(tmpOut, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("creating temporary archive: %w", err)
	}

	_, writeErr := arc.WriteTo(ctx, outF)
	_ = outF.Close()
	_ = f.Close() // close handle so Windows allows file replacement

	if writeErr != nil {
		_ = os.Remove(tmpOut)
		if errors.Is(writeErr, context.Canceled) || errors.Is(writeErr, context.DeadlineExceeded) || ctx.Err() != nil {
			fmt.Fprintf(stdout, "\n%s Delete operation cancelled\n\n", tuikit.BadgeFail())
			return ctx.Err()
		}
		return fmt.Errorf("writing updated archive: %w", writeErr)
	}

	_ = os.Remove(sz.Archive)
	if err := os.Rename(tmpOut, sz.Archive); err != nil {
		return fmt.Errorf("finalizing updated archive: %w", err)
	}

	fmt.Fprintf(stdout, "%s Successfully deleted %d entries in %v\n\n",
		tuikit.BadgePassed(), deletedCount, time.Since(start))
	return nil
}

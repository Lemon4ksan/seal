// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/cli/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
)

// ListCmd implements archive table listing with 7-Zip compatibility (command 'l').
type ListCmd struct{}

func (c *ListCmd) Name() string      { return "list" }
func (c *ListCmd) Aliases() []string { return []string{"l", "ls"} }
func (c *ListCmd) Synopsis() string  { return "List all entries and metadata in an archive" }
func (c *ListCmd) Usage() string     { return "seal l [flags] <archive-path> [patterns...]" }

func (c *ListCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	sz, err := util.ParseSevenZipArgs("l", args)
	if err != nil {
		return err
	}

	if sz.Archive == "" {
		return errors.New("archive file not specified")
	}

	archiver := seal.NewArchiver(seal.WithPassword(sz.Password))
	entries, err := archiver.GetEntries(ctx, seal.FromFilePath(sz.Archive))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	// Filter entries if patterns were specified
	if len(sz.IncludePatterns) > 0 || len(sz.ExcludePatterns) > 0 {
		var filtered []*seal.File
		for _, e := range entries {
			if util.MatchFilters(e.Name(), e.IsDir(), sz.IncludePatterns, sz.ExcludePatterns, sz.Recurse) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	filterInfo := ""
	if len(sz.IncludePatterns) > 0 {
		filterInfo = fmt.Sprintf(" matching [%s]", strings.Join(sz.IncludePatterns, ", "))
	}

	fmt.Fprintf(stdout, "\n%s %s (%d entries%s)\n\n",
		tuikit.Bold("Archive:"), tuikit.Cyan(sz.Archive), len(entries), filterInfo)

	table := tuikit.NewTable("Mode", "Size", "Compressed", "Ratio", "Name")
	table.SetAlignment(1, tuikit.AlignRight).
		SetAlignment(2, tuikit.AlignRight).
		SetAlignment(3, tuikit.AlignRight).
		SetIndent(0)

	var totalUncomp, totalComp int64
	for _, e := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		totalUncomp += e.Size()
		totalComp += e.CompressedSize()

		ratio := 0.0
		if e.Size() > 0 {
			ratio = float64(e.CompressedSize()) / float64(e.Size()) * 100.0
		}

		table.AddRow(
			e.Mode().String(),
			util.FormatBytes(e.Size()),
			util.FormatBytes(e.CompressedSize()),
			fmt.Sprintf("%.1f%%", ratio),
			e.Name(),
		)
	}

	if err := table.Render(stdout); err != nil {
		return err
	}

	globalRatio := 0.0
	if totalUncomp > 0 {
		globalRatio = float64(totalComp) / float64(totalUncomp) * 100.0
	}

	fmt.Fprintf(stdout, "\n%s %s uncompressed, %s compressed (%.1f%% ratio)\n\n",
		tuikit.Bold("Total:"), util.FormatBytes(totalUncomp), util.FormatBytes(totalComp), globalRatio)

	return nil
}

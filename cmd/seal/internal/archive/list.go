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

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
)

// ListCmd implements archive table listing.
type ListCmd struct{}

func (c *ListCmd) Name() string      { return "list" }
func (c *ListCmd) Aliases() []string { return []string{"l", "ls"} }
func (c *ListCmd) Synopsis() string  { return "List all entries and metadata in an archive" }
func (c *ListCmd) Usage() string     { return "seal list [flags] <archive-path>" }

func (c *ListCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	pwd := fs.String("p", "", "Decryption password")

	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := posArgs[0]

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	entries, err := archiver.GetEntries(ctx, seal.FromFilePath(archivePath))
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "\n%s %s (%d entries)\n\n", tuikit.Bold("Archive:"), tuikit.Cyan(archivePath), len(entries))

	table := tuikit.NewTable("Mode", "Size", "Compressed", "Ratio", "Name")
	table.SetAlignment(1, tuikit.AlignRight).
		SetAlignment(2, tuikit.AlignRight).
		SetAlignment(3, tuikit.AlignRight).
		SetIndent(0)

	var totalUncomp, totalComp int64
	for _, e := range entries {
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

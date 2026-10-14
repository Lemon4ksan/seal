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
	"strings"

	"github.com/lemon4ksan/ctrl/argkit"

	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/prolly"
)

// DiffCmd compares differences between snapshots or containers.
type DiffCmd struct{}

func (c *DiffCmd) Name() string      { return "diff" }
func (c *DiffCmd) Aliases() []string { return nil }
func (c *DiffCmd) Synopsis() string {
	return "Compare differences between two snapshots or containers in O(Δ) time"
}

func (c *DiffCmd) Usage() string {
	return "aeon diff [flags] <archive.eon> [snap1] [snap2] (or: aeon diff <archive1.eon> <archive2.eon>)"
}

func (c *DiffCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("diff", flag.ContinueOnError)
	fset.SetOutput(stderr)

	var passwords util.MultiStringFlag
	fset.Var(&passwords, "p", "Passphrase(s) to unlock encrypted container")
	fset.Var(&passwords, "password", "Passphrase(s) to unlock encrypted container")

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 1 {
		return errors.New(
			"usage: aeon diff [-p password] <archive.eon> [snap1] [snap2]\n       aeon diff [-p password] <archive1.eon> <archive2.eon>",
		)
	}

	var opts []aeon.ReaderOption
	if len(passwords) > 0 {
		opts = append(opts, aeon.WithReaderPasswords(passwords...))
	}

	// Cross-archive diff: aeon diff arch1.eon arch2.eon
	if len(remaining) == 2 && strings.HasSuffix(remaining[1], ".eon") {
		r1, f1, err := aeon.OpenFile(remaining[0], opts...)
		if err != nil {
			return fmt.Errorf("opening %s: %w", remaining[0], err)
		}
		defer f1.Close()

		r2, f2, err := aeon.OpenFile(remaining[1], opts...)
		if err != nil {
			return fmt.Errorf("opening %s: %w", remaining[1], err)
		}
		defer f2.Close()

		diffRes, err := aeon.DiffAcross(r1, "", r2, "")
		if err != nil {
			return fmt.Errorf("diff error: %w", err)
		}
		PrintDiff(diffRes, stdout)
		return nil
	}

	// Intra-archive diff: aeon diff arch.eon [snap1] [snap2]
	r, f, err := aeon.OpenFile(remaining[0], opts...)
	if err != nil {
		return fmt.Errorf("opening %s: %w", remaining[0], err)
	}
	defer f.Close()

	var snap1, snap2 string
	if len(remaining) == 2 {
		snap1 = remaining[1]
	} else if len(remaining) >= 3 {
		snap1 = remaining[1]
		snap2 = remaining[2]
	}

	diffRes, err := r.Diff(snap1, snap2)
	if err != nil {
		return fmt.Errorf("diff error: %w", err)
	}
	PrintDiff(diffRes, stdout)
	return nil
}

// PrintDiff formats and outputs a prolly DiffResult to w.
func PrintDiff(diff *prolly.DiffResult, w io.Writer) {
	if len(diff.Added) == 0 && len(diff.Modified) == 0 && len(diff.Deleted) == 0 {
		fmt.Fprintln(w, "No differences detected (identical Prolly Tree trees).")
		return
	}

	for _, a := range diff.Added {
		fmt.Fprintf(w, "+ Added:    %-40s (%s)\n", a.Name, util.FormatHumanBytes(a.PayloadSize))
	}
	for _, m := range diff.Modified {
		fmt.Fprintf(
			w,
			"~ Modified: %-40s (%s -> %s)\n",
			m.New.Name,
			util.FormatHumanBytes(m.Old.PayloadSize),
			util.FormatHumanBytes(m.New.PayloadSize),
		)
	}
	for _, d := range diff.Deleted {
		fmt.Fprintf(w, "- Deleted:  %-40s (%s)\n", d.Name, util.FormatHumanBytes(d.PayloadSize))
	}

	fmt.Fprintf(w, "\nDiff Summary: %d added, %d modified, %d deleted.\n",
		len(diff.Added), len(diff.Modified), len(diff.Deleted))
}

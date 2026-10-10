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
	"path/filepath"
	"slices"
	"time"

	"github.com/lemon4ksan/ctrl/argkit"

	aeon "github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
)

// LogCmd displays chronological history of snapshots.
type LogCmd struct{}

func (c *LogCmd) Name() string      { return "log" }
func (c *LogCmd) Aliases() []string { return nil }
func (c *LogCmd) Synopsis() string {
	return "Display chronological history of snapshots"
}
func (c *LogCmd) Usage() string {
	return "aeon log [flags] <archive.eon>"
}

func (c *LogCmd) Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fset := flag.NewFlagSet("log", flag.ContinueOnError)
	fset.SetOutput(stderr)

	var passwords util.MultiStringFlag
	fset.Var(&passwords, "p", "Passphrase(s) to unlock encrypted container")
	fset.Var(&passwords, "password", "Passphrase(s) to unlock encrypted container")

	remaining, err := argkit.ParseInterspersedFlags(fset, args)
	if err != nil {
		return err
	}

	if len(remaining) < 1 {
		return errors.New("usage: aeon log [-p password] <archive.eon>")
	}

	var opts []aeon.ReaderOption
	if len(passwords) > 0 {
		opts = append(opts, aeon.WithReaderPasswords(passwords...))
	}

	r, f, err := aeon.OpenFile(remaining[0], opts...)
	if err != nil {
		return fmt.Errorf("opening container: %w", err)
	}
	defer f.Close()

	if !r.HasProllyTree() || len(r.Snapshots()) == 0 {
		fmt.Fprintln(stdout, "No snapshots recorded in container (standard flat INDX catalog).")
		return nil
	}

	snaps := r.Snapshots()
	fmt.Fprintf(stdout, "Snapshot history for %s (%d snapshot(s)):\n\n", filepath.Base(remaining[0]), len(snaps))
	for _, s := range slices.Backward(snaps) {
		tStr := time.Unix(0, s.Timestamp).Format("2006-01-02 15:04:05")
		fmt.Fprintf(stdout, "Snapshot %s\n", s.ID)
		if s.ParentID != "" {
			fmt.Fprintf(stdout, "Parent:     %s\n", s.ParentID)
		}
		fmt.Fprintf(stdout, "Date:       %s\n", tStr)
		fmt.Fprintf(stdout, "Root Hash:  %x\n", s.RootHash)
		fmt.Fprintf(stdout, "Files:      %d (%s)\n", s.FileCount, util.FormatHumanBytes(s.TotalBytes))
		fmt.Fprintf(stdout, "\n    %s\n\n", s.Message)
	}
	return nil
}

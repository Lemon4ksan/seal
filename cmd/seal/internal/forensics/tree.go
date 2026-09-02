// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package forensics

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
)

// TreeCmd displays a visual ASCII tree of an archive.
type TreeCmd struct{}

func (c *TreeCmd) Name() string      { return "tree" }
func (c *TreeCmd) Aliases() []string { return nil }
func (c *TreeCmd) Synopsis() string  { return "Display visual ASCII directory tree of the archive" }
func (c *TreeCmd) Usage() string     { return "seal tree <archive-path>" }

func (c *TreeCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("tree", flag.ContinueOnError)
	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := posArgs[0]

	archiveInstance := seal.NewArchive()
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	if _, err := archiveInstance.Load(f, stat.Size()); err != nil {
		return err
	}

	fmt.Fprintln(stdout, archiveInstance.Tree())
	return nil
}

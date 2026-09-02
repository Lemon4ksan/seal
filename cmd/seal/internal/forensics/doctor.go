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
	"path/filepath"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container/zip"
)

// DoctorCmd repairs corrupted archives, reconstructs lost Central Directories, and sanitizes paths.
type DoctorCmd struct{}

func (c *DoctorCmd) Name() string      { return "doctor" }
func (c *DoctorCmd) Aliases() []string { return []string{"fix", "repair"} }
func (c *DoctorCmd) Synopsis() string {
	return "Repair damaged archives, salvage broken CDs, and sanitize Zip Slip"
}

func (c *DoctorCmd) Usage() string { return "seal doctor [flags] <damaged-archive> -o <repaired-path>" }

func (c *DoctorCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	outPath := fs.String("o", "", "Repaired output archive path (required)")
	pwd := fs.String("p", "", "Decryption password")
	rebuildCD := fs.Bool("rebuild-cd", false, "Force linear signature scan to reconstruct Central Directory")
	noSanitize := fs.Bool("no-sanitize", false, "Disable Zip Slip path sanitization")

	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("usage: seal doctor [options] <damaged-archive-path> -o <repaired-path>")
	}
	inPath := posArgs[0]
	if *outPath == "" {
		return errors.New("seal doctor: output path (-o) is required")
	}

	inF, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer inF.Close()

	stat, err := inF.Stat()
	if err != nil {
		return err
	}

	outF, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	defer outF.Close()

	fmt.Fprintf(
		stdout,
		"\n%s: Diagnosing and repairing %s...\n",
		tuikit.Bold("Seal Archive Doctor"),
		tuikit.Bold(filepath.Base(inPath)),
	)
	summary, err := zip.Repair(inF, stat.Size(), outF, zip.RepairOptions{
		RebuildCentralDirectory: *rebuildCD,
		SanitizePaths:           !*noSanitize,
		Password:                *pwd,
	})
	if err != nil {
		return fmt.Errorf("repair failed: %w", err)
	}

	box := tuikit.NewBox("Repair & Recovery Summary", 65).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("Input Archive", filepath.Base(inPath)).
		AddRow("Output Archive", tuikit.Bold(*outPath)).
		AddRow("Files Recovered", fmt.Sprintf("%d (%s salvaged)", summary.FilesRecovered, util.FormatBytes(summary.BytesRecovered)))

	if summary.FilesSkipped > 0 {
		box.AddRow("Files Skipped", tuikit.Yellow(fmt.Sprintf("%d (unrecoverable)", summary.FilesSkipped)))
	}
	if summary.PathsSanitized > 0 {
		box.AddRow("Paths Sanitized", tuikit.Green(fmt.Sprintf("%d (Zip Slip neutralized)", summary.PathsSanitized)))
	}
	cdStatus := generic.Ternary(summary.CentralDirectoryRebuilt,
		"Reconstructed via linear signature scan",
		"Preserved and sanitized")
	box.AddRow("Central Directory", cdStatus)

	fmt.Println()
	return box.Render(stdout)
}

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
	"strings"

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container/zip"
)

// InspectCmd performs deep byte diagnostics, format verification, and security audits.
type InspectCmd struct{}

func (c *InspectCmd) Name() string      { return "inspect" }
func (c *InspectCmd) Aliases() []string { return nil }
func (c *InspectCmd) Synopsis() string {
	return "Deep byte diagnostics, vintage OS analysis, and security checks"
}
func (c *InspectCmd) Usage() string { return "seal inspect <archive-path>" }

func (c *InspectCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("usage: seal inspect <archive-path>")
	}
	archivePath := posArgs[0]

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	report, err := zip.Inspect(f, stat.Size())
	if err != nil {
		return err
	}

	box := tuikit.NewBox("Seal Archive Inspector", 70).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("File Name", tuikit.Bold(filepath.Base(archivePath))).
		AddRow("File Size", util.FormatBytes(report.FileSize)).
		AddRow("Container Status", fmt.Sprintf("EOCD: %t | Zip64: %t | SFX: %t", report.HasEOCD, report.IsZip64, report.IsSFX))

	if report.IsSFX {
		box.AddRow("SFX Stub Size", util.FormatBytes(report.SFXStubSize))
	}
	box.AddRow("Entries Count", fmt.Sprintf("%d files", report.EntriesCount)).
		AddRow("Total Uncompressed", util.FormatBytes(report.TotalUncompressedSize)).
		AddRow("Total Compressed", util.FormatBytes(report.TotalCompressedSize)).
		AddRow("Global Ratio", fmt.Sprintf("%.1f%%", report.GlobalRatioPercent))

	if report.Comment != "" {
		box.AddRow("Global Comment", fmt.Sprintf("%q", report.Comment))
	}
	if len(report.VintageSystems) > 0 {
		box.AddRow("Vintage Creator OS", tuikit.Magenta(strings.Join(report.VintageSystems, ", ")))
	}
	if len(report.ExoticExtraFields) > 0 {
		var tags []string
		for _, info := range report.ExoticExtraFields {
			tags = append(tags, fmt.Sprintf("%s (%s)", info.TagHex, info.Name))
		}
		box.AddRow("Exotic ExtraTags", strings.Join(tags, ", "))
	}
	if len(report.UnassignedExtraFields) > 0 {
		var tags []string
		for _, info := range report.UnassignedExtraFields {
			tags = append(tags, info.TagHex)
		}
		box.AddRow("Unassigned Tags", tuikit.Yellow(strings.Join(tags, ", ")))
	}
	if report.TrailingGarbageSize > 0 {
		box.AddRow("Trailing Garbage", tuikit.Red(util.FormatBytes(report.TrailingGarbageSize)+" unreferenced bytes"))
	}

	fmt.Println()
	if err := box.Render(stdout); err != nil {
		return err
	}

	if len(report.Issues) > 0 {
		fmt.Fprintf(stdout, "\n%s (%d):\n", tuikit.Bold("Diagnostic Findings & Security Alerts"), len(report.Issues))
		for _, issue := range report.Issues {
			badge := tuikit.BadgeInfo()
			switch issue.Severity {
			case zip.SeverityWarning:
				badge = tuikit.BadgeWarn()
			case zip.SeverityError:
				badge = tuikit.BadgeFail()
			case zip.SeveritySecurityHazard:
				badge = tuikit.Red("SEC")
			}
			section := ""
			if issue.AppnoteSection != "" {
				section = fmt.Sprintf(" [APPNOTE %s]", issue.AppnoteSection)
			}
			fmt.Fprintf(stdout, "  [%s] %s%s: %s\n", badge, tuikit.Bold(issue.Code), section, issue.Message)
		}
	} else {
		fmt.Fprintf(stdout, "\n%s No structural issues or security hazards found.\n", tuikit.BadgePassed())
	}

	fmt.Println()
	return nil
}

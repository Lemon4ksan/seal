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
	"math/bits"
	"os"
	"path/filepath"
	"strings"

	"github.com/lemon4ksan/foundation/argkit"
	"github.com/lemon4ksan/cli/tuikit"

	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/eon"
	"github.com/lemon4ksan/seal/container/sevenzip"
	"github.com/lemon4ksan/seal/container/zip"
)

// InspectCmd performs deep byte diagnostics, format verification, and security audits.
type InspectCmd struct{}

func (c *InspectCmd) Name() string      { return "inspect" }
func (c *InspectCmd) Aliases() []string { return nil }
func (c *InspectCmd) Synopsis() string {
	return "Deep byte diagnostics, vintage OS analysis, and security checks"
}
func (c *InspectCmd) Usage() string { return "seal inspect [flags] <archive-path>" }

func (c *InspectCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	pwd := fs.String("p", "", "Decryption password")
	posArgs, err := argkit.ParseInterspersedFlags(fs, args)
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

	cType := util.DetectContainerType(archivePath)
	if cType == container.SevenZip {
		return inspectSevenZip(f, stat.Size(), archivePath, *pwd, stdout)
	}
	if cType == container.Eon {
		return inspectEon(f, stat.Size(), archivePath, stdout)
	}

	return inspectZip(f, stat.Size(), archivePath, stdout)
}

func inspectEon(rAt io.ReaderAt, size int64, archivePath string, stdout io.Writer) error {
	r, err := eon.NewReader(rAt, size)
	if err != nil {
		return err
	}

	chunks := r.Chunks()
	extents := r.Extents()
	files := r.Entries()

	var totalUncomp, totalComp uint64
	for _, ch := range chunks {
		totalUncomp += uint64(ch.UncompressedSize)
		totalComp += uint64(ch.CompressedSize)
	}

	ratio := 0.0
	if totalUncomp > 0 {
		ratio = float64(totalComp) / float64(totalUncomp) * 100.0
	}

	box := tuikit.NewBox("Seal EON Container Inspector", 70).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("File Name", tuikit.Bold(filepath.Base(archivePath))).
		AddRow("File Size", util.FormatBytes(size)).
		AddRow("Container Format", tuikit.Cyan("EON (Modern High-Performance Solid)")).
		AddRow("Block Alignment", fmt.Sprintf("%d bytes (%d-bit shift)", r.BlockSize(), bits.TrailingZeros64(uint64(r.BlockSize())))).
		AddRow("Chunks Count", fmt.Sprintf("%d physical chunk(s)", len(chunks))).
		AddRow("Catalog Extents", fmt.Sprintf("%d extent(s) / %d file(s)", len(extents), len(files))).
		AddRow("Total Uncompressed", util.FormatBytes(int64(totalUncomp))).
		AddRow("Total Compressed", util.FormatBytes(int64(totalComp))).
		AddRow("Global Ratio", fmt.Sprintf("%.2f%% (%.2fx reduction)", ratio, 100.0/max(ratio, 0.01)))

	fmt.Println()
	return box.Render(stdout)
}

func inspectZip(rAt io.ReaderAt, size int64, archivePath string, stdout io.Writer) error {
	report, err := zip.Inspect(rAt, size)
	if err != nil {
		return err
	}

	box := tuikit.NewBox("Seal Archive Inspector", 70).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("File Name", tuikit.Bold(filepath.Base(archivePath))).
		AddRow("File Size", util.FormatBytes(report.FileSize)).
		AddRow("Container Format", "ZIP (PKWARE)").
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

func inspectSevenZip(rAt io.ReaderAt, size int64, archivePath, pwd string, stdout io.Writer) error {
	report, err := sevenzip.Inspect(rAt, size, pwd)
	if err != nil {
		return err
	}

	box := tuikit.NewBox("Seal Archive Inspector", 70).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("File Name", tuikit.Bold(filepath.Base(archivePath))).
		AddRow("File Size", util.FormatBytes(report.FileSize)).
		AddRow("Container Format", "7-Zip (.7z)").
		AddRow("Version", fmt.Sprintf("v%d.%d", report.MajorVersion, report.MinorVersion)).
		AddRow("Signature Status", fmt.Sprintf("Valid: %t | SFX: %t", report.SignatureValid, report.IsSFX))

	if report.IsSFX {
		box.AddRow("SFX Stub Size", util.FormatBytes(report.SFXStubSize))
	}

	headerStatus := "Plain"
	if report.IsEncryptedHeader {
		headerStatus = "AES-256 Encrypted (Hidden file names)"
	} else if report.IsEncodedHeader {
		headerStatus = "LZMA2 Compressed"
	}
	box.AddRow("Header Status", headerStatus).
		AddRow("Folders (Solid Blocks)", fmt.Sprintf("%d blocks", report.FoldersCount))

	if len(report.Folders) > 0 {
		var coderNames []string
		for _, c := range report.Folders[0].Coders {
			coderNames = append(coderNames, c.MethodName)
		}
		box.AddRow("Primary Coders", strings.Join(coderNames, " -> "))
	}

	box.AddRow("Entries Count", fmt.Sprintf("%d files", report.EntriesCount)).
		AddRow("Total Uncompressed", util.FormatBytes(report.TotalUncompressedSize)).
		AddRow("Total Compressed", util.FormatBytes(report.TotalCompressedSize)).
		AddRow("Global Ratio", fmt.Sprintf("%.1f%%", report.GlobalRatioPercent))

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
			case sevenzip.SeverityWarning:
				badge = tuikit.BadgeWarn()
			case sevenzip.SeverityError:
				badge = tuikit.BadgeFail()
			case sevenzip.SeveritySecurityHazard:
				badge = tuikit.Red("SEC")
			}
			fmt.Fprintf(stdout, "  [%s] %s: %s\n", badge, tuikit.Bold(issue.Code), issue.Message)
		}
	} else {
		fmt.Fprintf(stdout, "\n%s No structural issues or security hazards found.\n", tuikit.BadgePassed())
	}

	fmt.Println()
	return nil
}

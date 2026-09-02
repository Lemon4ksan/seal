// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lemon4ksan/foundation/generic"
	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/codec"
	_ "github.com/lemon4ksan/seal/codec/legacy"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/option"
)

func main() {
	app := tuikit.NewApp("seal", seal.Version, "🦭 Seal — High-Performance Universal Archiving Engine",
		&CreateCmd{},
		&ExtractCmd{},
		&ListCmd{},
		&CatCmd{},
		&TestCmd{},
		&TreeCmd{},
		&InspectCmd{},
		&DoctorCmd{},
		&BenchCmd{},
	)

	app.CommandGroups = []tuikit.CommandGroup{
		{
			Title:    "Archive Operations",
			Commands: []string{"create", "extract", "list", "cat"},
		},
		{
			Title:    "Integrity, Forensics & Repair",
			Commands: []string{"test", "tree", "inspect", "doctor"},
		},
		{
			Title:    "System & Performance",
			Commands: []string{"bench"},
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		stop()
		fmt.Fprintf(os.Stderr, "\n%s %v\n", tuikit.BadgeFail(), err)
		os.Exit(1)
	}
	stop()
}

func detectContainerType(path string) container.Type {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".7z"):
		return container.SevenZip
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return container.TarGz
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return container.TarXz
	case strings.HasSuffix(lower, ".tar.zst"), strings.HasSuffix(lower, ".tzst"):
		return container.TarZst
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return container.TarBz2
	case strings.HasSuffix(lower, ".tar"):
		return container.Tar
	default:
		return container.Zip
	}
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cib", float64(b)/float64(div), "KMGTPE"[exp])
}

// -----------------------------------------------------------------------------
// 1. Create Command
// -----------------------------------------------------------------------------

type CreateCmd struct{}

func (c *CreateCmd) Name() string      { return "create" }
func (c *CreateCmd) Aliases() []string { return []string{"a", "c"} }
func (c *CreateCmd) Synopsis() string {
	return "Create an archive (7z, ZIP, TAR, TAR.GZ, TAR.XZ, TAR.ZST, TAR.BZ2)"
}
func (c *CreateCmd) Usage() string { return "seal create [flags] <files...> -o <archive-path>" }

func (c *CreateCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	outFile := fs.String("o", "archive.7z", "Output archive destination path")
	pwd := fs.String("p", "", "Password for encryption")
	methodStr := fs.String("m", "", "Compression method (lzma2, ppmd, zstd, deflate, gzip, store)")
	filterStr := fs.String("filter", "none", "Bytecode filter: bcj (x86), arm, none")
	level := fs.Int("l", 6, "Compression level (1-9)")
	comment := fs.String("c", "", "Archive comment")

	inputs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(inputs) == 0 {
		return errors.New("no input files or directories specified")
	}

	cType := detectContainerType(*outFile)
	method := codec.Deflate
	if *methodStr != "" {
		switch strings.ToLower(*methodStr) {
		case "store", "none":
			method = codec.Store
		case "gzip", "gz":
			method = codec.Gzip
		case "lzma2", "7z":
			method = codec.LZMA2
		case "ppmd":
			method = codec.PPMd
		case "zstd", "zstandard":
			method = codec.ZStandard
		case "cmpsc", "method16":
			method = codec.IBMCmpsc
		}
	} else {
		switch cType {
		case container.SevenZip:
			method = codec.LZMA2
		case container.TarGz:
			method = codec.Gzip
		case container.TarZst:
			method = codec.ZStandard
		case container.TarXz:
			method = codec.LZMA2
		default:
			method = codec.Deflate
		}
	}

	opts := []seal.Option{
		option.WithFormat(cType),
		seal.WithCompression(method, *level),
		seal.WithPassword(*pwd),
		seal.WithComment(*comment),
	}

	archiveInstance := seal.NewArchive(opts...)

	fmt.Fprintf(
		stdout,
		"\n%s Scanning inputs for %s (%s)...\n",
		tuikit.Cyan("🔍"),
		tuikit.Bold(*outFile),
		tuikit.Magenta(cType.String()),
	)
	var totalFiles, totalBytes int64

	for _, input := range inputs {
		info, err := os.Stat(input)
		if err != nil {
			return err
		}

		if info.IsDir() {
			if _, err := archiveInstance.AddDir(input); err != nil {
				return err
			}
		} else {
			if _, err := archiveInstance.AddFile(input); err != nil {
				return err
			}
		}
	}

	for _, f := range archiveInstance.Files() {
		totalFiles++
		totalBytes += f.UncompressedSize()
	}

	fmt.Fprintf(stdout, "%s Compressing %d entries (%s uncompressed) via %s (Level %d, Filter: %s)...\n",
		tuikit.Cyan("⚡"), totalFiles, formatBytes(totalBytes), tuikit.Bold(method.String()), *level, *filterStr)

	start := time.Now()

	err = seal.UseSink(seal.ToFilePath(*outFile), func(w io.Writer) error {
		_, err := archiveInstance.WriteTo(w)
		return err
	})
	if err != nil {
		_ = os.Remove(*outFile)
		return err
	}

	elapsed := time.Since(start)
	stat, _ := os.Stat(*outFile)
	var compSize int64
	if stat != nil {
		compSize = stat.Size()
	}

	ratio := 0.0
	if totalBytes > 0 {
		ratio = float64(compSize) / float64(totalBytes) * 100
	}
	speed := (float64(totalBytes) / (1024 * 1024)) / elapsed.Seconds()

	box := tuikit.NewBox("✨ Archive Created Successfully", 60).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("Output Archive", tuikit.Bold(*outFile)).
		AddRow("Entries Count", fmt.Sprintf("%d files", totalFiles)).
		AddRow("Original Size", formatBytes(totalBytes)).
		AddRow("Compressed Size", formatBytes(compSize)).
		AddRow("Compression Ratio", fmt.Sprintf("%.2f%%", ratio)).
		AddRow("Throughput", fmt.Sprintf("%.2f MB/s (%v)", speed, elapsed))

	fmt.Println()
	return box.Render(stdout)
}

// -----------------------------------------------------------------------------
// 2. Extract Command
// -----------------------------------------------------------------------------

type ExtractCmd struct{}

func (c *ExtractCmd) Name() string      { return "extract" }
func (c *ExtractCmd) Aliases() []string { return []string{"x", "e"} }
func (c *ExtractCmd) Synopsis() string  { return "Extract an archive to destination directory" }
func (c *ExtractCmd) Usage() string     { return "seal extract [flags] <archive-path> -d <output-dir>" }

func (c *ExtractCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	destDir := fs.String("d", ".", "Destination extraction directory")
	pwd := fs.String("p", "", "Decryption password")

	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := posArgs[0]

	fmt.Fprintf(
		stdout,
		"\n%s Extracting %s to %s...\n",
		tuikit.Cyan("📦"),
		tuikit.Bold(archivePath),
		tuikit.Bold(*destDir),
	)
	start := time.Now()

	var extractedCount int64
	archiver := seal.NewArchiver(
		seal.WithPassword(*pwd),
		seal.WithOnFileDone(func(name string, err error) {
			if err == nil {
				atomic.AddInt64(&extractedCount, 1)
			}
		}),
	)
	err = archiver.UnarchiveWithContext(ctx, seal.FromFilePath(archivePath), *destDir, seal.WithPassword(*pwd))
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "%s Extraction completed in %v (%d files extracted)\n\n",
		tuikit.BadgePass(), time.Since(start), extractedCount)
	return nil
}

// -----------------------------------------------------------------------------
// 3. List Command
// -----------------------------------------------------------------------------

type ListCmd struct{}

func (c *ListCmd) Name() string      { return "list" }
func (c *ListCmd) Aliases() []string { return []string{"l", "ls"} }
func (c *ListCmd) Synopsis() string  { return "List all entries and metadata in an archive" }
func (c *ListCmd) Usage() string     { return "seal list [flags] <archive-path>" }

func (c *ListCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
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
	entries, err := archiver.GetEntries(seal.FromFilePath(archivePath))
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
			formatBytes(e.Size()),
			formatBytes(e.CompressedSize()),
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
		tuikit.Bold("Total:"), formatBytes(totalUncomp), formatBytes(totalComp), globalRatio)

	return nil
}

// -----------------------------------------------------------------------------
// 4. Cat Command
// -----------------------------------------------------------------------------

type CatCmd struct{}

func (c *CatCmd) Name() string      { return "cat" }
func (c *CatCmd) Aliases() []string { return nil }
func (c *CatCmd) Synopsis() string  { return "Stream a specific file entry directly to stdout" }
func (c *CatCmd) Usage() string     { return "seal cat [flags] <archive-path> <entry-path>" }

func (c *CatCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("cat", flag.ContinueOnError)
	pwd := fs.String("p", "", "Decryption password")

	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 2 {
		return errors.New("usage: seal cat <archive> <entry-path>")
	}
	archivePath := posArgs[0]
	targetEntry := posArgs[1]

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	data, err := archiver.ReadFile(seal.FromFilePath(archivePath), targetEntry)
	if err != nil {
		return err
	}

	_, err = stdout.Write(data)
	return err
}

// -----------------------------------------------------------------------------
// 5. Test Command
// -----------------------------------------------------------------------------

type TestCmd struct{}

func (c *TestCmd) Name() string      { return "test" }
func (c *TestCmd) Aliases() []string { return []string{"t"} }
func (c *TestCmd) Synopsis() string  { return "Verify checksum and decompression integrity" }
func (c *TestCmd) Usage() string     { return "seal test [flags] <archive-path>" }

func (c *TestCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	pwd := fs.String("p", "", "Decryption password")

	posArgs, err := tuikit.ParseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(posArgs) < 1 {
		return errors.New("archive file not specified")
	}
	archivePath := posArgs[0]

	fmt.Fprintf(stdout, "\n%s Verifying integrity of %s...\n", tuikit.Cyan("🔒"), tuikit.Bold(archivePath))
	start := time.Now()

	archiver := seal.NewArchiver(seal.WithPassword(*pwd))
	err = archiver.Verify(seal.FromFilePath(archivePath), seal.WithPassword(*pwd))
	if err != nil {
		return fmt.Errorf("integrity verification failed: %w", err)
	}

	fmt.Fprintf(stdout, "%s All checksums and data streams verified successfully in %v\n\n",
		tuikit.BadgePassed(), time.Since(start))
	return nil
}

// -----------------------------------------------------------------------------
// 6. Tree Command
// -----------------------------------------------------------------------------

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

// -----------------------------------------------------------------------------
// 7. Inspect Command
// -----------------------------------------------------------------------------

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

	box := tuikit.NewBox("🔍 Seal Archive Inspector", 70).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("File Name", tuikit.Bold(filepath.Base(archivePath))).
		AddRow("File Size", formatBytes(report.FileSize)).
		AddRow("Container Status", fmt.Sprintf("EOCD: %t | Zip64: %t | SFX: %t", report.HasEOCD, report.IsZip64, report.IsSFX))

	if report.IsSFX {
		box.AddRow("SFX Stub Size", formatBytes(report.SFXStubSize))
	}
	box.AddRow("Entries Count", fmt.Sprintf("%d files", report.EntriesCount)).
		AddRow("Total Uncompressed", formatBytes(report.TotalUncompressedSize)).
		AddRow("Total Compressed", formatBytes(report.TotalCompressedSize)).
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
		box.AddRow("Trailing Garbage", tuikit.Red(formatBytes(report.TrailingGarbageSize)+" unreferenced bytes"))
	}

	fmt.Println()
	if err := box.Render(stdout); err != nil {
		return err
	}

	if len(report.Issues) > 0 {
		fmt.Fprintf(
			stdout,
			"\n%s %s (%d):\n",
			tuikit.Bold("📋"),
			tuikit.Bold("Diagnostic Findings & Security Alerts"),
			len(report.Issues),
		)
		for _, issue := range report.Issues {
			badge := tuikit.BadgeInfo()
			switch issue.Severity {
			case zip.SeverityWarning:
				badge = tuikit.BadgeWarn()
			case zip.SeverityError:
				badge = tuikit.BadgeFail()
			case zip.SeveritySecurityHazard:
				badge = tuikit.Red("🚨 SEC")
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

// -----------------------------------------------------------------------------
// 8. Doctor Command
// -----------------------------------------------------------------------------

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
		"\n%s %s: Diagnosing and repairing %s...\n",
		tuikit.Cyan("🩺"),
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

	box := tuikit.NewBox("✨ Repair & Recovery Summary", 65).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("Input Archive", filepath.Base(inPath)).
		AddRow("Output Archive", tuikit.Bold(*outPath)).
		AddRow("Files Recovered", fmt.Sprintf("%d (%s salvaged)", summary.FilesRecovered, formatBytes(summary.BytesRecovered)))

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

// -----------------------------------------------------------------------------
// 9. Bench Command
// -----------------------------------------------------------------------------

type BenchCmd struct{}

func (c *BenchCmd) Name() string      { return "bench" }
func (c *BenchCmd) Aliases() []string { return nil }
func (c *BenchCmd) Synopsis() string  { return "Run compression speed benchmark" }
func (c *BenchCmd) Usage() string     { return "seal bench [-size <MB>] [-files <count>] [-j <workers>]" }

func (c *BenchCmd) Run(_ context.Context, args []string, stdout, _ io.Writer) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	sizeMB := fs.Int("size", 50, "Total payload size in MB")
	filesCount := fs.Int("files", 100, "Number of generated entries")
	workers := fs.Int("j", runtime.NumCPU(), "Worker concurrency")

	if _, err := tuikit.ParseInterspersedFlags(fs, args); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "\n%s %s\n", tuikit.Bold("=== Seal Silicon Benchmark ==="), tuikit.Cyan("⚡"))
	fmt.Fprintf(stdout, "Payload: %d MB across %d entries, CPU cores: %d\n\n", *sizeMB, *filesCount, *workers)

	pattern := []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Integer nec odio. Praesent libero. ")
	totalBytes := (*sizeMB) * 1024 * 1024
	perFileBytes := totalBytes / (*filesCount)

	data := bytes.Repeat(pattern, (perFileBytes/len(pattern))+1)[:perFileBytes]

	runRun := func() time.Duration {
		runtime.GC()
		archiveInstance := seal.NewArchive(seal.WithCompression(codec.Deflate, 6))
		for i := 0; i < *filesCount; i++ {
			archiveInstance.AddBytes(data, fmt.Sprintf("bench_%04d.txt", i))
		}

		start := time.Now()
		_, err := archiveInstance.WriteTo(io.Discard)
		if err != nil {
			panic(err)
		}
		return time.Since(start)
	}

	// Warmup
	_ = runRun()

	elapsed := runRun()
	throughputMB := float64(*sizeMB) / elapsed.Seconds()

	box := tuikit.NewBox("🚀 Benchmark Results", 55).
		SetStyle(tuikit.BorderRounded).
		SetIndent(0).
		AddRow("Payload Size", fmt.Sprintf("%d MB", *sizeMB)).
		AddRow("Entries Count", fmt.Sprintf("%d files", *filesCount)).
		AddRow("Compression Codec", "DEFLATE (Level 6)").
		AddRow("Elapsed Time", fmt.Sprintf("%v", elapsed)).
		AddRow("Throughput", tuikit.Bold(fmt.Sprintf("%.2f MB/s", throughputMB)))

	return box.Render(stdout)
}

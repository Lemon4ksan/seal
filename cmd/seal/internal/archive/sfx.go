// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package archive

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lemon4ksan/ctrl/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/util"
	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
)

// SFXCmd implements In-Situ Self-Mounting Executable (SMX) creation compliant with EON Spec 3.4.
type SFXCmd struct{}

func (s *SFXCmd) Name() string      { return "sfx" }
func (s *SFXCmd) Aliases() []string { return []string{"smx", "capsule", "bundle"} }
func (s *SFXCmd) Synopsis() string {
	return "Create an In-Situ Self-Mounting Executable (SMX) embedding an EON archive"
}

func (s *SFXCmd) Usage() string {
	return `seal sfx [flags] <output.exe> <files...>

Creates a standalone executable (.exe) containing an embedded EON container.
Double-clicking the generated file launches the self-mounting runtime which
streams assets on-demand via PRZM without requiring external software.

Flags:
  -mx=<0..9>       Set compression level (default: 1 for ultra-fast line rate)
  -m=<method>      Set compression method (default: przm)
  -ms[=...]        Set solid archive mode (default: 64m)
  -solid[=...]     Alias for -ms
  -mmt=<N>         Set CPU compression threads (default: auto)
  -gpu             Enable NVIDIA CUDA acceleration for PRZM

Examples:
  seal sfx game_app.exe ./gamedata/
  seal sfx -ms=64m -m=przm -mx=1 wallpaper_app.exe "D:\Program Files\wallpaper_engine\assets"
`
}

func (s *SFXCmd) Run(ctx context.Context, args []string, stdout, _ io.Writer) error {
	sz, err := util.ParseSevenZipArgs("sfx", args)
	if err != nil {
		return err
	}

	outFile := sz.Archive
	if outFile == "" {
		return errors.New("missing output executable path")
	}
	if !strings.HasSuffix(strings.ToLower(outFile), ".exe") {
		outFile += ".exe"
	}

	rawInputs := sz.Items
	if len(rawInputs) == 0 {
		return errors.New("no input files or directories specified")
	}

	var inputs []string
	for _, input := range rawInputs {
		if strings.ContainsAny(input, "*?[") {
			matches, err := filepath.Glob(input)
			if err == nil && len(matches) > 0 {
				inputs = append(inputs, matches...)
				continue
			}
		}
		inputs = append(inputs, input)
	}

	method := codec.PRZM
	if sz.Method != "" {
		switch strings.ToLower(sz.Method) {
		case "zstd", "zstandard":
			method = codec.ZStandard
		case "store", "none":
			method = codec.Store
		default:
			method = codec.PRZM
		}
	}

	level := 1
	if sz.Level >= 0 {
		level = sz.Level
	}

	isSolid := true
	if sz.SolidExplicit {
		isSolid = sz.Solid
	}

	solidSize := sz.SolidSize
	if isSolid && solidSize <= 0 {
		solidSize = 64 * 1024 * 1024 // 64 MiB default
	}

	// 1. Locate or compile the SMX Launcher Stub
	stubPath, err := locateOrBuildStub()
	if err != nil {
		return fmt.Errorf("failed to prepare SMX stub executable: %w", err)
	}

	stubBytes, err := os.ReadFile(stubPath)
	if err != nil {
		return fmt.Errorf("failed to read SMX stub %s: %w", stubPath, err)
	}
	stubSize := int64(len(stubBytes))

	fmt.Fprintf(
		stdout,
		"\n%s Preparing In-Situ Self-Mounting Executable %s...\n",
		tuikit.BadgeInfo(),
		tuikit.Bold(outFile),
	)
	fmt.Fprintf(stdout, "Embedded Stub: %s (%s)\n", tuikit.Cyan(filepath.Base(stubPath)), util.FormatBytes(stubSize))

	// 2. Package inputs into EON container
	opts := []seal.Option{
		seal.WithContainer(container.Eon),
		seal.WithCompression(method, level),
		seal.WithSolid(isSolid, solidSize),
		seal.WithSolidExtension(sz.SolidExtension),
		seal.WithBytecodeFilter(sz.Filter),
	}
	if sz.Workers > 0 {
		opts = append(opts, seal.WithWorkers(sz.Workers))
	}

	archiveInstance := seal.NewArchive(opts...)
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

	var totalFiles, totalBytes int64
	for _, f := range archiveInstance.Files() {
		totalFiles++
		totalBytes += f.UncompressedSize()
	}

	solidDesc := "off"
	if isSolid {
		solidDesc = util.FormatBytes(solidSize)
	}
	fmt.Fprintf(stdout, "Compressing %d entries (%s uncompressed) via %s (Solid: %s)...\n",
		totalFiles, util.FormatBytes(totalBytes), tuikit.Bold(method.String()), tuikit.Cyan(solidDesc))

	start := time.Now()

	// 3. Create target executable with stub + EON archive
	targetFile, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", outFile, err)
	}

	// Write stub
	if _, err := targetFile.Write(stubBytes); err != nil {
		targetFile.Close()
		return fmt.Errorf("cannot write stub to %s: %w", outFile, err)
	}

	// Write EON payload directly after stub
	writeOpts := []seal.Option{
		seal.WithSolid(isSolid, solidSize),
		seal.WithSolidExtension(sz.SolidExtension),
	}
	payloadBytesWritten, err := archiveInstance.WriteTo(ctx, targetFile, writeOpts...)
	if err != nil {
		targetFile.Close()
		return fmt.Errorf("archive serialization failed: %w", err)
	}

	// 4. Patch Trailer at EOF:
	// Trailer layout (32 bytes):
	// [0:8]   CentralDirOffset (uint64)
	// [8:16]  RecoveryOffset (uint64)
	// [16:24] ArchiveStartOffset (uint64) -> Set to stubSize!
	// [24:28] Flags (uint32)              -> Add FLAG_SELF_MOUNTING (0x0080)!
	// [28:32] Magic ('!EON')
	totalFileSize, err := targetFile.Seek(0, io.SeekEnd)
	if err != nil {
		targetFile.Close()
		return err
	}

	var trailer [32]byte
	if _, err := targetFile.ReadAt(trailer[:], totalFileSize-32); err != nil {
		// If ReadAt on write-only handle fails, seek and read
		targetFile.Close()
		rf, oerr := os.OpenFile(outFile, os.O_RDWR, 0o755)
		if oerr != nil {
			return oerr
		}
		if _, err := rf.ReadAt(trailer[:], totalFileSize-32); err != nil {
			rf.Close()
			return err
		}
		binary.LittleEndian.PutUint64(trailer[16:24], uint64(stubSize))
		flags := binary.LittleEndian.Uint32(trailer[24:28]) | 0x0080 // FLAG_SELF_MOUNTING
		binary.LittleEndian.PutUint32(trailer[24:28], flags)
		if _, err := rf.WriteAt(trailer[:], totalFileSize-32); err != nil {
			rf.Close()
			return err
		}
		rf.Close()
	} else {
		binary.LittleEndian.PutUint64(trailer[16:24], uint64(stubSize))
		flags := binary.LittleEndian.Uint32(trailer[24:28]) | 0x0080 // FLAG_SELF_MOUNTING
		binary.LittleEndian.PutUint32(trailer[24:28], flags)
		if _, err := targetFile.WriteAt(trailer[:], totalFileSize-32); err != nil {
			targetFile.Close()
			return err
		}
		targetFile.Close()
	}

	elapsed := time.Since(start)
	ratio := float64(payloadBytesWritten) / float64(totalBytes) * 100.0

	fmt.Println()
	box := tuikit.NewBox("SMX Executable Created Successfully", 68)
	box.AddRow("Output Binary", tuikit.Bold(outFile))
	box.AddRow("SMX Launcher Stub", util.FormatBytes(stubSize))
	box.AddRow("Embedded EON Payload", util.FormatBytes(payloadBytesWritten))
	box.AddRow("Total Binary Size", util.FormatBytes(totalFileSize))
	box.AddRow("Entries Bundled", fmt.Sprintf("%d files / directories", totalFiles))
	box.AddRow(
		"Compression Ratio",
		fmt.Sprintf("%.2f%% (%s savings)", ratio, util.FormatBytes(totalBytes-payloadBytesWritten)),
	)
	box.AddRow(
		"Encoding Time",
		fmt.Sprintf("%v (%.2f MB/s)", elapsed, float64(totalBytes)/(1024*1024)/elapsed.Seconds()),
	)
	box.AddRow("Launch Instructions", tuikit.Green("Double-click "+filepath.Base(outFile)+" in Explorer to run!"))
	_ = box.Render(stdout)

	return nil
}

func locateOrBuildStub() (string, error) {
	// Check sibling directory or current working directory
	candidates := []string{
		"smx-runner.exe",
		"./cmd/smx-runner/smx-runner.exe",
	}

	exePath, err := os.Executable()
	if err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "smx-runner.exe"))
	}

	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return cand, nil
		}
	}

	// Try compiling on-the-fly
	tempStub := filepath.Join(os.TempDir(), "smx-runner.exe")
	cmd := exec.Command("go", "build", "-o", tempStub, "./cmd/smx-runner")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build smx-runner failed: %w (output: %s)", err, string(out))
	}
	return tempStub, nil
}

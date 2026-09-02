// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lemon4ksan/foundation/silicon/endian"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/vfs"
)

// RepairOptions configures the Archive Doctor recovery behavior.
type RepairOptions struct {
	// RebuildCentralDirectory performs a full linear scan of all Local File Headers to salvage data when the Central Directory is broken or absent.
	RebuildCentralDirectory bool
	// SanitizePaths strips directory traversal components ("../", "..\", leading "/") to secure the archive against Zip Slip.
	SanitizePaths bool
	// TargetCodec optionally re-compresses all entries using the specified modern algorithm (e.g. converting legacy Shrink/Reduce to Deflate).
	TargetCodec codec.Method
	// Password is used to decrypt password-protected entries.
	Password string
}

// RepairSummary records the statistics and log of actions taken during the archive repair process.
type RepairSummary struct {
	FilesRecovered          int      `json:"files_recovered"`
	FilesSkipped            int      `json:"files_skipped"`
	PathsSanitized          int      `json:"paths_sanitized"`
	BytesRecovered          int64    `json:"bytes_recovered"`
	CentralDirectoryRebuilt bool     `json:"central_directory_rebuilt"`
	Logs                    []string `json:"logs"`
}

// Repair analyzes, repairs, and produces a sanitized, structurally valid ZIP archive into dest.
func Repair(src io.ReaderAt, size int64, dest io.Writer, opts RepairOptions) (*RepairSummary, error) {
	if src == nil || size <= 0 || dest == nil {
		return nil, errors.New("zip: doctor invalid arguments")
	}

	summary := &RepairSummary{
		Logs: make([]string, 0, 16),
	}

	report, _ := Inspect(src, size)
	if report != nil && opts.SanitizePaths {
		for _, issue := range report.Issues {
			if issue.Code == "SEC_ZIP_SLIP" {
				summary.PathsSanitized++
				summary.Logs = append(summary.Logs, fmt.Sprintf("Sanitized hazardous path: %q", issue.FileName))
			}
		}
	}

	// 1. Try standard reader first to see if Central Directory is intact
	zr, err := NewReader(src, size, opts.Password, codec.DefaultRegistry)
	if err == nil && !opts.RebuildCentralDirectory {
		summary.Logs = append(
			summary.Logs,
			"Central Directory is intact; performing security sanitization and repackaging",
		)
		return repairFromIntactReader(zr, dest, opts, summary)
	}

	// 2. Central Directory is damaged or Rebuild is forced -> Run linear local header scanner
	summary.CentralDirectoryRebuilt = true
	summary.Logs = append(
		summary.Logs,
		"Central Directory damaged or missing; executing linear Local File Header signature scanner",
	)
	return salvageViaLinearScan(src, size, dest, opts, summary)
}

func repairFromIntactReader(
	zr *Reader,
	dest io.Writer,
	opts RepairOptions,
	summary *RepairSummary,
) (*RepairSummary, error) {
	zw := NewWriter(dest, codec.DefaultRegistry)

	for _, entry := range zr.Entries() {
		origName := entry.Name()
		name := origName

		if opts.SanitizePaths {
			name = sanitizeZipPath(name)
			if name != origName {
				summary.PathsSanitized++
				summary.Logs = append(summary.Logs, fmt.Sprintf("Sanitized path: %q -> %q", origName, name))
			}
		}

		rc, err := entry.Open()
		if err != nil {
			summary.FilesSkipped++
			summary.Logs = append(summary.Logs, fmt.Sprintf("Skipped damaged entry %q: %v", origName, err))
			continue
		}

		repairedFile, _ := file.New(name, entry.IsDir())
		repairedFile.WithModTime(entry.ModTime())

		if opts.TargetCodec != 0 {
			repairedFile.SetMethod(opts.TargetCodec)
		} else {
			repairedFile.SetMethod(entry.Method())
		}

		if err := zw.WriteEntry(repairedFile, rc); err != nil {
			_ = rc.Close()
			summary.FilesSkipped++
			summary.Logs = append(summary.Logs, fmt.Sprintf("Write error on %q: %v", name, err))
			continue
		}
		_ = rc.Close()

		summary.FilesRecovered++
		summary.BytesRecovered += entry.UncompressedSize()
	}

	if err := zw.Close(); err != nil {
		return summary, fmt.Errorf("doctor finalize: %w", err)
	}

	return summary, nil
}

func salvageViaLinearScan(
	src io.ReaderAt,
	size int64,
	dest io.Writer,
	opts RepairOptions,
	summary *RepairSummary,
) (*RepairSummary, error) {
	zw := NewWriter(dest, codec.DefaultRegistry)

	// Stage 1: Attempt sequential stream extraction using StreamReader
	sr := NewStreamReader(io.NewSectionReader(src, 0, size), opts.Password, codec.DefaultRegistry)
	for {
		entry, r, err := sr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			break
		}

		origName := entry.Name()
		name := origName
		if opts.SanitizePaths {
			name = sanitizeZipPath(name)
			if name != origName {
				summary.PathsSanitized++
				summary.Logs = append(summary.Logs, fmt.Sprintf("Sanitized path: %q -> %q", origName, name))
			}
		}

		repairedFile, _ := file.New(name, entry.IsDir())
		repairedFile.WithModTime(entry.ModTime())
		if opts.TargetCodec != 0 {
			repairedFile.SetMethod(opts.TargetCodec)
		} else {
			repairedFile.SetMethod(entry.Method())
		}

		if err := zw.WriteEntry(repairedFile, r); err != nil {
			summary.Logs = append(summary.Logs, fmt.Sprintf("Failed writing salvaged %q: %v", name, err))
			continue
		}

		summary.FilesRecovered++
		summary.BytesRecovered += entry.UncompressedSize()
		summary.Logs = append(summary.Logs, fmt.Sprintf("Salvaged entry %q (Size: %d)", name, entry.UncompressedSize()))
	}

	if summary.FilesRecovered > 0 {
		if err := zw.Close(); err != nil {
			return summary, fmt.Errorf("doctor finalize: %w", err)
		}
		return summary, nil
	}

	// Stage 2: Fallback to signature jumping scanner
	var offset int64
	buf := make([]byte, 64*1024)

	for offset < size {
		readLen := min(int64(len(buf)), size-offset)
		if readLen < 30 {
			break
		}
		n, _ := src.ReadAt(buf[:readLen], offset)
		if n < 30 {
			break
		}

		foundPos := -1
		for i := 0; i <= n-4; i++ {
			if endian.Load32(buf[i:], 0) == LocalFileHeaderSignature {
				foundPos = i
				break
			}
		}

		if foundPos == -1 {
			offset += int64(n - 3)
			continue
		}

		hdrOffset := offset + int64(foundPos)
		secR := io.NewSectionReader(src, hdrOffset+4, size-(hdrOffset+4))
		lfh, err := ReadLocalFileHeader(secR)
		if err != nil {
			offset = hdrOffset + 4
			continue
		}

		origName := lfh.Filename
		if origName == "" {
			origName = fmt.Sprintf("salvaged_file_%04d.dat", summary.FilesRecovered+1)
		}

		name := origName
		if opts.SanitizePaths {
			name = sanitizeZipPath(name)
			if name != origName {
				summary.PathsSanitized++
				summary.Logs = append(summary.Logs, fmt.Sprintf("Sanitized path: %q -> %q", origName, name))
			}
		}

		method := codec.Method(lfh.CompressionMethod)
		compSize := int64(lfh.CompressedSize)
		uncompSize := int64(lfh.UncompressedSize)
		payloadOffset := hdrOffset + 30 + int64(lfh.FilenameLength) + int64(lfh.ExtraFieldLength)

		isDir := strings.HasSuffix(name, "/")
		repairedFile, _ := file.New(name, isDir)
		modTime := dosDateTimeToTime(lfh.LastModFileDate, lfh.LastModFileTime)
		repairedFile.WithModTime(modTime)

		if opts.TargetCodec != 0 {
			repairedFile.SetMethod(opts.TargetCodec)
		} else {
			repairedFile.SetMethod(method)
		}

		if compSize > 0 && payloadOffset+compSize <= size {
			payloadReader := io.NewSectionReader(src, payloadOffset, compSize)
			var decReader io.Reader = payloadReader

			decompressor, decErr := codec.DefaultRegistry.Decompressor(method)
			if decErr == nil {
				if rc, err := decompressor.Decompress(payloadReader); err == nil {
					decReader = rc
				}
			}

			if err := zw.WriteEntry(repairedFile, decReader); err == nil {
				summary.FilesRecovered++
				summary.BytesRecovered += uncompSize
				summary.Logs = append(
					summary.Logs,
					fmt.Sprintf("Salvaged entry %q (Method: %s, Size: %d)", name, method, uncompSize),
				)
				offset = payloadOffset + compSize
				continue
			}
		} else if isDir {
			if err := zw.WriteEntry(repairedFile, bytes.NewReader(nil)); err == nil {
				summary.FilesRecovered++
				summary.Logs = append(summary.Logs, fmt.Sprintf("Salvaged directory %q", name))
			}
			offset = payloadOffset
			continue
		}

		offset = hdrOffset + 4
	}

	if err := zw.Close(); err != nil {
		return summary, fmt.Errorf("doctor finalize: %w", err)
	}

	return summary, nil
}

func sanitizeZipPath(name string) string {
	clean := vfs.CleanPath(name)
	for strings.HasPrefix(clean, "../") {
		clean = strings.TrimPrefix(clean, "../")
	}
	clean = strings.TrimPrefix(clean, "/")
	clean = strings.TrimPrefix(clean, "\\")
	if clean == "" || clean == "." {
		clean = "sanitized_file.dat"
	}
	return clean
}

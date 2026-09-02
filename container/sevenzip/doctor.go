// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/lemon4ksan/seal/file"
)

// RepairOptions configures the 7z archive recovery and repair engine.
type RepairOptions struct {
	SanitizePaths bool
	Password      string
}

// RepairSummary reports the quantitative results of the recovery operation.
type RepairSummary struct {
	FilesRecovered     int   `json:"files_recovered"`
	FilesSkipped       int   `json:"files_skipped"`
	BytesRecovered     int64 `json:"bytes_recovered"`
	PathsSanitized     int   `json:"paths_sanitized"`
	SignatureRealigned bool  `json:"signature_realigned"`
}

// Repair analyzes, salvages, and sanitizes a damaged or shifted 7z archive,
// writing a healthy, valid 7-Zip container into the destination writer.
func Repair(rAt io.ReaderAt, size int64, w io.Writer, opts RepairOptions) (*RepairSummary, error) {
	if rAt == nil || size < StartHeaderSize {
		return nil, errors.New("sevenzip doctor: archive size too small or nil reader")
	}

	summary := &RepairSummary{}

	// 1. Check for signature alignment or SFX prepended bytes
	sigOffset, err := findSignature(rAt, size)
	if err != nil {
		return nil, fmt.Errorf("sevenzip doctor: cannot find 7z signature: %w", err)
	}

	readerAt := rAt
	adjustedSize := size
	if sigOffset > 0 {
		summary.SignatureRealigned = true
		readerAt = io.NewSectionReader(rAt, sigOffset, size-sigOffset)
		adjustedSize = size - sigOffset
	}

	// 2. Open archive via Reader
	zr, err := NewReader(readerAt, adjustedSize, opts.Password)
	if err != nil {
		return nil, fmt.Errorf("sevenzip doctor: header unrecoverable: %w", err)
	}

	var recoveredFiles []*file.File

	for _, entry := range zr.Entries() {
		origName := entry.Name()
		cleanName := origName

		if opts.SanitizePaths {
			cleanName = sanitize7zPath(origName)
			if cleanName != origName {
				summary.PathsSanitized++
			}
		}

		if entry.IsDir() {
			dirF, err := file.New(cleanName, true)
			if err != nil {
				summary.FilesSkipped++
				continue
			}
			dirF.WithModTime(entry.ModTime())
			recoveredFiles = append(recoveredFiles, dirF)
			summary.FilesRecovered++
			continue
		}

		rc, err := entry.Open()
		if err != nil {
			summary.FilesSkipped++
			continue
		}

		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			summary.FilesSkipped++
			continue
		}

		newF, err := file.New(cleanName, false)
		if err != nil {
			summary.FilesSkipped++
			continue
		}
		newF.WithData(data)
		newF.WithModTime(entry.ModTime())

		recoveredFiles = append(recoveredFiles, newF)
		summary.FilesRecovered++
		summary.BytesRecovered += int64(len(data))
	}

	// 3. Serialize healthy recovered archive
	zw := NewWriterWithOptions(w, WriterOptions{
		Password:       opts.Password,
		EncryptHeaders: opts.Password != "",
	})

	if _, err := zw.WriteArchive(recoveredFiles); err != nil {
		return summary, fmt.Errorf("sevenzip doctor: failed serializing repaired archive: %w", err)
	}

	return summary, nil
}

func sanitize7zPath(name string) string {
	normalized := filepath.ToSlash(name)
	normalized = strings.TrimPrefix(normalized, "/")
	normalized = strings.TrimPrefix(normalized, "\\")

	parts := strings.Split(normalized, "/")
	var cleanParts []string

	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			continue
		}
		cleanParts = append(cleanParts, p)
	}

	if len(cleanParts) == 0 {
		return "salvaged_entry"
	}

	res := strings.Join(cleanParts, "/")
	if strings.HasSuffix(name, "/") && !strings.HasSuffix(res, "/") {
		res += "/"
	}
	return res
}

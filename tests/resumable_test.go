// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
)

func TestResumableArchiving_Zip_Lifecycle(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	sourceDir := filepath.Join(testDir, "source")
	_ = os.MkdirAll(sourceDir, 0o755)

	numFiles := 5
	for i := 1; i <= numFiles; i++ {
		fname := fmt.Sprintf("data_%d.txt", i)
		content := fmt.Sprintf("Payload content for file index %d in resumable test", i)
		_ = os.WriteFile(filepath.Join(sourceDir, fname), []byte(content), 0o644)
	}

	archivePath := filepath.Join(testDir, "interrupted.zip")

	// Phase 1: Start archiving with a context canceled during processing
	ctx, cancel := context.WithCancel(t.Context())

	archiver := seal.NewArchiver()

	// Cancel context when second file is being processed
	fileDoneCount := 0
	cancelOpt := seal.WithOnFileDone(func(_ string, _ error) {
		fileDoneCount++
		if fileDoneCount >= 2 {
			cancel()
		}
	})

	err := archiver.ArchiveDir(
		ctx,
		sourceDir,
		seal.ToFilePath(archivePath),
		seal.WithContainer(container.Zip),
		seal.WithResumable(true),
		seal.WithSessionID("test-sess-zip"),
		cancelOpt,
	)

	if !errors.Is(err, seal.ErrArchiveFrozen) {
		t.Fatalf("Expected ErrArchiveFrozen, got: %v", err)
	}

	// Phase 2: Verify the archive on disk is a 100% VALID zip archive
	st, canResume, err := seal.CanResume(t.Context(), archivePath)
	if err != nil {
		t.Fatalf("CanResume failed: %v", err)
	}
	if !canResume || st == nil {
		t.Fatalf("Expected canResume=true, got false")
	}
	if st.DoneFiles < 2 {
		t.Fatalf("Expected at least 2 done files, got %d", st.DoneFiles)
	}
	if len(st.RemainingFiles) == 0 {
		t.Fatalf("Expected remaining files, got 0")
	}

	// Verify partial archive is readable and lists files
	listEntries, err := seal.List(t.Context(), seal.FromFilePath(archivePath))
	if err != nil {
		t.Fatalf("seal.List on frozen archive failed: %v", err)
	}
	if len(listEntries) != st.DoneFiles {
		t.Fatalf("List count mismatch: got %d, want %d", len(listEntries), st.DoneFiles)
	}

	// Phase 3: Resume archive creation to 100% completion
	resumeErr := seal.ResumeArchive(t.Context(), archivePath)
	if resumeErr != nil {
		t.Fatalf("ResumeArchive failed: %v", resumeErr)
	}

	// Phase 4: Verify completed archive
	_, canResumeAfter, err := seal.CanResume(t.Context(), archivePath)
	if err != nil {
		t.Fatalf("CanResume after completion failed: %v", err)
	}
	if canResumeAfter {
		t.Fatalf("Expected checkpoint to be removed after 100%% completion, but still present")
	}

	finalEntries, err := seal.List(t.Context(), seal.FromFilePath(archivePath))
	if err != nil {
		t.Fatalf("Final seal.List failed: %v", err)
	}
	if len(finalEntries) != numFiles {
		t.Fatalf("Expected %d total files, got %d", numFiles, len(finalEntries))
	}

	// Verify extraction of completed zip
	extractZipDir := filepath.Join(testDir, "extracted_zip")
	if err := seal.Unarchive(t.Context(), seal.FromFilePath(archivePath), extractZipDir); err != nil {
		t.Fatalf("seal.Unarchive on completed zip failed: %v", err)
	}
	for i := 1; i <= numFiles; i++ {
		fname := fmt.Sprintf("data_%d.txt", i)
		expected := fmt.Sprintf("Payload content for file index %d in resumable test", i)
		actual, err := os.ReadFile(filepath.Join(extractZipDir, fname))
		if err != nil {
			t.Fatalf("Failed to read extracted zip file %s: %v", fname, err)
		}
		if string(actual) != expected {
			t.Fatalf("Content mismatch for %s: got %q, want %q", fname, string(actual), expected)
		}
	}
}

func TestResumableArchiving_7z_Lifecycle(t *testing.T) {
	t.Parallel()
	testDir := t.TempDir()
	sourceDir := filepath.Join(testDir, "source_7z")
	_ = os.MkdirAll(sourceDir, 0o755)

	numFiles := 5
	for i := 1; i <= numFiles; i++ {
		fname := fmt.Sprintf("binary_%d.dat", i)
		content := fmt.Sprintf("High compression content payload for 7z test %d", i)
		_ = os.WriteFile(filepath.Join(sourceDir, fname), []byte(content), 0o644)
	}

	archivePath := filepath.Join(testDir, "interrupted.7z")

	// Phase 1: Start archiving with a context canceled after first file
	ctx, cancel := context.WithCancel(t.Context())

	archiver := seal.NewArchiver()

	fileCount := 0
	cancelOpt := seal.WithOnFileDone(func(_ string, _ error) {
		fileCount++
		if fileCount >= 2 {
			cancel()
		}
	})

	err := archiver.ArchiveDir(
		ctx,
		sourceDir,
		seal.ToFilePath(archivePath),
		seal.WithContainer(container.SevenZip),
		seal.WithResumable(true),
		seal.WithSessionID("test-sess-7z"),
		cancelOpt,
	)

	if !errors.Is(err, seal.ErrArchiveFrozen) {
		t.Fatalf("Expected ErrArchiveFrozen, got: %v", err)
	}

	// Phase 2: Verify stealth checkpoint in 7z
	st, canResume, err := seal.CanResume(t.Context(), archivePath)
	if err != nil {
		t.Fatalf("CanResume on 7z failed: %v", err)
	}
	if !canResume || st == nil {
		t.Fatalf("Expected canResume=true on 7z, got false")
	}

	// Read entries from frozen archive
	listEntries, err := seal.List(t.Context(), seal.FromFilePath(archivePath))
	if err != nil {
		t.Fatalf("seal.List on frozen 7z archive failed: %v", err)
	}
	if len(listEntries) == 0 {
		t.Fatalf("Expected partial entries in frozen 7z, got 0")
	}

	// Phase 3: Resume 7z archive to completion
	resumeErr := seal.ResumeArchive(t.Context(), archivePath)
	if resumeErr != nil {
		t.Fatalf("ResumeArchive on 7z failed: %v", resumeErr)
	}

	// Phase 4: Verify 100% completed 7z archive
	finalEntries, err := seal.List(t.Context(), seal.FromFilePath(archivePath))
	if err != nil {
		t.Fatalf("Final 7z seal.List failed: %v", err)
	}
	if len(finalEntries) != numFiles {
		t.Fatalf("Expected %d total files in 7z, got %d", numFiles, len(finalEntries))
	}

	// Extract all files to ensure complete integrity
	extractDir := filepath.Join(testDir, "extracted_7z")
	if err := seal.Unarchive(t.Context(), seal.FromFilePath(archivePath), extractDir); err != nil {
		t.Fatalf("seal.Unarchive on completed 7z failed: %v", err)
	}

	for i := 1; i <= numFiles; i++ {
		fname := fmt.Sprintf("binary_%d.dat", i)
		expected := fmt.Sprintf("High compression content payload for 7z test %d", i)
		actual, err := os.ReadFile(filepath.Join(extractDir, fname))
		if err != nil {
			t.Fatalf("Failed to read extracted file %s: %v", fname, err)
		}
		if string(actual) != expected {
			t.Fatalf("Content mismatch for %s: got %q, want %q", fname, string(actual), expected)
		}
	}
}

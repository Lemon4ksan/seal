// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

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
	require.ErrorIs(t, err, seal.ErrArchiveFrozen)

	// Phase 2: Verify the archive on disk is a 100% VALID zip archive
	st, canResume, err := seal.CanResume(t.Context(), archivePath)
	require.NoError(t, err)
	require.True(t, canResume)
	require.NotNil(t, st)
	assert.GreaterOrEqual(t, st.DoneFiles, 2)
	assert.NotEmpty(t, st.RemainingFiles)

	// Verify partial archive is readable and lists files
	listEntries, err := seal.List(t.Context(), seal.FromFilePath(archivePath))
	require.NoError(t, err)
	assert.Equal(t, st.DoneFiles, len(listEntries))

	// Phase 3: Resume archive creation to 100% completion
	resumeErr := seal.ResumeArchive(t.Context(), archivePath)
	require.NoError(t, resumeErr)

	// Phase 4: Verify completed archive
	_, canResumeAfter, err := seal.CanResume(t.Context(), archivePath)
	require.NoError(t, err)
	assert.False(t, canResumeAfter, "Expected checkpoint to be removed after 100% completion, but still present")

	finalEntries, err := seal.List(t.Context(), seal.FromFilePath(archivePath))
	require.NoError(t, err)
	assert.Equal(t, numFiles, len(finalEntries))

	// Verify extraction of completed zip
	extractZipDir := filepath.Join(testDir, "extracted_zip")
	err = seal.Unarchive(t.Context(), seal.FromFilePath(archivePath), extractZipDir)
	require.NoError(t, err)
	for i := 1; i <= numFiles; i++ {
		fname := fmt.Sprintf("data_%d.txt", i)
		expected := fmt.Sprintf("Payload content for file index %d in resumable test", i)
		actual, err := os.ReadFile(filepath.Join(extractZipDir, fname))
		require.NoError(t, err)
		assert.Equal(t, expected, string(actual))
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
	require.ErrorIs(t, err, seal.ErrArchiveFrozen)

	// Phase 2: Verify stealth checkpoint in 7z
	st, canResume, err := seal.CanResume(t.Context(), archivePath)
	require.NoError(t, err)
	require.True(t, canResume)
	require.NotNil(t, st)

	// Read entries from frozen archive
	listEntries, err := seal.List(t.Context(), seal.FromFilePath(archivePath))
	require.NoError(t, err)
	assert.NotEmpty(t, listEntries)

	// Phase 3: Resume 7z archive to completion
	resumeErr := seal.ResumeArchive(t.Context(), archivePath)
	require.NoError(t, resumeErr)

	// Phase 4: Verify 100% completed 7z archive
	finalEntries, err := seal.List(t.Context(), seal.FromFilePath(archivePath))
	require.NoError(t, err)
	assert.Equal(t, numFiles, len(finalEntries))

	// Extract all files to ensure complete integrity
	extractDir := filepath.Join(testDir, "extracted_7z")
	err = seal.Unarchive(t.Context(), seal.FromFilePath(archivePath), extractDir)
	require.NoError(t, err)

	for i := 1; i <= numFiles; i++ {
		fname := fmt.Sprintf("binary_%d.dat", i)
		expected := fmt.Sprintf("High compression content payload for 7z test %d", i)
		actual, err := os.ReadFile(filepath.Join(extractDir, fname))
		require.NoError(t, err)
		assert.Equal(t, expected, string(actual))
	}
}

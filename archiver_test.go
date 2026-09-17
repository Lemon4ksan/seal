// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/option"
)

func TestArchiver_ArchiveDirAndUnarchive_ZIP(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	destZip := filepath.Join(tempDir, "bundle.zip")
	outDir := filepath.Join(tempDir, "extracted")

	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("Hello Seal!"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "nested", "doc.md"),
		[]byte("# Seal Documentation"),
		0o644,
	))

	// Create initial archive
	err := seal.ArchiveDir(ctx, srcDir, seal.ToFilePath(destZip))
	require.NoError(t, err)

	// Verify catalog metadata
	count, err := seal.FileCount(ctx, seal.FromFilePath(destZip))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, 2)

	exists, err := seal.ExistsIn(ctx, seal.FromFilePath(destZip), "hello.txt")
	require.NoError(t, err)
	assert.True(t, exists)

	// Stream payload without unpacking container
	data, err := seal.ReadFile(ctx, seal.FromFilePath(destZip), "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "Hello Seal!", string(data))

	// Extract individual file entry
	extractedSingle := filepath.Join(tempDir, "single_hello.txt")
	err = seal.ExtractFile(ctx, seal.FromFilePath(destZip), "hello.txt", extractedSingle)
	require.NoError(t, err)
	singleData, err := os.ReadFile(extractedSingle)
	require.NoError(t, err)
	assert.Equal(t, "Hello Seal!", string(singleData))

	// Extract complete filesystem tree
	err = seal.Unarchive(ctx, seal.FromFilePath(destZip), outDir)
	require.NoError(t, err)

	extractedDoc, err := os.ReadFile(filepath.Join(outDir, "nested", "doc.md"))
	require.NoError(t, err)
	assert.Equal(t, "# Seal Documentation", string(extractedDoc))

	// Validate container checksums and structural integrity
	err = seal.Verify(ctx, seal.FromFilePath(destZip))
	require.NoError(t, err)

	// Generate ASCII directory visualization
	tree, err := seal.Tree(ctx, seal.FromFilePath(destZip))
	require.NoError(t, err)
	assert.NotEmpty(t, tree)
}

func TestArchiver_ArchiveDirAndUnarchive_TAR(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src_tar")
	destTar := filepath.Join(tempDir, "bundle.tar")
	outDir := filepath.Join(tempDir, "extracted_tar")

	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "data.bin"), []byte("Tar payload 12345"), 0o644))

	archiver := seal.NewArchiver(option.WithContainer(container.Tar))
	err := archiver.ArchiveDir(ctx, srcDir, seal.ToFilePath(destTar))
	require.NoError(t, err)

	data, err := seal.ReadFile(ctx, seal.FromFilePath(destTar), "data.bin")
	require.NoError(t, err)
	assert.Equal(t, "Tar payload 12345", string(data))

	err = seal.Unarchive(ctx, seal.FromFilePath(destTar), outDir)
	require.NoError(t, err)

	outData, err := os.ReadFile(filepath.Join(outDir, "data.bin"))
	require.NoError(t, err)
	assert.Equal(t, "Tar payload 12345", string(outData))
}

func TestArchiver_ArchiveDirAndUnarchive_TarGz(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src_tgz")
	destTgz := filepath.Join(tempDir, "bundle.tar.gz")
	outDir := filepath.Join(tempDir, "extracted_tgz")

	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "config.yaml"),
		[]byte("env: production\nport: 8080"),
		0o644,
	))

	archiver := seal.NewArchiver(option.WithContainer(container.TarGz))
	err := archiver.ArchiveDir(ctx, srcDir, seal.ToFilePath(destTgz))
	require.NoError(t, err)

	data, err := seal.ReadFile(ctx, seal.FromFilePath(destTgz), "config.yaml")
	require.NoError(t, err)
	assert.Equal(t, "env: production\nport: 8080", string(data))

	err = seal.Unarchive(ctx, seal.FromFilePath(destTgz), outDir)
	require.NoError(t, err)

	outData, err := os.ReadFile(filepath.Join(outDir, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "env: production\nport: 8080", string(outData))
}

func TestArchiver_ArchiveDirAndUnarchive_SevenZip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src_7z")
	dest7z := filepath.Join(tempDir, "bundle.7z")
	outDir := filepath.Join(tempDir, "extracted_7z")

	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("Hello 7-Zip Universal!"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(srcDir, "nested", "binary.dat"),
		[]byte("7z Substream payload"),
		0o644,
	))

	archiver := seal.NewArchiver(option.WithContainer(container.SevenZip))
	err := archiver.ArchiveDir(ctx, srcDir, seal.ToFilePath(dest7z))
	require.NoError(t, err)

	// Verify direct file payload extraction using container format auto-detection
	data, err := seal.ReadFile(ctx, seal.FromFilePath(dest7z), "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "Hello 7-Zip Universal!", string(data))

	// Verify recursive extraction of entire 7z directory hierarchy
	err = seal.Unarchive(ctx, seal.FromFilePath(dest7z), outDir)
	require.NoError(t, err)

	outHello, err := os.ReadFile(filepath.Join(outDir, "hello.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello 7-Zip Universal!", string(outHello))

	outBinary, err := os.ReadFile(filepath.Join(outDir, "nested", "binary.dat"))
	require.NoError(t, err)
	assert.Equal(t, "7z Substream payload", string(outBinary))
}

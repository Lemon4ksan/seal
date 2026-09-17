// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	_ "github.com/lemon4ksan/seal/container/iso"
)

func TestISO_EndToEnd_Integration(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(srcDir, "boot", "isolinux"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "boot", "isolinux", "isolinux.bin"), []byte("BOOTLOADER BINARY DATA"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "autorun.inf"), []byte("[autorun]\nopen=setup.exe\n"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "setup.exe"), []byte("MZ EXECUTABLE STUB"), 0o755)

	outDir := t.TempDir()
	isoPath := filepath.Join(outDir, "live_system.iso")

	// 1. ArchiveDir with container.Iso
	err := seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(isoPath),
		seal.WithContainer(container.Iso),
	)
	require.NoError(t, err)

	// 2. Inspect with seal.List
	entries, err := seal.List(t.Context(), seal.FromFilePath(isoPath))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 3)

	// 3. Check file presence with seal.ExistsIn
	exists, err := seal.ExistsIn(t.Context(), seal.FromFilePath(isoPath), "autorun.inf")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = seal.ExistsIn(t.Context(), seal.FromFilePath(isoPath), "boot/isolinux/isolinux.bin")
	require.NoError(t, err)
	assert.True(t, exists)

	// 4. Test tree rendering
	tree, err := seal.Tree(t.Context(), seal.FromFilePath(isoPath))
	require.NoError(t, err)
	assert.NotEmpty(t, tree)

	// 5. Extract with seal.Unarchive
	extractDir := t.TempDir()
	err = seal.Unarchive(t.Context(), seal.FromFilePath(isoPath), extractDir)
	require.NoError(t, err)

	infData, err := os.ReadFile(filepath.Join(extractDir, "autorun.inf"))
	require.NoError(t, err)
	assert.Equal(t, "[autorun]\nopen=setup.exe\n", string(infData))

	binData, err := os.ReadFile(filepath.Join(extractDir, "boot", "isolinux", "isolinux.bin"))
	require.NoError(t, err)
	assert.Equal(t, "BOOTLOADER BINARY DATA", string(binData))
}

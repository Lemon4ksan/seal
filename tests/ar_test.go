// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	_ "github.com/lemon4ksan/seal/container/ar"
)

func TestAR_EndToEnd_Integration(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(srcDir, "debian-binary"), []byte("2.0\n"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "control.tar.gz"), []byte("MOCK_CONTROL_TAR_GZ"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "data.tar.xz"), []byte("MOCK_DATA_TAR_XZ"), 0o644)

	outDir := t.TempDir()
	debPath := filepath.Join(outDir, "package_1.0.0_amd64.deb")

	// 1. ArchiveDir with container.Ar
	err := seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(debPath),
		seal.WithContainer(container.Ar),
	)
	require.NoError(t, err)

	// 2. Inspect with seal.List
	entries, err := seal.List(t.Context(), seal.FromFilePath(debPath))
	require.NoError(t, err)
	require.Len(t, entries, 3)

	// 3. Check file presence with seal.ExistsIn
	exists, err := seal.ExistsIn(t.Context(), seal.FromFilePath(debPath), "debian-binary")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = seal.ExistsIn(t.Context(), seal.FromFilePath(debPath), "data.tar.xz")
	require.NoError(t, err)
	assert.True(t, exists)

	// 4. Test tree rendering
	tree, err := seal.Tree(t.Context(), seal.FromFilePath(debPath))
	require.NoError(t, err)
	assert.NotEmpty(t, tree)

	// 5. Extract with seal.Unarchive
	extractDir := t.TempDir()
	err = seal.Unarchive(t.Context(), seal.FromFilePath(debPath), extractDir)
	require.NoError(t, err)

	binData, err := os.ReadFile(filepath.Join(extractDir, "debian-binary"))
	require.NoError(t, err)
	assert.Equal(t, "2.0\n", string(binData))

	ctrlData, err := os.ReadFile(filepath.Join(extractDir, "control.tar.gz"))
	require.NoError(t, err)
	assert.Equal(t, "MOCK_CONTROL_TAR_GZ", string(ctrlData))

	dataData, err := os.ReadFile(filepath.Join(extractDir, "data.tar.xz"))
	require.NoError(t, err)
	assert.Equal(t, "MOCK_DATA_TAR_XZ", string(dataData))
}

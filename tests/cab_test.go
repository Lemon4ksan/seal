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
	_ "github.com/lemon4ksan/seal/container/cab"
)

func TestCAB_EndToEnd_Integration(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(srcDir, "drivers", "net"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "drivers", "net", "adapter.sys"), []byte("SYS DRIVER BINARY CONTENT"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "setup.inf"), []byte("[Version]\nSignature=\"$Windows NT$\"\n"), 0o644)

	outDir := t.TempDir()
	cabPath := filepath.Join(outDir, "driver_package.cab")

	// 1. ArchiveDir with container.Cab
	err := seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(cabPath),
		seal.WithContainer(container.Cab),
	)
	require.NoError(t, err)

	// 2. Inspect with seal.List
	entries, err := seal.List(t.Context(), seal.FromFilePath(cabPath))
	require.NoError(t, err)
	require.Len(t, entries, 4)

	// 3. Check file presence with seal.ExistsIn
	exists, err := seal.ExistsIn(t.Context(), seal.FromFilePath(cabPath), "setup.inf")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = seal.ExistsIn(t.Context(), seal.FromFilePath(cabPath), "drivers/net/adapter.sys")
	require.NoError(t, err)
	assert.True(t, exists)

	// 4. Test tree rendering
	tree, err := seal.Tree(t.Context(), seal.FromFilePath(cabPath))
	require.NoError(t, err)
	assert.NotEmpty(t, tree)

	// 5. Extract with seal.Unarchive
	extractDir := t.TempDir()
	err = seal.Unarchive(t.Context(), seal.FromFilePath(cabPath), extractDir)
	require.NoError(t, err)

	infData, err := os.ReadFile(filepath.Join(extractDir, "setup.inf"))
	require.NoError(t, err)
	assert.Equal(t, "[Version]\nSignature=\"$Windows NT$\"\n", string(infData))

	sysData, err := os.ReadFile(filepath.Join(extractDir, "drivers", "net", "adapter.sys"))
	require.NoError(t, err)
	assert.Equal(t, "SYS DRIVER BINARY CONTENT", string(sysData))
}

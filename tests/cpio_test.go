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
	_ "github.com/lemon4ksan/seal/container/cpio"
)

func TestCPIO_EndToEnd_Integration(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(srcDir, "bin"), 0o755)
	_ = os.MkdirAll(filepath.Join(srcDir, "lib"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "init"), []byte("#!/bin/sh\nexec /bin/busybox\n"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "bin", "busybox"), []byte("BUSYBOX MULTI-CALL BINARY"), 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "lib", "ld-linux.so.2"), []byte("ELF DYNAMIC LINKER"), 0o755)

	outDir := t.TempDir()
	cpioPath := filepath.Join(outDir, "initramfs.cpio")

	// 1. ArchiveDir with container.Cpio
	err := seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(cpioPath),
		seal.WithContainer(container.Cpio),
	)
	require.NoError(t, err)

	// 2. Inspect with seal.List
	entries, err := seal.List(t.Context(), seal.FromFilePath(cpioPath))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 3)

	// 3. Check file presence with seal.ExistsIn
	exists, err := seal.ExistsIn(t.Context(), seal.FromFilePath(cpioPath), "init")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = seal.ExistsIn(t.Context(), seal.FromFilePath(cpioPath), "bin/busybox")
	require.NoError(t, err)
	assert.True(t, exists)

	// 4. Test tree rendering
	tree, err := seal.Tree(t.Context(), seal.FromFilePath(cpioPath))
	require.NoError(t, err)
	assert.NotEmpty(t, tree)

	// 5. Extract with seal.Unarchive
	extractDir := t.TempDir()
	err = seal.Unarchive(t.Context(), seal.FromFilePath(cpioPath), extractDir)
	require.NoError(t, err)

	initData, err := os.ReadFile(filepath.Join(extractDir, "init"))
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\nexec /bin/busybox\n", string(initData))

	boxData, err := os.ReadFile(filepath.Join(extractDir, "bin", "busybox"))
	require.NoError(t, err)
	assert.Equal(t, "BUSYBOX MULTI-CALL BINARY", string(boxData))

	ldData, err := os.ReadFile(filepath.Join(extractDir, "lib", "ld-linux.so.2"))
	require.NoError(t, err)
	assert.Equal(t, "ELF DYNAMIC LINKER", string(ldData))
}

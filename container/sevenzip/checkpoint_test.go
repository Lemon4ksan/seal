// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container/sevenzip"
	"github.com/lemon4ksan/seal/file"
)

func Test7z_StealthCheckpoint_Roundtrip(t *testing.T) {
	t.Parallel()
	checkpointPayload := []byte(`{"magic":"SEAL_CHECKPOINT_V1","session_id":"sess-999","done_files":1,"total_files":2}`)

	f1, err := file.New("hello.txt", false)
	require.NoError(t, err)
	f1.WithData([]byte("Hello Resumable 7-Zip World!"))

	var buf bytes.Buffer
	w := sevenzip.NewWriterWithOptions(&buf, sevenzip.WriterOptions{
		Method:         sevenzip.MethodLZMA2Default,
		CheckpointData: checkpointPayload,
	})

	n, err := w.WriteArchive([]*file.File{f1})
	require.NoError(t, err)
	assert.Greater(t, n, int64(0))

	archiveBytes := buf.Bytes()

	// 1. Read using Seal's sevenzip.Reader
	rAt := bytes.NewReader(archiveBytes)
	zr, err := sevenzip.NewReader(rAt, int64(len(archiveBytes)), "")
	require.NoError(t, err)

	cpData, ok := zr.Checkpoint()
	require.True(t, ok)
	assert.Equal(t, checkpointPayload, cpData)

	// 2. Verify file extraction
	entries := zr.Entries()
	require.Len(t, entries, 1)
	rc, err := entries[0].Open()
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()

	extracted, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "Hello Resumable 7-Zip World!", string(extracted))

	// 3. Cross-validate with official 7z.exe if installed
	sevenZipBin, err := exec.LookPath("7z")
	if err == nil {
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, "test_checkpoint.7z")
		err := os.WriteFile(archivePath, archiveBytes, 0o644)
		require.NoError(t, err)

		cmd := exec.CommandContext(t.Context(), sevenZipBin, "t", archivePath)
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "Official 7z.exe failed to test archive:\nError: %v\nOutput: %s", err, string(output))
		t.Logf("Official 7z.exe verified stealth checkpoint archive successfully:\n%s", string(output))
	}
}

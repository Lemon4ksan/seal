// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	archivezip "archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/file"
)

func TestZip_StealthCheckpoint_Roundtrip(t *testing.T) {
	t.Parallel()
	checkpointPayload := []byte(
		`{"magic":"SEAL_CHECKPOINT_V1","session_id":"zip-sess-777","done_files":1,"total_files":2}`,
	)

	f1, err := file.New("notes.txt", false)
	require.NoError(t, err)
	f1.WithData([]byte("Hello Resumable ZIP World!"))

	eng := zip.New()

	var buf bytes.Buffer
	cfg := container.Config{
		CheckpointData: checkpointPayload,
	}

	n, err := eng.WriteArchive(&buf, []*file.File{f1}, cfg)
	require.NoError(t, err)
	assert.Greater(t, n, int64(0))

	archiveBytes := buf.Bytes()

	// 1. Verify standard Go archive/zip reader considers it 100% valid
	stdZipReader, err := archivezip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	require.NoError(t, err)
	require.Len(t, stdZipReader.File, 1)

	stdFile, err := stdZipReader.File[0].Open()
	require.NoError(t, err)
	stdContent, err := io.ReadAll(stdFile)
	_ = stdFile.Close()
	require.NoError(t, err)
	assert.Equal(t, "Hello Resumable ZIP World!", string(stdContent))

	// 2. Read using Seal's zip.Reader
	rAt := bytes.NewReader(archiveBytes)
	zr, err := zip.NewReader(rAt, int64(len(archiveBytes)), "", nil)
	require.NoError(t, err)

	cpData, ok := zr.Checkpoint()
	require.True(t, ok)
	assert.Equal(t, checkpointPayload, cpData)
}

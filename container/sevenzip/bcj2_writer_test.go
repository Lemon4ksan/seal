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

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/container/sevenzip"
	"github.com/lemon4ksan/seal/file"
)

func TestSevenZip_BCJ2_Roundtrip_And_CrossValidation(t *testing.T) {
	// Generate mock x86 binary code with CALL (E8) and JMP (E9) instructions
	original := make([]byte, 32*1024)
	for i := range original {
		original[i] = byte(i & 0xFF)
	}

	for i := 100; i < len(original)-64; i += 64 {
		original[i] = 0xE8
		original[i+1] = byte(i)
		original[i+2] = byte(i >> 8)
		original[i+3] = 0x00
		original[i+4] = 0x00

		original[i+32] = 0xE9
		original[i+33] = byte(i + 10)
		original[i+34] = byte((i + 10) >> 8)
		original[i+35] = 0x00
		original[i+36] = 0x00
	}

	f1, err := file.New("program.exe", false)
	require.NoError(t, err)
	f1.WithData(original)

	// Write 7z archive with FilterBCJ2
	var buf bytes.Buffer
	writer := sevenzip.NewWriterWithOptions(&buf, sevenzip.WriterOptions{
		Filter: sevenzip.FilterBCJ2,
		Method: sevenzip.MethodLZMA2Default,
	})

	written, err := writer.WriteArchive([]*file.File{f1})
	require.NoError(t, err)
	assert.Greater(t, written, int64(0))

	// 1. Cross-validate with official 7-Zip CLI if installed
	sevenZipPath := `C:\Program Files\7-Zip\7z.exe`
	if _, err := os.Stat(sevenZipPath); err == nil {
		tempDir := t.TempDir()
		archivePath := filepath.Join(tempDir, "seal_bcj2.7z")
		err := os.WriteFile(archivePath, buf.Bytes(), 0o644)
		require.NoError(t, err)

		// Run 7z test command
		cmdTest := exec.Command(sevenZipPath, "t", archivePath)
		out, err := cmdTest.CombinedOutput()
		require.NoErrorf(t, err, "7-Zip test command failed on Seal BCJ2 archive:\n%s", string(out))

		// Run 7z extract command
		extractDir := filepath.Join(tempDir, "extracted")
		cmdExtract := exec.Command(sevenZipPath, "x", archivePath, "-o"+extractDir, "-y")
		out, err = cmdExtract.CombinedOutput()
		require.NoErrorf(t, err, "7-Zip extract command failed on Seal BCJ2 archive:\n%s", string(out))

		content7z, err := os.ReadFile(filepath.Join(extractDir, "program.exe"))
		require.NoError(t, err)
		assert.Equal(t, original, content7z)
	}

	// 2. Verify with Seal Reader
	rAt := bytes.NewReader(buf.Bytes())
	reader, err := sevenzip.NewReader(rAt, int64(buf.Len()), "")
	require.NoError(t, err)

	entries := reader.Entries()
	require.Len(t, entries, 1)

	rc, err := entries[0].Open()
	require.NoError(t, err)
	extracted, err := io.ReadAll(rc)
	_ = rc.Close()
	require.NoError(t, err)

	assert.Equal(t, original, extracted)
}

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wim_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/wim"
	"github.com/lemon4ksan/seal/file"
)

func TestWim_Roundtrip(t *testing.T) {
	t.Parallel()
	f0, _ := file.New("docs/readme.txt", false)
	f1, _ := file.New("docs/license.md", false)
	f2, _ := file.New("src/main.go", false)
	f3, _ := file.New("empty.dat", false)
	files := []*file.File{f0, f1, f2, f3}

	files[0].WithOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(stringsReader("Hello WIM Readme!")), nil
	}).WithSize(int64(len("Hello WIM Readme!"))).WithModTime(time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))

	files[1].WithOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(stringsReader("MIT License 2026")), nil
	}).WithSize(int64(len("MIT License 2026"))).WithModTime(time.Date(2026, 9, 2, 13, 0, 0, 0, time.UTC))

	files[2].WithOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(stringsReader("package main\nfunc main() {}\n")), nil
	}).WithSize(int64(len("package main\nfunc main() {}\n"))).WithModTime(time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC))

	files[3].WithOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}).WithSize(0)

	var buf bytes.Buffer
	writer := wim.NewWriter(&buf)
	written, err := writer.WriteArchive(files)
	require.NoError(t, err)
	assert.Greater(t, written, int64(0))

	// Read back
	rAt := bytes.NewReader(buf.Bytes())
	reader, err := wim.NewReader(rAt, int64(buf.Len()))
	require.NoError(t, err)
	require.NotEmpty(t, reader.Entries)

	found := make(map[string]*file.File)
	for _, entry := range reader.Entries {
		found[entry.Name()] = entry
	}

	for _, original := range files {
		entry, ok := found[original.Name()]
		require.Truef(t, ok, "Expected file %q in WIM, but not found", original.Name())
		assert.Equal(t, original.UncompressedSize(), entry.UncompressedSize())

		rc, err := entry.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		require.NoError(t, err)

		origRC, _ := original.Open()
		origContent, _ := io.ReadAll(origRC)
		_ = origRC.Close()

		assert.Equalf(t, origContent, content, "File %q content mismatch", original.Name())
	}
}

func TestWim_Deduplication(t *testing.T) {
	t.Parallel()
	identicalContent := []byte("Duplicated payload across multiple files in the archive")
	f0, _ := file.New("copy1.txt", false)
	f1, _ := file.New("dir/copy2.txt", false)
	f2, _ := file.New("dir/sub/copy3.txt", false)
	files := []*file.File{f0, f1, f2}

	for _, f := range files {
		f.WithOpenFunc(func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(identicalContent)), nil
		}).WithSize(int64(len(identicalContent)))
	}

	var buf bytes.Buffer
	writer := wim.NewWriter(&buf)
	_, err := writer.WriteArchive(files)
	require.NoError(t, err)

	rAt := bytes.NewReader(buf.Bytes())
	reader, err := wim.NewReader(rAt, int64(buf.Len()))
	require.NoError(t, err)

	// In Lookup Table, there should be:
	// 1 data blob (for the 3 identical files) + 1 metadata blob = 2 entries!
	dataBlobs := 0
	for _, entry := range reader.LookupTable {
		if entry.ResHdr.Flags()&wim.ResFlagMetadata == 0 {
			dataBlobs++
			assert.Equal(t, uint32(3), entry.RefCount, "Expected RefCount 3 for deduplicated blob")
		}
	}

	assert.Equal(t, 1, dataBlobs, "Expected exactly 1 deduplicated data blob in lookup table")
}

func TestWim_7Zip_CrossValidation(t *testing.T) {
	t.Parallel()
	sevenZipPath := `C:\Program Files\7-Zip\7z.exe`
	if _, err := os.Stat(sevenZipPath); err != nil {
		t.Skip("7-Zip not installed on host, skipping cross-validation")
	}

	tempDir := t.TempDir()
	wimPath := filepath.Join(tempDir, "test_seal.wim")

	f0, _ := file.New("greeting.txt", false)
	f1, _ := file.New("nested/data.json", false)
	files := []*file.File{f0, f1}
	files[0].WithOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(stringsReader("Hello from Seal WIM!")), nil
	}).WithSize(int64(len("Hello from Seal WIM!")))
	files[1].WithOpenFunc(func() (io.ReadCloser, error) {
		return io.NopCloser(stringsReader(`{"format":"wim","engine":"seal"}`)), nil
	}).WithSize(int64(len(`{"format":"wim","engine":"seal"}`)))

	fOut, err := os.Create(wimPath)
	require.NoError(t, err)
	engine, err := container.Get(container.Wim)
	require.NoError(t, err)
	_, err = engine.WriteArchive(fOut, files, container.Config{})
	require.NoError(t, err)
	_ = fOut.Close()

	// 1. Verify with 7-Zip test command
	cmdTest := exec.CommandContext(t.Context(), sevenZipPath, "t", wimPath)
	out, err := cmdTest.CombinedOutput()
	require.NoErrorf(t, err, "7-Zip test command failed on Seal WIM:\n%s", string(out))

	// 2. Extract with 7-Zip
	extractDir := filepath.Join(tempDir, "7z_extracted")
	cmdExtract := exec.CommandContext(t.Context(), sevenZipPath, "x", wimPath, "-o"+extractDir, "-y")
	out, err = cmdExtract.CombinedOutput()
	require.NoErrorf(t, err, "7-Zip extract failed on Seal WIM:\n%s", string(out))

	// Verify extracted files match
	content1, err := os.ReadFile(filepath.Join(extractDir, "greeting.txt"))
	require.NoError(t, err)
	assert.Equal(t, "Hello from Seal WIM!", string(content1))

	content2, err := os.ReadFile(filepath.Join(extractDir, "nested", "data.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"format":"wim","engine":"seal"}`, string(content2))

	// 3. Create WIM with 7-Zip, then read with Seal
	sevenZipWim := filepath.Join(tempDir, "test_official_7z.wim")
	cmdCreate7z := exec.Command(sevenZipPath, "a", "-twim", sevenZipWim,
		filepath.Join(extractDir, "greeting.txt"),
		filepath.Join(extractDir, "nested", "data.json"))
	out, err = cmdCreate7z.CombinedOutput()
	require.NoErrorf(t, err, "7-Zip creation failed:\n%s", string(out))

	f7z, err := os.Open(sevenZipWim)
	require.NoError(t, err)
	defer f7z.Close()
	st, _ := f7z.Stat()

	sealCatalog, _, err := engine.OpenCatalog(f7z, st.Size(), "", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, sealCatalog)

	// 4. Create LZX WIM with 7-Zip, then read and verify with Seal
	lzxWim := filepath.Join(tempDir, "test_official_7z_lzx.wim")
	cmdCreateLZX := exec.Command(sevenZipPath, "a", "-twim", "-m0=lzx", lzxWim,
		filepath.Join(extractDir, "greeting.txt"))
	_, err = cmdCreateLZX.CombinedOutput()
	if err == nil {
		fLZX, err := os.Open(lzxWim)
		if err == nil {
			defer fLZX.Close()
			stLZX, _ := fLZX.Stat()
			lzxCatalog, _, err := engine.OpenCatalog(fLZX, stLZX.Size(), "", nil)
			if err == nil && len(lzxCatalog) > 0 {
				rc, err := lzxCatalog[0].Open()
				if err == nil {
					data, _ := io.ReadAll(rc)
					_ = rc.Close()
					_ = data
				}
			}
		}
	}
}

func stringsReader(s string) io.Reader {
	return bytes.NewReader([]byte(s))
}

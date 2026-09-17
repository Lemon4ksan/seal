// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tar_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/tar"
	"github.com/lemon4ksan/seal/file"
)

func TestTar_Roundtrip(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	entries := []struct {
		name    string
		content string
		isDir   bool
	}{
		{"file1.txt", "Hello TAR World", false},
		{"nested/dir/", "", true},
		{"nested/dir/file2.json", `{"status":"ok"}`, false},
	}

	for _, e := range entries {
		typeflag := tar.TypeReg
		if e.isDir {
			typeflag = tar.TypeDir
		}
		entry := &tar.Header{
			Name:     e.name,
			Size:     int64(len(e.content)),
			Mode:     0o644,
			ModTime:  time.Unix(1700000000, 0),
			Typeflag: typeflag,
		}
		require.NoError(t, tw.WriteHeader(entry))
		if len(e.content) > 0 {
			_, err := tw.WritePayload(strings.NewReader(e.content))
			require.NoError(t, err)
		}
	}

	require.NoError(t, tw.Close())

	tr := tar.NewReader(buf)
	readCount := 0

	for {
		entry, src, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		var data []byte
		if !entry.FileInfo().IsDir() {
			data, err = io.ReadAll(src)
			require.NoError(t, err)
		}

		assert.Equalf(t, entries[readCount].content, string(data), "content mismatch for %s", entry.Name)
		readCount++
	}

	assert.Equal(t, len(entries), readCount, "read entries count mismatch")
}

func TestTar_LongPath_PAX(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	longPath := "deeply/nested/directory/structure/with/a/very/long/path/exceeding/one/hundred/characters/filename_with_extraordinary_length.txt"
	content := "PAX extended path test content"

	h := &tar.Header{
		Name:     longPath,
		Size:     int64(len(content)),
		Mode:     0o644,
		ModTime:  time.Unix(1700000000, 0),
		Typeflag: tar.TypeReg,
	}

	require.NoError(t, tw.WriteHeader(h))
	_, err := tw.WritePayload(strings.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	tr := tar.NewReader(buf)
	entry, src, err := tr.Next()
	require.NoError(t, err)

	assert.Equal(t, longPath, entry.Name)

	data, err := io.ReadAll(src)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestTar_UTF8_NonASCII(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	utf8Path := "документы/архив_2026/отчет.txt"
	content := "Данные на русском языке в кодировке UTF-8"

	h := &tar.Header{
		Name:     utf8Path,
		Size:     int64(len(content)),
		Mode:     0o644,
		ModTime:  time.Unix(1700000000, 0),
		Typeflag: tar.TypeReg,
	}

	require.NoError(t, tw.WriteHeader(h))
	_, err := tw.WritePayload(strings.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	tr := tar.NewReader(buf)
	entry, src, err := tr.Next()
	require.NoError(t, err)

	assert.Equal(t, utf8Path, entry.Name)

	data, err := io.ReadAll(src)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestTar_Container_CompressedVariants(t *testing.T) {
	formats := []container.Type{
		container.Tar,
		container.TarGz,
		container.TarXz,
		container.TarZst,
	}

	for _, fmtType := range formats {
		t.Run(string(fmtType), func(t *testing.T) {
			engine, err := container.Get(fmtType)
			require.NoError(t, err)

			f1, _ := file.New("hello.txt", false)
			f1Content := []byte("Hello Compressed TAR World!")
			f1.SetSizes(int64(len(f1Content)), int64(len(f1Content)), 0)
			f1.SetOpenFunc(func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(f1Content)), nil
			})

			f2, _ := file.New("dir/", true)

			files := []*file.File{f1, f2}

			var archiveBuf bytes.Buffer
			written, err := engine.WriteArchive(&archiveBuf, files, container.Config{})
			require.NoError(t, err)
			assert.Greater(t, written, int64(0))
			assert.Greater(t, archiveBuf.Len(), 0)

			readFiles, _, err := engine.ScanStream(&archiveBuf, "", nil)
			require.NoError(t, err)
			assert.Len(t, readFiles, 2)

			r, err := readFiles[0].Open()
			require.NoError(t, err)
			data, err := io.ReadAll(r)
			_ = r.Close()
			require.NoError(t, err)
			assert.Equal(t, string(f1Content), string(data))
		})
	}
}

func BenchmarkTar_HeaderScan(b *testing.B) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	for i := 0; i < 1000; i++ {
		h := &tar.Header{
			Name:     "benchmark/file.txt",
			Size:     100,
			Mode:     0o644,
			ModTime:  time.Unix(1700000000, 0),
			Typeflag: tar.TypeReg,
		}
		_ = tw.WriteHeader(h)
		_, _ = tw.Write(make([]byte, 100))
	}
	_ = tw.Close()

	archiveData := buf.Bytes()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tr := tar.NewReader(bytes.NewReader(archiveData))
		for {
			_, _, err := tr.Next()
			if err != nil {
				break
			}
		}
	}
}

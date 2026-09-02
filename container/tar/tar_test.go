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
		if err := tw.WriteHeader(entry); err != nil {
			t.Fatalf("WriteHeader failed: %v", err)
		}
		if len(e.content) > 0 {
			if _, err := tw.WritePayload(strings.NewReader(e.content)); err != nil {
				t.Fatalf("WritePayload failed: %v", err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	tr := tar.NewReader(buf)
	readCount := 0

	for {
		entry, src, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tr.Next failed: %v", err)
		}

		var data []byte
		if !entry.FileInfo().IsDir() {
			data, err = io.ReadAll(src)
			if err != nil {
				t.Fatalf("io.ReadAll failed: %v", err)
			}
		}

		if string(data) != entries[readCount].content {
			t.Errorf("content mismatch for %s: got %q, want %q", entry.Name, string(data), entries[readCount].content)
		}
		readCount++
	}

	if readCount != len(entries) {
		t.Errorf("read entries count mismatch: got %d, want %d", readCount, len(entries))
	}
}

func TestTar_LongPath_PAX(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	longPath := "deeply/nested/directory/structure/with/a/very/long/path/exceeding/one/hundred/characters/filename_with_extraordinary_length.txt"
	content := "PAX extended path test content"

	h := &tar.Header{
		Name:     longPath,
		Size:     int64(len(content)),
		Mode:     0644,
		ModTime:  time.Unix(1700000000, 0),
		Typeflag: tar.TypeReg,
	}

	if err := tw.WriteHeader(h); err != nil {
		t.Fatalf("WriteHeader long path failed: %v", err)
	}
	if _, err := tw.WritePayload(strings.NewReader(content)); err != nil {
		t.Fatalf("WritePayload failed: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	tr := tar.NewReader(buf)
	entry, src, err := tr.Next()
	if err != nil {
		t.Fatalf("tr.Next failed: %v", err)
	}

	if entry.Name != longPath {
		t.Errorf("long path mismatch: got %q, want %q", entry.Name, longPath)
	}

	data, err := io.ReadAll(src)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: got %q, want %q", string(data), content)
	}
}

func TestTar_UTF8_NonASCII(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	utf8Path := "документы/архив_2026/отчет.txt"
	content := "Данные на русском языке в кодировке UTF-8"

	h := &tar.Header{
		Name:     utf8Path,
		Size:     int64(len(content)),
		Mode:     0644,
		ModTime:  time.Unix(1700000000, 0),
		Typeflag: tar.TypeReg,
	}

	if err := tw.WriteHeader(h); err != nil {
		t.Fatalf("WriteHeader UTF-8 failed: %v", err)
	}
	if _, err := tw.WritePayload(strings.NewReader(content)); err != nil {
		t.Fatalf("WritePayload failed: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	tr := tar.NewReader(buf)
	entry, src, err := tr.Next()
	if err != nil {
		t.Fatalf("tr.Next failed: %v", err)
	}

	if entry.Name != utf8Path {
		t.Errorf("UTF-8 path mismatch: got %q, want %q", entry.Name, utf8Path)
	}

	data, err := io.ReadAll(src)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: got %q, want %q", string(data), content)
	}
}

func TestTar_Container_CompressedVariants(t *testing.T) {
	formats := []container.Type{
		container.Tar,
		container.TarGz,
		container.TarXz,
	}

	for _, fmtType := range formats {
		t.Run(string(fmtType), func(t *testing.T) {
			engine, err := container.Get(fmtType)
			if err != nil {
				t.Fatalf("container.Get(%s) failed: %v", fmtType, err)
			}

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
			if err != nil {
				t.Fatalf("WriteArchive failed: %v", err)
			}
			if written <= 0 || archiveBuf.Len() == 0 {
				t.Fatalf("unexpected written size: %d", written)
			}

			readFiles, _, err := engine.ReadIndex(&archiveBuf, nil, 0, "", nil)
			if err != nil {
				t.Fatalf("ReadIndex failed: %v", err)
			}

			if len(readFiles) != 2 {
				t.Fatalf("expected 2 files, got %d", len(readFiles))
			}

			r, err := readFiles[0].Open()
			if err != nil {
				t.Fatalf("failed to open read file: %v", err)
			}
			data, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				t.Fatalf("failed to read file data: %v", err)
			}
			if string(data) != string(f1Content) {
				t.Errorf("content mismatch: got %q, want %q", string(data), string(f1Content))
			}
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
			Mode:     0644,
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

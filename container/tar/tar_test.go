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

	"github.com/lemon4ksan/seal/container/tar"
)

func TestTar_Roundtrip(t *testing.T) {
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	entries := []struct {
		name    string
		content string
	}{
		{"file1.txt", "Hello TAR World"},
		{"nested/file2.json", `{"status":"ok"}`},
	}

	for _, e := range entries {
		entry := &tar.Entry{
			Name:    e.name,
			Size:    int64(len(e.content)),
			Mode:    0o644,
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(entry); err != nil {
			t.Fatalf("WriteHeader failed: %v", err)
		}
		if _, err := tw.WritePayload(strings.NewReader(e.content)); err != nil {
			t.Fatalf("WritePayload failed: %v", err)
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

		data, err := io.ReadAll(src)
		if err != nil {
			t.Fatalf("io.ReadAll failed: %v", err)
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

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/file"
)

func TestValidation_InvalidEntries(t *testing.T) {
	t.Run("EmptyFilename", func(t *testing.T) {
		_, err := file.New("", false)
		if err == nil {
			t.Fatalf("expected error on empty filename, got nil")
		}
	})

	t.Run("FilenameTooLong", func(t *testing.T) {
		hugeName := strings.Repeat("a", 70000)
		f, err := file.New("short.txt", false)
		if err != nil {
			t.Fatalf("file.New failed: %v", err)
		}
		f.SetName(hugeName)

		if err := f.Validate(); !errors.Is(err, file.ErrFilenameTooLong) {
			t.Fatalf("expected ErrFilenameTooLong, got %v", err)
		}

		arc := seal.NewArchive()
		arc.AddEntry(f)
		var buf bytes.Buffer
		_, err = arc.WriteTo(&buf)
		if err == nil {
			t.Fatalf("expected error writing archive with too long filename, got nil")
		}
	})

	t.Run("NullByteInPath", func(t *testing.T) {
		f, err := file.New("test.txt", false)
		if err != nil {
			t.Fatalf("file.New failed: %v", err)
		}
		f.SetName("invalid\x00path.txt")

		if err := f.Validate(); !errors.Is(err, file.ErrNullByteInPath) {
			t.Fatalf("expected ErrNullByteInPath, got %v", err)
		}

		arc := seal.NewArchive()
		arc.AddEntry(f)
		var buf bytes.Buffer
		_, err = arc.WriteTo(&buf)
		if err == nil {
			t.Fatalf("expected error writing archive with null byte, got nil")
		}
	})

	t.Run("NegativeFileSize", func(t *testing.T) {
		f, err := file.New("test.txt", false)
		if err != nil {
			t.Fatalf("file.New failed: %v", err)
		}
		f.SetSizes(-100, 0, 0)

		if err := f.Validate(); !errors.Is(err, file.ErrNegativeSize) {
			t.Fatalf("expected ErrNegativeSize, got %v", err)
		}

		arc := seal.NewArchive()
		arc.AddEntry(f)
		var buf bytes.Buffer
		_, err = arc.WriteTo(&buf)
		if err == nil {
			t.Fatalf("expected error writing archive with negative size, got nil")
		}
	})
}

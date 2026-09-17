// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lemon4ksan/foundation/testing/require"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/file"
)

func TestValidation_InvalidEntries(t *testing.T) {
	t.Parallel()

	t.Run("EmptyFilename", func(t *testing.T) {
		t.Parallel()
		_, err := file.New("", false)
		require.Error(t, err)
	})

	t.Run("FilenameTooLong", func(t *testing.T) {
		t.Parallel()
		hugeName := strings.Repeat("a", 70000)
		f, err := file.New("short.txt", false)
		require.NoError(t, err)
		f.SetName(hugeName)

		require.ErrorIs(t, f.Validate(), file.ErrFilenameTooLong)

		arc := seal.NewArchive()
		arc.Add(f)
		var buf bytes.Buffer
		_, err = arc.WriteTo(t.Context(), &buf)
		require.Error(t, err)
	})

	t.Run("NullByteInPath", func(t *testing.T) {
		t.Parallel()
		f, err := file.New("test.txt", false)
		require.NoError(t, err)
		f.SetName("invalid\x00path.txt")

		require.ErrorIs(t, f.Validate(), file.ErrNullByteInPath)

		arc := seal.NewArchive()
		arc.Add(f)
		var buf bytes.Buffer
		_, err = arc.WriteTo(t.Context(), &buf)
		require.Error(t, err)
	})

	t.Run("NegativeFileSize", func(t *testing.T) {
		t.Parallel()
		f, err := file.New("test.txt", false)
		require.NoError(t, err)
		f.SetSizes(-100, 0, 0)

		require.ErrorIs(t, f.Validate(), file.ErrNegativeSize)

		arc := seal.NewArchive()
		arc.Add(f)
		var buf bytes.Buffer
		_, err = arc.WriteTo(t.Context(), &buf)
		require.Error(t, err)
	})
}

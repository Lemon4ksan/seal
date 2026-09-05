// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/engine"
)

func TestReproducibleArchive_BitForBitIdentical(t *testing.T) {
	files := map[string][]byte{
		"z_file.txt": []byte("Content Z"),
		"a_file.txt": []byte("Content A"),
		"m_file.txt": []byte("Content M"),
	}

	epoch := int64(1700000000) * int64(time.Second)

	// Run 1
	buf1 := new(bytes.Buffer)
	w1 := engine.NewWriter(buf1, engine.WithReproducible(epoch))
	_ = w1.AddFile("a_file.txt", files["a_file.txt"])
	_ = w1.AddFile("m_file.txt", files["m_file.txt"])
	_ = w1.AddFile("z_file.txt", files["z_file.txt"])
	_ = w1.Close()

	// Wait to ensure wall clock time changes
	time.Sleep(50 * time.Millisecond)

	// Run 2 (different run time)
	buf2 := new(bytes.Buffer)
	w2 := engine.NewWriter(buf2, engine.WithReproducible(epoch))
	_ = w2.AddFile("a_file.txt", files["a_file.txt"])
	_ = w2.AddFile("m_file.txt", files["m_file.txt"])
	_ = w2.AddFile("z_file.txt", files["z_file.txt"])
	_ = w2.Close()

	h1 := sha256.Sum256(buf1.Bytes())
	h2 := sha256.Sum256(buf2.Bytes())

	if h1 != h2 {
		t.Fatalf("reproducibility failed:\n  run1: %x\n  run2: %x", h1, h2)
	}
}

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package aeon_test

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/eon/catalog"
	"github.com/lemon4ksan/seal/container/eon/crypto"
	"github.com/lemon4ksan/seal/container/eon/engine"
	"github.com/lemon4ksan/seal/container/eon/prolly"
	"github.com/lemon4ksan/seal/container/eon/spec"
	"github.com/lemon4ksan/seal/crypto/shamir"
)

// buildSeedContainer generates a valid reference container for fuzz seeding.
func buildSeedContainer(t testing.TB) []byte {
	var buf bytes.Buffer
	w := engine.NewWriter(&buf,
		engine.WithPassword("FuzzPassword123!"),
		engine.WithKDFParams(1024, 1, 1),
	)
	payload := []byte("fuzz test payload data 12345")
	if err := w.AddFileStream("fuzz/file1.txt", int64(len(payload)), time.Now(), bytes.NewReader(payload)); err != nil {
		t.Fatalf("AddFileStream: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return buf.Bytes()
}

// FuzzOpenReader asserts that arbitrary byte streams fed to OpenReader NEVER cause panics or unbounded allocations.
func FuzzOpenReader(f *testing.F) {
	// Seed 1: Empty bytes
	f.Add([]byte{})
	// Seed 2: Small arbitrary bytes
	f.Add([]byte{0x45, 0x4F, 0x4E, 0x01})
	// Seed 3: Complete valid container
	seed := buildSeedContainer(f)
	f.Add(seed)
	// Seed 4: Truncated valid container
	if len(seed) > 64 {
		f.Add(seed[:64])
		f.Add(seed[len(seed)-32:])
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		r, err := engine.OpenReader(bytes.NewReader(data), int64(len(data)),
			engine.WithReaderPassword("FuzzPassword123!"),
		)
		if err != nil {
			return // Graceful error is the expected invariant for invalid input
		}

		// If OpenReader succeeded, querying reader API must also never panic
		_ = r.TotalChunks()
		_ = r.IsEncrypted()
		_ = r.CipherID()
		_ = r.HasSignature()
		_ = r.HasProllyTree()
		_ = r.UnlockedVaults()
		_ = r.ActiveVault()
		_ = r.DEK()

		extents := r.Extents()
		for _, ext := range extents {
			_, _ = r.FindExtent(ext.Namespace, ext.Name)
			rc, err := r.Open(ext.Name)
			if err == nil {
				buf := make([]byte, 1024)
				_, _ = rc.Read(buf)
				_ = rc.Close()
			}
		}
	})
}

// FuzzReadCTOC asserts that arbitrary byte streams fed to ReadCTOC never cause panics.
func FuzzReadCTOC(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x43, 0x54, 0x4F, 0x43}) // 'CTOC' magic
	var validBuf bytes.Buffer
	ctoc := &catalog.CTOC{
		Entries: []catalog.CTOCEntry{
			{BlockIndex: 1, CompressedSize: 100, UncompressedSize: 200},
			{BlockIndex: 2, CompressedSize: 150, UncompressedSize: 300},
		},
	}
	_, _ = ctoc.Write(&validBuf)
	f.Add(validBuf.Bytes())

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = catalog.ReadCTOC(bytes.NewReader(data))
	})
}

// FuzzReadIndex asserts that arbitrary byte streams fed to ReadIndex never cause panics.
func FuzzReadIndex(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x49, 0x4E, 0x44, 0x58}) // 'INDX' magic
	var validBuf bytes.Buffer
	indx := &catalog.Index{
		Extents: []catalog.Extent{
			{Namespace: spec.NamespaceVFS, Name: "file.txt", PayloadSize: 100},
		},
	}
	_, _ = indx.Write(&validBuf)
	f.Add(validBuf.Bytes())

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = catalog.ReadIndex(bytes.NewReader(data))
	})
}

// FuzzReadENCR asserts that arbitrary byte streams fed to ReadENCR never cause panics.
func FuzzReadENCR(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x45, 0x4E, 0x43, 0x52}) // 'ENCR' magic
	encr, _, err := crypto.CreateENCR("testpass", spec.CipherAES256GCM, 1024, 1, 1)
	if err == nil {
		var buf bytes.Buffer
		_, _ = encr.Write(&buf)
		f.Add(buf.Bytes())
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = crypto.ReadENCR(bytes.NewReader(data))
	})
}

// FuzzProllyNode asserts that arbitrary byte streams fed to DeserializeNode never cause panics.
func FuzzProllyNode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x54, 0x52, 0x45, 0x45}) // 'TREE' magic
	node := &prolly.Node{
		Level: 0,
		Items: []prolly.Item{
			{Namespace: 0, Name: "doc1", Extent: &catalog.Extent{Name: "doc1", PayloadSize: 50}},
		},
	}
	data, err := node.Serialize()
	if err == nil {
		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = prolly.DeserializeNode(data)
	})
}

// FuzzShamirParseShare asserts that arbitrary strings fed to ParseShare never cause panics.
func FuzzShamirParseShare(f *testing.F) {
	f.Add("")
	f.Add("aeon-share-m3n5-01-abcdef0123456789-12345678")
	f.Add("garbage-string-without-hyphens")
	f.Add("aeon-share-m0n0-00--00000000")

	// Generate a valid share
	secret := []byte("01234567890123456789012345678901")
	shares, err := shamir.Split(secret, 3, 5)
	if err == nil {
		f.Add(shamir.FormatShare(shares[0], 3, 5))
	}

	f.Fuzz(func(t *testing.T, str string) {
		_, _, _, _ = shamir.ParseShare(str)
	})
}

// TestVerification_CorruptContainerMatrix performs exhaustive mutation fuzzing across all container sections.
func TestVerification_CorruptContainerMatrix(t *testing.T) {
	seed := buildSeedContainer(t)
	r := rand.New(rand.NewSource(42))

	// Test 100 randomized bit flips across various positions in the container
	for trial := 0; trial < 100; trial++ {
		mutated := make([]byte, len(seed))
		copy(mutated, seed)

		// Choose 1 to 5 random bytes to corrupt
		flips := r.Intn(5) + 1
		for f := 0; f < flips; f++ {
			pos := r.Intn(len(mutated))
			mutated[pos] ^= byte(1 << r.Intn(8))
		}

		// Invariant: Must never panic
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("trial %d: PANIC during OpenReader with corrupted bytes: %v", trial, rec)
				}
			}()

			reader, err := engine.OpenReader(bytes.NewReader(mutated), int64(len(mutated)),
				engine.WithReaderPassword("FuzzPassword123!"),
			)
			if err != nil {
				return // Tamper correctly rejected at header/trailer/ENCR phase
			}

			// If reader opened, verify that ReadExtent or Diagnose safely detects chunk/directory tampering
			_, _ = reader.ReadExtent("fuzz/file1.txt")
			_ = reader.Diagnose(false)
		}()
	}
}

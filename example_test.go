// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/extension/signature"
)

// ExampleArchiveDir demonstrates compressing a directory with cancellation support.
func ExampleArchiveDir() {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "seal-example-*")
	defer os.RemoveAll(tmpDir)

	srcDir := filepath.Join(tmpDir, "src")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("Hello Seal!"), 0o644)

	destArchive := filepath.Join(tmpDir, "archive.zip")

	err := seal.ArchiveDir(ctx, srcDir, seal.ToFilePath(destArchive), seal.WithContainer(container.Zip))
	if err != nil {
		fmt.Printf("ArchiveDir error: %v\n", err)
		return
	}

	exists, _ := seal.ExistsIn(ctx, seal.FromFilePath(destArchive), "hello.txt")
	fmt.Printf("hello.txt in archive: %v\n", exists)

	// Output:
	// hello.txt in archive: true
}

// ExampleNewArchive demonstrates constructing an in-memory virtual archive without touching disk.
func ExampleNewArchive() {
	ctx := context.Background()

	arc := seal.NewArchive(
		seal.WithContainer(container.Zip),
		seal.WithCompression(seal.Deflate, 5),
	)

	arc.AddString("version 1.0.0", "VERSION")
	arc.AddBytes([]byte(`{"status":"healthy"}`), "health.json")

	var buf bytes.Buffer
	written, err := arc.WriteTo(ctx, &buf)
	if err != nil {
		fmt.Printf("WriteTo error: %v\n", err)
		return
	}

	fmt.Printf("Virtual archive produced %t across %d files\n", written > 0, len(arc.Files()))

	// Output:
	// Virtual archive produced true across 2 files
}

// ExampleWithExtension demonstrates attaching an arbitrary pluggable metadata extension.
func ExampleWithExtension() {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "seal-ext-*")
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "bundle.zip")

	rawLicense := extension.NewRaw("acme.license", []byte("PRO-TIER-LICENSE-KEY-2026"))

	arc := seal.NewArchive(seal.WithContainer(container.Zip))
	arc.AddString("app code", "main.go")

	var buf bytes.Buffer
	_, _ = arc.WriteTo(ctx, &buf, seal.WithExtension(rawLicense))
	_ = os.WriteFile(archivePath, buf.Bytes(), 0o644)

	readLic := extension.NewRaw("acme.license", nil)
	_ = seal.InspectExtensions(ctx, archivePath, readLic)

	fmt.Printf("Recovered Extension Payload: %s\n", string(readLic.Data))

	// Output:
	// Recovered Extension Payload: PRO-TIER-LICENSE-KEY-2026
}

// ExampleVerifySignature demonstrates digitally signing and cryptographically verifying an archive.
func ExampleVerifySignature() {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "seal-sign-*")
	defer os.RemoveAll(tmpDir)

	pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)
	signer, _ := signature.NewSigner(privKey, "Release Manager <signer@example.com>")

	archivePath := filepath.Join(tmpDir, "signed_bundle.zip")

	arc := seal.NewArchive(seal.WithContainer(container.Zip))
	arc.AddString("critical binary content", "payload.bin")

	var buf bytes.Buffer
	_, _ = arc.WriteTo(ctx, &buf, seal.WithExtension(signer))
	_ = os.WriteFile(archivePath, buf.Bytes(), 0o644)

	// Verify cryptographic signature
	sigState, err := seal.VerifySignature(ctx, archivePath, pubKey)
	if err != nil {
		fmt.Printf("Verification error: %v\n", err)
		return
	}

	fmt.Printf("Signature Valid: true, Signer: %s\n", sigState.Signer)

	// Output:
	// Signature Valid: true, Signer: Release Manager <signer@example.com>
}

// ExampleCanResume demonstrates inspecting an archive for a stealth resumption checkpoint.
func ExampleCanResume() {
	ctx := context.Background()
	tmpDir, _ := os.MkdirTemp("", "seal-resume-*")
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "sample.zip")
	arc := seal.NewArchive(seal.WithContainer(container.Zip))
	arc.AddString("content", "test.txt")

	var buf bytes.Buffer
	_, _ = arc.WriteTo(ctx, &buf)
	_ = os.WriteFile(archivePath, buf.Bytes(), 0o644)

	_, hasCheckpoint, _ := seal.CanResume(ctx, archivePath)
	fmt.Printf("Completed archive has checkpoint: %v\n", hasCheckpoint)

	// Output:
	// Completed archive has checkpoint: false
}

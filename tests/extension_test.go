// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/extension"
	"github.com/lemon4ksan/seal/extension/provenance"
	"github.com/lemon4ksan/seal/extension/signature"
)

// TestLicenseExtension represents a custom user-defined extension plugin.
type TestLicenseExtension struct {
	LicenseKey string `json:"license_key"`
	Tier       string `json:"tier"`
	CustomerID string `json:"customer_id"`
}

func (l *TestLicenseExtension) ID() extension.ID {
	return "acme.license"
}

func (l *TestLicenseExtension) OnWrite(_ context.Context, _ extension.ArchiveInfo) ([]byte, error) {
	return json.Marshal(l)
}

func (l *TestLicenseExtension) OnRead(_ context.Context, data []byte, _ extension.ArchiveInfo) error {
	return json.Unmarshal(data, l)
}

func TestExtension_MultiFormat_Roundtrip(t *testing.T) {
	t.Parallel()

	formats := []struct {
		name      string
		container container.Type
		ext       string
	}{
		{name: "ZIP", container: container.Zip, ext: ".zip"},
		{name: "7z", container: container.SevenZip, ext: ".7z"},
		{name: "TAR", container: container.Tar, ext: ".tar"},
	}

	for _, tc := range formats {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srcDir := t.TempDir()
			_ = os.WriteFile(filepath.Join(srcDir, "app.go"), []byte("package main\nfunc main() {}\n"), 0o644)
			_ = os.WriteFile(filepath.Join(srcDir, "config.json"), []byte(`{"env":"prod"}`), 0o644)

			outDir := t.TempDir()
			archivePath := filepath.Join(outDir, "bundle"+tc.ext)

			origLic := &TestLicenseExtension{
				LicenseKey: "LIC-SEAL-2026-XYZ",
				Tier:       "enterprise",
				CustomerID: "corp-8888",
			}
			rawSig := extension.NewRaw("acme.signature", []byte{0xCA, 0xFE, 0xBA, 0xBE, 0x42})

			// 1. Create archive with multiple extensions
			err := seal.ArchiveDir(
				t.Context(),
				srcDir,
				seal.ToFilePath(archivePath),
				seal.WithContainer(tc.container),
				seal.WithExtension(origLic, rawSig),
			)
			if err != nil {
				t.Fatalf("ArchiveDir failed: %v", err)
			}

			// 2. Extract archive to make sure normal extraction is completely unaffected
			extractDir := t.TempDir()
			if err := seal.Unarchive(t.Context(), seal.FromFilePath(archivePath), extractDir); err != nil {
				t.Fatalf("Unarchive failed: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(extractDir, "config.json"))
			if err != nil || string(data) != `{"env":"prod"}` {
				t.Fatalf("Extracted file mismatch or error: %v, data=%q", err, string(data))
			}

			// 3. Inspect extensions
			readLic := &TestLicenseExtension{}
			readSig := extension.NewRaw("acme.signature", nil)
			if err := seal.InspectExtensions(t.Context(), archivePath, readLic, readSig); err != nil {
				t.Fatalf("InspectExtensions failed: %v", err)
			}

			if readLic.LicenseKey != origLic.LicenseKey || readLic.Tier != origLic.Tier || readLic.CustomerID != origLic.CustomerID {
				t.Fatalf("License mismatch: got %+v, want %+v", readLic, origLic)
			}

			if !bytes.Equal(readSig.Data, rawSig.Data) {
				t.Fatalf("Signature mismatch: got %x, want %x", readSig.Data, rawSig.Data)
			}

			// 4. GetExtension direct lookup
			rawBytes, err := seal.GetExtension(t.Context(), archivePath, "acme.signature")
			if err != nil {
				t.Fatalf("GetExtension failed: %v", err)
			}
			if !bytes.Equal(rawBytes, rawSig.Data) {
				t.Fatalf("GetExtension payload mismatch")
			}
		})
	}
}

func TestExtension_FirstParty_SignatureAndProvenance(t *testing.T) {
	t.Parallel()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	signer, err := signature.NewSigner(privKey, "Lemon4ksan <release@seal.dev>")
	if err != nil {
		t.Fatalf("NewSigner failed: %v", err)
	}

	prov := provenance.New(provenance.Info{
		Commit:  "1234567890abcdef",
		Branch:  "v1.0-release",
		Builder: "GitHub-Actions",
	})

	srcDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(srcDir, "binary"), []byte("ELF executable mock content"), 0o755)

	outDir := t.TempDir()
	archivePath := filepath.Join(outDir, "release.zip")

	// Create archive with both first-party extensions
	err = seal.ArchiveDir(
		t.Context(),
		srcDir,
		seal.ToFilePath(archivePath),
		seal.WithContainer(container.Zip),
		seal.WithExtension(signer, prov),
	)
	if err != nil {
		t.Fatalf("ArchiveDir failed: %v", err)
	}

	// 1. Verify Signature
	sigState, err := seal.VerifySignature(t.Context(), archivePath, pubKey)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}
	if sigState.Signer != "Lemon4ksan <release@seal.dev>" {
		t.Fatalf("Signer mismatch: got %q", sigState.Signer)
	}

	// 2. Reject verification with wrong public key
	wrongPub, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := seal.VerifySignature(t.Context(), archivePath, wrongPub); !errors.Is(err, seal.ErrSignatureInvalid) {
		t.Fatalf("Expected ErrSignatureInvalid with wrong key, got: %v", err)
	}

	// 3. Inspect Provenance
	provInfo, err := seal.InspectProvenance(t.Context(), archivePath)
	if err != nil {
		t.Fatalf("InspectProvenance failed: %v", err)
	}
	if provInfo.Commit != "1234567890abcdef" || provInfo.Branch != "v1.0-release" || provInfo.Builder != "GitHub-Actions" {
		t.Fatalf("Provenance mismatch: got %+v", provInfo)
	}
}


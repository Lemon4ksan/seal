// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tests

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lemon4ksan/foundation/testing/assert"
	"github.com/lemon4ksan/foundation/testing/require"

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
			require.NoError(t, err)

			// 2. Extract archive to make sure normal extraction is completely unaffected
			extractDir := t.TempDir()
			err = seal.Unarchive(t.Context(), seal.FromFilePath(archivePath), extractDir)
			require.NoError(t, err)
			data, err := os.ReadFile(filepath.Join(extractDir, "config.json"))
			require.NoError(t, err)
			assert.Equal(t, `{"env":"prod"}`, string(data))

			// 3. Inspect extensions
			readLic := &TestLicenseExtension{}
			readSig := extension.NewRaw("acme.signature", nil)
			err = seal.InspectExtensions(t.Context(), archivePath, readLic, readSig)
			require.NoError(t, err)

			assert.Equal(t, origLic.LicenseKey, readLic.LicenseKey)
			assert.Equal(t, origLic.Tier, readLic.Tier)
			assert.Equal(t, origLic.CustomerID, readLic.CustomerID)
			assert.Equal(t, rawSig.Data, readSig.Data)

			// 4. GetExtension direct lookup
			rawBytes, err := seal.GetExtension(t.Context(), archivePath, "acme.signature")
			require.NoError(t, err)
			assert.Equal(t, rawSig.Data, rawBytes)
		})
	}
}

func TestExtension_FirstParty_SignatureAndProvenance(t *testing.T) {
	t.Parallel()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signer, err := signature.NewSigner(privKey, "Lemon4ksan <release@seal.dev>")
	require.NoError(t, err)

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
	require.NoError(t, err)

	// 1. Verify Signature
	sigState, err := seal.VerifySignature(t.Context(), archivePath, pubKey)
	require.NoError(t, err)
	assert.Equal(t, "Lemon4ksan <release@seal.dev>", sigState.Signer)

	// 2. Reject verification with wrong public key
	wrongPub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, err = seal.VerifySignature(t.Context(), archivePath, wrongPub)
	require.ErrorIs(t, err, seal.ErrSignatureInvalid)

	// 3. Inspect Provenance
	provInfo, err := seal.InspectProvenance(t.Context(), archivePath)
	require.NoError(t, err)
	assert.Equal(t, "1234567890abcdef", provInfo.Commit)
	assert.Equal(t, "v1.0-release", provInfo.Branch)
	assert.Equal(t, "GitHub-Actions", provInfo.Builder)
}

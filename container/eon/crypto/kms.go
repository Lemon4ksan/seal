// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"context"

	"github.com/lemon4ksan/sein/crypto/kms"
)

// KMSClient defines the interface for envelope encryption key management services (Vault, HSM, mock KMS).
type KMSClient = kms.Client

// MockKMSClient provides an in-memory AES-GCM envelope encryption provider for testing.
type MockKMSClient struct {
	inner *kms.MockClient
}

// NewMockKMSClient creates a test KMS client with a random master key.
func NewMockKMSClient() *MockKMSClient {
	return &MockKMSClient{
		inner: kms.NewMockClient(),
	}
}

// Encrypt encrypts a 32-byte DEK using the specified master key ID/URI.
func (m *MockKMSClient) Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, error) {
	return m.inner.Encrypt(ctx, keyID, plaintext)
}

// Decrypt decrypts a KMS ciphertext blob to recover the 32-byte DEK.
func (m *MockKMSClient) Decrypt(ctx context.Context, keyID string, ciphertext []byte) ([]byte, error) {
	return m.inner.Decrypt(ctx, keyID, ciphertext)
}

// VaultClient implements HashiCorp Vault Transit Secret Engine envelope encryption.
type VaultClient struct {
	inner *kms.VaultClient
}

// NewVaultClient creates a Vault Transit client from environment or explicit credentials.
func NewVaultClient(addr, token string) *VaultClient {
	return &VaultClient{
		inner: kms.NewVaultClient(kms.VaultConfig{
			Address: addr,
			Token:   token,
		}),
	}
}

// Encrypt encrypts a 32-byte DEK using HashiCorp Vault Transit engine.
func (v *VaultClient) Encrypt(ctx context.Context, keyPath string, plaintext []byte) ([]byte, error) {
	return v.inner.Encrypt(ctx, keyPath, plaintext)
}

// Decrypt decrypts a Vault Transit ciphertext blob to recover the 32-byte DEK.
func (v *VaultClient) Decrypt(ctx context.Context, keyPath string, ciphertext []byte) ([]byte, error) {
	return v.inner.Decrypt(ctx, keyPath, ciphertext)
}

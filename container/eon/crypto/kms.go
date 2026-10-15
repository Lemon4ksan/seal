// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lemon4ksan/foundation/silicon/randkit"
)

// KMSClient defines the interface for envelope encryption key management services (Vault, HSM, mock KMS).
type KMSClient interface {
	// Encrypt encrypts a 32-byte DEK using the specified master key ID/URI.
	Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, error)

	// Decrypt decrypts a KMS ciphertext blob to recover the 32-byte DEK.
	Decrypt(ctx context.Context, keyID string, ciphertext []byte) ([]byte, error)
}

// MockKMSClient provides an in-memory AES-GCM envelope encryption provider for testing.
type MockKMSClient struct {
	mu         sync.RWMutex
	defaultKey [32]byte
	namedKeys  map[string][32]byte
}

// NewMockKMSClient creates a test KMS client with a random master key.
func NewMockKMSClient(masterKey ...[]byte) *MockKMSClient {
	m := &MockKMSClient{
		namedKeys: make(map[string][32]byte),
	}
	if len(masterKey) > 0 && len(masterKey[0]) == 32 {
		copy(m.defaultKey[:], masterKey[0])
	} else {
		randBytes := randkit.MustSecureBytes(32)
		copy(m.defaultKey[:], randBytes)
	}
	return m
}

// SetKey assigns an explicit 32-byte master key for a specific keyID.
func (m *MockKMSClient) SetKey(keyID string, key [32]byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.namedKeys[keyID] = key
}

func (m *MockKMSClient) getKey(keyID string) [32]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if k, ok := m.namedKeys[keyID]; ok {
		return k
	}
	return m.defaultKey
}

// Encrypt encrypts a 32-byte DEK using the specified master key ID/URI.
func (m *MockKMSClient) Encrypt(_ context.Context, keyID string, plaintext []byte) ([]byte, error) {
	key := m.getKey(keyID)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("mock kms: cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mock kms: gcm init: %w", err)
	}
	nonce := randkit.MustSecureBytes(gcm.NonceSize())
	ct := gcm.Seal(nonce, nonce, plaintext, []byte(keyID))
	return ct, nil
}

// Decrypt decrypts a KMS ciphertext blob to recover the 32-byte DEK.
func (m *MockKMSClient) Decrypt(_ context.Context, keyID string, ciphertext []byte) ([]byte, error) {
	key := m.getKey(keyID)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("mock kms: cipher init: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mock kms: gcm init: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("mock kms: ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	pt, err := gcm.Open(nil, nonce, ct, []byte(keyID))
	if err != nil {
		return nil, fmt.Errorf("mock kms: decryption failed: %w", err)
	}
	return pt, nil
}

// VaultClient implements HashiCorp Vault Transit Secret Engine envelope encryption.
type VaultClient struct {
	address    string
	token      string
	namespace  string
	httpClient *http.Client
}

// NewVaultClient creates a Vault Transit client from environment or explicit credentials.
func NewVaultClient(addr, token string) *VaultClient {
	if addr == "" {
		addr = "http://127.0.0.1:8200"
	}
	addr = strings.TrimSuffix(addr, "/")
	return &VaultClient{
		address:    addr,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Encrypt encrypts a 32-byte DEK using HashiCorp Vault Transit engine.
func (v *VaultClient) Encrypt(ctx context.Context, keyPath string, plaintext []byte) ([]byte, error) {
	keyPath = strings.TrimPrefix(keyPath, "vault://")
	url := fmt.Sprintf("%s/v1/transit/encrypt/%s", v.address, keyPath)

	b64Plain := base64.StdEncoding.EncodeToString(plaintext)
	payload, _ := json.Marshal(map[string]string{"plaintext": b64Plain})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", v.token)
	if v.namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.namespace)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault kms: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault kms: http status %d: %s", resp.StatusCode, string(body))
	}

	var vaultResp struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return nil, fmt.Errorf("vault kms: decode response: %w", err)
	}
	return []byte(vaultResp.Data.Ciphertext), nil
}

// Decrypt decrypts a Vault Transit ciphertext blob to recover the 32-byte DEK.
func (v *VaultClient) Decrypt(ctx context.Context, keyPath string, ciphertext []byte) ([]byte, error) {
	keyPath = strings.TrimPrefix(keyPath, "vault://")
	url := fmt.Sprintf("%s/v1/transit/decrypt/%s", v.address, keyPath)

	payload, _ := json.Marshal(map[string]string{"ciphertext": string(ciphertext)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", v.token)
	if v.namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.namespace)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vault kms: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault kms: http status %d: %s", resp.StatusCode, string(body))
	}

	var vaultResp struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vaultResp); err != nil {
		return nil, fmt.Errorf("vault kms: decode response: %w", err)
	}

	rawPt, err := base64.StdEncoding.DecodeString(vaultResp.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("vault kms: base64 decode: %w", err)
	}
	return rawPt, nil
}

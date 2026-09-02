// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package crypto

import (
	"io"
	"sync"

	"github.com/lemon4ksan/seal/file"
)

// Cipher provides stream encryption and decryption for a specific algorithm.
type Cipher interface {
	Method() file.EncryptionMethod
	NewEncryptWriter(w io.Writer, password string) (io.WriteCloser, error)
	NewDecryptReader(r io.Reader, password string) (io.Reader, error)
}

// Registry manages a thread-safe registry of ciphers.
type Registry struct {
	mu      sync.RWMutex
	ciphers map[file.EncryptionMethod]Cipher
}

// NewRegistry constructs an empty crypto registry.
func NewRegistry() *Registry {
	return &Registry{
		ciphers: make(map[file.EncryptionMethod]Cipher),
	}
}

// Register adds a Cipher to the registry.
func (r *Registry) Register(c Cipher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ciphers[c.Method()] = c
}

// Get retrieves a Cipher by method.
func (r *Registry) Get(m file.EncryptionMethod) (Cipher, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.ciphers[m]
	return c, ok
}

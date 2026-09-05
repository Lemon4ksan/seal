// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunk

import (
	"sync"
)

// Handler defines a consumer or processor for a specific EON namespace.
type Handler interface {
	// Namespace returns the 16-bit namespace ID handled by this implementation.
	Namespace() uint16

	// Description returns a human-readable identifier of this chunk engine.
	Description() string

	// Handle processes an uncompressed extent payload from this namespace.
	Handle(name string, payload []byte) error
}

// Pipeline orchestrates custom chunk ingestion and extraction.
type Pipeline struct {
	mu       sync.RWMutex
	handlers map[uint16]Handler
}

// NewPipeline constructs an empty chunk processing pipeline.
func NewPipeline() *Pipeline {
	return &Pipeline{
		handlers: make(map[uint16]Handler),
	}
}

// DefaultPipeline is the global pipeline registry.
var DefaultPipeline = NewPipeline()

// Register assigns a custom handler for a designated namespace.
func (p *Pipeline) Register(h Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[h.Namespace()] = h
}

// Lookup queries a registered handler. Returns false if no handler is registered.
func (p *Pipeline) Lookup(namespace uint16) (Handler, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	h, ok := p.handlers[namespace]
	return h, ok
}

// Dispatch routes an extent payload to its registered handler.
// Conforming to Rule 6 of the EON specification: if no handler is registered,
// the extent is safely skipped without returning an error.
func (p *Pipeline) Dispatch(namespace uint16, name string, payload []byte) error {
	h, ok := p.Lookup(namespace)
	if !ok {
		// Rule 6: Unrecognized namespaces MUST be bypassed gracefully
		return nil
	}
	return h.Handle(name, payload)
}

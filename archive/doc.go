// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package archive provides an in-memory mutable archive builder, manager, and virtual filesystem.
//
// # Architectural Role
//
// [Archive] maintains virtual hierarchical paths, arbitrates entry collisions, applies sorting and filtering
// strategies, and exposes a standard [io/fs.FS] virtual filesystem interface. It orchestrates serialization
// through [github.com/lemon4ksan/seal/container.Container] implementations while remaining completely decoupled
// from disk I/O and raw wire protocols.
package archive

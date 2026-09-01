// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package file provides the canonical, 100% lock-free in-memory archive entry entity.
//
// # Lock-Free Architecture
//
// [File] utilizes hardware atomic primitives ([sync/atomic.Pointer], [sync/atomic.Int64], [sync/atomic.Uint32])
// with Copy-On-Write (COW) metadata synchronization. It eliminates all mutex contention across parallel
// extraction, compression, inspection, and VFS reading pipelines.
package file

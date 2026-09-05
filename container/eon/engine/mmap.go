// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package engine

import (
	"github.com/lemon4ksan/foundation/fskit"
)

// MmapFile represents a memory-mapped file supporting random-access reads and zero-copy slicing.
type MmapFile = fskit.MmapFile

// OpenMmap maps the specified file path directly into the process virtual memory address space.
var OpenMmap = fskit.OpenMmap

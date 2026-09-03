// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rar

import "errors"

// ErrCreateNotSupported is returned when attempting to create or write a RAR archive.
//
// RAR compression is proprietary to RARLAB (Alexander and Eugene Roshal). The official
// unRAR licensing terms explicitly prohibit re-creating the RAR compression algorithm.
// Consequently, Seal strictly supports reading and extracting RAR archives only.
var ErrCreateNotSupported = errors.New(
	"rar: creating RAR archives is proprietary and restricted by license; only extraction is supported",
)

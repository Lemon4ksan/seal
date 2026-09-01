// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package container defines the pluggable format specification and wire-level framing contracts.
//
// It encapsulates binary format detection ([SniffHeader]), extension mapping ([DetectExtension]),
// and dynamic backend registration ([Register], [Get]).
//
// All format-specific logic (ZIP Central Directory, USTAR records, PAX headers, GZIP stream multiplexing)
// is strictly confined to subpackages ([github.com/lemon4ksan/seal/container/zip],
// [github.com/lemon4ksan/seal/container/tar], [github.com/lemon4ksan/seal/container/targz]).
package container

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package option

import "github.com/lemon4ksan/seal/vfs"

// SecuritySettings defines extraction security and containment constraints.
type SecuritySettings struct {
	// AllowSymlinks enables the extraction of symbolic links.
	// WARNING: Enabling this on untrusted archives can lead to path traversal attacks.
	AllowSymlinks bool

	// RequireEncryption encloses a mandatory minimum encryption level (e.g. AES256).
	RequireEncryption bool

	// ResourceLimits establishes extraction resource limits (preventing zip-bombs and disk exhaustion).
	ResourceLimits ResourceLimits
}

// ResourceLimits aliases runtime constraints for archive extraction.
type ResourceLimits = vfs.ResourceLimits

//go:build !windows

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

// DefaultHostSystem represents the default host system for non-Windows platforms.
const DefaultHostSystem = HostSystemUNIX

// GetOEMCP returns 0 on non-Windows platforms.
func GetOEMCP() uint32 {
	return 0
}

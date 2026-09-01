//go:build !windows

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

// DefaultHostSystem represents the default host system for non-Windows platforms.
const DefaultHostSystem = HostSystemUNIX

// unixNanoToWinFiletime converts Unix epoch timestamps into Windows FILETIME (100-nanosecond intervals since 1601).
func unixNanoToWinFiletime(sec int64, nsec int64) uint64 {
	// Seconds between 1601-01-01 and 1970-01-01 (UTC).
	const intervalsDiff = 11644473600

	ticks := (uint64(sec) + intervalsDiff) * 10000000
	ticks += uint64(nsec) / 100
	return ticks
}

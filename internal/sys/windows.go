//go:build windows

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

import (
	"os"
	"syscall"
)

// DefaultHostSystem represents the default host system for the Windows runtime.
const DefaultHostSystem = HostSystemFAT

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	procGetOEMCP = kernel32.NewProc("GetOEMCP")
)

// GetOEMCP returns the active Windows OEM code page identifier.
func GetOEMCP() uint32 {
	if procGetOEMCP.Find() != nil {
		return 437
	}
	r, _, _ := procGetOEMCP.Call()
	if r == 0 {
		return 437
	}
	return uint32(r)
}

// GetFileMetadata extracts low-level NTFS timestamps from Windows file info.
func GetFileMetadata(stat os.FileInfo) Metadata {
	s, ok := stat.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return Metadata{}
	}
	return Metadata{
		LastWriteTime:  uint64(s.LastWriteTime.HighDateTime)<<32 | uint64(s.LastWriteTime.LowDateTime),
		LastAccessTime: uint64(s.LastAccessTime.HighDateTime)<<32 | uint64(s.LastAccessTime.LowDateTime),
		CreationTime:   uint64(s.CreationTime.HighDateTime)<<32 | uint64(s.CreationTime.LowDateTime),
	}
}

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sys

// Metadata encapsulates operating system-specific timestamp metadata.
// Times are represented as Windows FILETIME (100-nanosecond intervals since Jan 1, 1601 UTC).
type Metadata struct {
	LastAccessTime uint64
	LastWriteTime  uint64
	CreationTime   uint64
}

// HostSystem identifies the host operating system platform where the archive entry was produced.
type HostSystem uint8

// Specification-defined host system identifiers according to PKWARE APPNOTE.TXT.
const (
	HostSystemFAT       HostSystem = 0  // MS-DOS, OS/2, FAT/FAT32 filesystem
	HostSystemAmiga     HostSystem = 1  // Amiga
	HostSystemOpenVMS   HostSystem = 2  // OpenVMS
	HostSystemUNIX      HostSystem = 3  // UNIX and POSIX-compliant environments
	HostSystemVMCMS     HostSystem = 4  // VM/CMS
	HostSystemAtariST   HostSystem = 5  // Atari ST
	HostSystemOS2HPFS   HostSystem = 6  // OS/2 HPFS
	HostSystemMacintosh HostSystem = 7  // Classic Macintosh
	HostSystemZSystem   HostSystem = 8  // Z-System
	HostSystemCPM       HostSystem = 9  // CP/M
	HostSystemNTFS      HostSystem = 10 // Windows NTFS
	HostSystemMVS       HostSystem = 11 // MVS (OS/390 - z/OS)
	HostSystemVSE       HostSystem = 12 // VSE
	HostSystemAcornRisc HostSystem = 13 // Acorn RISC OS
	HostSystemVFAT      HostSystem = 14 // Windows VFAT
	HostSystemAltMVS    HostSystem = 15 // Alternate MVS
	HostSystemBeOS      HostSystem = 16 // BeOS
	HostSystemTandem    HostSystem = 17 // Tandem NSK
	HostSystemOS400     HostSystem = 18 // IBM OS/400
	HostSystemDarwin    HostSystem = 19 // macOS (Darwin)
)

// IsWindows reports whether the host system corresponds to a Windows or DOS-derived platform.
func (h HostSystem) IsWindows() bool {
	switch h {
	case HostSystemFAT, HostSystemNTFS, HostSystemVFAT:
		return true
	default:
		return false
	}
}

// IsUnix reports whether the host system corresponds to a UNIX-like platform.
func (h HostSystem) IsUnix() bool {
	switch h {
	case HostSystemUNIX, HostSystemDarwin, HostSystemBeOS:
		return true
	default:
		return false
	}
}

// String returns the canonical name of the host system.
func (h HostSystem) String() string {
	switch h {
	case HostSystemFAT:
		return "FAT"
	case HostSystemAmiga:
		return "Amiga"
	case HostSystemOpenVMS:
		return "OpenVMS"
	case HostSystemUNIX:
		return "UNIX"
	case HostSystemVMCMS:
		return "VM/CMS"
	case HostSystemAtariST:
		return "Atari ST"
	case HostSystemOS2HPFS:
		return "OS/2 HPFS"
	case HostSystemMacintosh:
		return "Macintosh"
	case HostSystemZSystem:
		return "Z-System"
	case HostSystemCPM:
		return "CP/M"
	case HostSystemNTFS:
		return "Windows NTFS"
	case HostSystemMVS:
		return "MVS"
	case HostSystemVSE:
		return "VSE"
	case HostSystemAcornRisc:
		return "Acorn RISC"
	case HostSystemVFAT:
		return "VFAT"
	case HostSystemAltMVS:
		return "Alternate MVS"
	case HostSystemBeOS:
		return "BeOS"
	case HostSystemTandem:
		return "Tandem"
	case HostSystemOS400:
		return "OS/400"
	case HostSystemDarwin:
		return "macOS (Darwin)"
	default:
		return "Unknown"
	}
}

// POSIX file mode bitmasks.
const (
	S_IFMT   = 0o170000 // Type of file bitmask
	S_IFSOCK = 0o140000 // Socket
	S_IFLNK  = 0o120000 // Symbolic link
	S_IFREG  = 0o100000 // Regular file
	S_IFBLK  = 0o060000 // Block device
	S_IFDIR  = 0o040000 // Directory
	S_IFCHR  = 0o020000 // Character device
	S_IFIFO  = 0o010000 // FIFO / named pipe
	S_ISUID  = 0o004000 // Set-user-ID bit
	S_ISGID  = 0o002000 // Set-group-ID bit
	S_ISVTX  = 0o001000 // Sticky bit
)

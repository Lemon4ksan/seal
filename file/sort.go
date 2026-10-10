// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package file

import (
	"path/filepath"
	"slices"
	"strings"
)

// ExtensionGroup maps a file extension to a logical content category for solid archive ordering.
// Files in the same category share similar lexical and binary structures, drastically maximizing
// LZ77, PRZM, and LZMA2 cross-file dictionary match redundancy.
func ExtensionGroup(ext string) int {
	switch ext {
	case ".txt", ".json", ".xml", ".html", ".htm", ".css", ".js", ".ts", ".jsx", ".tsx",
		".go", ".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".cs", ".py", ".rs", ".java",
		".md", ".log", ".yaml", ".yml", ".ini", ".cfg", ".conf", ".toml", ".sql", ".bat", ".cmd", ".ps1", ".sh":
		return 0 // Code & Text

	case ".exe", ".dll", ".sys", ".node", ".so", ".dylib", ".pdb", ".lib", ".obj",
		".o", ".a", ".ocx", ".bin", ".dat", ".rsrc", ".nvph", ".spv":
		return 1 // Executables, Binaries & Shaders

	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".rtf", ".csv", ".tsv":
		return 2 // Documents & Tabular

	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".ico", ".svg", ".bmp", ".mp3", ".wav", ".ogg", ".flac",
		".mp4", ".mkv", ".avi", ".mov", ".zip", ".gz", ".xz", ".7z", ".tar", ".bz2", ".zst", ".rar", ".iso":
		return 3 // Media & Compressed

	default:
		return 4 // Other
	}
}

// SortByExtension sorts files stably by directory status, extension group, extension, and path name.
// This is the canonical solid archive ordering used by 7-Zip, RAR, and EON to maximize compression ratio.
func SortByExtension(files []*File) []*File {
	sortedFiles := make([]*File, len(files))
	copy(sortedFiles, files)
	slices.SortStableFunc(sortedFiles, func(a, b *File) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		extA := strings.ToLower(filepath.Ext(a.Name()))
		extB := strings.ToLower(filepath.Ext(b.Name()))

		grpA := ExtensionGroup(extA)
		grpB := ExtensionGroup(extB)
		if grpA != grpB {
			return grpA - grpB
		}

		if extA != extB {
			return strings.Compare(extA, extB)
		}
		return strings.Compare(a.Name(), b.Name())
	})
	return sortedFiles
}

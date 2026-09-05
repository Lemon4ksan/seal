// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package option

import (
	"path"
	"strings"

	"github.com/lemon4ksan/foundation/generic"
)

// Filter represents a predicate function to selectively include or exclude archive entries.
type Filter func(entryName string, isDir bool) bool

// FilterOnly retains only entries that match one of the specified paths.
func FilterOnly(names ...string) Filter {
	cleanNames := generic.Map(names, func(n string) string {
		return strings.TrimPrefix(path.Clean(strings.ReplaceAll(n, "\\", "/")), "/")
	})
	lookup := generic.NewSet(cleanNames...)
	return func(entryName string, isDir bool) bool {
		clean := strings.TrimPrefix(path.Clean(strings.ReplaceAll(entryName, "\\", "/")), "/")
		return lookup.Has(clean)
	}
}

// FilterFromDir retains only entries residing under the specified directory path.
func FilterFromDir(dirPath string) Filter {
	prefix := strings.TrimPrefix(path.Clean(strings.ReplaceAll(dirPath, "\\", "/")), "/")
	if prefix == "" || prefix == "." {
		return func(_ string, _ bool) bool { return true }
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	dirEntryName := strings.TrimSuffix(prefix, "/")

	return func(entryName string, _ bool) bool {
		clean := strings.TrimPrefix(path.Clean(strings.ReplaceAll(entryName, "\\", "/")), "/")
		return strings.HasPrefix(clean, prefix) || clean == dirEntryName
	}
}

// FilterExcludeDir excludes the specified directory and all its descendants.
func FilterExcludeDir(dirPath string) Filter {
	prefix := strings.TrimPrefix(path.Clean(strings.ReplaceAll(dirPath, "\\", "/")), "/")
	if prefix == "" || prefix == "." {
		return func(_ string, _ bool) bool { return false }
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	dirEntryName := strings.TrimSuffix(prefix, "/")

	return func(entryName string, _ bool) bool {
		clean := strings.TrimPrefix(path.Clean(strings.ReplaceAll(entryName, "\\", "/")), "/")
		return !strings.HasPrefix(clean, prefix) && clean != dirEntryName
	}
}

// DefaultStoreExtensions contains file extensions that are pre-compressed and should not undergo re-compression.
var DefaultStoreExtensions = generic.NewSet(
	"7z", "aar", "ace", "apk", "arc", "arj",
	"br", "bz2", "cab", "deb", "dmg", "epub",
	"gz", "jar", "lz4", "lzma", "lzo", "rar",
	"rpm", "tar", "tgz", "war", "xz", "zip",
	"zst",
	// Media Files
	"mp4", "mkv", "avi", "mov", "webm",
	"jpg", "jpeg", "png", "gif", "webp",
	"mp3", "ogg", "flac",
	// Compressed Documents
	"pdf", "docx", "xlsx", "pptx",
)

// IsPreCompressedExtension reports whether the given filename extension belongs to an already compressed format.
func IsPreCompressedExtension(filename string) bool {
	ext := path.Ext(filename)
	if ext == "" {
		return false
	}
	cleanExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	return DefaultStoreExtensions.Has(cleanExt)
}

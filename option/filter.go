// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package option

import (
	"path"
	"strings"
)

// Filter represents a predicate function to selectively include or exclude archive entries.
type Filter func(entryName string, isDir bool) bool

// FilterOnly retains only entries that match one of the specified paths.
func FilterOnly(names ...string) Filter {
	lookup := make(map[string]struct{}, len(names))
	for _, n := range names {
		clean := strings.TrimPrefix(path.Clean(strings.ReplaceAll(n, "\\", "/")), "/")
		lookup[clean] = struct{}{}
	}
	return func(entryName string, isDir bool) bool {
		clean := strings.TrimPrefix(path.Clean(strings.ReplaceAll(entryName, "\\", "/")), "/")
		_, ok := lookup[clean]
		return ok
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
var DefaultStoreExtensions = map[string]struct{}{
	"7z": {}, "aar": {}, "ace": {}, "apk": {}, "arc": {}, "arj": {},
	"br": {}, "bz2": {}, "cab": {}, "deb": {}, "dmg": {}, "epub": {},
	"gz": {}, "jar": {}, "lz4": {}, "lzma": {}, "lzo": {}, "rar": {},
	"rpm": {}, "tar": {}, "tgz": {}, "war": {}, "xz": {}, "zip": {},
	"zst": {},
	// Media Files
	"mp4": {}, "mkv": {}, "avi": {}, "mov": {}, "webm": {},
	"jpg": {}, "jpeg": {}, "png": {}, "gif": {}, "webp": {},
	"mp3": {}, "ogg": {}, "flac": {},
	// Compressed Documents
	"pdf": {}, "docx": {}, "xlsx": {}, "pptx": {},
}

// IsPreCompressedExtension reports whether the given filename extension belongs to an already compressed format.
func IsPreCompressedExtension(filename string) bool {
	ext := path.Ext(filename)
	if ext == "" {
		return false
	}
	cleanExt := strings.ToLower(strings.TrimPrefix(ext, "."))
	_, ok := DefaultStoreExtensions[cleanExt]
	return ok
}

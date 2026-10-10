// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package util provides CLI helper utilities for container detection and byte formatting.
package util

import (
	"fmt"
	"strings"

	"github.com/lemon4ksan/seal/container"
)

// DetectContainerType infers the archive container type from the file path extension.
func DetectContainerType(path string) container.Type {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".eon"):
		return container.Eon
	case strings.HasSuffix(lower, ".7z"):
		return container.SevenZip
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return container.TarGz
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"):
		return container.TarXz
	case strings.HasSuffix(lower, ".tar.zst"), strings.HasSuffix(lower, ".tzst"):
		return container.TarZst
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return container.TarBz2
	case strings.HasSuffix(lower, ".tar"):
		return container.Tar
	default:
		return container.Zip
	}
}

// FormatBytes formats a byte count into a human-readable string (B, KiB, MiB, GiB).
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cib", float64(b)/float64(div), "KMGTPE"[exp])
}

// ParseByteSize parses human-readable byte sizes like "64m", "256m", "1g", "32k", or "1048576" into bytes.
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty byte size")
	}
	multiplier := int64(1)
	if strings.HasSuffix(s, "g") || strings.HasSuffix(s, "gb") || strings.HasSuffix(s, "gib") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimRight(s, "gib")
	} else if strings.HasSuffix(s, "m") || strings.HasSuffix(s, "mb") || strings.HasSuffix(s, "mib") {
		multiplier = 1024 * 1024
		s = strings.TrimRight(s, "mib")
	} else if strings.HasSuffix(s, "k") || strings.HasSuffix(s, "kb") || strings.HasSuffix(s, "kib") {
		multiplier = 1024
		s = strings.TrimRight(s, "kib")
	} else if strings.HasSuffix(s, "b") {
		s = strings.TrimSuffix(s, "b")
	}

	var val int64
	_, err := fmt.Sscanf(s, "%d", &val)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", s, err)
	}
	return val * multiplier, nil
}


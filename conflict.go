// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

// DefaultConflictHandler overwrites existing entries (Last-Write-Wins strategy).
func DefaultConflictHandler(_, _ *File) (ConflictAction, string) {
	return ActionReplace, ""
}

// SkipConflictHandler ignores incoming duplicate entries and preserves the existing entry.
func SkipConflictHandler(_, _ *File) (ConflictAction, string) {
	return ActionSkip, ""
}

// ErrorConflictHandler rejects duplicate entries with an [ErrDuplicateEntry] error.
func ErrorConflictHandler(_, _ *File) (ConflictAction, string) {
	return ActionError, ""
}

// UpdateConflictHandler replaces the existing entry only if the incoming file has a newer modification timestamp.
func UpdateConflictHandler(existing, incoming *File) (ConflictAction, string) {
	if incoming.ModTime().After(existing.ModTime()) {
		return ActionReplace, ""
	}
	return ActionSkip, ""
}

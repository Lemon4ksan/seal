// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vfs

import "github.com/lemon4ksan/foundation/vfs"

// Type aliases re-exporting the foundation/vfs engine.
type (
	VFSEntry       = vfs.VFSEntry
	EntryProvider  = vfs.EntryProvider
	FS             = vfs.FS
	TreeNode       = vfs.TreeNode
	ResourceLimits = vfs.ResourceLimits
	SecureWriter   = vfs.SecureWriter
)

// Error variables re-exported from foundation/vfs.
var (
	ErrInsecurePath  = vfs.ErrInsecurePath
	ErrResourceLimit = vfs.ErrResourceLimit
)

// Function re-exports from foundation/vfs.
var (
	NewFS           = vfs.NewFS
	CleanPath       = vfs.CleanPath
	SafePath        = vfs.SafePath
	NewTreeNode     = vfs.NewTreeNode
	BuildTree       = vfs.BuildTree
	RenderTree      = vfs.RenderTree
	NewSecureWriter = vfs.NewSecureWriter
)

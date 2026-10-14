// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (amd64 || arm || arm64) && !appengine && gc && !noasm
// +build amd64 arm arm64
// +build !appengine
// +build gc
// +build !noasm

package lz4block

//go:noescape
func decodeBlock(dst, src, dict []byte) int

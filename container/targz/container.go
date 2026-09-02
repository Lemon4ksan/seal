// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package targz

import (
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/tar"
)

// Engine implements [container.Container] for GZIP-compressed TAR archives (.tar.gz, .tgz).
type Engine = tar.Engine

// New constructs a new GZIP TAR [container.Container] engine.
func New() container.Container {
	return tar.NewFormat(container.TarGz)
}

// NewEngine constructs a new concrete [*tar.Engine] instance.
func NewEngine() *tar.Engine {
	return tar.NewFormat(container.TarGz)
}

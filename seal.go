// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"fmt"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/tar"
	"github.com/lemon4ksan/seal/container/targz"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/option"
)

// Version represents the current release semver string of Seal.
const Version = "1.0.0"

// DefaultEngine resolves standard built-in container engines by type without global state.
func DefaultEngine(t ContainerType) (Container, error) {
	switch t {
	case container.Zip:
		return zip.New(), nil
	case container.Tar:
		return tar.New(), nil
	case container.TarGz:
		return targz.New(), nil
	default:
		return nil, fmt.Errorf("seal: unsupported container type %q", t)
	}
}

// NewArchive constructs a new universal [Archive] instance with the specified options.
// If no container engine is explicitly provided, it defaults to [container.Zip].
func NewArchive(opts ...Option) *Archive {
	var explicitEngine container.Container
	for _, opt := range opts {
		if opt.Kind == option.OptContainer || opt.Kind == option.OptFormat {
			if eng, err := DefaultEngine(opt.Container); err == nil {
				explicitEngine = eng
			}
		}
	}

	if explicitEngine == nil {
		explicitEngine = zip.New()
	}

	opts = append([]Option{archive.WithEngine(explicitEngine)}, opts...)
	return archive.New(opts...)
}

// NewZip constructs a new [Archive] configured specifically for the ZIP container.
func NewZip(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithEngine(zip.New())}, opts...)...)
}

// NewTar constructs a new [Archive] configured specifically for the TAR container.
func NewTar(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithEngine(tar.New())}, opts...)...)
}

// NewTarGz constructs a new [Archive] configured specifically for the TAR.GZ container.
func NewTarGz(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithEngine(targz.New())}, opts...)...)
}

// NewFile constructs a standalone [File] entry.
func NewFile(name string, isDir bool) (*File, error) {
	return file.New(name, isDir)
}

// RegisterContainer registers a [Container] backend for a specific container type.
func RegisterContainer(containerType ContainerType, factory container.Factory) {
	container.Register(containerType, factory)
}

// GetContainer returns a new [Container] instance for the requested container type.
func GetContainer(containerType ContainerType) (Container, error) {
	return DefaultEngine(containerType)
}

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"fmt"

	"github.com/lemon4ksan/foundation/generic"

	"github.com/lemon4ksan/seal/archive"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/container/eon"
	"github.com/lemon4ksan/seal/container/rar"
	"github.com/lemon4ksan/seal/container/sevenzip"
	"github.com/lemon4ksan/seal/container/tar"
	"github.com/lemon4ksan/seal/container/targz"
	"github.com/lemon4ksan/seal/container/wim"
	"github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/option"
)

// Version represents the current release semver string of Seal.
const Version = "0.1.0"

// ErrArchiveFrozen indicates that archive creation was gracefully frozen on context cancellation,
// producing a valid partial archive with an embedded stealth checkpoint.
var ErrArchiveFrozen = container.ErrArchiveFrozen

// DefaultContainer resolves standard built-in container engines by type without global state.
func DefaultContainer(t ContainerType) (Container, error) {
	switch t {
	case container.Eon:
		return eon.New(), nil
	case container.Zip:
		return zip.New(), nil
	case container.Tar:
		return tar.New(), nil
	case container.TarGz:
		return targz.New(), nil
	case container.TarBz2:
		return tar.NewFormat(container.TarBz2), nil
	case container.TarXz:
		return tar.NewFormat(container.TarXz), nil
	case container.TarZst:
		return tar.NewFormat(container.TarZst), nil
	case container.SevenZip:
		return sevenzip.New(), nil
	case container.Wim:
		return wim.New(), nil
	case container.Rar:
		return rar.New(), nil
	default:
		return nil, fmt.Errorf("seal: unsupported container type %q", t)
	}
}

// NewArchive constructs a new universal [Archive] instance with the specified options.
// If no container engine is explicitly provided, it defaults to [container.Zip].
func NewArchive(opts ...Option) *Archive {
	var explicitEngine container.Container
	if opt, ok := generic.Find(opts, func(o Option) bool { return o.Kind == option.OptContainer }); ok {
		if eng, err := DefaultContainer(opt.Container); err == nil {
			explicitEngine = eng
		}
	}

	if explicitEngine == nil {
		explicitEngine = zip.New()
	}

	opts = append([]Option{archive.WithContainer(explicitEngine)}, opts...)
	return archive.New(opts...)
}

// NewZip constructs a new [Archive] configured specifically for the ZIP container.
func NewZip(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithContainer(zip.New())}, opts...)...)
}

// NewTar constructs a new [Archive] configured specifically for the TAR container.
func NewTar(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithContainer(tar.New())}, opts...)...)
}

// NewTarGz constructs a new [Archive] configured specifically for the TAR.GZ container.
func NewTarGz(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithContainer(targz.New())}, opts...)...)
}

// NewTarBz2 constructs a new [Archive] configured specifically for the TAR.BZ2 container.
func NewTarBz2(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithContainer(tar.NewFormat(container.TarBz2))}, opts...)...)
}

// NewTarXz constructs a new [Archive] configured specifically for the TAR.XZ container.
func NewTarXz(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithContainer(tar.NewFormat(container.TarXz))}, opts...)...)
}

// NewTarZst constructs a new [Archive] configured specifically for the TAR.ZST container.
func NewTarZst(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithContainer(tar.NewFormat(container.TarZst))}, opts...)...)
}

// New7z constructs a new [Archive] configured specifically for the 7-Zip container.
func New7z(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithContainer(sevenzip.New())}, opts...)...)
}

// NewEon constructs a new [Archive] configured specifically for the EON container (.eon).
func NewEon(opts ...Option) *Archive {
	return NewArchive(append([]Option{archive.WithContainer(eon.New())}, opts...)...)
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
	return DefaultContainer(containerType)
}

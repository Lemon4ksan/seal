// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tar

import (
	"bytes"
	"compress/bzip2"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"github.com/lemon4ksan/foundation/codec/compress/gzip"
	"github.com/lemon4ksan/foundation/codec/compress/lzma"
	"github.com/lemon4ksan/foundation/codec/compress/zstd"

	"github.com/lemon4ksan/seal/codec"
	"github.com/lemon4ksan/seal/container"
	"github.com/lemon4ksan/seal/file"
	"github.com/lemon4ksan/seal/vfs"
)

func init() {
	container.Register(container.Tar, func() container.Container { return NewFormat(container.Tar) })
	container.Register(container.TarGz, func() container.Container { return NewFormat(container.TarGz) })
	container.Register(container.TarBz2, func() container.Container { return NewFormat(container.TarBz2) })
	container.Register(container.TarXz, func() container.Container { return NewFormat(container.TarXz) })
	container.Register(container.TarZst, func() container.Container { return NewFormat(container.TarZst) })
}

// Engine implements [container.Container] for TAR and its compressed variants (.tar, .tar.gz, .tar.bz2, .tar.xz, .tar.zst).
type Engine struct {
	format container.Type
}

// New constructs a new uncompressed TAR [container.Container] engine.
func New() container.Container {
	return &Engine{format: container.Tar}
}

// NewEngine constructs a new concrete [*Engine] instance for plain TAR.
func NewEngine() *Engine {
	return &Engine{format: container.Tar}
}

// NewFormat constructs a [container.Container] engine for a specific TAR family format.
func NewFormat(format container.Type) *Engine {
	return &Engine{format: format}
}

// Type returns the container format identifier.
func (e *Engine) Type() container.Type {
	if e.format == "" {
		return container.Tar
	}
	return e.format
}

// OpenCatalog reads the TAR archive from a random-access source by streaming it.
func (e *Engine) OpenCatalog(
	rAt io.ReaderAt,
	size int64,
	password string,
	reg *codec.Registry,
) ([]*file.File, string, error) {
	if rAt == nil || size < 0 {
		return nil, "", errors.New("tar: invalid source reader")
	}
	return e.ScanStream(io.NewSectionReader(rAt, 0, size), password, reg)
}

// ScanStream sequentially parses TAR headers and entries from a stream.
func (e *Engine) ScanStream(
	r io.Reader,
	_ string,
	_ *codec.Registry,
) ([]*file.File, string, error) {
	if r == nil {
		return nil, "", errors.New("tar: nil stream reader")
	}
	rawReader, closer, err := e.wrapDecompressor(r)
	if err != nil {
		return nil, "", err
	}
	if closer != nil {
		defer closer.Close()
	}

	tr := NewReader(rawReader)
	var files []*file.File
	var extData string

	for {
		h, payloadR, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, "", err
		}

		if ext, ok := h.PAXRecords["SEAL.EXT"]; ok && extData == "" {
			if raw, err := base64.StdEncoding.DecodeString(ext); err == nil {
				extData = string(raw)
			}
		}

		cleanName := vfs.CleanPath(h.Name)
		isDir := h.Typeflag == TypeDir || strings.HasSuffix(cleanName, "/")
		if isDir && !strings.HasSuffix(cleanName, "/") {
			cleanName += "/"
		}

		var data []byte
		if !isDir && h.Size > 0 {
			buf, err := io.ReadAll(payloadR)
			if err != nil {
				return nil, "", err
			}
			data = buf
		}

		f, err := file.New(cleanName, isDir)
		if err != nil {
			return nil, "", err
		}
		f.WithData(data).
			WithMode(h.Mode).
			WithModTime(h.ModTime)
		files = append(files, f)
	}

	return files, extData, nil
}

// WriteArchive serializes the entries into dest and returns the total bytes written.
func (e *Engine) WriteArchive(dest io.Writer, files []*file.File, cfg container.Config) (int64, error) {
	switch e.Type() {
	case container.TarXz:
		// Compress TAR payload via LZMA2 compressor
		var tarBuf bytes.Buffer
		countingW := &countingWriter{w: &tarBuf}
		tw := NewWriter(countingW)

		if err := writeEntries(tw, files, cfg); err != nil {
			return 0, err
		}
		if err := tw.Close(); err != nil {
			return 0, err
		}

		compressor := lzma.NewCompressor2(8 * 1024 * 1024)
		return compressor.Compress(&tarBuf, dest)

	case container.TarZst:
		zw, err := zstd.NewWriter(dest)
		if err != nil {
			return 0, err
		}
		countingW := &countingWriter{w: zw}
		tw := NewWriter(countingW)

		if err := writeEntries(tw, files, cfg); err != nil {
			_ = zw.Close()
			return countingW.n, err
		}
		if err := tw.Close(); err != nil {
			_ = zw.Close()
			return countingW.n, err
		}
		if err := zw.Close(); err != nil {
			return countingW.n, err
		}
		return countingW.n, nil

	case container.TarGz:
		level := cfg.DefaultLevel
		if level == 0 {
			level = gzip.DefaultCompression
		}
		gw, err := gzip.NewWriterLevel(dest, level)
		if err != nil {
			return 0, err
		}
		countingW := &countingWriter{w: gw}
		tw := NewWriter(countingW)

		if err := writeEntries(tw, files, cfg); err != nil {
			_ = gw.Close()
			return countingW.n, err
		}
		if err := tw.Close(); err != nil {
			_ = gw.Close()
			return countingW.n, err
		}
		if err := gw.Close(); err != nil {
			return countingW.n, err
		}
		return countingW.n, nil

	default:
		// Plain uncompressed TAR
		countingW := &countingWriter{w: dest}
		tw := NewWriter(countingW)

		if err := writeEntries(tw, files, cfg); err != nil {
			return countingW.n, err
		}
		if err := tw.Close(); err != nil {
			return countingW.n, err
		}
		return countingW.n, nil
	}
}

func writeEntries(tw *Writer, files []*file.File, cfg container.Config) error {
	for i, entry := range files {
		cleanName := vfs.CleanPath(entry.Name())
		typeFlag := TypeReg
		if entry.IsDir() {
			typeFlag = TypeDir
			if !strings.HasSuffix(cleanName, "/") {
				cleanName += "/"
			}
		}

		h := &Header{
			Name:     cleanName,
			Size:     entry.UncompressedSize(),
			Mode:     entry.Mode(),
			ModTime:  entry.ModTime(),
			Typeflag: typeFlag,
		}

		if i == 0 && len(cfg.ExtensionData) > 0 {
			if h.PAXRecords == nil {
				h.PAXRecords = make(map[string]string)
			}
			h.PAXRecords["SEAL.EXT"] = base64.StdEncoding.EncodeToString(cfg.ExtensionData)
		}

		if err := tw.WriteHeader(h); err != nil {
			if cfg.OnFileDone != nil {
				cfg.OnFileDone(entry, err)
			}
			return err
		}

		if !entry.IsDir() && entry.UncompressedSize() > 0 {
			r, err := entry.Open()
			if err != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(entry, err)
				}
				return err
			}
			_, writeErr := tw.WritePayload(r)
			_ = r.Close()
			if writeErr != nil {
				if cfg.OnFileDone != nil {
					cfg.OnFileDone(entry, writeErr)
				}
				return writeErr
			}
		}

		if cfg.OnFileDone != nil {
			cfg.OnFileDone(entry, nil)
		}
	}
	return nil
}

func (e *Engine) wrapDecompressor(r io.Reader) (io.Reader, io.Closer, error) {
	switch e.Type() {
	case container.TarGz:
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return gz, gz, nil
	case container.TarZst:
		zr, err := zstd.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return zr, io.NopCloser(zr), nil
	case container.TarXz:
		dec := lzma.NewDecompressor2(8 * 1024 * 1024)
		rc, err := dec.Decompress(r)
		if err != nil {
			return nil, nil, err
		}
		return rc, rc, nil
	case container.TarBz2:
		bz := bzip2.NewReader(r)
		return bz, io.NopCloser(bz), nil
	default:
		return r, nil, nil
	}
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += int64(n)
	return n, err
}

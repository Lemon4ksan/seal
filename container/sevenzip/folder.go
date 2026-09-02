// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/lemon4ksan/seal/codec/bcj"
	"github.com/lemon4ksan/seal/codec/bcj2"
	"github.com/lemon4ksan/seal/codec/delta"
	"github.com/lemon4ksan/seal/codec/lzma"
	"github.com/lemon4ksan/seal/codec/ppmd"
)

// ErrUnsupportedMethod indicates a 7z coder method ID not supported.
var ErrUnsupportedMethod = errors.New("sevenzip: unsupported compression method")

// DecodeFolderStream constructs a decoding pipeline for a solid 7z Folder block.
func DecodeFolderStream(folder *Folder, packedStreams []io.Reader, password string) (io.ReadCloser, error) {
	if len(folder.Coders) == 0 {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	// Check if this is a standard BCJ2 graph (4 streams: Main, Call, Jump, RC)
	// Coder 0: BCJ2 (4 in, 1 out)
	// Coders 1..3: LZMA/LZMA2 decompressors for streams 0..2, Stream 3 is raw RC
	if len(folder.Coders) == 4 && len(packedStreams) == 4 && folder.Coders[0].MethodID == MethodBCJ2 {
		r0, err := decodeSingleCoder(folder.Coders[1], packedStreams[0], nil, password)
		if err != nil {
			return nil, err
		}
		defer r0.Close()

		r1, err := decodeSingleCoder(folder.Coders[2], packedStreams[1], nil, password)
		if err != nil {
			return nil, err
		}
		defer r1.Close()

		r2, err := decodeSingleCoder(folder.Coders[3], packedStreams[2], nil, password)
		if err != nil {
			return nil, err
		}
		defer r2.Close()

		r3 := packedStreams[3] // RC stream is raw copy

		dec := bcj2.NewStreamDecoder(r0, r1, r2, r3)
		return io.NopCloser(dec), nil
	}

	// Linear coder pipeline (1 packed stream, N coders linked in sequence)
	// Outer coder (Coders[len-1]) -> ... -> Inner coder (Coders[0])
	if len(packedStreams) == 1 {
		var curr io.Reader
		curr = packedStreams[0]
		for i := len(folder.Coders) - 1; i >= 0; i-- {
			rc, err := decodeSingleCoder(folder.Coders[i], curr, folder.UnpackSizes, password)
			if err != nil {
				return nil, err
			}
			curr = rc
		}
		if closer, ok := curr.(io.ReadCloser); ok {
			return closer, nil
		}
		return io.NopCloser(curr), nil
	}

	return nil, fmt.Errorf("sevenzip: unsupported folder coder graph with %d coders", len(folder.Coders))
}

func decodeSingleCoder(coder CoderInfo, in io.Reader, unpackSizes []uint64, password string) (io.ReadCloser, error) {
	switch coder.MethodID {
	case MethodCopy:
		return io.NopCloser(in), nil

	case MethodLZMA:
		unpackSize := ^uint64(0)
		if len(unpackSizes) > 0 {
			unpackSize = unpackSizes[0]
		}
		var dictSize uint32 = 8 * 1024 * 1024
		var lc, lp, pb uint = 3, 0, 2

		if len(coder.Properties) >= 5 {
			h, err := lzma.ParseHeader(coder.Properties)
			if err == nil {
				dictSize = h.DictSize
				p := h.Properties
				pb = uint(p / (9 * 5))
				p %= 9 * 5
				lp = uint(p / 9)
				lc = uint(p % 9)
			}
		}

		dec := lzma.NewDecoderCore(lc, lp, pb, dictSize, unpackSize)
		var rd lzma.RangeDecoder
		if err := rd.Init(in); err != nil {
			return nil, err
		}
		var out bytes.Buffer
		if _, err := dec.DecodeStream(
			&rd,
			&out,
			unpackSize,
		); err != nil && !errors.Is(err, io.EOF) &&
			!errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, err
		}
		return io.NopCloser(&out), nil

	case MethodLZMA2:
		var dictSize uint32 = 8 * 1024 * 1024
		if len(coder.Properties) > 0 {
			d, err := lzma.DictSizeFromProp(coder.Properties[0])
			if err == nil {
				dictSize = d
			}
		}
		decomp := lzma.NewDecompressor2(dictSize)
		return decomp.Decompress(in)

	case MethodBCJ:
		return io.NopCloser(bcj.NewReader(in, bcj.X86)), nil

	case MethodDelta:
		distance := 1
		if len(coder.Properties) > 0 {
			distance = int(coder.Properties[0]) + 1
		}
		r, err := delta.NewReader(in, distance)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(r), nil

	case MethodPPMD:
		order := ppmd.DefaultOrder
		memSize := uint32(ppmd.DefaultMemSize)
		if len(coder.Properties) >= 5 {
			props, err := ppmd.ParseProperties(coder.Properties)
			if err == nil {
				order = props.Order
				memSize = props.MemSize
			}
		}
		unpackSize := ^uint64(0)
		if len(unpackSizes) > 0 {
			unpackSize = unpackSizes[0]
		}
		decomp := ppmd.NewDecompressor(order, memSize)
		return decomp.Decompress(in, unpackSize)

	case MethodAES:
		if password == "" {
			return nil, ErrInvalidPassword
		}
		decR, err := DecryptStream(in, password, coder.Properties)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(decR), nil

	default:
		return nil, fmt.Errorf("%w: ID=0x%X", ErrUnsupportedMethod, coder.MethodID)
	}
}

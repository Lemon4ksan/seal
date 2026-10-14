// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/lemon4ksan/seal/container/eon/codec"
	"github.com/lemon4ksan/seal/container/eon/spec"
)

// ErrInvalidPipelineDescriptor indicates a malformed pipeline header.
var ErrInvalidPipelineDescriptor = errors.New("filter: invalid pipeline descriptor")

// Stage describes an individual filter step within a pipeline.
type Stage struct {
	FilterID uint8
	Params   []byte
}

// Pipeline orchestrates a sequence of pre-filters followed by a compression codec.
type Pipeline struct {
	Stages     []Stage
	CodecID    uint8
	CodecLevel uint8
}

// NewPipeline creates a new chunk compression pipeline.
func NewPipeline(codecID, codecLevel uint8, stages ...Stage) *Pipeline {
	return &Pipeline{
		Stages:     stages,
		CodecID:    codecID,
		CodecLevel: codecLevel,
	}
}

// AddStage appends a filter stage with optional parameters.
func (p *Pipeline) AddStage(filterID uint8, params ...byte) *Pipeline {
	p.Stages = append(p.Stages, Stage{
		FilterID: filterID,
		Params:   params,
	})
	return p
}

// MarshalDescriptor serializes the binary pipeline header.
// Layout:
//   - 4 bytes: Magic 'PIPE' (0x45504950 LE)
//   - 1 byte:  StageCount (1..255)
//   - For each stage:
//   - 1 byte: FilterID
//   - 1 byte: ParamLen
//   - N bytes: Params
func (p *Pipeline) MarshalDescriptor() []byte {
	var buf bytes.Buffer
	buf.Write(spec.MagicPipeline[:])
	buf.WriteByte(uint8(len(p.Stages)))
	for _, s := range p.Stages {
		buf.WriteByte(s.FilterID)
		buf.WriteByte(uint8(len(s.Params)))
		if len(s.Params) > 0 {
			buf.Write(s.Params)
		}
	}
	return buf.Bytes()
}

// UnmarshalDescriptor parses a binary pipeline header from payload.
// Returns the parsed stages and the offset where the compressed payload starts.
func UnmarshalDescriptor(payload []byte) ([]Stage, int, error) {
	if len(payload) < 5 {
		return nil, 0, fmt.Errorf("%w: payload too short (%d bytes)", ErrInvalidPipelineDescriptor, len(payload))
	}
	if binary.LittleEndian.Uint32(payload[0:4]) != spec.MagicPipelineLE {
		return nil, 0, fmt.Errorf("%w: magic mismatch", ErrInvalidPipelineDescriptor)
	}

	count := int(payload[4])
	offset := 5
	stages := make([]Stage, 0, count)

	for i := 0; i < count; i++ {
		if offset+2 > len(payload) {
			return nil, 0, fmt.Errorf("%w: truncated stage %d header", ErrInvalidPipelineDescriptor, i)
		}
		fID := payload[offset]
		pLen := int(payload[offset+1])
		offset += 2
		if offset+pLen > len(payload) {
			return nil, 0, fmt.Errorf("%w: truncated stage %d params", ErrInvalidPipelineDescriptor, i)
		}
		var params []byte
		if pLen > 0 {
			params = make([]byte, pLen)
			copy(params, payload[offset:offset+pLen])
			offset += pLen
		}
		stages = append(stages, Stage{
			FilterID: fID,
			Params:   params,
		})
	}

	return stages, offset, nil
}

// Encode executes all forward filter stages in sequence, compresses the result using the codec,
// and prepends the binary PipelineDescriptor header.
func (p *Pipeline) Encode(raw []byte) ([]byte, error) {
	buf := raw
	for _, stage := range p.Stages {
		f, err := Get(stage.FilterID)
		if err != nil {
			return nil, fmt.Errorf("filter 0x%02X lookup: %w", stage.FilterID, err)
		}
		buf, err = f.Encode(buf, stage.Params)
		if err != nil {
			return nil, fmt.Errorf("filter %s (0x%02X) encode: %w", f.Name(), stage.FilterID, err)
		}
	}

	c, err := codec.Get(p.CodecID)
	if err != nil {
		return nil, fmt.Errorf("pipeline codec 0x%02X lookup: %w", p.CodecID, err)
	}

	compressed, err := c.Compress(buf, int(p.CodecLevel))
	if err != nil {
		return nil, fmt.Errorf("pipeline codec %s compress: %w", c.Name(), err)
	}

	hdr := p.MarshalDescriptor()
	result := make([]byte, len(hdr)+len(compressed))
	copy(result, hdr)
	copy(result[len(hdr):], compressed)
	return result, nil
}

// Decode decodes a pipeline-encoded payload by reading the descriptor, decompressing
// via the specified codec, and executing all filter stages in reverse (LIFO) order.
func Decode(payload []byte, codecID uint8) ([]byte, error) {
	stages, offset, err := UnmarshalDescriptor(payload)
	if err != nil {
		return nil, err
	}

	c, err := codec.Get(codecID)
	if err != nil {
		return nil, fmt.Errorf("pipeline codec 0x%02X lookup: %w", codecID, err)
	}

	decompressed, err := c.Decompress(payload[offset:])
	if err != nil {
		return nil, fmt.Errorf("pipeline codec %s decompress: %w", c.Name(), err)
	}

	buf := decompressed
	for i := len(stages) - 1; i >= 0; i-- {
		stage := stages[i]
		f, err := Get(stage.FilterID)
		if err != nil {
			return nil, fmt.Errorf("filter 0x%02X lookup: %w", stage.FilterID, err)
		}
		buf, err = f.Decode(buf, stage.Params)
		if err != nil {
			return nil, fmt.Errorf("filter %s (0x%02X) decode: %w", f.Name(), stage.FilterID, err)
		}
	}

	return buf, nil
}

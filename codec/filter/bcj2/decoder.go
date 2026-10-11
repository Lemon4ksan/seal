// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bcj2

import (
	"encoding/binary"
	"errors"
	"io"

	"github.com/lemon4ksan/seal/internal/pool"
)

// Decoder implements the low-level BCJ2 state machine to reconstruct original bytecode from 4 streams.
type Decoder struct {
	Bufs   [NumStreams][]byte
	Dest   []byte
	state  uint32
	ip     uint32
	temp   uint32
	range_ uint32
	code   uint32
	probs  [2 + 256]uint16
}

// NewDecoder creates an initialized BCJ2 Decoder.
func NewDecoder() *Decoder {
	d := &Decoder{}
	d.Init()
	return d
}

// Init resets the decoder state machine.
func (d *Decoder) Init() {
	d.state = StreamRC
	d.ip = 0
	d.temp = 0
	d.range_ = 0
	d.code = 0
	for i := range d.probs {
		d.probs[i] = bitModelTotal >> 1
	}
}

// SetIP sets the starting instruction pointer base (default 0).
func (d *Decoder) SetIP(ip uint32) {
	d.ip = ip
}

// IsMaybeFinished checks if decoding appears complete based on internal state.
func (d *Decoder) IsMaybeFinished() bool {
	return d.state == StreamMain && d.code == 0
}

// Decode processes the input streams into Dest.
func (d *Decoder) Decode() error {
	v := d.temp

	// Prime range coder register from RC stream (5 bytes)
	if d.range_ <= 5 {
		d.state = decStateError
		for d.range_ != 5 {
			if d.range_ == 1 && d.code != 0 {
				return ErrData
			}
			if len(d.Bufs[StreamRC]) == 0 {
				d.state = StreamRC
				return nil
			}
			d.code = (d.code << 8) | uint32(d.Bufs[StreamRC][0])
			d.Bufs[StreamRC] = d.Bufs[StreamRC][1:]
			d.range_++
		}
		if d.code == 0xffffffff {
			return ErrData
		}
		d.range_ = 0xffffffff
	}

	state := d.state

	// Handle interrupted 32-bit stream read (CALL/JUMP offsets)
	if is32BitStream(state) {
		cur := d.Bufs[state]
		if len(cur) < 4 {
			return nil
		}
		val := binary.BigEndian.Uint32(cur)
		d.Bufs[state] = cur[4:]

		ip := d.ip + 4
		v = val - ip
		d.ip = ip
		state = decStateOrig0
	}

	// Handle interrupted writing to Dest
	if state >= decStateOrig0 && state <= decStateOrig3 {
		for {
			if len(d.Dest) == 0 {
				d.state = state
				d.temp = v
				return nil
			}
			d.Dest[0] = byte(v)
			d.Dest = d.Dest[1:]
			state++
			if state == decStateOrig3+1 {
				break
			}
			v >>= 8
		}
	}

	// Main loop
	for {
		if d.range_ < kTopValue {
			if len(d.Bufs[StreamRC]) == 0 {
				d.state = StreamRC
				d.temp = v
				return nil
			}
			d.range_ <<= 8
			d.code = (d.code << 8) | uint32(d.Bufs[StreamRC][0])
			d.Bufs[StreamRC] = d.Bufs[StreamRC][1:]
		}

		src := d.Bufs[StreamMain]
		dest := d.Dest
		limit := min(len(dest), len(src))

		var processed, srcPtr, destPtr int

		for srcPtr < limit {
			b := src[srcPtr]
			dest[destPtr] = b
			v = (v << 24) | uint32(b)
			srcPtr++
			destPtr++
			processed++

			if ((b + (0x100 - 0xe8)) & 0xfe) == 0 {
				break
			}
			if (v>>24) == 0x0f && (b&0xf0) == 0x80 {
				break
			}
		}

		d.Bufs[StreamMain] = src[srcPtr:]
		d.Dest = dest[destPtr:]
		d.ip += uint32(processed)

		if srcPtr == limit {
			if len(d.Bufs[StreamMain]) == 0 {
				d.state = StreamMain
			} else {
				d.state = decStateOrig
			}
			d.temp = v
			return nil
		}

		// Probability context
		prevByte := byte(v >> 24)
		c := ((v + 0x17) >> 6) & 1
		probIndex := ((0 - c) & uint32(prevByte)) + c + ((v >> 5) & 1)

		prob := d.probs[probIndex]
		bound := (d.range_ >> numBitModelTotalBits) * uint32(prob)

		if d.code < bound {
			d.range_ = bound
			d.probs[probIndex] = prob + ((bitModelTotal - prob) >> kNumMoveBits)
			continue
		}

		d.range_ -= bound
		d.code -= bound
		d.probs[probIndex] = prob - (prob >> kNumMoveBits)

		cj := (((v + 0x57) >> 6) & 1) + StreamCall

		curBuf := d.Bufs[cj]
		if len(curBuf) < 4 {
			d.state = cj
			break
		}

		val := binary.BigEndian.Uint32(curBuf)
		d.Bufs[cj] = curBuf[4:]

		ip := d.ip + 4
		val -= ip
		d.ip = ip

		dest = d.Dest
		if len(dest) < 4 {
			for i := 0; i < len(dest); i++ {
				dest[i] = byte(val)
				val >>= 8
			}
			d.temp = val
			d.state = decStateOrig0 + uint32(len(dest))
			d.Dest = nil
			break
		}

		binary.LittleEndian.PutUint32(dest, val)
		d.Dest = dest[4:]
	}

	if d.range_ < kTopValue && len(d.Bufs[StreamRC]) > 0 {
		d.range_ <<= 8
		d.code = (d.code << 8) | uint32(d.Bufs[StreamRC][0])
		d.Bufs[StreamRC] = d.Bufs[StreamRC][1:]
	}

	return nil
}

// StreamDecoder merges 4 streams into a single streaming io.Reader.
type StreamDecoder struct {
	dec        *Decoder
	ins        [NumStreams]io.Reader
	bufs       [NumStreams][]byte
	extraSizes [NumStreams]int
	readRes    [NumStreams]error
	outBuf     []byte
	outPos     int
	outEnd     int
}

// NewStreamDecoder constructs a BCJ2 decoder reading from 4 separate stream readers.
func NewStreamDecoder(main, call, jump, rc io.Reader) *StreamDecoder {
	sd := &StreamDecoder{
		dec: NewDecoder(),
		ins: [NumStreams]io.Reader{main, call, jump, rc},
	}
	const bufSize = 64 * 1024
	for i := range NumStreams {
		sd.bufs[i] = make([]byte, bufSize)
	}
	sd.outBuf = make([]byte, bufSize)
	return sd
}

func (sd *StreamDecoder) readInStream(streamIdx uint32, r io.Reader) error {
	validSlice := sd.dec.Bufs[streamIdx]
	validLen := len(validSlice)
	extraLen := sd.extraSizes[streamIdx]
	backing := sd.bufs[streamIdx]

	totalPreserved := validLen + extraLen
	if totalPreserved > 0 {
		fullLeftover := validSlice[:totalPreserved]
		copy(backing, fullLeftover)
	}

	n, err := r.Read(backing[totalPreserved:])
	total := totalPreserved + n

	newExtra := 0
	limit := total
	if is32BitStream(streamIdx) {
		newExtra = total & 3
		limit = total - newExtra
	}

	sd.extraSizes[streamIdx] = newExtra
	sd.dec.Bufs[streamIdx] = backing[:limit]

	if err != nil {
		if err == io.EOF && n > 0 {
			if is32BitStream(streamIdx) && total < 4 {
				return io.EOF
			}
			return nil
		}
		return err
	}
	return nil
}

// Read implements io.Reader to yield decompressed reconstructed bytes.
func (sd *StreamDecoder) Read(p []byte) (int, error) {
	if sd.outPos < sd.outEnd {
		n := copy(p, sd.outBuf[sd.outPos:sd.outEnd])
		sd.outPos += n
		return n, nil
	}

	sd.outPos = 0
	sd.outEnd = 0
	sd.dec.Dest = sd.outBuf

	for {
		startDestLen := len(sd.dec.Dest)
		if err := sd.dec.Decode(); err != nil {
			return 0, err
		}

		written := startDestLen - len(sd.dec.Dest)
		if written > 0 {
			sd.outEnd = written
			break
		}

		if sd.dec.state < NumStreams {
			idx := sd.dec.state
			if errors.Is(sd.readRes[idx], io.EOF) {
				return 0, io.EOF
			}

			if err := sd.readInStream(idx, sd.ins[idx]); err != nil {
				if errors.Is(err, io.EOF) {
					sd.readRes[idx] = io.EOF
				} else {
					return 0, err
				}
			}

			if len(sd.dec.Bufs[idx]) == 0 && errors.Is(sd.readRes[idx], io.EOF) {
				return 0, io.EOF
			}
		} else if written == 0 && sd.dec.state == decStateOrig {
			if len(sd.dec.Bufs[StreamMain]) == 0 {
				if !errors.Is(sd.readRes[StreamMain], io.EOF) {
					if err := sd.readInStream(StreamMain, sd.ins[StreamMain]); err != nil {
						if errors.Is(err, io.EOF) {
							sd.readRes[StreamMain] = io.EOF
						} else {
							return 0, err
						}
					}
				}
			}
			if len(sd.dec.Bufs[StreamMain]) == 0 {
				return 0, io.EOF
			}
		}
	}

	n := copy(p, sd.outBuf[sd.outPos:sd.outEnd])
	sd.outPos += n
	return n, nil
}

// DecodeStreams decodes the 4 BCJ2 input streams into dest.
func DecodeStreams(main, call, jump, rc io.Reader, dest io.Writer) error {
	sd := NewStreamDecoder(main, call, jump, rc)
	bufPtr := pool.Get64K()
	defer pool.Put64K(bufPtr)

	_, err := io.CopyBuffer(dest, sd, (*bufPtr))
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

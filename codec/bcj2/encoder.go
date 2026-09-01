// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bcj2

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	encRelLimitDefault = 0x0F << 24
	encFileUnlimited   = ^uint64(0) - 1
)

// Encoder implements the low-level BCJ2 encoding state machine splitting data into 4 streams.
type Encoder struct {
	Bufs          [NumStreams][]byte
	Src           []byte
	state         uint32
	finishMode    FinishMode
	flushRem      byte
	isFlushState  bool
	cache         byte
	range_        uint32
	low           uint64
	cacheSize     uint64
	ip64          uint64
	fileIp64      uint64
	fileSize64_m1 uint64
	relLimit      uint32
	tempTarget    uint32
	temp          [8]byte
	tempPos       uint32
	probs         [2 + 256]uint16
	context       byte
}

// NewEncoder creates an initialized BCJ2 Encoder.
func NewEncoder() *Encoder {
	e := &Encoder{}
	e.Init()
	return e
}

// Init resets the encoder state machine.
func (e *Encoder) Init() {
	e.state = encStateOrig
	e.finishMode = EncFinishModeContinue
	e.context = 0
	e.flushRem = 5
	e.isFlushState = false
	e.cache = 0
	e.range_ = 0xffffffff
	e.low = 0
	e.cacheSize = 1
	e.ip64 = 0
	e.fileIp64 = 0
	e.fileSize64_m1 = encFileUnlimited
	e.relLimit = encRelLimitDefault
	e.tempPos = 0
	for i := range e.probs {
		e.probs[i] = bitModelTotal >> 1
	}
}

// SetFileSize sets known input file size to limit jump boundaries.
func (e *Encoder) SetFileSize(size uint64) {
	if size > 0 {
		e.fileSize64_m1 = size - 1
	} else {
		e.fileSize64_m1 = encFileUnlimited
	}
}

// SetFinishMode sets the flush mode.
func (e *Encoder) SetFinishMode(m FinishMode) {
	e.finishMode = m
}

// IsFinished reports whether all data and Range Coder bytes have been flushed.
func (e *Encoder) IsFinished() bool {
	return e.flushRem == 0
}

func (e *Encoder) shiftLow() bool {
	low := uint32(e.low)
	high := uint32(e.low >> 32)

	if low < 0xff000000 || high != 0 {
		buf := e.Bufs[StreamRC]
		for {
			if len(buf) == 0 {
				e.state = StreamRC
				e.Bufs[StreamRC] = nil
				return true
			}
			buf[0] = e.cache + byte(high)
			buf = buf[1:]
			e.cache = 0xff
			e.cacheSize--
			if e.cacheSize == 0 {
				break
			}
		}
		e.Bufs[StreamRC] = buf
		e.cache = byte(low >> 24)
	}
	e.cacheSize++
	e.low = uint64(low << 8)
	return false
}

// Encode drives the encoding state machine.
func (e *Encoder) Encode() {
	if e.tempPos > 0 {
		origSrc := e.Src
		origFinishMode := e.finishMode
		extra := 0

		for {
			srcPtr := origSrc
			if len(srcPtr) > 0 {
				e.finishMode = EncFinishModeContinue
			}
			e.Src = e.temp[:e.tempPos]
			e.encode2()

			consumed := int(len(e.temp[:e.tempPos]) - len(e.Src))
			remaining := int(e.tempPos) - consumed
			copy(e.temp[:], e.temp[consumed:e.tempPos])
			e.tempPos = uint32(remaining)

			e.Src = srcPtr
			e.finishMode = origFinishMode

			if e.state != encStateOrig {
				if extra >= remaining {
					extra = remaining
				}
				e.Src = origSrc[len(origSrc)-extra:]
				return
			}

			if len(origSrc) == 0 {
				return
			}

			if extra >= int(e.tempPos) {
				e.Src = origSrc[extra:]
				e.tempPos = 0
				break
			}

			e.temp[e.tempPos] = origSrc[0]
			e.tempPos++
			origSrc = origSrc[1:]
			extra++
		}
	}

	e.encode2()

	if e.state == encStateOrig {
		rem := len(e.Src)
		for i := 0; i < rem; i++ {
			e.temp[i] = e.Src[i]
		}
		e.tempPos = uint32(rem)
		e.Src = e.Src[rem:]
	}
}

func (e *Encoder) encode2() {
	if !e.isFlushState {
		state := e.state
		if is32BitStream(state) {
			buf := e.Bufs[state]
			if len(buf) < 4 {
				return
			}
			binary.BigEndian.PutUint32(buf, e.tempTarget)
			e.Bufs[state] = buf[4:]
		}

		e.state = encStateOrig
		v := uint32(e.context)

		for {
			if e.range_ < kTopValue {
				e.context = byte(v)
				if e.shiftLow() {
					return
				}
				e.range_ <<= 8
				v = uint32(e.context)
			}

			src := e.Src
			dest := e.Bufs[StreamMain]
			limit := min(len(dest), len(src))

			srcPtr := 0
			destPtr := 0

			for srcPtr < limit {
				b := src[srcPtr]
				dest[destPtr] = b
				v = (v << 24) | uint32(b)

				srcPtr++
				destPtr++

				if ((b + (0x100 - 0xe8)) & 0xfe) == 0 {
					break
				}
				if (v>>24) == 0x0f && (b&0xf0) == 0x80 {
					break
				}
			}

			processed := srcPtr
			e.ip64 += uint64(processed)
			e.Bufs[StreamMain] = dest[destPtr:]
			e.Src = src[processed:]

			if srcPtr == limit {
				e.context = byte(v)
				if len(e.Src) > 0 {
					e.state = StreamMain
					return
				}
				if e.finishMode != EncFinishModeEndStream {
					return
				}
				e.isFlushState = true
				break
			}

			convFlag := false
			var rel uint32

			if len(e.Src) >= 4 {
				rel = binary.LittleEndian.Uint32(e.Src)
				ip := e.ip64 - e.fileIp64
				checkVal := ((v + 0x20) >> 5) & 1

				if ip > uint64(checkVal) {
					absPos := int64(ip) + 4 + int64(int32(rel))
					if uint64(absPos) <= e.fileSize64_m1 {
						t := uint32(int32(rel) + int32(e.relLimit))
						if (t >> 1) < e.relLimit {
							convFlag = true
						}
					}
				}
			} else if e.finishMode == EncFinishModeContinue {
				e.ip64--
				e.Bufs[StreamMain] = dest[destPtr-1:]
				e.Src = src[processed-1:]
				v >>= 24
				e.context = byte(v)
				return
			}

			prevByte := byte(v >> 24)
			c := ((v + 0x17) >> 6) & 1
			probIndex := ((0 - c) & uint32(prevByte)) + c + ((v >> 5) & 1)

			prob := e.probs[probIndex]
			bound := (e.range_ >> numBitModelTotalBits) * uint32(prob)

			if !convFlag {
				e.range_ = bound
				e.probs[probIndex] = prob + ((bitModelTotal - prob) >> kNumMoveBits)
				continue
			}

			e.low += uint64(bound)
			e.range_ -= bound
			e.probs[probIndex] = prob - (prob >> kNumMoveBits)

			cj := (((v + 0x57) >> 6) & 1) + StreamCall
			ip := e.ip64 + 4
			e.ip64 = ip
			e.Src = e.Src[4:]

			abs := uint32(ip) + rel
			buf := e.Bufs[cj]
			if len(buf) == 0 {
				e.state = cj
				e.tempTarget = abs
				e.context = byte(v)
				return
			}
			binary.BigEndian.PutUint32(buf, abs)
			e.Bufs[cj] = buf[4:]
		}
	}

	for e.flushRem != 0 {
		if e.shiftLow() {
			return
		}
		e.flushRem--
	}
	e.state = encStateFinished
}

// EncodeStreams reads data from src and encodes it into the 4 BCJ2 output stream writers.
func EncodeStreams(src io.Reader, main, call, jump, rc io.Writer) error {
	enc := NewEncoder()
	enc.SetFinishMode(EncFinishModeContinue)

	const bufSize = 64 * 1024
	bufs := [NumStreams][]byte{
		make([]byte, bufSize),
		make([]byte, bufSize),
		make([]byte, bufSize),
		make([]byte, bufSize),
	}
	inBuf := make([]byte, bufSize)
	outs := [NumStreams]io.Writer{main, call, jump, rc}

	for i := range NumStreams {
		enc.Bufs[i] = bufs[i]
	}

	readDone := false

	for {
		enc.Encode()

		if enc.IsFinished() {
			break
		}

		if enc.state < NumStreams {
			idx := enc.state
			written := len(bufs[idx]) - len(enc.Bufs[idx])
			if written > 0 {
				if _, err := outs[idx].Write(bufs[idx][:written]); err != nil {
					return err
				}
			}
			enc.Bufs[idx] = bufs[idx]
		} else {
			if enc.state != encStateOrig {
				return errors.New("bcj2: encoder internal error")
			}

			if !readDone {
				n, err := src.Read(inBuf)
				if n > 0 {
					enc.Src = inBuf[:n]
				}
				if err != nil {
					readDone = true
					if err != io.EOF {
						return err
					}
					enc.SetFinishMode(EncFinishModeEndStream)
				} else if n == 0 {
					readDone = true
					enc.SetFinishMode(EncFinishModeEndStream)
				}
			} else {
				enc.SetFinishMode(EncFinishModeEndStream)
			}
		}
	}

	// Final flush
	for i := range NumStreams {
		written := len(bufs[i]) - len(enc.Bufs[i])
		if written > 0 {
			if _, err := outs[i].Write(bufs[i][:written]); err != nil {
				return err
			}
		}
	}

	return nil
}

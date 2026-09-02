// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lzma

import (
	"encoding/binary"
	"math/bits"
	"unsafe"
)

// EncoderCore implements LZMA1 / LZMA2 stream compression.
type EncoderCore struct {
	lc uint
	lp uint
	pb uint

	posMask    uint32
	lpMask     uint32
	prevShift  uint
	dictMask   uint32
	isPowerOf2 bool

	state int
	reps  [4]uint32

	isMatch     [kNumStates][kNumPosStatesMax]uint16
	isRep       [kNumStates]uint16
	isRepG0     [kNumStates]uint16
	isRepG1     [kNumStates]uint16
	isRepG2     [kNumStates]uint16
	isRep0Long  [kNumStates][kNumPosStatesMax]uint16
	posSlot     [kNumLenToPosStates][1 << (kNumPosSlotBits + 1)]uint16
	posDecoders [10][1 << 6]uint16
	posAlign    [1 << (kNumAlignBits + 1)]uint16

	lenDecoder    LenDecoder
	repLenDecoder LenDecoder
	litProbs      [][0x300]uint16

	dictSize uint32
	head     [1 << 16]int
	head2    [1 << 16]int
	prev     []int

	chainLimit     int
	fastBytesLimit int
}

// NewEncoderCore creates an LZMA encoder core.
func NewEncoderCore(lc, lp, pb uint, dictSize uint32) *EncoderCore {
	return NewEncoderCoreWithOptions(Options{
		Lc:          lc,
		Lp:          lp,
		Pb:          pb,
		DictSize:    dictSize,
		ChainLength: 16,
		FastBytes:   32,
	})
}

// NewEncoderCoreWithOptions creates an LZMA encoder core configured with custom options.
func NewEncoderCoreWithOptions(opts Options) *EncoderCore {
	dictSize := opts.DictSize
	if dictSize < 4096 {
		dictSize = 4096
	}
	prevSize := min(dictSize, 65536)
	dictMask := prevSize - 1
	isPowerOf2 := (prevSize & dictMask) == 0

	chainLen := opts.ChainLength
	if chainLen <= 0 {
		chainLen = 16
	}
	fastBytes := opts.FastBytes
	if fastBytes <= 0 {
		fastBytes = 32
	}

	e := &EncoderCore{
		lc:             opts.Lc,
		lp:             opts.Lp,
		pb:             opts.Pb,
		posMask:        (1 << opts.Pb) - 1,
		lpMask:         (1 << opts.Lp) - 1,
		prevShift:      8 - opts.Lc,
		dictSize:       prevSize,
		dictMask:       dictMask,
		isPowerOf2:     isPowerOf2,
		prev:           make([]int, prevSize),
		chainLimit:     chainLen,
		fastBytesLimit: fastBytes,
	}
	e.InitProbs()
	return e
}

// InitProbs initializes probability tables.
func (e *EncoderCore) InitProbs() {
	e.state = 0
	e.reps = [4]uint32{0, 0, 0, 0}

	for i := range e.isMatch {
		for j := range e.isMatch[i] {
			e.isMatch[i][j] = probInitVal
		}
	}
	for i := range e.isRep {
		e.isRep[i] = probInitVal
		e.isRepG0[i] = probInitVal
		e.isRepG1[i] = probInitVal
		e.isRepG2[i] = probInitVal
	}
	for i := range e.isRep0Long {
		for j := range e.isRep0Long[i] {
			e.isRep0Long[i][j] = probInitVal
		}
	}
	for i := range e.posSlot {
		for j := range e.posSlot[i] {
			e.posSlot[i][j] = probInitVal
		}
	}
	for i := range e.posDecoders {
		for j := range e.posDecoders[i] {
			e.posDecoders[i][j] = probInitVal
		}
	}
	for i := range e.posAlign {
		e.posAlign[i] = probInitVal
	}

	e.lenDecoder.Init()
	e.repLenDecoder.Init()

	numLit := 1 << (e.lc + e.lp)
	e.litProbs = make([][0x300]uint16, numLit)
	for i := range e.litProbs {
		for j := range e.litProbs[i] {
			e.litProbs[i][j] = probInitVal
		}
	}

	for i := range e.head {
		e.head[i] = -1
	}
	for i := range e.head2 {
		e.head2[i] = -1
	}
	for i := range e.prev {
		e.prev[i] = -1
	}
}

// Reset clears state and tables for buffer recycling without reallocating slices.
func (e *EncoderCore) Reset() {
	e.state = 0
	e.reps = [4]uint32{0, 0, 0, 0}

	for i := range e.isMatch {
		for j := range e.isMatch[i] {
			e.isMatch[i][j] = probInitVal
		}
	}
	for i := range e.isRep {
		e.isRep[i] = probInitVal
		e.isRepG0[i] = probInitVal
		e.isRepG1[i] = probInitVal
		e.isRepG2[i] = probInitVal
	}
	for i := range e.isRep0Long {
		for j := range e.isRep0Long[i] {
			e.isRep0Long[i][j] = probInitVal
		}
	}
	for i := range e.posSlot {
		for j := range e.posSlot[i] {
			e.posSlot[i][j] = probInitVal
		}
	}
	for i := range e.posDecoders {
		for j := range e.posDecoders[i] {
			e.posDecoders[i][j] = probInitVal
		}
	}
	for i := range e.posAlign {
		e.posAlign[i] = probInitVal
	}

	e.lenDecoder.Init()
	e.repLenDecoder.Init()

	for i := range e.litProbs {
		for j := range e.litProbs[i] {
			e.litProbs[i][j] = probInitVal
		}
	}

	for i := range e.head {
		e.head[i] = -1
	}
	for i := range e.head2 {
		e.head2[i] = -1
	}
	for i := range e.prev {
		e.prev[i] = -1
	}
}

func (e *EncoderCore) prevIdx(pos int) int {
	if e.isPowerOf2 {
		return pos & int(e.dictMask)
	}
	return pos % int(e.dictSize)
}

func (e *EncoderCore) getLitSubCoder(pos uint32, prevByte byte) *[0x300]uint16 {
	idx := ((pos & e.lpMask) << e.lc) + (uint32(prevByte) >> e.prevShift)
	return &e.litProbs[idx]
}

func getPosSlot(dist uint32) uint32 {
	if dist < 4 {
		return dist
	}
	slot := uint32(4)
	for {
		numDirectBits := (slot >> 1) - 1
		base := (2 | (slot & 1)) << numDirectBits
		limit := base + (1 << numDirectBits)
		if dist < limit {
			return slot
		}
		slot++
	}
}

func findMatchLength(a, b []byte) int {
	maxLen := min(len(a), len(b))
	if maxLen > kMatchMaxLen {
		maxLen = kMatchMaxLen
	}
	var i int
	for i+32 <= maxLen {
		diff0 := binary.LittleEndian.Uint64(a[i:]) ^ binary.LittleEndian.Uint64(b[i:])
		if diff0 != 0 {
			return i + (bits.TrailingZeros64(diff0) >> 3)
		}
		diff1 := binary.LittleEndian.Uint64(a[i+8:]) ^ binary.LittleEndian.Uint64(b[i+8:])
		if diff1 != 0 {
			return i + 8 + (bits.TrailingZeros64(diff1) >> 3)
		}
		diff2 := binary.LittleEndian.Uint64(a[i+16:]) ^ binary.LittleEndian.Uint64(b[i+16:])
		if diff2 != 0 {
			return i + 16 + (bits.TrailingZeros64(diff2) >> 3)
		}
		diff3 := binary.LittleEndian.Uint64(a[i+24:]) ^ binary.LittleEndian.Uint64(b[i+24:])
		if diff3 != 0 {
			return i + 24 + (bits.TrailingZeros64(diff3) >> 3)
		}
		i += 32
	}
	for i+8 <= maxLen {
		diff := binary.LittleEndian.Uint64(a[i:]) ^ binary.LittleEndian.Uint64(b[i:])
		if diff != 0 {
			return i + (bits.TrailingZeros64(diff) >> 3)
		}
		i += 8
	}
	for i < maxLen && a[i] == b[i] {
		i++
	}
	return i
}

// EncodeStream compresses src into re with local register pinning.
func (e *EncoderCore) EncodeStream(src []byte, re *RangeEncoder) error {
	var prevByte byte
	srcLen := len(src)

	low := re.low
	range_ := re.range_
	cacheSize := re.cacheSize
	cache := re.cache
	bufPos := re.pos
	buf := re.buf[:]
	w := re.w

	var encErr error

	writeByteFast := func(b byte) {
		if bufPos < len(buf) {
			buf[bufPos] = b
			bufPos++
			return
		}
		if bufPos > 0 {
			if _, err := w.Write(buf[:bufPos]); err != nil {
				encErr = err
				return
			}
			bufPos = 0
		}
		buf[0] = b
		bufPos = 1
	}

	shiftLowFast := func() {
		l := uint32(low)
		high := uint32(low >> 32)
		if l < 0xFF000000 || high != 0 {
			temp := cache
			for {
				writeByteFast(temp + byte(high))
				if encErr != nil {
					return
				}
				temp = 0xFF
				cacheSize--
				if cacheSize == 0 {
					break
				}
			}
			cache = byte(l >> 24)
		}
		cacheSize++
		low = uint64(l << 8)
	}

	encodeBitFast := func(prob *uint16, bit int) {
		p := uint32(*prob)
		bound := (range_ >> kNumBitModelTotalBits) * p
		if bit == 0 {
			range_ = bound
			*prob += uint16((kBitModelTotal - p) >> kNumMoveBits)
		} else {
			low += uint64(bound)
			range_ -= bound
			*prob -= uint16(p >> kNumMoveBits)
		}
		if range_ < kTopValue {
			range_ <<= 8
			shiftLowFast()
		}
	}

	encodeDirectBitsFast := func(val uint32, numBits int) {
		for i := numBits - 1; i >= 0; i-- {
			range_ >>= 1
			if ((val >> i) & 1) != 0 {
				low += uint64(range_)
			}
			if range_ < kTopValue {
				range_ <<= 8
				shiftLowFast()
			}
		}
	}

	encodeTreeFast := func(probs []uint16, numBits int, symbol uint32) {
		var m uint32 = 1
		for i := numBits - 1; i >= 0; i-- {
			bit := int((symbol >> i) & 1)
			encodeBitFast(&probs[m], bit)
			m = (m << 1) | uint32(bit)
		}
	}

	encodeReverseTreeFast := func(probs []uint16, numBits int, symbol uint32) {
		var m uint32 = 1
		for i := 0; i < numBits; i++ {
			bit := int((symbol >> i) & 1)
			encodeBitFast(&probs[m], bit)
			m = (m << 1) | uint32(bit)
		}
	}

	encodeLenFast := func(ld *LenDecoder, lenVal, posState int) {
		if lenVal < kLenNumLowSymbols {
			encodeBitFast(&ld.choice, 0)
			encodeTreeFast(ld.low[posState][:], kLenNumLowBits, uint32(lenVal))
		} else {
			encodeBitFast(&ld.choice, 1)
			if lenVal < kLenNumLowSymbols+kLenNumMidSymbols {
				encodeBitFast(&ld.choice2, 0)
				encodeTreeFast(ld.mid[posState][:], kLenNumMidBits, uint32(lenVal-kLenNumLowSymbols))
			} else {
				encodeBitFast(&ld.choice2, 1)
				encodeTreeFast(ld.high[:], kLenNumHighBits, uint32(lenVal-kLenNumLowSymbols-kLenNumMidSymbols))
			}
		}
	}

	isMatchPtr := unsafe.Pointer(&e.isMatch[0][0])
	isRepPtr := unsafe.Pointer(&e.isRep[0])
	isRepG0Ptr := unsafe.Pointer(&e.isRepG0[0])
	isRepG1Ptr := unsafe.Pointer(&e.isRepG1[0])
	isRepG2Ptr := unsafe.Pointer(&e.isRepG2[0])
	isRep0LongPtr := unsafe.Pointer(&e.isRep0Long[0][0])
	posSlotPtr := unsafe.Pointer(&e.posSlot[0][0])

	chainLimit := e.chainLimit
	if chainLimit <= 0 {
		chainLimit = 16
	}
	fastBytesLimit := e.fastBytesLimit
	if fastBytesLimit <= 0 {
		fastBytesLimit = 32
	}

	for pos := 0; pos < srcLen && encErr == nil; {
		posState := int(uint32(pos) & e.posMask)

		bestLen := 0
		bestDist := 0

		// 1. Repetition matches: Check if the current position repeats one of the last 4 distances.
		// Repetitions are extremely cheap in LZMA (1 to 3 bits), so we check them before searching the dictionary.
		for repIdx, dist := range e.reps {
			d := int(dist) + 1
			if pos >= d {
				matchLen := findMatchLength(src[pos:], src[pos-d:])
				if matchLen >= kMatchMinLen && matchLen > bestLen {
					bestLen = matchLen
					bestDist = -1 - repIdx
				}
			}
		}

		// 2. Dual-Hash 2-Byte Mini Table: Check for immediate 2-byte token matches.
		// In LZMA, a 2-byte match is only entropy-positive when distance is ultra-short (< 64 bytes).
		// For larger distances, encoding a 2-byte match costs more bits than emitting two literals.
		if pos+2 <= srcLen {
			u16 := binary.LittleEndian.Uint16(src[pos:])
			m2 := e.head2[u16]
			e.head2[u16] = pos
			if m2 >= 0 && pos-m2 < 64 && bestLen < 2 {
				if src[pos] == src[m2] && src[pos+1] == src[m2+1] {
					bestLen = 2
					bestDist = (pos - m2) - 1
				}
			}
		}

		// 3. 4-Byte Hash Chain Dictionary Search:
		// Uses multiplicative hashing and immediate 32-bit candidate rejection to avoid SIMD overhead on false positives.
		dictMask := int(e.dictMask)
		dictSize := int(e.dictSize)

		if pos+4 <= srcLen {
			u32 := binary.LittleEndian.Uint32(src[pos:])
			h := (u32 * 0x1E35A7BD) >> 16
			matchPos := e.head[h]
			e.head[h] = pos
			e.prev[pos&dictMask] = matchPos

			chainLen := 0
			for matchPos >= 0 && pos-matchPos < dictSize && chainLen < chainLimit {
				d := pos - matchPos
				// Quick rejection check: verify first and last expected match bytes before invoking SIMD
				if bestLen >= 4 {
					if src[pos+bestLen-1] != src[matchPos+bestLen-1] || src[pos] != src[matchPos] {
						matchPos = e.prev[matchPos&dictMask]
						chainLen++
						continue
					}
				} else {
					if binary.LittleEndian.Uint32(src[pos:]) != binary.LittleEndian.Uint32(src[matchPos:]) {
						matchPos = e.prev[matchPos&dictMask]
						chainLen++
						continue
					}
				}
				matchLen := findMatchLength(src[pos:], src[matchPos:])
				if matchLen > bestLen {
					bestLen = matchLen
					bestDist = d - 1
					if bestLen >= fastBytesLimit {
						break
					}
				}
				matchPos = e.prev[matchPos&dictMask]
				chainLen++
			}
		} else if pos+kMatchMinLen <= srcLen {
			h := (int(src[pos]) | (int(src[pos+1]) << 8)) & 0xFFFF
			matchPos := e.head[h]
			e.head[h] = pos
			e.prev[pos&dictMask] = matchPos

			chainLen := 0
			for matchPos >= 0 && pos-matchPos < dictSize && chainLen < chainLimit {
				d := pos - matchPos
				matchLen := findMatchLength(src[pos:], src[matchPos:])
				if matchLen > bestLen {
					bestLen = matchLen
					bestDist = d - 1
					if bestLen >= fastBytesLimit {
						break
					}
				}
				matchPos = e.prev[matchPos&dictMask]
				chainLen++
			}
		}

		isMatchProb := (*uint16)(unsafe.Add(isMatchPtr, (e.state<<4+posState)*2))
		if bestLen < kMatchMinLen {
			// ShortRep: If the current byte matches the byte at reps[0], encode as a 1-byte repetition.
			// ShortRep costs only ~4 bits in the Range Coder compared to 8..9 bits for a literal.
			if pos >= int(e.reps[0])+1 && src[pos] == src[pos-int(e.reps[0])-1] {
				encodeBitFast(isMatchProb, 1)
				isRepProb := (*uint16)(unsafe.Add(isRepPtr, e.state*2))
				encodeBitFast(isRepProb, 1)
				isRepG0Prob := (*uint16)(unsafe.Add(isRepG0Ptr, e.state*2))
				encodeBitFast(isRepG0Prob, 0)
				isRep0LongProb := (*uint16)(unsafe.Add(isRep0LongPtr, (e.state<<4+posState)*2))
				encodeBitFast(isRep0LongProb, 0)

				e.state = stateUpdateShortRep(e.state)
				prevByte = src[pos]
				pos++
				continue
			}

			// Encode Literal byte using literal sub-coder tree
			encodeBitFast(isMatchProb, 0)
			probs := e.getLitSubCoder(uint32(pos), prevByte)
			curByte := src[pos]

			var m uint32 = 1
			if !stateIsChar(e.state) {
				matchByte := src[pos-int(e.reps[0])-1]
				offs := uint32(0x100)
				for i := 7; i >= 0; i-- {
					matchBit := int((matchByte >> i) & 1)
					bit := int((curByte >> i) & 1)
					probIdx := offs + (uint32(matchBit) << 8) + m
					encodeBitFast(&probs[probIdx], bit)
					m = (m << 1) | uint32(bit)
					if matchBit != bit {
						for j := i - 1; j >= 0; j-- {
							bit := int((curByte >> j) & 1)
							encodeBitFast(&probs[m], bit)
							m = (m << 1) | uint32(bit)
						}
						break
					}
				}
			} else {
				for i := 7; i >= 0; i-- {
					bit := int((curByte >> i) & 1)
					encodeBitFast(&probs[m], bit)
					m = (m << 1) | uint32(bit)
				}
			}

			prevByte = curByte
			e.state = stateUpdateLiteral(e.state)
			pos++
			continue
		}

		// 4. 1-Step Lookahead: If current match is relatively short (< 32 bytes), check pos+1.
		// If pos+1 yields a significantly longer match, taking a literal now and the long match next is optimal.
		if bestLen < 32 && pos+5 <= srcLen {
			nextU32 := binary.LittleEndian.Uint32(src[pos+1:])
			nextH := (nextU32 * 0x1E35A7BD) >> 16
			nextMatchPos := e.head[nextH]
			if nextMatchPos >= 0 && (pos+1)-nextMatchPos < int(e.dictSize) {
				nextLen := findMatchLength(src[pos+1:], src[nextMatchPos:])
				if nextLen > bestLen+1 {
					// Emitting literal for current byte yields higher net compression
					encodeBitFast(isMatchProb, 0)
					probs := e.getLitSubCoder(uint32(pos), prevByte)
					curByte := src[pos]
					var m uint32 = 1
					if !stateIsChar(e.state) {
						matchByte := src[pos-int(e.reps[0])-1]
						offs := uint32(0x100)
						for i := 7; i >= 0; i-- {
							matchBit := int((matchByte >> i) & 1)
							bit := int((curByte >> i) & 1)
							probIdx := offs + (uint32(matchBit) << 8) + m
							encodeBitFast(&probs[probIdx], bit)
							m = (m << 1) | uint32(bit)
							if matchBit != bit {
								for j := i - 1; j >= 0; j-- {
									bit := int((curByte >> j) & 1)
									encodeBitFast(&probs[m], bit)
									m = (m << 1) | uint32(bit)
								}
								break
							}
						}
					} else {
						for i := 7; i >= 0; i-- {
							bit := int((curByte >> i) & 1)
							encodeBitFast(&probs[m], bit)
							m = (m << 1) | uint32(bit)
						}
					}
					prevByte = curByte
					e.state = stateUpdateLiteral(e.state)
					pos++
					continue
				}
			}
		}

		// Encode Match / Repetition
		encodeBitFast(isMatchProb, 1)

		isRepProb := (*uint16)(unsafe.Add(isRepPtr, e.state*2))
		if bestDist < 0 {
			// Repetition
			repIdx := -bestDist - 1
			encodeBitFast(isRepProb, 1)

			isRepG0Prob := (*uint16)(unsafe.Add(isRepG0Ptr, e.state*2))
			if repIdx == 0 {
				encodeBitFast(isRepG0Prob, 0)
				isRep0LongProb := (*uint16)(unsafe.Add(isRep0LongPtr, (e.state<<4+posState)*2))
				encodeBitFast(isRep0LongProb, 1)
			} else {
				encodeBitFast(isRepG0Prob, 1)
				isRepG1Prob := (*uint16)(unsafe.Add(isRepG1Ptr, e.state*2))
				if repIdx == 1 {
					encodeBitFast(isRepG1Prob, 0)
				} else {
					encodeBitFast(isRepG1Prob, 1)
					isRepG2Prob := (*uint16)(unsafe.Add(isRepG2Ptr, e.state*2))
					if repIdx == 2 {
						encodeBitFast(isRepG2Prob, 0)
					} else {
						encodeBitFast(isRepG2Prob, 1)
					}
				}
			}

			lenVal := bestLen - kMatchMinLen
			encodeLenFast(&e.repLenDecoder, lenVal, posState)

			// Update Reps
			dist := e.reps[repIdx]
			for i := repIdx; i > 0; i-- {
				e.reps[i] = e.reps[i-1]
			}
			e.reps[0] = dist

			e.state = stateUpdateRep(e.state)
		} else {
			// Normal Match
			encodeBitFast(isRepProb, 0)

			lenVal := bestLen - kMatchMinLen
			encodeLenFast(&e.lenDecoder, lenVal, posState)

			dist := uint32(bestDist)
			posSlot := getPosSlot(dist)
			lenToPos := lenVal
			if lenToPos >= kNumLenToPosStates {
				lenToPos = kNumLenToPosStates - 1
			}

			subPosSlot := (*[1 << kNumPosSlotBits]uint16)(unsafe.Add(posSlotPtr, (lenToPos<<kNumPosSlotBits)*2))[:]
			encodeTreeFast(subPosSlot, kNumPosSlotBits, posSlot)

			if posSlot >= 4 {
				numDirectBits := int((posSlot >> 1) - 1)
				base := (2 | (posSlot & 1)) << numDirectBits
				directBits := dist - base

				if posSlot < 14 {
					subProbs := e.posDecoders[posSlot-4][:]
					encodeReverseTreeFast(subProbs, numDirectBits, directBits)
				} else {
					alignBits := directBits & ((1 << kNumAlignBits) - 1)
					directHigh := directBits >> kNumAlignBits
					encodeDirectBitsFast(directHigh, numDirectBits-kNumAlignBits)
					encodeReverseTreeFast(e.posAlign[:], kNumAlignBits, alignBits)
				}
			}

			// Update Reps
			e.reps[3] = e.reps[2]
			e.reps[2] = e.reps[1]
			e.reps[1] = e.reps[0]
			e.reps[0] = dist

			e.state = stateUpdateMatch(e.state)
		}

		prevByte = src[pos+bestLen-1]
		pos += bestLen
	}

	re.low = low
	re.range_ = range_
	re.cacheSize = cacheSize
	re.cache = cache
	re.pos = bufPos

	return encErr
}

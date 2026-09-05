// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package przm

import (
	"encoding/binary"
	"math/bits"
	"sync"

	"github.com/lemon4ksan/foundation/bufkit"
	"github.com/lemon4ksan/foundation/codec/filter/bcj"
	"github.com/lemon4ksan/foundation/silicon/simd"
)

const (
	hashTableSize = 65536
	hashTableMask = hashTableSize - 1
	minMatch      = 4
	maxLiteralRun = 64
	initPadSize   = 1024 * 1024
)

func ensureBufCap(b []byte, n int) []byte {
	if cap(b) < n {
		return bufkit.AlignedBytes(n, bufkit.CacheLineSize)[:0]
	}
	return b[:0]
}

type scratchpad struct {
	head         []int32
	cmdBuf       []byte
	offsetBuf    []byte
	literalBuf   []byte
	transBuf     []byte
	nibbleBuf    []byte
	nibbleLowBuf []byte
	splitBuf     []byte
}

var padPool = sync.Pool{
	New: func() any {
		return &scratchpad{
			head:         make([]int32, hashTableSize*2),
			cmdBuf:       bufkit.AlignedBytes(initPadSize, bufkit.CacheLineSize)[:0],
			offsetBuf:    bufkit.AlignedBytes(initPadSize/2, bufkit.CacheLineSize)[:0],
			literalBuf:   bufkit.AlignedBytes(initPadSize, bufkit.CacheLineSize)[:0],
			transBuf:     bufkit.AlignedBytes(initPadSize, bufkit.CacheLineSize)[:0],
			nibbleBuf:    bufkit.AlignedBytes(initPadSize, bufkit.CacheLineSize)[:0],
			nibbleLowBuf: bufkit.AlignedBytes(initPadSize/2, bufkit.CacheLineSize)[:0],
			splitBuf:     bufkit.AlignedBytes(initPadSize*2, bufkit.CacheLineSize)[:0],
		}
	},
}

type matchCandidate struct {
	mType      int // 1: Repeat, 2: Stride, 3: Ring8Ext, 4: ExplicitExt, 5: DeltaMaskExpl, 6: DeltaMaskRing8, 7: ShortDict, 8: ExtDict
	length     int
	distance   int
	ringK      int
	strideM    int
	dictTok    int
	cost       int
	score      int
	mismatches int
	diffOffs   [2]int
	diffXORs   [2]byte
}

func uvarintLen(x uint64) int {
	return (bits.Len64(x|1) + 6) / 7
}

// packNibblePlanes splits a literal byte slice into a Low-Nibble plane (packed 2 per byte)
// and a High-Nibble plane (runs of identical high nibbles).
// Returns the packed stream and true if packed size is at least 8% smaller than raw lits.
func packNibblePlanes(lits []byte, out []byte, lowBuf []byte) ([]byte, bool) {
	n := len(lits)
	if n < 32 {
		return nil, false
	}

	// 1. Pack Low-Nibble Plane (exactly ceil(n/2) bytes)
	numPairs := (n + 1) / 2
	if cap(lowBuf) < numPairs {
		lowBuf = make([]byte, numPairs)
	} else {
		lowBuf = lowBuf[:numPairs]
	}

	j := 0
	for ; j+4 <= numPairs && 2*j+7 < n; j += 4 {
		idx := j * 2
		b0 := lits[idx] & 0x0F
		b1 := lits[idx+1] & 0x0F
		b2 := lits[idx+2] & 0x0F
		b3 := lits[idx+3] & 0x0F
		b4 := lits[idx+4] & 0x0F
		b5 := lits[idx+5] & 0x0F
		b6 := lits[idx+6] & 0x0F
		b7 := lits[idx+7] & 0x0F

		lowBuf[j] = (b0 << 4) | b1
		lowBuf[j+1] = (b2 << 4) | b3
		lowBuf[j+2] = (b4 << 4) | b5
		lowBuf[j+3] = (b6 << 4) | b7
	}
	for ; j < numPairs; j++ {
		idx := j * 2
		b0 := lits[idx] & 0x0F
		b1 := uint8(0)
		if idx+1 < n {
			b1 = lits[idx+1] & 0x0F
		}
		lowBuf[j] = (b0 << 4) | b1
	}

	// 2. Pack High-Nibble Plane (Nibble-RLE tokens: (runLen-1)<<4 | highNibble)
	out = out[:0]
	// 8-byte header: numLits (uint32), lowBytes (uint32)
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(n))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(numPairs))
	out = append(out, hdr[:]...)
	out = append(out, lowBuf...)

	for i := 0; i < n; {
		h := lits[i] >> 4
		runLen := 1
		for i+runLen < n && (lits[i+runLen]>>4) == h && runLen < 16 {
			runLen++
		}
		token := (uint8(runLen-1) << 4) | (h & 0x0F)
		out = append(out, token)
		i += runLen
	}

	// Only accept if at least 8% smaller than raw
	if len(out) < int(float64(n)*0.92) {
		return out, true
	}
	return nil, false
}

// detectX86 inspects initial block bytes to determine if the payload is x86/x86_64 machine code
// with relative CALL (0xE8) / JMP (0xE9) instructions and standard function frames.
// Accelerated with 64-bit SWAR vector scanning (simd.IndexByteTwoSWAR).
func detectX86(src []byte) bool {
	sampleLen := min(len(src), 8192)
	if sampleLen < 64 {
		return false
	}

	callCount := 0
	x86Markers := 0
	limit := sampleLen - 5

	for i := 0; i < limit; {
		next := simd.IndexByteTwoSWAR(src[i:limit], 0xE8, 0xE9)
		if next < 0 {
			// Scan remainder for prologues / epilogues
			for ; i < sampleLen-2; i++ {
				if src[i] == 0x55 && src[i+1] == 0x48 && src[i+2] == 0x89 {
					x86Markers += 2
					i += 2
				} else if src[i] == 0xC3 || src[i] == 0xC9 {
					x86Markers++
				}
			}
			break
		}

		pos := i + next
		// Check for function frames in the skipped range
		for ; i < pos; i++ {
			if src[i] == 0x55 && i+2 < sampleLen && src[i+1] == 0x48 && src[i+2] == 0x89 {
				x86Markers += 2
				i += 2
			} else if src[i] == 0xC3 || src[i] == 0xC9 {
				x86Markers++
			}
		}

		rel := binary.LittleEndian.Uint32(src[pos+1 : pos+5])
		msb := byte(rel >> 24)
		if msb == 0x00 || msb == 0xFF {
			callCount++
			if callCount >= 8 {
				return true
			}
			i = pos + 5
		} else {
			i = pos + 1
		}
	}

	return callCount >= 4 && (callCount+x86Markers) >= 8
}

// detectStride scans initial bytes to identify periodic record or column strides S in [4, 256].
func detectStride(src []byte) (int, bool) {
	sampleLen := min(len(src), 2048)
	if sampleLen < 32 {
		return 1, false
	}

	candidates := []int{4, 8, 12, 16, 20, 24, 28, 32, 40, 48, 56, 64, 72, 80, 96, 128, 160, 192, 256}
	bestStride := 1
	bestScore := 0

	for _, s := range candidates {
		if s*2 > sampleLen {
			break
		}
		score := 0
		checkCount := sampleLen - s
		for i := 0; i < checkCount; i += 4 {
			if src[i] == src[i+s] {
				score++
			}
		}
		numChecks := (checkCount + 3) / 4
		// Baseline matching ratio (at least 20% match rate and significant periodicity)
		if score > bestScore && score > (numChecks/5) {
			bestScore = score
			bestStride = s
		}
	}

	if bestScore > 0 && bestStride >= 4 {
		return bestStride, true
	}
	return 1, false
}

// Compress encodes src into PRZM format according to PRZM-SPEC-0.1.
func Compress(src []byte, level int) ([]byte, error) {
	uSize := len(src)
	if uSize == 0 {
		hdr := [HeaderSize]byte{}
		binary.LittleEndian.PutUint32(hdr[0:4], MagicPRZM)
		hdr[4] = FlagUncompressed
		return hdr[:], nil
	}

	if uSize > MaxBlockSize {
		return nil, ErrExceedsMaxBlock
	}

	pad := padPool.Get().(*scratchpad)
	defer padPool.Put(pad)

	clear(pad.head)
	pad.cmdBuf = ensureBufCap(pad.cmdBuf, uSize)
	pad.offsetBuf = ensureBufCap(pad.offsetBuf, uSize/2)
	pad.literalBuf = ensureBufCap(pad.literalBuf, uSize)

	origSrc := src
	origUSize := uSize

	isX86 := detectX86(src)
	flags := uint8(0)
	callCount := 0
	if isX86 {
		if cap(pad.transBuf) < uSize {
			pad.transBuf = make([]byte, uSize)
		} else {
			pad.transBuf = pad.transBuf[:uSize]
		}
		copy(pad.transBuf, src)
		bcj.Filter(bcj.X86, pad.transBuf, 0, true)
		src = pad.transBuf
		flags |= FlagBCJ

		if splitSrc, count := splitX86(src, uSize, pad.splitBuf); count >= 8 {
			src = splitSrc
			uSize = len(splitSrc)
			callCount = count
			flags |= FlagBCJSplit
		}
	}

	stride, hasStride := 1, false
	if !isX86 {
		stride, hasStride = detectStride(src)
		if hasStride {
			flags |= FlagStride
		}
	}

	dictID := uint32(0)
	var activeMatcher *dictMatcher
	if !isX86 && !hasStride {
		dictID = detectDictionary(src)
		switch dictID {
		case DictIDJSON:
			activeMatcher = &jsonMatcher
			flags |= FlagDictRef
		case DictIDCode:
			activeMatcher = &codeMatcher
			flags |= FlagDictRef
		}
	}

	if uSize > 65535 {
		flags |= FlagExtOffset
	}

	calcOffCost := func(d int) int {
		if d < 128 {
			return 1
		}
		if d < 16384 {
			return 2
		}
		return 3
	}

	writeOffset := func(d int) {
		if d < 0x80 {
			pad.offsetBuf = append(pad.offsetBuf, byte(d))
		} else if d < 0x4000 {
			pad.offsetBuf = append(pad.offsetBuf, byte(d|0x80), byte(d>>7))
		} else {
			var buf [8]byte
			n := binary.PutUvarint(buf[:], uint64(d))
			pad.offsetBuf = append(pad.offsetBuf, buf[:n]...)
		}
	}

	// Initialize Ring-16 locality queue
	q := [16]int{1, stride, 2 * stride, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384}

	flushLiterals := func(lits []byte) {
		nLits := len(lits)
		if nLits == 0 {
			return
		}
		if nLits <= 8 {
			pad.cmdBuf = append(pad.cmdBuf, 0xF2+byte(nLits))
		} else if nLits < 0x80 {
			pad.cmdBuf = append(pad.cmdBuf, OpcodePrefixLiteral, byte(nLits))
		} else if nLits < 0x4000 {
			pad.cmdBuf = append(pad.cmdBuf, OpcodePrefixLiteral, byte(nLits|0x80), byte(nLits>>7))
		} else {
			pad.cmdBuf = append(pad.cmdBuf, OpcodePrefixLiteral)
			var varBuf [8]byte
			n := binary.PutUvarint(varBuf[:], uint64(nLits))
			pad.cmdBuf = append(pad.cmdBuf, varBuf[:n]...)
		}
		pad.literalBuf = append(pad.literalBuf, lits...)
	}

	extendMatch := func(pos, d, maxLen int) int {
		mLen := 4
		for pos+mLen+8 <= uSize && mLen+8 <= maxLen {
			diff := binary.LittleEndian.Uint64(src[pos+mLen:pos+mLen+8]) ^ binary.LittleEndian.Uint64(src[pos-d+mLen:pos-d+mLen+8])
			if diff != 0 {
				return mLen + (bits.TrailingZeros64(diff) >> 3)
			}
			mLen += 8
			if mLen >= 32 && pos+maxLen <= uSize {
				rem := maxLen - mLen
				if rem >= 32 {
					return mLen + simd.FindMatchLengthVector(src[pos+mLen:pos+maxLen], src[pos-d+mLen:pos-d+maxLen], rem)
				}
			}
		}
		for pos+mLen < uSize && mLen < maxLen && src[pos+mLen] == src[pos-d+mLen] {
			mLen++
		}
		return mLen
	}

	evalCandidate := func(pos, d, maxLen int) matchCandidate {
		if d <= 0 || pos < d || pos+4 > uSize {
			return matchCandidate{}
		}
		if binary.LittleEndian.Uint32(src[pos:pos+4]) != binary.LittleEndian.Uint32(src[pos-d:pos-d+4]) {
			return matchCandidate{}
		}
		mLen := extendMatch(pos, d, maxLen)
		if mLen < minMatch {
			return matchCandidate{}
		}

		ringK := -1
		for k := range 16 {
			if q[k] == d {
				ringK = k
				break
			}
		}

		if ringK >= 0 {
			if ringK <= 3 && mLen <= 7 {
				return matchCandidate{
					mType:    1,
					length:   mLen,
					distance: d,
					ringK:    ringK,
					cost:     1,
					score:    mLen - 1,
				}
			}
			cost := 2
			if mLen > 18 {
				cost += uvarintLen(uint64(mLen - 19))
			}
			return matchCandidate{
				mType:    3,
				length:   mLen,
				distance: d,
				ringK:    ringK,
				cost:     cost,
				score:    mLen - cost,
			}
		}

		// Explicit Extended Match
		offCost := calcOffCost(d)
		if mLen < 4 || (mLen < 5 && d >= 128) {
			return matchCandidate{}
		}
		cost := 1 + offCost
		if mLen > 18 {
			cost += uvarintLen(uint64(mLen - 19))
		}
		return matchCandidate{
			mType:    4,
			length:   mLen,
			distance: d,
			cost:     cost,
			score:    mLen - cost,
		}
	}

	findBest := func(pos int) matchCandidate {
		if pos+minMatch > uSize {
			return matchCandidate{}
		}

		best := matchCandidate{}

		if activeMatcher != nil {
			tokIdx, tokLen := activeMatcher.match(src, pos)
			if tokLen >= 5 {
				var cost int
				var mType int
				if tokIdx < 8 {
					cost = 1
					mType = 7 // OpcodePrefixShortDict
				} else {
					cost = 2
					mType = 8 // OpcodePrefixDictToken
				}
				score := tokLen - cost
				if score > best.score {
					best = matchCandidate{
						mType:   mType,
						length:  tokLen,
						dictTok: tokIdx,
						cost:    cost,
						score:   score,
					}
				}
			}
		}

		checkDelta := func(d int, ringK int) {
			if d < 14 || pos < d || pos+8 > uSize || pos-d+8 > uSize {
				return
			}
			diff8 := binary.LittleEndian.Uint64(src[pos:pos+8]) ^ binary.LittleEndian.Uint64(src[pos-d:pos-d+8])
			if diff8 != 0 {
				c := 0
				if uint8(diff8) != 0 {
					c++
				}
				if uint8(diff8>>8) != 0 {
					c++
				}
				if uint8(diff8>>16) != 0 {
					c++
				}
				if uint8(diff8>>24) != 0 {
					c++
				}
				if c > 2 {
					return
				}
				if uint8(diff8>>32) != 0 {
					c++
				}
				if c > 2 {
					return
				}
				if uint8(diff8>>40) != 0 {
					c++
				}
				if c > 2 {
					return
				}
				if uint8(diff8>>48) != 0 {
					c++
				}
				if c > 2 {
					return
				}
				if uint8(diff8>>56) != 0 {
					c++
				}
				if c > 2 {
					return
				}
			}
			checkLimit := min(1024, d)
			if pos+checkLimit > uSize {
				checkLimit = uSize - pos
			}
			if checkLimit < 14 {
				return
			}

			mismatches := 0
			var diffOffsets [2]int
			var diffXORs [2]byte
			curLen := 0
			i := 0
			for i < checkLimit {
				if i+8 <= checkLimit {
					diff := binary.LittleEndian.Uint64(src[pos+i:pos+i+8]) ^ binary.LittleEndian.Uint64(src[pos-d+i:pos-d+i+8])
					if diff == 0 {
						i += 8
						curLen = i
						continue
					}
				}
				if src[pos+i] != src[pos-d+i] {
					if mismatches >= 2 {
						break
					}
					diffOffsets[mismatches] = i
					diffXORs[mismatches] = src[pos+i] ^ src[pos-d+i]
					mismatches++
				}
				curLen = i + 1
				i++
			}

			for curLen > 0 && mismatches > 0 && diffOffsets[mismatches-1] == curLen-1 {
				mismatches--
				curLen--
			}

			if ringK < 0 {
				for k := range 8 {
					if q[k] == d {
						ringK = k
						break
					}
				}
			}

			minDeltaLen := 64
			minScore := best.score + 12
			if best.score == 0 {
				minScore = 24
			}

			if mismatches >= 1 && mismatches <= 2 && curLen >= minDeltaLen {
				var cost int
				var mType int
				if ringK >= 0 {
					mType = 6 // DeltaMaskRing8
					cost = 2 + 1 + mismatches*2
				} else {
					mType = 5 // DeltaMaskExplicit
					cost = 2 + calcOffCost(d) + 1 + mismatches*2
				}
				if curLen > 258 {
					cost += uvarintLen(uint64(curLen - 259))
				}
				if curLen > 256 {
					cost += mismatches
				}
				score := curLen - cost
				if score >= minScore {
					best = matchCandidate{
						mType:      mType,
						length:     curLen,
						distance:   d,
						ringK:      ringK,
						cost:       cost,
						score:      score,
						mismatches: mismatches,
						diffOffs:   diffOffsets,
						diffXORs:   diffXORs,
					}
				}
			}
		}

		// 1. Hash table lookup (2-way bucket) - Most likely and longest exact matches
		h := (binary.LittleEndian.Uint32(src[pos:pos+4]) * 0x1E35A7BD) >> 16
		h &= hashTableMask
		p0 := pad.head[h*2]
		p1 := pad.head[h*2+1]

		d0 := 0
		if p0 > 0 {
			prevPos := int(p0 - 1)
			d := pos - prevPos
			if d > 0 && d < 65535 {
				d0 = d
				cand := evalCandidate(pos, d, 65535)
				if cand.score > best.score {
					best = cand
				}
				if level >= 3 && cand.score < 12 {
					checkDelta(d, -1)
				}
			}
		}
		if p1 > 0 {
			prevPos := int(p1 - 1)
			d := pos - prevPos
			if d > 0 && d < 65535 && d != d0 {
				cand := evalCandidate(pos, d, 65535)
				if cand.score > best.score {
					best = cand
				}
				if level >= 3 && cand.score < 12 {
					checkDelta(d, -1)
				}
			}
		}

		// If hash table already found a strong match >= 8 with high score, skip smaller checks
		if best.length >= 8 && best.score >= 6 {
			return best
		}

		// 2. Check Ring locality queue (top-4 in fast mode, top-16 in normal/ultra mode)
		ringLimit := 16
		if level <= 2 {
			ringLimit = 4
		}
		for k := range ringLimit {
			d := q[k]
			if d == d0 {
				continue
			}
			cand := evalCandidate(pos, d, 65535)
			if cand.score > best.score {
				best = cand
			}
		}

		// If a good match >= 8 is found, skip stride and delta-mask
		if best.length >= 8 {
			return best
		}

		// 3. Check Stride (m * S for m >= 3, since m=1,2 are checked in Ring-8 queue)
		if hasStride && best.length < 8 {
			for m := 3; m <= 8; m++ {
				d := m * stride
				if pos >= d && pos+4 <= uSize && binary.LittleEndian.Uint32(src[pos:pos+4]) == binary.LittleEndian.Uint32(src[pos-d:pos-d+4]) {
					mLen := extendMatch(pos, d, 19)
					if mLen >= minMatch {
						cost := 2
						score := mLen - cost
						if score > best.score {
							best = matchCandidate{
								mType:    2,
								length:   mLen,
								distance: d,
								strideM:  m,
								cost:     cost,
								score:    score,
							}
						}
					}
				}
			}
		}

		// 4. Fast-filtered Delta-mask fallback check on q[0]
		if level >= 3 && best.length < 8 {
			if q[0] >= 48 && pos >= q[0] {
				checkDelta(q[0], 0)
			}
		}

		return best
	}

	insertHash := func(p int) {
		if p+4 <= uSize {
			h := (binary.LittleEndian.Uint32(src[p:p+4]) * 0x1E35A7BD) >> 16
			h &= hashTableMask
			pad.head[h*2+1] = pad.head[h*2]
			pad.head[h*2] = int32(p + 1)
		}
	}

	pos := 0
	litStart := 0
	misses := 0

	for pos+minMatch <= uSize {
		// 1. RLE Check (run of identical bytes >= 12)
		if pos+12 <= uSize && src[pos] == src[pos+1] && src[pos] == src[pos+2] && src[pos] == src[pos+3] {
			rleByte := src[pos]
			rleLen := 4
			for pos+rleLen < uSize && src[pos+rleLen] == rleByte && rleLen < 65535 {
				rleLen++
			}
			if rleLen >= 12 {
				if pos > litStart {
					flushLiterals(src[litStart:pos])
				}
				pad.cmdBuf = append(pad.cmdBuf, OpcodePrefixRLE)
				var varBuf [8]byte
				n := binary.PutUvarint(varBuf[:], uint64(rleLen))
				pad.cmdBuf = append(pad.cmdBuf, varBuf[:n]...)
				pad.literalBuf = append(pad.literalBuf, rleByte)

				insertHash(pos)
				pos += rleLen
				litStart = pos
				continue
			}
		}

		best := findBest(pos)

		if best.score >= 1 && best.length >= minMatch {
			// 1-step lazy evaluation lookahead taking advantage of zero-overhead Fused Tokens (level >= 3)
			if level >= 3 && best.length <= 6 && pos+1+minMatch <= uSize {
				insertHash(pos)
				nextBest := findBest(pos + 1)
				if nextBest.score > best.score || nextBest.length > best.length+1 {
					pos++
					continue
				}
			}

			// Backward match extension into uncommitted literals!
			if best.mType != 5 && best.mType != 6 && best.mType != 7 && best.mType != 8 {
				for pos > litStart && pos-best.distance > 0 && src[pos-1] == src[pos-best.distance-1] && best.length < 65530 {
					pos--
					best.length++
					if best.mType == 1 && (best.length > 7 || best.ringK > 3) {
						best.mType = 3
					}
					if best.mType == 2 && best.length > 19 {
						pos++
						best.length--
						break
					}
				}
			}

			litLen := pos - litStart

			switch best.mType {
			case 1: // Fused Repeat (k <= 3, length 4..7)
				if litLen > 7 {
					flushLiterals(src[litStart : pos-7])
					litStart = pos - 7
					litLen = 7
				}
				op := uint8((litLen << 4) | ((best.length - 4) << 2) | best.ringK)
				pad.cmdBuf = append(pad.cmdBuf, op)
				if litLen > 0 {
					pad.literalBuf = append(pad.literalBuf, src[litStart:pos]...)
				}
				for j := best.ringK; j > 0; j-- {
					q[j] = q[j-1]
				}
				q[0] = best.distance

			case 2: // Stride (m 1..8, length 4..19)
				if litLen > 15 {
					flushLiterals(src[litStart : pos-15])
					litStart = pos - 15
					litLen = 15
				}
				op := OpcodePrefixStride | uint8(best.strideM-1)
				token := uint8((litLen << 4) | (best.length - 4))
				pad.cmdBuf = append(pad.cmdBuf, op, token)
				if litLen > 0 {
					pad.literalBuf = append(pad.literalBuf, src[litStart:pos]...)
				}
				for j := 15; j > 0; j-- {
					q[j] = q[j-1]
				}
				q[0] = best.distance

			case 3: // Long Repeat (k 0..15, length 4..N) with Fused Literals
				if litLen > 15 {
					flushLiterals(src[litStart : pos-15])
					litStart = pos - 15
					litLen = 15
				}
				baseLen := best.length - 4
				if baseLen > 14 {
					baseLen = 15
				}
				op := OpcodePrefixLongRepeat | uint8(best.ringK)
				token := uint8((litLen << 4) | baseLen)
				pad.cmdBuf = append(pad.cmdBuf, op, token)
				if baseLen == 15 {
					var varBuf [8]byte
					n := binary.PutUvarint(varBuf[:], uint64(best.length-19))
					pad.cmdBuf = append(pad.cmdBuf, varBuf[:n]...)
				}
				if litLen > 0 {
					pad.literalBuf = append(pad.literalBuf, src[litStart:pos]...)
				}
				for j := best.ringK; j > 0; j-- {
					q[j] = q[j-1]
				}
				q[0] = best.distance

			case 4: // Fused Extended Match (length 4..N, explicit offset)
				if litLen > 3 {
					flushLiterals(src[litStart : pos-3])
					litStart = pos - 3
					litLen = 3
				}
				baseLen := best.length - 4
				if baseLen > 14 {
					baseLen = 15
				}
				op := OpcodeFlagFusedExt | uint8(litLen<<4) | uint8(baseLen)
				pad.cmdBuf = append(pad.cmdBuf, op)
				if baseLen == 15 {
					var varBuf [8]byte
					n := binary.PutUvarint(varBuf[:], uint64(best.length-19))
					pad.cmdBuf = append(pad.cmdBuf, varBuf[:n]...)
				}
				if litLen > 0 {
					pad.literalBuf = append(pad.literalBuf, src[litStart:pos]...)
				}
				writeOffset(best.distance)
				for j := 15; j > 0; j-- {
					q[j] = q[j-1]
				}
				q[0] = best.distance

			case 5: // DeltaMaskExplicit
				if litLen > 0 {
					flushLiterals(src[litStart:pos])
				}
				flags |= FlagDeltaMask
				pad.cmdBuf = append(pad.cmdBuf, OpcodePrefixDeltaExpl)
				baseLen := best.length - 4
				if baseLen >= 255 {
					pad.cmdBuf = append(pad.cmdBuf, 255)
					var varBuf [8]byte
					n := binary.PutUvarint(varBuf[:], uint64(best.length-259))
					pad.cmdBuf = append(pad.cmdBuf, varBuf[:n]...)
				} else {
					pad.cmdBuf = append(pad.cmdBuf, uint8(baseLen))
				}
				writeOffset(best.distance)
				pad.literalBuf = append(pad.literalBuf, uint8(best.mismatches))
				for m := 0; m < best.mismatches; m++ {
					if best.length <= 256 {
						pad.literalBuf = append(pad.literalBuf, uint8(best.diffOffs[m]))
					} else {
						var rel [2]byte
						binary.LittleEndian.PutUint16(rel[:], uint16(best.diffOffs[m]))
						pad.literalBuf = append(pad.literalBuf, rel[:]...)
					}
					pad.literalBuf = append(pad.literalBuf, best.diffXORs[m])
				}
				for j := 15; j > 0; j-- {
					q[j] = q[j-1]
				}
				q[0] = best.distance

			case 6: // DeltaMaskRing8
				if litLen > 0 {
					flushLiterals(src[litStart:pos])
				}
				flags |= FlagDeltaMask
				op := OpcodePrefixDeltaRing | uint8(best.ringK)
				pad.cmdBuf = append(pad.cmdBuf, op)
				baseLen := best.length - 4
				if baseLen >= 255 {
					pad.cmdBuf = append(pad.cmdBuf, 255)
					var varBuf [8]byte
					n := binary.PutUvarint(varBuf[:], uint64(best.length-259))
					pad.cmdBuf = append(pad.cmdBuf, varBuf[:n]...)
				} else {
					pad.cmdBuf = append(pad.cmdBuf, uint8(baseLen))
				}
				pad.literalBuf = append(pad.literalBuf, uint8(best.mismatches))
				for m := 0; m < best.mismatches; m++ {
					if best.length <= 256 {
						pad.literalBuf = append(pad.literalBuf, uint8(best.diffOffs[m]))
					} else {
						var rel [2]byte
						binary.LittleEndian.PutUint16(rel[:], uint16(best.diffOffs[m]))
						pad.literalBuf = append(pad.literalBuf, rel[:]...)
					}
					pad.literalBuf = append(pad.literalBuf, best.diffXORs[m])
				}
				for j := best.ringK; j > 0; j-- {
					q[j] = q[j-1]
				}
				q[0] = best.distance

			case 7: // OpcodePrefixShortDict (0xD8 .. 0xDF)
				if litLen > 0 {
					flushLiterals(src[litStart:pos])
				}
				op := OpcodePrefixShortDict | uint8(best.dictTok)
				pad.cmdBuf = append(pad.cmdBuf, op)

			case 8: // OpcodePrefixDictToken (0xF2 <tokID>)
				if litLen > 0 {
					flushLiterals(src[litStart:pos])
				}
				pad.cmdBuf = append(pad.cmdBuf, OpcodePrefixDictToken, uint8(best.dictTok))
			}

			insertHash(pos)
			if best.length > 2 && pos+best.length-1+4 <= uSize {
				insertHash(pos + best.length - 1)
			}
			pos += best.length
			litStart = pos
			misses = 0
			continue
		}

		insertHash(pos)
		misses++
		step := min(1+(misses>>6), 32)
		if level <= 2 {
			step = min(1+(misses>>4), 32)
		}
		pos += step
	}

	// Flush remaining literals
	if litStart < uSize {
		flushLiterals(src[litStart:])
	}

	// Try SIMD Nibble-Plane packing on literalBuf
	litSection := pad.literalBuf
	if packedLits, ok := packNibblePlanes(pad.literalBuf, pad.nibbleBuf, pad.nibbleLowBuf); ok {
		litSection = packedLits
		flags |= FlagNibble
	}

	// Calculate Frame Layout & Size
	frameParamsSize := 0
	if hasStride {
		frameParamsSize += 1
	}
	if (flags & FlagDictRef) != 0 {
		frameParamsSize += 4
	}
	if (flags & FlagBCJSplit) != 0 {
		frameParamsSize += 4
	}

	secDirSize := 4
	if (flags & FlagExtOffset) != 0 {
		secDirSize = 6
	}

	sec1Size := 8 // 8-byte distribution params (ModelType = 0 uniform/raw)
	totalCompressedSize := HeaderSize + frameParamsSize + secDirSize + sec1Size + len(pad.cmdBuf) + len(pad.offsetBuf) + len(litSection)

	// Fallback to uncompressed if compressed payload exceeds raw data
	if totalCompressedSize >= origUSize {
		rawFrame := make([]byte, HeaderSize+origUSize)
		binary.LittleEndian.PutUint32(rawFrame[0:4], MagicPRZM)
		rawFrame[4] = FlagUncompressed
		rawFrame[5] = byte(origUSize)
		rawFrame[6] = byte(origUSize >> 8)
		rawFrame[7] = byte(origUSize >> 16)
		copy(rawFrame[HeaderSize:], origSrc)
		return rawFrame, nil
	}

	// Build Final Output Frame
	out := make([]byte, totalCompressedSize)
	binary.LittleEndian.PutUint32(out[0:4], MagicPRZM)
	out[4] = flags
	out[5] = byte(origUSize)
	out[6] = byte(origUSize >> 8)
	out[7] = byte(origUSize >> 16)

	wOffset := HeaderSize
	if hasStride {
		out[wOffset] = uint8(stride - 1)
		wOffset++
	}
	if (flags & FlagDictRef) != 0 {
		binary.LittleEndian.PutUint32(out[wOffset:wOffset+4], dictID)
		wOffset += 4
	}
	if (flags & FlagBCJSplit) != 0 {
		binary.LittleEndian.PutUint32(out[wOffset:wOffset+4], uint32(callCount))
		wOffset += 4
	}

	// Section Directory
	if (flags & FlagExtOffset) == 0 {
		binary.LittleEndian.PutUint16(out[wOffset:wOffset+2], uint16(len(pad.cmdBuf)))
		binary.LittleEndian.PutUint16(out[wOffset+2:wOffset+4], uint16(len(pad.offsetBuf)))
		wOffset += 4
	} else {
		cmdLen := len(pad.cmdBuf)
		offLen := len(pad.offsetBuf)
		out[wOffset] = byte(cmdLen)
		out[wOffset+1] = byte(cmdLen >> 8)
		out[wOffset+2] = byte(cmdLen >> 16)
		out[wOffset+3] = byte(offLen)
		out[wOffset+4] = byte(offLen >> 8)
		out[wOffset+5] = byte(offLen >> 16)
		wOffset += 6
	}

	// Section 1: Distribution Parameters (Uniform/Raw)
	for i := range sec1Size {
		out[wOffset+i] = 0
	}
	wOffset += sec1Size

	// Section 2: Command Stream
	copy(out[wOffset:wOffset+len(pad.cmdBuf)], pad.cmdBuf)
	wOffset += len(pad.cmdBuf)

	// Section 3: Offset Channel
	copy(out[wOffset:wOffset+len(pad.offsetBuf)], pad.offsetBuf)
	wOffset += len(pad.offsetBuf)

	// Section 4: Literal, Mask, and RLE Channel
	copy(out[wOffset:wOffset+len(litSection)], litSection)

	return out, nil
}

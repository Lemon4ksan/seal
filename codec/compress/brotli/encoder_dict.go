// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package brotli

/* Dictionary data (words and transforms) for 1 possible context */
type encoderDictionary struct {
	words                 *dictionary
	cutoffTransformsCount uint32
	cutoffTransforms      uint64
	hash_table            []uint16
	buckets               []uint16
	dict_words            []dictWord
}

func initEncoderDictionary(dict *encoderDictionary) {
	dict.words = getDictionary()

	dict.hash_table = kStaticDictionaryHash[:]
	dict.buckets = kStaticDictionaryBuckets[:]
	dict.dict_words = kStaticDictionaryWords[:]

	dict.cutoffTransformsCount = kCutoffTransformsCount
	dict.cutoffTransforms = kCutoffTransforms
}

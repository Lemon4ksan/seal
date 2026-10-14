// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fse_test

import (
	"bytes"
	"fmt"
	"log"

	"github.com/lemon4ksan/seal/codec/compress/fse"
)

func ExampleCompress() {
	// FSE is best for blocks with an uneven distribution of bytes.
	data := []byte("aaabbbbccccccdddddddd")

	// Provide a Scratch buffer to avoid allocations.
	var s fse.Scratch

	// Compress the data.
	compressed, err := fse.Compress(data, &s)
	if err != nil {
		log.Fatal(err)
	}

	// Decompress the data using the same or another Scratch.
	// You can safely pass the compressed slice.
	decompressed, err := fse.Decompress(compressed, &s)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(bytes.Equal(data, decompressed))
	// Output: true
}

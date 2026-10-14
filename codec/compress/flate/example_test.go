// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package flate_test

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
)

func ExampleNewWriter() {
	var buf bytes.Buffer

	w, err := flate.NewWriter(&buf, 1)
	if err != nil {
		panic(err)
	}
	_, err = w.Write([]byte("hello flate world"))
	if err != nil {
		panic(err)
	}
	w.Close()

	r := flate.NewReader(&buf)
	defer r.Close()

	out, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(out))
	// Output: hello flate world
}

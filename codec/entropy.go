// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codec

import (
	"path/filepath"
	"strings"

	"github.com/lemon4ksan/foundation/fskit"

	"github.com/lemon4ksan/seal/codec/entropy"
)

// IsIncompressibleName checks if a filename extension corresponds to already compressed or encrypted media.
var IsIncompressibleName = fskit.IsIncompressibleExtension

// ShannonEntropy calculates the entropy of the byte sample in bits per byte (0.0 to 8.0).
var ShannonEntropy = entropy.ShannonEntropy

// IsIncompressibleSample returns true if the sample has entropy >= 7.92 bits/byte.
var IsIncompressibleSample = entropy.IsIncompressibleSample

// IsTensorName checks if a filename extension corresponds to ML weights, model checkpoints, or float arrays.
func IsTensorName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".safetensors",
		".pt",
		".bin",
		".npy",
		".npz",
		".onnx",
		".gguf",
		".weight",
		".weights",
		".ckpt",
		".tflite",
		".h5":
		return true
	default:
		return false
	}
}

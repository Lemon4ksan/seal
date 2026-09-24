package przm

import (
	"math/rand"
	"testing"
)

func BenchmarkPredicatePushdown(b *testing.B) {
	// Generate 1MB of columnar data (stride=64)
	stride := 64
	numRows := 1024 * 16 // 1MB total
	src := make([]byte, numRows*stride)

	// Fill with random data
	for i := range src {
		src[i] = byte(rand.Intn(256))
	}

	// Insert a predictable predicate column at ColIdx 10
	// We make it match our target value 1% of the time, so 99% is skipped
	predCol := 10
	targetVal := byte(0x42)
	for r := 0; r < numRows; r++ {
		if r%6400 == 0 { // Match exactly once every 100 chunks
			src[r*stride+predCol] = targetVal
		} else {
			src[r*stride+predCol] = 0x00
		}
	}

	colToCluster := make([]int, stride)
	var cls0, cls1 []int
	for i := 0; i < stride; i++ {
		if i == predCol {
			cls0 = append(cls0, i)
			colToCluster[i] = 0
		} else {
			cls1 = append(cls1, i)
			colToCluster[i] = 1
		}
	}
	clusters := []ChannelCluster{
		{ColumnIndices: cls0},
		{ColumnIndices: cls1},
	}
	schema := &SynthesizedSchema{
		Stride:          stride,
		NumRows:         numRows,
		Clusters:        clusters,
		ColumnToCluster: colToCluster,
		PredicateColIdx: -1,
	}

	// Frame without predicate
	frameFull, err := VMEncode(src, schema)
	if err != nil {
		b.Fatalf("VMEncode failed: %v", err)
	}

	// Frame with predicate pushdown
	schema.PredicateColIdx = predCol
	schema.PredicateVal = targetVal
	framePred, err := VMEncode(src, schema)
	if err != nil {
		b.Fatalf("VMEncode failed: %v", err)
	}

	b.Run("FullDecoding", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := VMDecode(frameFull)
			if err != nil {
				b.Fatalf("Decode failed: %v", err)
			}
			_ = out
		}
	})

	b.Run("PredicatePushdown", func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			out, err := VMDecode(framePred)
			if err != nil {
				b.Fatalf("Decode failed: %v", err)
			}
			_ = out
		}
	})
}

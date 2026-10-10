package codec

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/lemon4ksan/przm"
)

// CompressPRZM compresses src using the PRZM (PrismLZ) structured dictionary compression format.
func CompressPRZM(src, dst []byte, level ...int) ([]byte, error) {
	lvl := 3
	if len(level) > 0 {
		lvl = level[0]
	}
	
	opts := []przm.Option{}
	if os.Getenv("PRZM_GPU") == "1" {
		opts = append(opts, przm.WithGPUAcceleration())
	}
	
	compressed, err := przm.CompressWithOptions(src, przm.CompressOptions{Level: lvl}, opts...)
	if err != nil {
		return nil, err
	}
	if cap(dst) >= len(compressed) {
		dst = dst[:len(compressed)]
		copy(dst, compressed)
		return dst, nil
	}
	return compressed, nil
}

// Unprzm decompresses a PRZM payload from src into dst.
func Unprzm(src, dst []byte) ([]byte, error) {
	decompressed, err := przm.Decompress(src)
	if err != nil {
		return nil, err
	}
	if cap(dst) >= len(decompressed) {
		dst = dst[:len(decompressed)]
		copy(dst, decompressed)
		return dst, nil
	}
	return decompressed, nil
}

// PRZMCompressor implements streaming PRZM compression.
type PRZMCompressor struct {
	level int
}

// NewPRZMCompressor creates a new PRZM compressor.
func NewPRZMCompressor(level int) Compressor {
	if level <= 0 {
		level = 3
	}
	return &PRZMCompressor{level: level}
}

// Compress reads from src, compresses via PRZM, and writes length-prefixed blocks into dest.
func (c *PRZMCompressor) Compress(src io.Reader, dest io.Writer) (int64, error) {
	raw, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}
	opts := []przm.Option{}
	if os.Getenv("PRZM_GPU") == "1" {
		opts = append(opts, przm.WithGPUAcceleration())
	}
	compressed, err := przm.CompressWithOptions(raw, przm.CompressOptions{Level: c.level}, opts...)
	if err != nil {
		return 0, err
	}
	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(compressed)))
	if _, err := dest.Write(lenBuf[:]); err != nil {
		return 0, err
	}
	if _, err := dest.Write(compressed); err != nil {
		return 0, err
	}
	return int64(len(raw)), nil
}

// PRZMDecompressor implements PRZM decompression.
type PRZMDecompressor struct{}

// NewPRZMDecompressor creates a new PRZM decompressor.
func NewPRZMDecompressor() Decompressor {
	return &PRZMDecompressor{}
}

// Decompress wraps the stream, reading length-prefixed or bare PRZM frames.
func (d *PRZMDecompressor) Decompress(src io.Reader) (io.ReadCloser, error) {
	payload, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	if len(payload) < 4 {
		return nil, fmt.Errorf("przm: stream too short (%d bytes)", len(payload))
	}
	// Check if bare PRZM frame (starts with 'PRZM' magic)
	if binary.LittleEndian.Uint32(payload[0:4]) == przm.MagicPRZM {
		decompressed, err := przm.Decompress(payload)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(decompressed)), nil
	}
	// Length-prefixed block
	frameLen := int(binary.LittleEndian.Uint32(payload[0:4]))
	if len(payload) >= 4+frameLen {
		decompressed, err := przm.Decompress(payload[4 : 4+frameLen])
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(decompressed)), nil
	}
	return nil, fmt.Errorf("przm: invalid stream header")
}

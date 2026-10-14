// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Volume represents a single volume segment with random-access read capability.
type Volume struct {
	DiskIndex int
	Reader    io.ReaderAt
	Size      int64
	Path      string
}

// MultiVolumeReader presents multiple volume parts (.z01, .z02, ..., .zip) as a unified virtual ReaderAt.
type MultiVolumeReader struct {
	volumes   []Volume
	offsets   []int64 // Cumulative start offset of each volume
	totalSize int64
}

// NewMultiVolumeReader constructs a virtual [io.ReaderAt] spanning multiple volume segments in sequence.
func NewMultiVolumeReader(volumes []Volume) (*MultiVolumeReader, error) {
	if len(volumes) == 0 {
		return nil, errors.New("zip: no volumes provided")
	}

	offsets := make([]int64, len(volumes))
	var total int64
	for i, v := range volumes {
		offsets[i] = total
		total += v.Size
	}

	return &MultiVolumeReader{
		volumes:   volumes,
		offsets:   offsets,
		totalSize: total,
	}, nil
}

// Size returns the total cumulative size of all volume segments combined.
func (mvr *MultiVolumeReader) Size() int64 {
	return mvr.totalSize
}

// TotalVolumes returns the total number of volume segments.
func (mvr *MultiVolumeReader) TotalVolumes() int {
	return len(mvr.volumes)
}

// ReadAt reads len(p) bytes from the multi-volume archive starting at virtual byte offset off.
// If the read spans across a volume boundary, it transparently stitches the chunks across volumes.
func (mvr *MultiVolumeReader) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= mvr.totalSize {
		return 0, io.EOF
	}

	totalRead := 0
	for totalRead < len(p) && off < mvr.totalSize {
		volIdx := mvr.findVolumeIndex(off)
		if volIdx < 0 || volIdx >= len(mvr.volumes) {
			break
		}

		vol := mvr.volumes[volIdx]
		volStart := mvr.offsets[volIdx]
		offsetInVol := off - volStart
		availInVol := vol.Size - offsetInVol
		toRead := min(int64(len(p)-totalRead), availInVol)

		n, err := vol.Reader.ReadAt(p[totalRead:totalRead+int(toRead)], offsetInVol)
		totalRead += n
		off += int64(n)

		if err != nil && err != io.EOF {
			return totalRead, err
		}
		if n == 0 {
			break
		}
	}

	if totalRead < len(p) {
		return totalRead, io.EOF
	}
	return totalRead, nil
}

func (mvr *MultiVolumeReader) findVolumeIndex(off int64) int {
	for i := len(mvr.offsets) - 1; i >= 0; i-- {
		if off >= mvr.offsets[i] {
			return i
		}
	}
	return 0
}

// OpenMultiVolumeFiles detects and opens all segment parts (.z01, .z02, ... .zip) from primaryZipPath.
func OpenMultiVolumeFiles(primaryZipPath string) (*MultiVolumeReader, func() error, error) {
	ext := filepath.Ext(primaryZipPath)
	base := strings.TrimSuffix(primaryZipPath, ext)

	// Scan for .z01, .z02, ...
	var volPaths []string
	disk := 1
	for {
		zExt := fmt.Sprintf(".z%02d", disk)
		candidate := base + zExt
		if _, err := os.Stat(candidate); err == nil {
			volPaths = append(volPaths, candidate)
			disk++
		} else {
			break
		}
	}
	// Add final .zip
	volPaths = append(volPaths, primaryZipPath)

	var volumes []Volume
	var closers []io.Closer

	closeAll := func() error {
		var firstErr error
		for _, c := range closers {
			if err := c.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}

	for i, path := range volPaths {
		f, err := os.Open(path)
		if err != nil {
			_ = closeAll()
			return nil, nil, err
		}
		closers = append(closers, f)
		fi, err := f.Stat()
		if err != nil {
			_ = closeAll()
			return nil, nil, err
		}
		volumes = append(volumes, Volume{
			DiskIndex: i,
			Reader:    f,
			Size:      fi.Size(),
			Path:      path,
		})
	}

	mvr, err := NewMultiVolumeReader(volumes)
	if err != nil {
		_ = closeAll()
		return nil, nil, err
	}

	return mvr, closeAll, nil
}

// VolumeOpener creates a new writer for the given disk index (0 = .z01, 1 = .z02, ..., isLast = .zip).
type VolumeOpener func(diskIndex int, isLast bool) (io.WriteCloser, error)

// MultiVolumeWriter splits a ZIP archive across multiple fixed-size volume files.
type MultiVolumeWriter struct {
	opener        VolumeOpener
	maxVolumeSize int64
	currentDisk   int
	currentWriter io.WriteCloser
	writtenInDisk int64
	totalWritten  int64
	diskStartOffs []int64
	closed        bool
	mu            sync.Mutex
}

// NewMultiVolumeWriter constructs a writer that rotates to a new volume whenever maxVolumeSize is reached.
func NewMultiVolumeWriter(opener VolumeOpener, maxVolumeSize int64) *MultiVolumeWriter {
	if maxVolumeSize <= 0 {
		maxVolumeSize = 100 * 1024 * 1024 // Default 100MB
	}
	return &MultiVolumeWriter{
		opener:        opener,
		maxVolumeSize: maxVolumeSize,
	}
}

// Write appends bytes to the current volume, transparently rotating when maxVolumeSize is reached.
func (mvw *MultiVolumeWriter) Write(p []byte) (int, error) {
	mvw.mu.Lock()
	defer mvw.mu.Unlock()

	if mvw.closed {
		return 0, errors.New("zip: write to closed multi-volume writer")
	}

	if mvw.currentWriter == nil {
		if err := mvw.rotateVolume(false); err != nil {
			return 0, err
		}
	}

	totalWritten := 0
	for len(p) > 0 {
		remainingInDisk := mvw.maxVolumeSize - mvw.writtenInDisk
		if remainingInDisk <= 0 {
			if err := mvw.rotateVolume(false); err != nil {
				return totalWritten, err
			}
			remainingInDisk = mvw.maxVolumeSize
		}

		toWrite := len(p)
		if int64(toWrite) > remainingInDisk {
			toWrite = int(remainingInDisk)
		}

		n, err := mvw.currentWriter.Write(p[:toWrite])
		mvw.writtenInDisk += int64(n)
		mvw.totalWritten += int64(n)
		totalWritten += n
		p = p[n:]

		if err != nil {
			return totalWritten, err
		}
	}

	return totalWritten, nil
}

func (mvw *MultiVolumeWriter) rotateVolume(isLast bool) error {
	if mvw.currentWriter != nil {
		if err := mvw.currentWriter.Close(); err != nil {
			return err
		}
	}

	w, err := mvw.opener(mvw.currentDisk, isLast)
	if err != nil {
		return err
	}
	mvw.currentWriter = w
	mvw.diskStartOffs = append(mvw.diskStartOffs, mvw.totalWritten)
	mvw.writtenInDisk = 0
	mvw.currentDisk++
	return nil
}

// TotalWritten returns the total number of bytes written across all volumes.
func (mvw *MultiVolumeWriter) TotalWritten() int64 {
	mvw.mu.Lock()
	defer mvw.mu.Unlock()
	return mvw.totalWritten
}

// TotalDisks returns the number of disks created so far.
func (mvw *MultiVolumeWriter) TotalDisks() int {
	mvw.mu.Lock()
	defer mvw.mu.Unlock()
	return mvw.currentDisk
}

// Close closes the active volume writer.
func (mvw *MultiVolumeWriter) Close() error {
	mvw.mu.Lock()
	defer mvw.mu.Unlock()

	if mvw.closed {
		return nil
	}
	mvw.closed = true

	if mvw.currentWriter != nil {
		return mvw.currentWriter.Close()
	}
	return nil
}

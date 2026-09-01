// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"errors"
	"fmt"
	"io/fs"

	zipcontainer "github.com/lemon4ksan/seal/container/zip"
	"github.com/lemon4ksan/seal/vfs"
)

// FileError annotates an error with contextual information about the archive file entry where the failure originated.
type FileError struct {
	Op   string // Operation that failed (e.g. "open", "read", "compress", "extract", "verify")
	Path string // Archive relative path of the file entry
	Err  error  // Underlying cause or sentinel error
}

// Error returns a formatted error string.
func (e *FileError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("seal: %s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("seal: %s %q: %v", e.Op, e.Path, e.Err)
}

// Unwrap returns the underlying wrapped error.
func (e *FileError) Unwrap() error {
	return e.Err
}

// Global sentinel errors.
var (
	// ErrFormat indicates that the input payload is not a valid archive or has an invalid container structure.
	ErrFormat = errors.New("seal: not a valid archive")

	// ErrUnsupportedFormat indicates that the archive format is unrecognized or unsupported.
	ErrUnsupportedFormat = errors.New("seal: unsupported archive format")

	// ErrNotImplemented indicates an unimplemented execution path.
	ErrNotImplemented = errors.New("seal: feature not implemented")

	// ErrAlgorithm indicates that the requested compression method is unregistered or unsupported.
	ErrAlgorithm = errors.New("seal: unsupported compression algorithm")

	// ErrPasswordMismatch indicates that an archive entry requires encryption and either no password or an incorrect password was supplied.
	ErrPasswordMismatch = zipcontainer.ErrPasswordMismatch

	// ErrChecksum indicates that a CRC-32, Adler-32, or hash integrity check failed on decompression.
	ErrChecksum = errors.New("seal: checksum verification failed")

	// ErrSizeMismatch indicates that the actual uncompressed payload length did not match the header descriptor.
	ErrSizeMismatch = errors.New("seal: uncompressed size mismatch")

	// ErrFileNotFound indicates that the requested file entry was not found in the archive.
	ErrFileNotFound = fmt.Errorf("seal: file not found: %w", fs.ErrNotExist)

	// ErrInsecurePath indicates a path traversal attempt escaping the target boundary (Zip Slip / Tar Slip).
	ErrInsecurePath = vfs.ErrInsecurePath

	// ErrDuplicateEntry indicates that an entry with the identical normalized path already exists in the archive.
	ErrDuplicateEntry = errors.New("seal: duplicate entry path")

	// ErrFilenameTooLong indicates that a filename exceeds the container specification limit (e.g. 65,535 bytes).
	ErrFilenameTooLong = errors.New("seal: filename exceeds container limit")

	// ErrResourceLimit indicates that decompression exceeded configured resource limits (e.g. decompression bomb).
	ErrResourceLimit = vfs.ErrResourceLimit
)

// IsNotFound reports whether err signifies that an archive file entry was not found.
func IsNotFound(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrFileNotFound)
}

// IsPasswordMismatch reports whether err signifies an authentication or decryption failure.
func IsPasswordMismatch(err error) bool {
	return errors.Is(err, ErrPasswordMismatch) || errors.Is(err, zipcontainer.ErrPasswordMismatch) ||
		errors.Is(err, zipcontainer.ErrAuthenticationFail)
}

// IsCorrupted reports whether err indicates data corruption or checksum mismatch.
func IsCorrupted(err error) bool {
	return errors.Is(err, ErrChecksum) || errors.Is(err, ErrSizeMismatch) || errors.Is(err, ErrFormat)
}

// IsZipSlip reports whether err indicates an insecure path or directory traversal vulnerability.
func IsZipSlip(err error) bool {
	return errors.Is(err, ErrInsecurePath) || errors.Is(err, vfs.ErrInsecurePath)
}

// IsResourceLimit reports whether err indicates that extraction exceeded maximum size or ratio limits.
func IsResourceLimit(err error) bool {
	return errors.Is(err, ErrResourceLimit) || errors.Is(err, vfs.ErrResourceLimit)
}

// IsDuplicate reports whether err indicates a duplicate entry collision.
func IsDuplicate(err error) bool {
	return errors.Is(err, ErrDuplicateEntry)
}

// IsUnsupportedFormat reports whether err indicates an unrecognized archive format.
func IsUnsupportedFormat(err error) bool {
	return errors.Is(err, ErrUnsupportedFormat)
}

// wrapErr is an internal helper to wrap errors into a [FileError].
func wrapErr(op, entryName string, err error) error {
	if err == nil {
		return nil
	}
	return &FileError{
		Op:   op,
		Path: entryName,
		Err:  err,
	}
}

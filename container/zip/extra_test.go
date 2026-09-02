// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package zip_test

import (
	"testing"
	"time"

	"github.com/lemon4ksan/seal/container/zip"
)

func TestExtendedTimestamp_Roundtrip(t *testing.T) {
	mtime := time.Unix(1700000000, 0).UTC()
	atime := time.Unix(1700000100, 0).UTC()
	ctime := time.Unix(1700000200, 0).UTC()

	// Local header encoding (all 3 timestamps)
	encLocal := zip.EncodeExtendedTimestamp(mtime, atime, ctime, false)
	// Skip 4-byte header ID and length
	parsedMtime, parsedAtime, parsedCtime, ok := zip.ParseExtendedTimestamp(encLocal[4:])
	if !ok {
		t.Fatalf("ParseExtendedTimestamp failed")
	}
	if !parsedMtime.Equal(mtime) {
		t.Errorf("mtime mismatch: got %v, want %v", parsedMtime, mtime)
	}
	if !parsedAtime.Equal(atime) {
		t.Errorf("atime mismatch: got %v, want %v", parsedAtime, atime)
	}
	if !parsedCtime.Equal(ctime) {
		t.Errorf("ctime mismatch: got %v, want %v", parsedCtime, ctime)
	}

	// Central directory encoding (only mtime)
	encCD := zip.EncodeExtendedTimestamp(mtime, atime, ctime, true)
	parsedMtimeCD, parsedAtimeCD, _, ok := zip.ParseExtendedTimestamp(encCD[4:])
	if !ok {
		t.Fatalf("ParseExtendedTimestamp (CD) failed")
	}
	if !parsedMtimeCD.Equal(mtime) {
		t.Errorf("CD mtime mismatch: got %v, want %v", parsedMtimeCD, mtime)
	}
	if !parsedAtimeCD.IsZero() {
		t.Errorf("CD atime should be zero, got %v", parsedAtimeCD)
	}
}

func TestUnicodePath_Roundtrip(t *testing.T) {
	utf8Name := "тест/документ_привет.txt"
	rawName := "test/dok_privet.txt"

	enc := zip.EncodeUnicodePath(utf8Name, rawName)
	parsedName, ok := zip.ParseUnicodePath(enc[4:], rawName)
	if !ok {
		t.Fatalf("ParseUnicodePath failed")
	}
	if parsedName != utf8Name {
		t.Errorf("Unicode path mismatch: got %q, want %q", parsedName, utf8Name)
	}

	// If header name changed, validation must fail
	_, okTampered := zip.ParseUnicodePath(enc[4:], "tampered_name.txt")
	if okTampered {
		t.Errorf("expected CRC mismatch on tampered header name, but succeeded")
	}
}

func TestUnicodeComment_Roundtrip(t *testing.T) {
	utf8Comment := "Архивный комментарий на русском языке"
	rawComment := "Archive comment"

	enc := zip.EncodeUnicodeComment(utf8Comment, rawComment)
	parsedComment, ok := zip.ParseUnicodeComment(enc[4:], rawComment)
	if !ok {
		t.Fatalf("ParseUnicodeComment failed")
	}
	if parsedComment != utf8Comment {
		t.Errorf("Unicode comment mismatch: got %q, want %q", parsedComment, utf8Comment)
	}
}

func TestInfoZipUIDGID_Roundtrip(t *testing.T) {
	uid := uint32(1001)
	gid := uint32(1002)

	enc := zip.EncodeInfoZipUIDGID(uid, gid)
	parsedUID, parsedGID, ok := zip.ParseInfoZipUIDGID(enc[4:])
	if !ok {
		t.Fatalf("ParseInfoZipUIDGID failed")
	}
	if parsedUID != uid || parsedGID != gid {
		t.Errorf("UID/GID mismatch: got (%d, %d), want (%d, %d)", parsedUID, parsedGID, uid, gid)
	}
}

func TestDataStreamAlignment_Encoding(t *testing.T) {
	enc := zip.EncodeDataStreamAlignment(4096, 12)
	if len(enc) != 4+2+12 {
		t.Errorf("expected length %d, got %d", 4+2+12, len(enc))
	}
}

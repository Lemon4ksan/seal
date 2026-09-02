// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/seal/file"
)

func TestSevenZip_Doctor_SignatureRealignment(t *testing.T) {
	var buf inMemoryWriteSeeker
	zw := NewWriter(&buf)

	f1, _ := file.New("notes.txt", false)
	f1.WithData([]byte("Important meeting notes."))
	f2, _ := file.New("calc.exe", false)
	f2.WithData([]byte("Binary executable payload."))

	_, err := zw.WriteArchive([]*file.File{f1, f2})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}

	validArchive := buf.buf

	// Prepend 1024 bytes of damaged SFX / garbage bytes to shift the signature
	junkPrefix := bytes.Repeat([]byte{0xCC}, 1024)
	shiftedArchive := make([]byte, 0, len(junkPrefix)+len(validArchive))
	shiftedArchive = append(shiftedArchive, junkPrefix...)
	shiftedArchive = append(shiftedArchive, validArchive...)

	var repairedBuf inMemoryWriteSeeker
	summary, err := Repair(bytes.NewReader(shiftedArchive), int64(len(shiftedArchive)), &repairedBuf, RepairOptions{
		SanitizePaths: true,
	})
	if err != nil {
		t.Fatalf("Repair failed on shifted archive: %v", err)
	}

	if !summary.SignatureRealigned {
		t.Errorf("expected SignatureRealigned to be true")
	}
	if summary.FilesRecovered != 2 {
		t.Errorf("expected 2 files recovered, got %d", summary.FilesRecovered)
	}

	// Verify repaired archive can be opened directly without shift
	zr, err := NewReader(bytes.NewReader(repairedBuf.buf), int64(len(repairedBuf.buf)), "")
	if err != nil {
		t.Fatalf("opening repaired archive failed: %v", err)
	}
	if len(zr.Entries()) != 2 {
		t.Fatalf("expected 2 entries in repaired archive, got %d", len(zr.Entries()))
	}
}

func TestSevenZip_Doctor_PathSanitization(t *testing.T) {
	var buf inMemoryWriteSeeker
	zw := NewWriter(&buf)

	payload := []byte("echo hacked > /tmp/hacked")
	malicious, _ := file.New("exploit", false)
	malicious.WithRawName("../../../etc/cron.d/exploit")
	malicious.WithData(payload)

	_, err := zw.WriteArchive([]*file.File{malicious})
	if err != nil {
		t.Fatalf("WriteArchive failed: %v", err)
	}

	rawArchive := buf.buf

	var repairedBuf inMemoryWriteSeeker
	summary, err := Repair(bytes.NewReader(rawArchive), int64(len(rawArchive)), &repairedBuf, RepairOptions{
		SanitizePaths: true,
	})
	if err != nil {
		t.Fatalf("Repair failed: %v", err)
	}

	if summary.PathsSanitized != 1 {
		t.Errorf("expected 1 path sanitized, got %d", summary.PathsSanitized)
	}
	if summary.FilesRecovered != 1 {
		t.Errorf("expected 1 file recovered, got %d", summary.FilesRecovered)
	}

	// Verify path in repaired archive is clean
	zr, err := NewReader(bytes.NewReader(repairedBuf.buf), int64(len(repairedBuf.buf)), "")
	if err != nil {
		t.Fatalf("opening repaired archive failed: %v", err)
	}

	entries := zr.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Name() != "etc/cron.d/exploit" {
		t.Errorf("expected sanitized name 'etc/cron.d/exploit', got %q", entries[0].Name())
	}

	rc, err := entries[0].Open()
	if err != nil {
		t.Fatalf("open entry failed: %v", err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()

	if !bytes.Equal(data, payload) {
		t.Errorf("content corrupted during repair: got %q, want %q", string(data), string(payload))
	}
}

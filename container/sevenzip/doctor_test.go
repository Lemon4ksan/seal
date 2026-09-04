// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sevenzip

import (
	"bytes"
	"io"
	"testing"

	"github.com/lemon4ksan/foundation/testkit/assert"
	"github.com/lemon4ksan/foundation/testkit/require"

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
	require.NoError(t, err)

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
	require.NoError(t, err)

	assert.True(t, summary.SignatureRealigned)
	assert.Equal(t, 2, summary.FilesRecovered)

	// Verify repaired archive can be opened directly without shift
	zr, err := NewReader(bytes.NewReader(repairedBuf.buf), int64(len(repairedBuf.buf)), "")
	require.NoError(t, err)
	assert.Len(t, zr.Entries(), 2)
}

func TestSevenZip_Doctor_PathSanitization(t *testing.T) {
	var buf inMemoryWriteSeeker
	zw := NewWriter(&buf)

	payload := []byte("echo hacked > /tmp/hacked")
	malicious, _ := file.New("exploit", false)
	malicious.WithRawName("../../../etc/cron.d/exploit")
	malicious.WithData(payload)

	_, err := zw.WriteArchive([]*file.File{malicious})
	require.NoError(t, err)

	rawArchive := buf.buf

	var repairedBuf inMemoryWriteSeeker
	summary, err := Repair(bytes.NewReader(rawArchive), int64(len(rawArchive)), &repairedBuf, RepairOptions{
		SanitizePaths: true,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, summary.PathsSanitized)
	assert.Equal(t, 1, summary.FilesRecovered)

	// Verify path in repaired archive is clean
	zr, err := NewReader(bytes.NewReader(repairedBuf.buf), int64(len(repairedBuf.buf)), "")
	require.NoError(t, err)

	entries := zr.Entries()
	require.Len(t, entries, 1)

	assert.Equal(t, "etc/cron.d/exploit", entries[0].Name())

	rc, err := entries[0].Open()
	require.NoError(t, err)
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	require.NoError(t, err)

	assert.Equal(t, payload, data)
}

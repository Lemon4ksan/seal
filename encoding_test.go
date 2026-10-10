// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"testing"
)

func TestDecodeCP866(t *testing.T) {
	// "Привет, мир!" in CP866
	cp866Bytes := "\x8f\xe0\xa8\xa2\xa5\xe2, \xac\xa8\xe0!"
	expected := "Привет, мир!"

	got := DecodeCP866(cp866Bytes)
	if got != expected {
		t.Fatalf("DecodeCP866 failed: got %q, expected %q", got, expected)
	}
}

func TestDecodeCP437(t *testing.T) {
	// "résumé" in CP437: r \x82 s \x82 m \x82
	cp437Bytes := "r\x82sum\x82"
	expected := "résumé"

	got := DecodeCP437(cp437Bytes)
	if got != expected {
		t.Fatalf("DecodeCP437 failed: got %q, expected %q", got, expected)
	}
}

func TestDecoderByName(t *testing.T) {
	tests := []struct {
		encoding string
		input    string
		expected string
	}{
		{"866", "\x8f\xe0\xa8\xa2\xa5\xe2", "Привет"},
		{"cp866", "\x8f\xe0\xa8\xa2\xa5\xe2", "Привет"},
		{"windows-1251", "\xcf\xf0\xe8\xe2\xe5\xf2", "Привет"},
		{"1251", "\xcf\xf0\xe8\xe2\xe5\xf2", "Привет"},
		{"cp1251", "\xcf\xf0\xe8\xe2\xe5\xf2", "Привет"},
		{"shift_jis", "\x93\xfa\x96\x7b", "日本"},
		{"932", "\x93\xfa\x96\x7b", "日本"},
		{"gbk", "\xd6\xd0\xce\xc4", "中文"},
		{"utf-8", "Привет", "Привет"},
	}

	for _, tc := range tests {
		dec, err := DecoderByName(tc.encoding)
		if err != nil {
			t.Fatalf("DecoderByName(%q) returned error: %v", tc.encoding, err)
		}
		got := dec(tc.input)
		if got != tc.expected {
			t.Errorf("DecoderByName(%q)(%q): got %q, expected %q", tc.encoding, tc.input, got, tc.expected)
		}
	}
}

func TestEncoderByName(t *testing.T) {
	enc, err := EncoderByName("cp866")
	if err != nil {
		t.Fatalf("EncoderByName(cp866) error: %v", err)
	}

	encoded, err := enc("Привет")
	if err != nil {
		t.Fatalf("enc(Привет) error: %v", err)
	}
	expected := "\x8f\xe0\xa8\xa2\xa5\xe2"
	if encoded != expected {
		t.Fatalf("EncoderByName: got %q, expected %q", encoded, expected)
	}
}

func TestDecodeText(t *testing.T) {
	// Case 1: isUTF8Flag is true
	if got := DecodeText("Hello", true, nil); got != "Hello" {
		t.Errorf("got %q, expected %q", got, "Hello")
	}

	// Case 2: valid UTF-8 string even when flag is false
	if got := DecodeText("Привет", false, nil); got != "Привет" {
		t.Errorf("got %q, expected %q", got, "Привет")
	}

	// Case 3: non-UTF8 string with custom fallback
	cp1251Bytes := "\xcf\xf0\xe8\xe2\xe5\xf2" // "Привет" in 1251
	dec1251, _ := DecoderByName("1251")
	if got := DecodeText(cp1251Bytes, false, dec1251); got != "Привет" {
		t.Errorf("got %q, expected %q", got, "Привет")
	}

	// Case 4: non-UTF8 string with DefaultTextDecoder
	cp437Bytes := "r\x82sum\x82"
	got := DecodeText(cp437Bytes, false, nil)
	if got == "" {
		t.Errorf("expected non-empty decoded text")
	}
}

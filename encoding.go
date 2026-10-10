// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package seal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/lemon4ksan/seal/internal/sys"
	"github.com/lemon4ksan/seal/text/encoding"
	"github.com/lemon4ksan/seal/text/encoding/charmap"
	"github.com/lemon4ksan/seal/text/encoding/htmlindex"
)

// TextDecoder defines a function converting encoded legacy strings into standard UTF-8.
type TextDecoder func(string) string

// TextEncoder defines a function converting UTF-8 strings into encoded legacy bytes/strings.
type TextEncoder func(string) (string, error)

// DecodeText converts filename or comment metadata to UTF-8 using flags and heuristic fallbacks.
func DecodeText(data string, isUTF8Flag bool, fallback TextDecoder) string {
	if isUTF8Flag {
		return data
	}

	if utf8.ValidString(data) {
		return data
	}

	if fallback != nil {
		return fallback(data)
	}

	return DefaultTextDecoder()(data)
}

// DecodeCP866 converts CP866 (Cyrillic DOS) legacy string to UTF-8.
func DecodeCP866(s string) string {
	res, err := charmap.CodePage866.NewDecoder().String(s)
	if err != nil {
		return s
	}
	return res
}

// DecodeCP437 converts CP437 (US DOS) legacy string to UTF-8.
func DecodeCP437(s string) string {
	res, err := charmap.CodePage437.NewDecoder().String(s)
	if err != nil {
		return s
	}
	return res
}

// LookupEncoding resolves a character encoding by name, IANA tag, or numeric code page identifier.
func LookupEncoding(name string) (encoding.Encoding, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, fmt.Errorf("encoding: empty encoding name")
	}

	// Direct aliases and common code page mappings
	switch name {
	case "866", "cp866", "ibm866", "ibm-866", "csibm866":
		return charmap.CodePage866, nil
	case "437", "cp437", "ibm437", "ibm-437", "csibm437":
		return charmap.CodePage437, nil
	case "850", "cp850", "ibm850", "ibm-850":
		return charmap.CodePage850, nil
	case "852", "cp852", "ibm852", "ibm-852":
		return charmap.CodePage852, nil
	case "855", "cp855", "ibm855", "ibm-855":
		return charmap.CodePage855, nil
	case "932", "cp932", "sjis", "shift_jis", "shift-jis", "ms932":
		return htmlindex.Get("shift_jis")
	case "936", "cp936", "gbk", "ms936":
		return htmlindex.Get("gbk")
	case "949", "cp949", "euckr", "euc-kr", "ms949":
		return htmlindex.Get("euc-kr")
	case "950", "cp950", "big5", "ms950":
		return htmlindex.Get("big5")
	case "1251", "cp1251", "windows-1251", "win-1251":
		return charmap.Windows1251, nil
	case "1252", "cp1252", "windows-1252", "win-1252":
		return charmap.Windows1252, nil
	case "1250", "cp1250", "windows-1250":
		return charmap.Windows1250, nil
	case "1253", "cp1253", "windows-1253":
		return charmap.Windows1253, nil
	case "1254", "cp1254", "windows-1254":
		return charmap.Windows1254, nil
	case "1255", "cp1255", "windows-1255":
		return charmap.Windows1255, nil
	case "1256", "cp1256", "windows-1256":
		return charmap.Windows1256, nil
	case "1257", "cp1257", "windows-1257":
		return charmap.Windows1257, nil
	case "1258", "cp1258", "windows-1258":
		return charmap.Windows1258, nil
	case "koi8-r", "koi8r":
		return charmap.KOI8R, nil
	case "koi8-u", "koi8u":
		return charmap.KOI8U, nil
	case "iso-8859-1", "latin1":
		return charmap.ISO8859_1, nil
	case "iso-8859-5":
		return charmap.ISO8859_5, nil
	case "utf-8", "utf8":
		return encoding.Nop, nil
	}

	// If numeric code page like "1251" or "cp1251"
	if strings.HasPrefix(name, "cp") {
		numPart := strings.TrimPrefix(name, "cp")
		if _, err := strconv.Atoi(numPart); err == nil {
			if enc, err := LookupEncoding(numPart); err == nil {
				return enc, nil
			}
			if enc, err := htmlindex.Get("windows-" + numPart); err == nil {
				return enc, nil
			}
		}
	}

	// Try standard htmlindex lookup
	return htmlindex.Get(name)
}

// DecoderByName returns a [TextDecoder] for the named character encoding or code page.
func DecoderByName(name string) (TextDecoder, error) {
	enc, err := LookupEncoding(name)
	if err != nil {
		return nil, err
	}
	if enc == encoding.Nop {
		return func(s string) string { return s }, nil
	}

	return func(s string) string {
		dec := enc.NewDecoder()
		res, err := dec.String(s)
		if err != nil {
			return s
		}
		return res
	}, nil
}

// EncoderByName returns a [TextEncoder] converting UTF-8 strings into the target character encoding.
func EncoderByName(name string) (TextEncoder, error) {
	enc, err := LookupEncoding(name)
	if err != nil {
		return nil, err
	}
	if enc == encoding.Nop {
		return func(s string) (string, error) { return s, nil }, nil
	}

	return func(s string) (string, error) {
		encoder := enc.NewEncoder()
		return encoder.String(s)
	}, nil
}

var (
	defaultDecoderOnce sync.Once
	defaultDecoder     TextDecoder
)

// DefaultTextDecoder returns the host system's recommended fallback decoder for non-UTF8 strings.
// On Windows, it detects the active OEM code page (e.g. CP866 on Cyrillic locales, CP932 on Japanese).
// On non-Windows platforms, it inspects locale environment variables or defaults to CP437 per PKWARE spec.
func DefaultTextDecoder() TextDecoder {
	defaultDecoderOnce.Do(func() {
		// 1. Check Windows OEM Code Page
		if oem := sys.GetOEMCP(); oem > 0 && oem != 437 {
			if dec, err := DecoderByName(strconv.Itoa(int(oem))); err == nil {
				defaultDecoder = dec
				return
			}
		}

		// 2. Check Unix locale environment variables
		for _, env := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
			if val := os.Getenv(env); val != "" {
				// e.g. "ru_RU.CP1251" or "ja_JP.SJIS"
				if dot := strings.Index(val, "."); dot >= 0 {
					charset := val[dot+1:]
					if at := strings.Index(charset, "@"); at >= 0 {
						charset = charset[:at]
					}
					if dec, err := DecoderByName(charset); err == nil {
						defaultDecoder = dec
						return
					}
				}
				// e.g. "ru_RU" -> windows-1251
				defCharset := htmlindex.LanguageDefault(val)
				if defCharset != "" && defCharset != "windows-1252" {
					if dec, err := DecoderByName(defCharset); err == nil {
						defaultDecoder = dec
						return
					}
				}
			}
		}

		// 3. PKWARE ZIP default
		defaultDecoder = DecodeCP437
	})

	return defaultDecoder
}

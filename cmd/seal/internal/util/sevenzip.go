// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"path"
	"strconv"
	"strings"
)

// OverwritePolicy defines how existing files are handled on extraction.
type OverwritePolicy int

const (
	OverwriteAll            OverwritePolicy = iota // -aoa (default)
	OverwriteSkip                                  // -aos
	OverwriteRenameNew                             // -aou
	OverwriteRenameExisting                        // -aot
)

func (p OverwritePolicy) String() string {
	switch p {
	case OverwriteSkip:
		return "skip"
	case OverwriteRenameNew:
		return "rename-new"
	case OverwriteRenameExisting:
		return "rename-existing"
	default:
		return "overwrite"
	}
}

// SevenZipArgs represents normalized command-line arguments and flags compatible with 7-Zip.
type SevenZipArgs struct {
	Command         string
	Archive         string
	OutputDir       string
	Password        string
	Method          string
	Level           int
	Workers         int
	IncludePatterns []string
	ExcludePatterns []string
	Recurse         bool
	Overwrite       OverwritePolicy
	AssumeYes       bool
	DeleteSource    bool
	StdOut          bool
	StdIn           bool
	LogLevel        int
	StreamRedir     map[string]int
	Filter          string
	Comment         string
	Items           []string
	GPUAcceleration bool
	Solid           bool
	SolidExplicit   bool
	SolidSize       int64
	SolidExtension  bool
	Encoding        string
}

// ParseSevenZipArgs parses raw command-line arguments using 7-Zip switch conventions.
func ParseSevenZipArgs(cmdName string, rawArgs []string) (*SevenZipArgs, error) {
	s := &SevenZipArgs{
		Command:     cmdName,
		Level:       -1, // -1 means default
		Workers:     0,  // 0 means auto
		Recurse:     true,
		Overwrite:   OverwriteAll,
		StreamRedir: make(map[string]int),
		Filter:      "none",
	}

	var positionals []string
	n := len(rawArgs)

	for i := 0; i < n; i++ {
		arg := rawArgs[i]

		// Handle strict POSIX separator
		if arg == "--" {
			positionals = append(positionals, rawArgs[i+1:]...)
			break
		}

		// Handle flags
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			lower := strings.ToLower(arg)

			// Output directory: -o<dir> or -o <dir>
			if strings.HasPrefix(arg, "-o") {
				val := strings.TrimPrefix(arg[2:], "=")
				if val != "" {
					s.OutputDir = val
				} else if i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-") {
					s.OutputDir = rawArgs[i+1]
					i++
				}
				continue
			}

			// Destination directory alias: -d <dir> or -d=<dir>
			if strings.HasPrefix(arg, "-d") && (len(arg) == 2 || arg[2] == '=') {
				val := ""
				if len(arg) > 2 && arg[2] == '=' {
					val = arg[3:]
				} else if i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-") {
					val = rawArgs[i+1]
					i++
				}
				if val != "" {
					s.OutputDir = val
				}
				continue
			}

			// Password: -p<pwd> or -p <pwd> or -password
			if strings.HasPrefix(arg, "-p") {
				if strings.HasPrefix(arg, "-password") {
					val := strings.TrimPrefix(arg, "-password")
					if strings.HasPrefix(val, "=") {
						s.Password = val[1:]
					} else if i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-") {
						s.Password = rawArgs[i+1]
						i++
					}
					continue
				}
				val := strings.TrimPrefix(arg[2:], "=")
				switch {
				case val != "":
					s.Password = val
				case i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-"):
					s.Password = rawArgs[i+1]
					i++
				default:
					s.Password = ""
				}
				continue
			}

			// Compression level: -mx[=0..9] or -mx0..9
			if strings.HasPrefix(lower, "-mx") {
				val := strings.TrimPrefix(lower, "-mx")
				val = strings.TrimPrefix(val, "=")
				if val != "" {
					if lvl, err := strconv.Atoi(val); err == nil {
						s.Level = lvl
					}
				}
				continue
			}

			// Compression method: -m0=<method> or -m=<method> or -m <method>
			if strings.HasPrefix(lower, "-m0=") || strings.HasPrefix(lower, "-m=") || lower == "-m" {
				var val string
				switch {
				case strings.HasPrefix(lower, "-m0="):
					val = arg[5:]
				case strings.HasPrefix(lower, "-m="):
					val = arg[3:]
				case i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-"):
					val = rawArgs[i+1]
					i++
				}
				if val != "" {
					s.Method = val
				}
				continue
			}

			// Multithreading: -mmt[=on|off|N] or -mmt<N>
			if strings.HasPrefix(lower, "-mmt") {
				val := strings.TrimPrefix(lower, "-mmt")
				val = strings.TrimPrefix(val, "=")
				if val == "on" || val == "" {
					s.Workers = 0 // auto
				} else if val == "off" {
					s.Workers = 1
				} else if w, err := strconv.Atoi(val); err == nil {
					s.Workers = w
				}
				continue
			}

			// GPU Acceleration: -gpu
			if lower == "-gpu" {
				s.GPUAcceleration = true
				continue
			}

			// Code page / Encoding: -mcp[=<codepage>] or -mcp:<codepage> or --encoding[=<enc>] or -encoding[=...]
			if strings.HasPrefix(lower, "-mcp") || strings.HasPrefix(lower, "--encoding") ||
				strings.HasPrefix(lower, "-encoding") ||
				strings.HasPrefix(lower, "-charset") {
				var val string
				switch {
				case strings.HasPrefix(lower, "--encoding"):
					val = arg[10:]
				case strings.HasPrefix(lower, "-encoding"):
					val = arg[9:]
				case strings.HasPrefix(lower, "-charset"):
					val = arg[8:]
				default:
					val = arg[4:]
				}
				val = strings.TrimPrefix(val, "=")
				val = strings.TrimPrefix(val, ":")
				if val != "" {
					s.Encoding = val
				} else if i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-") {
					s.Encoding = rawArgs[i+1]
					i++
				}
				continue
			}

			// Solid mode: -ms[=on|off|e|f|<size>] or -solid[=on|off|e|<size>] or --solid[=...]
			if strings.HasPrefix(lower, "-ms") || strings.HasPrefix(lower, "-solid") ||
				strings.HasPrefix(lower, "--solid") ||
				lower == "--no-solid" {
				if lower == "--no-solid" {
					s.Solid = false
					s.SolidExplicit = true
					continue
				}
				val := ""
				switch {
				case strings.HasPrefix(lower, "--solid"):
					val = strings.TrimPrefix(lower, "--solid")
				case strings.HasPrefix(lower, "-solid"):
					val = strings.TrimPrefix(lower, "-solid")
				default:
					val = strings.TrimPrefix(lower, "-ms")
				}
				val = strings.TrimPrefix(val, "=")
				val = strings.TrimPrefix(val, ":")
				s.SolidExplicit = true
				switch val {
				case "", "on", "+":
					s.Solid = true
				case "off", "-":
					s.Solid = false
				case "e":
					s.Solid = true
					s.SolidExtension = true
				case "f":
					s.Solid = true
					s.SolidSize = 1 << 30 // 1 GiB full solid block
				default:
					s.Solid = true
					if strings.Contains(val, "e") {
						s.SolidExtension = true
						val = strings.ReplaceAll(val, "e", "")
					}
					if sz, err := ParseByteSize(val); err == nil && sz > 0 {
						s.SolidSize = sz
					}
				}
				continue
			}

			// Exclude pattern: -x![pat] or -xr![pat] or -x=<pat> or -x <pat>
			if strings.HasPrefix(lower, "-x!") || strings.HasPrefix(lower, "-xr!") || strings.HasPrefix(lower, "-x=") ||
				lower == "-x" {
				var pat string
				switch {
				case strings.HasPrefix(lower, "-xr!"):
					pat = arg[5:]
				case strings.HasPrefix(lower, "-x!"):
					pat = arg[3:]
				case strings.HasPrefix(lower, "-x="):
					pat = arg[3:]
				case i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-"):
					pat = rawArgs[i+1]
					i++
				}
				if pat != "" {
					s.ExcludePatterns = append(s.ExcludePatterns, pat)
				}
				continue
			}

			// Include pattern: -i![pat] or -ir![pat] or -i=<pat> or -i <pat>
			if strings.HasPrefix(lower, "-i!") || strings.HasPrefix(lower, "-ir!") || strings.HasPrefix(lower, "-i=") ||
				lower == "-i" {
				var pat string
				switch {
				case strings.HasPrefix(lower, "-ir!"):
					pat = arg[5:]
				case strings.HasPrefix(lower, "-i!"):
					pat = arg[3:]
				case strings.HasPrefix(lower, "-i="):
					pat = arg[3:]
				case i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-"):
					pat = rawArgs[i+1]
					i++
				}
				if pat != "" {
					s.IncludePatterns = append(s.IncludePatterns, pat)
				}
				continue
			}

			// Overwrite mode: -ao{a|s|u|t}
			if strings.HasPrefix(lower, "-ao") {
				mode := strings.TrimPrefix(lower, "-ao")
				switch mode {
				case "a":
					s.Overwrite = OverwriteAll
				case "s":
					s.Overwrite = OverwriteSkip
				case "u":
					s.Overwrite = OverwriteRenameNew
				case "t":
					s.Overwrite = OverwriteRenameExisting
				}
				continue
			}

			// Assume yes: -y
			if lower == "-y" || lower == "--yes" {
				s.AssumeYes = true
				continue
			}

			// Delete source: -sdel
			if lower == "-sdel" {
				s.DeleteSource = true
				continue
			}

			// Stdout: -so
			if lower == "-so" {
				s.StdOut = true
				continue
			}

			// Stdin: -si
			if lower == "-si" {
				s.StdIn = true
				continue
			}

			// Log level: -bb[0-3]
			if strings.HasPrefix(lower, "-bb") {
				val := strings.TrimPrefix(lower, "-bb")
				if lvl, err := strconv.Atoi(val); err == nil {
					s.LogLevel = lvl
				}
				continue
			}

			// Stream redirection: -bso[0-2], -bse[0-2], -bsp[0-2]
			if strings.HasPrefix(lower, "-bso") || strings.HasPrefix(lower, "-bse") ||
				strings.HasPrefix(lower, "-bsp") {
				streamKey := lower[1:4]
				valStr := lower[4:]
				if v, err := strconv.Atoi(valStr); err == nil {
					s.StreamRedir[streamKey] = v
				}
				continue
			}

			// Recurse subdirectories: -r or -r-
			if lower == "-r" {
				s.Recurse = true
				continue
			}
			if lower == "-r-" {
				s.Recurse = false
				continue
			}

			// Level flag alias: -l <level> or -l=<level>
			if strings.HasPrefix(lower, "-l") {
				val := strings.TrimPrefix(lower, "-l")
				val = strings.TrimPrefix(val, "=")
				if val != "" {
					if lvl, err := strconv.Atoi(val); err == nil {
						s.Level = lvl
					}
				} else if i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-") {
					if lvl, err := strconv.Atoi(rawArgs[i+1]); err == nil {
						s.Level = lvl
					}
					i++
				}
				continue
			}

			// Filter: -filter <f> or -filter=<f>
			if strings.HasPrefix(lower, "-filter") {
				val := strings.TrimPrefix(lower, "-filter")
				val = strings.TrimPrefix(val, "=")
				if val != "" {
					s.Filter = val
				} else if i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-") {
					s.Filter = rawArgs[i+1]
					i++
				}
				continue
			}

			// Comment: -c <comment> or -c=<comment>
			if strings.HasPrefix(lower, "-c") && !strings.HasPrefix(lower, "-cm") {
				val := strings.TrimPrefix(arg, "-c")
				val = strings.TrimPrefix(val, "=")
				if val != "" {
					s.Comment = val
				} else if i+1 < len(rawArgs) && !strings.HasPrefix(rawArgs[i+1], "-") {
					s.Comment = rawArgs[i+1]
					i++
				}
				continue
			}

			// Pass through unknown flags or store
			continue
		}

		// Positional argument
		positionals = append(positionals, arg)
	}

	// Route positionals based on command
	switch cmdName {
	case "create", "pack", "a", "c":
		if s.OutputDir != "" {
			s.Archive = s.OutputDir
			s.Items = positionals
		} else if len(positionals) >= 1 {
			s.Archive = positionals[0]
			s.Items = positionals[1:]
		}
	case "extract", "x", "e":
		if len(positionals) >= 1 {
			s.Archive = positionals[0]
			// Remaining positional arguments are file inclusion patterns
			if len(positionals) > 1 {
				s.IncludePatterns = append(s.IncludePatterns, positionals[1:]...)
			}
		}
		if s.OutputDir == "" {
			s.OutputDir = "."
		}
	case "list", "l", "ls":
		if len(positionals) >= 1 {
			s.Archive = positionals[0]
			if len(positionals) > 1 {
				s.IncludePatterns = append(s.IncludePatterns, positionals[1:]...)
			}
		}
	case "test", "t":
		if len(positionals) >= 1 {
			s.Archive = positionals[0]
			if len(positionals) > 1 {
				s.IncludePatterns = append(s.IncludePatterns, positionals[1:]...)
			}
		}
	case "delete", "del", "d":
		if len(positionals) >= 1 {
			s.Archive = positionals[0]
			if len(positionals) > 1 {
				s.Items = positionals[1:]
			}
		}
	case "update", "u":
		if len(positionals) >= 1 {
			s.Archive = positionals[0]
			if len(positionals) > 1 {
				s.Items = positionals[1:]
			}
		}
	default:
		if len(positionals) >= 1 {
			s.Archive = positionals[0]
			s.Items = positionals[1:]
		}
	}

	return s, nil
}

// MatchPattern tests whether the entry relative path matches a pattern.
// If recurse is true and pattern contains no path separators, it tests against the base filename
// as well as any subpath.
func MatchPattern(pattern, entryPath string, recurse bool) bool {
	normPat := strings.ReplaceAll(pattern, "\\", "/")
	normPath := strings.TrimPrefix(strings.ReplaceAll(entryPath, "\\", "/"), "/")

	// Exact match
	if normPat == normPath {
		return true
	}

	// Match entire directory or subpath
	if strings.HasSuffix(normPat, "/") {
		if strings.HasPrefix(normPath, normPat) {
			return true
		}
	}

	// Wildcard pattern matching
	// If pattern has no slash: e.g. "*.go" or "README.md"
	if !strings.Contains(normPat, "/") {
		if !recurse {
			// Must be in root directory
			if strings.Contains(normPath, "/") {
				return false
			}
			ok, _ := path.Match(normPat, normPath)
			return ok
		}
		// Recurse: match against base name
		baseName := path.Base(normPath)
		if ok, _ := path.Match(normPat, baseName); ok {
			return true
		}
		return false
	}

	// Pattern has slashes: e.g. "docs/*" or "src/**/*.go"
	// Handle ** (arbitrary depth)
	if strings.Contains(normPat, "**") {
		parts := strings.SplitN(normPat, "**", 2)
		prefix := parts[0]
		suffix := strings.TrimPrefix(parts[1], "/")
		if !strings.HasPrefix(normPath, prefix) {
			return false
		}
		rest := strings.TrimPrefix(normPath, prefix)
		if suffix == "" {
			return true
		}
		if !strings.Contains(suffix, "/") {
			if ok, _ := path.Match(suffix, path.Base(rest)); ok {
				return true
			}
		}
		if ok, _ := path.Match(suffix, rest); ok {
			return true
		}
		for i := 0; i < len(rest); i++ {
			if rest[i] == '/' {
				if ok, _ := path.Match(suffix, rest[i+1:]); ok {
					return true
				}
			}
		}
		return false
	}

	// Direct glob match
	if ok, _ := path.Match(normPat, normPath); ok {
		return true
	}

	// If pattern is a directory prefix: "docs/*" matching "docs/a/b.txt"
	if before, ok := strings.CutSuffix(normPat, "/*"); ok {
		dirPrefix := before + "/"
		if strings.HasPrefix(normPath, dirPrefix) {
			return true
		}
	}

	return false
}

// MatchFilters determines if a path should be included based on include and exclude patterns.
func MatchFilters(entryPath string, isDir bool, includes, excludes []string, recurse bool) bool {
	normPath := strings.TrimPrefix(strings.ReplaceAll(entryPath, "\\", "/"), "/")

	// Excludes take precedence
	for _, excl := range excludes {
		if MatchPattern(excl, normPath, recurse) {
			return false
		}
	}

	// If no includes are specified, everything is included
	if len(includes) == 0 {
		return true
	}

	// Check if matches any include pattern
	for _, incl := range includes {
		if MatchPattern(incl, normPath, recurse) {
			return true
		}
		// Also allow directory leading up to included file
		if isDir {
			normIncl := strings.ReplaceAll(incl, "\\", "/")
			if strings.HasPrefix(normIncl, normPath+"/") {
				return true
			}
		}
	}

	return false
}

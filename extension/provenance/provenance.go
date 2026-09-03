// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package provenance implements build passport and VCS supply-chain metadata
// embedded as stealth archive extensions.
package provenance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/lemon4ksan/seal/extension"
)

const (
	// ExtensionID is the unique namespace for embedded build provenance.
	ExtensionID extension.ID = "seal.provenance"
)

// Info encapsulates build passport metadata.
type Info struct {
	Commit    string    `json:"commit"`
	Branch    string    `json:"branch,omitempty"`
	Tag       string    `json:"tag,omitempty"`
	RepoURL   string    `json:"repo_url,omitempty"`
	BuildTime time.Time `json:"build_time"`
	GoVersion string    `json:"go_version"`
	OS        string    `json:"os"`
	Arch      string    `json:"arch"`
	Builder   string    `json:"builder,omitempty"`
}

// Provenance implements [extension.Extension] to persist build origin metadata in archives.
type Provenance struct {
	info Info
}

// New constructs a Provenance extension with explicit metadata.
func New(info Info) *Provenance {
	if info.BuildTime.IsZero() {
		info.BuildTime = time.Now().UTC()
	}
	if info.GoVersion == "" {
		info.GoVersion = runtime.Version()
	}
	if info.OS == "" {
		info.OS = runtime.GOOS
	}
	if info.Arch == "" {
		info.Arch = runtime.GOARCH
	}
	return &Provenance{info: info}
}

// Detect gathers build provenance from CI environment variables or local Git metadata.
func Detect(ctx context.Context) *Provenance {
	info := Info{
		BuildTime: time.Now().UTC(),
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}

	// 1. Check CI environment variables
	if sha := os.Getenv("GITHUB_SHA"); sha != "" {
		info.Commit = sha
		info.Branch = os.Getenv("GITHUB_REF_NAME")
		info.RepoURL = os.Getenv("GITHUB_REPOSITORY")
		info.Builder = "GitHub Actions"
	} else if sha := os.Getenv("GIT_COMMIT"); sha != "" {
		info.Commit = sha
		info.Branch = os.Getenv("GIT_BRANCH")
		info.Builder = "Jenkins/CI"
	} else if sha := os.Getenv("CI_COMMIT_SHA"); sha != "" {
		info.Commit = sha
		info.Branch = os.Getenv("CI_COMMIT_REF_NAME")
		info.Builder = "GitLab CI"
	}

	// 2. Local git inspection fallback
	if info.Commit == "" {
		if gitBin, err := exec.LookPath("git"); err == nil {
			cmd := exec.CommandContext(ctx, gitBin, "rev-parse", "HEAD")
			if out, err := cmd.Output(); err == nil {
				info.Commit = strings.TrimSpace(string(out))
			}
			cmdBranch := exec.CommandContext(ctx, gitBin, "rev-parse", "--abbrev-ref", "HEAD")
			if out, err := cmdBranch.Output(); err == nil {
				info.Branch = strings.TrimSpace(string(out))
			}
		}
	}

	return New(info)
}

// ID returns the extension identifier.
func (p *Provenance) ID() extension.ID {
	return ExtensionID
}

// OnWrite serializes the provenance info to JSON bytes.
func (p *Provenance) OnWrite(_ context.Context, _ extension.ArchiveInfo) ([]byte, error) {
	return json.Marshal(p.info)
}

// OnRead deserializes provenance info from payload bytes.
func (p *Provenance) OnRead(_ context.Context, data []byte, _ extension.ArchiveInfo) error {
	return json.Unmarshal(data, &p.info)
}

// Info returns the provenance metadata.
func (p *Provenance) Info() *Info {
	return &p.info
}

// DecodeInfo parses a serialized provenance Info from JSON bytes.
func DecodeInfo(data []byte) (*Info, error) {
	var info Info
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("provenance: decode: %w", err)
	}
	return &info, nil
}

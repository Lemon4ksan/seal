// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lemon4ksan/ctrl/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/archive"
	"github.com/lemon4ksan/seal/cmd/seal/internal/bench"
	"github.com/lemon4ksan/seal/cmd/seal/internal/cloud"
	"github.com/lemon4ksan/seal/cmd/seal/internal/forensics"
	"github.com/lemon4ksan/seal/cmd/seal/internal/prolly"
	"github.com/lemon4ksan/seal/cmd/seal/internal/security"
	_ "github.com/lemon4ksan/seal/codec/legacy"
)

func main() {
	app := tuikit.NewApp("seal", seal.Version, "Universal High-Performance Archiving & Storage Engine",
		&archive.CreateCmd{},
		&archive.ExtractCmd{},
		&archive.ListCmd{},
		&archive.DeleteCmd{},
		&archive.CatCmd{},
		&archive.UpdateCmd{},
		&archive.VacuumCmd{},
		&forensics.TestCmd{},
		&forensics.TreeCmd{},
		&forensics.InspectCmd{},
		&forensics.DoctorCmd{},
		&forensics.MountCmd{},
		&prolly.SnapshotCmd{},
		&prolly.LogCmd{},
		&prolly.DiffCmd{},
		&cloud.ServeCmd{},
		&cloud.PushCmd{},
		&cloud.PullCmd{},
		&security.KeygenCmd{},
		&security.VaultCmd{},
		&security.KeyCmd{},
		&bench.BenchCmd{},
	)

	app.CommandGroups = []tuikit.CommandGroup{
		{
			Title:    "Archive Operations (7-Zip & Modern)",
			Commands: []string{"create", "extract", "list", "delete", "cat", "update", "vacuum"},
		},
		{
			Title:    "Virtual Filesystem & Cloud",
			Commands: []string{"mount", "serve", "push", "pull"},
		},
		{
			Title:    "Git-like Versioning & Snapshots",
			Commands: []string{"snapshot", "log", "diff"},
		},
		{
			Title:    "Enterprise Security & Vaults",
			Commands: []string{"keygen", "vault", "key"},
		},
		{
			Title:    "Integrity, Forensics & Repair",
			Commands: []string{"test", "tree", "inspect", "doctor"},
		},
		{
			Title:    "System & Performance",
			Commands: []string{"bench"},
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		stop()
		fmt.Fprintf(os.Stderr, "\n%s %v\n", tuikit.BadgeFail(), err)
		os.Exit(1)
	}
	stop()
}

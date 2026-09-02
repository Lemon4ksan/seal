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

	"github.com/lemon4ksan/foundation/tuikit"

	"github.com/lemon4ksan/seal"
	"github.com/lemon4ksan/seal/cmd/seal/internal/archive"
	"github.com/lemon4ksan/seal/cmd/seal/internal/bench"
	"github.com/lemon4ksan/seal/cmd/seal/internal/forensics"
	_ "github.com/lemon4ksan/seal/codec/legacy"
)

func main() {
	app := tuikit.NewApp("seal", seal.Version, "🦭 Seal — High-Performance Universal Archiving Engine",
		&archive.CreateCmd{},
		&archive.ExtractCmd{},
		&archive.ListCmd{},
		&archive.CatCmd{},
		&forensics.TestCmd{},
		&forensics.TreeCmd{},
		&forensics.InspectCmd{},
		&forensics.DoctorCmd{},
		&bench.BenchCmd{},
	)

	app.CommandGroups = []tuikit.CommandGroup{
		{
			Title:    "Archive Operations",
			Commands: []string{"create", "extract", "list", "cat"},
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

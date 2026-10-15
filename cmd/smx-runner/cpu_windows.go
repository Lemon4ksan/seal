//go:build windows

// Copyright (c) 2026 Lemon4ksan All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"syscall"
	"time"
)

func getCPUSnapshot() ProcessCPUSnapshot {
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return ProcessCPUSnapshot{WallTime: time.Now()}
	}
	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return ProcessCPUSnapshot{WallTime: time.Now()}
	}
	return ProcessCPUSnapshot{
		WallTime:   time.Now(),
		KernelTime: time.Duration(kernel.Nanoseconds()),
		UserTime:   time.Duration(user.Nanoseconds()),
	}
}

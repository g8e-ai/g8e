//go:build !linux

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

func discoverLocalOperators() ([]localOperatorProcess, error) {
	return nil, fmt.Errorf("operator stop: local worker discovery is currently supported on Linux only")
}

// waitPIDTermination waits for the process with the given PID to exit on non-Linux platforms.
func waitPIDTermination(pid int, timeout time.Duration) (bool, error) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return true, nil
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timer.C:
			return false, nil
		case <-ticker.C:
			if err := p.Signal(syscall.Signal(0)); err != nil {
				return true, nil
			}
		}
	}
}

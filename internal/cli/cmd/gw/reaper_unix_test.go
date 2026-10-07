// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows

package gw

import (
	"context"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func startZombieReaper(t *testing.T) {
	t.Helper()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				var ws syscall.WaitStatus
				pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
				if err != nil || pid == 0 {
					time.Sleep(5 * time.Millisecond)
				}
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		<-done
	})
}

func reapRemainingChild(t *testing.T, fileSvc fs.RuntimeFileService) {
	t.Helper()
	pidRel := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	data, err := fileSvc.ReadFile(context.Background(), pidRel)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil || pid == 0 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	var ws syscall.WaitStatus
	_, _ = syscall.Wait4(pid, &ws, 0, nil)
	_ = fileSvc.Remove(context.Background(), pidRel)
}

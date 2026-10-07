// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows

package gw

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

func startZombieReaper(t *testing.T) {
	t.Helper()
	// No-op on Windows: Windows does not have zombie processes or SIGCHLD.
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
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
	_ = fileSvc.Remove(context.Background(), pidRel)
}

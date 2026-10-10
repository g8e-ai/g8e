// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows

package ssh

import (
	"context"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ProxyCommand constructs an exec.Cmd for running a ProxyCommand on Windows.
func ProxyCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, filepath.Base(constants.PathBinSh), "-c", command) //nolint:gosec // Windows shell proxy command requires sh invocation
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd
}

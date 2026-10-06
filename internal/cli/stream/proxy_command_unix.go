// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows

package stream

import (
	"context"
	"os/exec"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func proxyCommand(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, constants.PathBinSh, "-c", command)
}

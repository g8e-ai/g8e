// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package ensemble manages the local g8ee service.
package ensemble

import (
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ensemble",
		Short: "Manage the local g8ee ensemble service",
		Long:  "Manage the local Python ensemble service started by make full. Uses the active virtual environment, repository .venv, or python3. Run from the repository root.",
	}
	cmd.AddCommand(
		hostLifecycleCmd("start", "Start g8ee with local Python"),
		hostLifecycleCmd("stop", "Stop the local g8ee process"),
		hostLifecycleCmd("restart", "Restart g8ee with local Python"),
		hostResetIdentityCmd(),
		hostReadCmd("status", "Show local g8ee status"),
		hostReadCmd("logs", "Show local g8ee logs"),
	)
	return cmd
}

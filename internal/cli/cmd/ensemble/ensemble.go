// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package ensemble manages the g8ee service in the local unified Compose stack.
package ensemble

import (
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/docker"
	"github.com/spf13/cobra"
)

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ensemble",
		Short: "Manage the local g8ee ensemble service",
		Long:  "Manage only the ensemble service in the repository's unified Docker Compose stack. Run from the repository root.",
	}
	cmd.AddCommand(
		composeCmd("start", "Start g8ee", []string{"up", "-d", "ensemble"}),
		composeCmd("stop", "Stop g8ee", []string{"stop", "ensemble"}),
		composeCmd("restart", "Restart g8ee", []string{"restart", "ensemble"}),
		composeCmd("status", "Show g8ee container status", []string{"ps", "ensemble"}),
		composeCmd("logs", "Show g8ee logs", []string{"logs", "--tail=100", "ensemble"}),
	)
	return cmd
}

func composeCmd(name, short string, args []string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return docker.RunDockerComposeContext(cmd.Context(), args)
		},
	}
}

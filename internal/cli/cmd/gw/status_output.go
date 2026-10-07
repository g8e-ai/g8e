// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	"github.com/spf13/cobra"
)

func printGatewayStatus(cmd *cobra.Command, cfg *config.Config, clientFactory authcmd.APIClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) error {
	fileSvc, err := fileSvcFactory("", slog.Default())
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	client, clientErr := clientFactory(fileSvc, cfg)
	healthy, pid := false, 0
	if clientErr == nil {
		body, err := client.Get(constants.APIPaths.Health)
		var health models.HealthResponse
		healthy = err == nil && json.Unmarshal(body, &health) == nil && health.Status == constants.GatewayModeStatusOK
		if healthy {
			pid = health.PID
		}
	} else {
		client = nil
	}
	running := healthy
	if !healthy {
		pm, err := platform.NewProcessManager(fileSvc)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrInternal, err)
		}
		running, pid, err = pm.OperatorStatus()
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrPIDReadFailed, err)
		}
	}
	if !running {
		cmd.Println("Gateway  stopped")
		return nil
	}
	state := "running"
	if healthy {
		state = "online"
	}
	if pid > 0 {
		cmd.Printf("Gateway  %s (PID %d)\n", state, pid)
	} else {
		cmd.Printf("Gateway  %s\n", state)
	}
	fmt.Fprintln(cmd.OutOrStdout())
	printOperatorTables(cmd.OutOrStdout(), collectOperatorInventory(client, fileSvc, cfg))
	fmt.Fprintln(cmd.OutOrStdout())
	printEnrollmentSummary(cmd.OutOrStdout(), client)
	origin := network.LocalhostHTTPSURL(constants.Ports.OperatorHttps)
	if profile, err := serve.ReadLaunchProfile(fileSvc); err == nil && profile.Config.PublicBaseURL != "" {
		origin = strings.TrimRight(profile.Config.PublicBaseURL, "/")
	}
	fmt.Fprintln(cmd.OutOrStdout())
	cmd.Printf("Console  %s/console/\n", origin)
	return nil
}

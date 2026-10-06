// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"encoding/json"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/spf13/cobra"
)

func printBriefStatus(cmd *cobra.Command, cfg *config.Config, clientFactory authcmd.APIClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) error {
	fileSvc, err := fileSvcFactory("", slog.Default())
	if err != nil {
		return err
	}
	client, clientErr := clientFactory(fileSvc, cfg)
	healthy := false
	if clientErr == nil {
		body, err := client.Get("/api/v1/health")
		var health models.HealthResponse
		healthy = err == nil && json.Unmarshal(body, &health) == nil && health.Status == constants.GatewayModeStatusOK
	}
	if healthy {
		cmd.Println("  Gateway    online")
	} else {
		pm, err := platform.NewProcessManager(fileSvc)
		if err != nil {
			return err
		}
		running, _, err := pm.OperatorStatus()
		if err != nil {
			return err
		}
		if running {
			cmd.Println("  Gateway    running; health unavailable")
		} else {
			cmd.Println("  Gateway    stopped")
		}
	}
	if clientErr != nil {
		cmd.Println("  Operators  unavailable; enroll your CLI identity to view connections")
		//nolint:nilerr // intentional fallback: brief status displays unenrollment message without erroring
		return nil
	}
	path := constants.APIPaths.Operators
	if creds, _ := auth.LoadCredentials(fileSvc, cfg); creds != nil && creds.UserID != "" {
		path += "?user_id=" + creds.UserID
	}
	body, err := client.Get(path)
	var response models.OperatorSlotResponse
	if err != nil || json.Unmarshal(body, &response) != nil || !response.Success {
		cmd.Println("  Operators  unavailable; check enrollment or Gateway logs")
		//nolint:nilerr // intentional fallback: brief status prints unavailable message without erroring
		return nil
	}
	connected := make([]models.OperatorDocumentGo, 0)
	for _, op := range response.Operators {
		if isOperatorConnected(op) {
			connected = append(connected, op)
		}
	}
	cmd.Printf("  Operators  %d connected\n", len(connected))
	for _, op := range connected {
		cmd.Printf("    %s on %s (%s)\n", operatorRoleDisplay(op), operatorHostnameDisplay(op), op.Status)
	}
	return nil
}

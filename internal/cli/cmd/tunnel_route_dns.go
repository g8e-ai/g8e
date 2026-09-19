// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/infra/cloudflaredns"
)

func tunnelRouteDNSCmd() *cobra.Command {
	var tunnelName string
	var apiToken string

	cmd := &cobra.Command{
		Use:   "route-dns",
		Short: "Route a hostname to a Cloudflare tunnel in the correct DNS zone",
		Long: `Create or update the proxied CNAME that points a hostname at a Cloudflare tunnel.

Use this when 'cloudflared tunnel route dns' provisions the wrong zone (for example
opendevops.ai.g8e.ai instead of opendevops.ai) because cloudflared login only
authorized a different domain.

Requires a Cloudflare API token with DNS edit permission for the target zone.
Set CLOUDFLARE_API_TOKEN or pass --api-token.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkCloudflared(); err != nil {
				return err
			}
			if tunnelName == "" {
				return fmt.Errorf("%w: --name is required", constants.ErrMissingRequiredField)
			}

			hostname := strings.TrimSpace(args[0])
			token := strings.TrimSpace(apiToken)
			if token == "" {
				token = strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN"))
			}
			if token == "" {
				token = strings.TrimSpace(os.Getenv("CF_API_TOKEN"))
			}
			if token == "" {
				return fmt.Errorf("%w: set CLOUDFLARE_API_TOKEN or pass --api-token", constants.ErrMissingRequiredField)
			}

			tunnelID, err := getTunnelID(tunnelName)
			if err != nil {
				return err
			}

			client, err := cloudflaredns.NewClient(token)
			if err != nil {
				return err
			}

			cmd.Printf("[g8e] Routing %s -> tunnel %s (%s)\n", hostname, tunnelName, tunnelID)
			if err := client.UpsertTunnelCNAME(context.Background(), hostname, tunnelID); err != nil {
				return err
			}

			cmd.Println("[g8e] DNS record updated.")
			if err := waitForHostnameResolution(hostname, 30*time.Second); err != nil {
				cmd.Printf("[g8e] Warning: %v\n", err)
			} else {
				cmd.Printf("[g8e] %s resolves.\n", hostname)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&tunnelName, "name", "", "Tunnel name (required)")
	cmd.Flags().StringVar(&apiToken, "api-token", "", "Cloudflare API token (default: CLOUDFLARE_API_TOKEN or CF_API_TOKEN)")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func waitForHostnameResolution(hostname string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := net.LookupHost(hostname); err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("%s did not resolve within %s", hostname, timeout)
}

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

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/serve"
)

var osExit = os.Exit

type versionInfoKey struct{}

func NewRootCmd(version string, vi serve.VersionInfo) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "g8e",
		Version: version,
		Short:   "g8e Platform Manager - CLI for the g8e Gateway, g8e Operator, and platform setup",
		Long: `g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg), g8e Operator (g8eo), and platform setup.

Running './g8e' with no arguments launches the Tactical Governance Console (TUI).`,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd, args, defaultTUIDeps())
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			endpoint, err := cmd.Flags().GetString("endpoint")
			if err != nil {
				return fmt.Errorf("root: get endpoint flag: %w", err)
			}
			port, err := cmd.Flags().GetInt("port")
			if err != nil {
				return fmt.Errorf("root: get port flag: %w", err)
			}
			if endpoint != "" {
				if port > 0 {
					if strings.Contains(endpoint, "://") {
						config.SetEndpointOverride(endpoint)
					} else {
						host := endpoint
						if h, _, err := net.SplitHostPort(host); err == nil {
							host = h
						}
						config.SetHTTPEndpointOverride(endpoint)
						config.SetHTTPSEndpointOverride(fmt.Sprintf("%s:%d", host, port))
					}
				} else {
					config.SetEndpointOverride(endpoint)
				}
			} else if port > 0 {
				config.SetHTTPEndpointOverride("localhost")
				config.SetHTTPSEndpointOverride(fmt.Sprintf("localhost:%d", port))
			}
			return nil
		},
	}

	ctx := rootCmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	rootCmd.SetContext(context.WithValue(ctx, versionInfoKey{}, vi))

	rootCmd.PersistentFlags().StringP("endpoint", "e", "", "Gateway HTTP discovery endpoint (host or host:port) for remote enrollment")
	rootCmd.PersistentFlags().IntP("port", "p", 0, "Gateway HTTPS/mTLS port (overrides default 8443; use with --endpoint)")

	rootCmd.AddCommand(
		gatewayCmd(),
		authCmd(),
		mcpCmd(),
		operatorCmd(),
		vaultCmd(),
		testCmd(),
		demosCmd(),
		dockerCmd(),
		auditCmd(),
		reportCmd(),
		swaggerCmd(),
		tuiCmd(),
		versionCmd(),
		complianceCmd(),
	)

	return rootCmd
}

func versionInfoFromCmd(cmd *cobra.Command) serve.VersionInfo {
	if vi, ok := cmd.Context().Value(versionInfoKey{}).(serve.VersionInfo); ok {
		return vi
	}
	return serve.VersionInfo{}
}

func ExecuteWithVersionInfo(version, buildID, buildTime, platform string) {
	vi := serve.VersionInfo{
		Version:   version,
		BuildID:   buildID,
		BuildTime: buildTime,
		Platform:  platform,
	}
	rootCmd := NewRootCmd(version, vi)
	rootCmd.SetVersionTemplate(`{{with .Version}}{{printf "g8e version %s\n" .}}{{end}}`)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}
}

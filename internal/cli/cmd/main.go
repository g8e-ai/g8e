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

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/audit"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	compliancecmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/compliance"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/demos"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/docker"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/eval"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/gw"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/mcp"
	operatorcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/operator"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/public"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/report"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/swagger"
	testcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/test"
	tuicmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/tui"
	vaultcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/vault"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/version"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
)

var osExit = os.Exit

func NewRootCmd(cliVersion string, vi serve.VersionInfo) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "g8e",
		Version: cliVersion,
		Short:   "g8e Platform Manager - CLI for the g8e Gateway, g8e Operator, and platform setup",
		Long: `g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg), g8e Operator (g8eo), and platform setup.

Run 'g8e tui' to launch the Tactical Governance Console (TUI).`,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
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
	rootCmd.SetContext(shared.ContextWithVersionInfo(ctx, vi))

	rootCmd.PersistentFlags().StringP("endpoint", "e", "", "Gateway HTTP discovery endpoint (host or host:port) for remote enrollment")
	rootCmd.PersistentFlags().IntP("port", "p", 0, "Gateway HTTPS/mTLS port (overrides default 8443; use with --endpoint)")
	rootCmd.PersistentFlags().Bool("json", false, "Emit machine-readable JSON output (pretty-printed)")

	rootCmd.AddCommand(
		gw.Cmd(),
		authcmd.Cmd(),
		mcp.Cmd(),
		operatorcmd.Cmd(),
		vaultcmd.Cmd(),
		testcmd.Cmd(),
		demos.Cmd(),
		docker.Cmd(),
		audit.Cmd(),
		report.Cmd(),
		public.Cmd(),
		swagger.Cmd(),
		tuicmd.Cmd(),
		version.Cmd(),
		compliancecmd.Cmd(),
		eval.Cmd(),
	)

	return rootCmd
}

func ExecuteWithVersionInfo(vi serve.VersionInfo) {
	rootCmd := NewRootCmd(vi.Version, vi)
	rootCmd.SetVersionTemplate(`{{with .Version}}{{printf "g8e version %s\n" .}}{{end}}`)
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}
}

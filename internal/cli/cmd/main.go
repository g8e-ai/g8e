// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"context"
	"fmt"
	"os"

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
The CLI manages the g8e Gateway (g8eg), g8e Operator (g8eo), and platform setup.`,
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
					config.SetEndpointOverrideWithPort(endpoint, port)
				} else {
					config.SetEndpointOverride(endpoint)
				}
			} else if port > 0 {
				config.SetEndpointOverrideWithPort("localhost", port)
			}
			return nil
		},
	}

	ctx := rootCmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	rootCmd.SetContext(context.WithValue(ctx, versionInfoKey{}, vi))

	rootCmd.PersistentFlags().StringP("endpoint", "e", "", "Gateway endpoint (host or host:port) for remote enrollment")
	rootCmd.PersistentFlags().IntP("port", "p", 0, "Gateway HTTPS port (overrides default 8443; use with --endpoint)")

	rootCmd.AddCommand(
		gatewayCmd(),
		authCmd(),
		mcpCmd(),
		operatorCmd(),
		vaultCmd(),
		testCmd(),
		demosCmd(),
		auditCmd(),
		reportCmd(),
		swaggerCmd(),
		agentHarnessCmd(),
		tuiCmd(),
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

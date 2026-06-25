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
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/serve"
)

// versionInfo holds build-time version metadata for serve commands
var versionInfo serve.VersionInfo

func NewRootCmd(version string) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "g8e",
		Version: version,
		Short:   "g8e Platform Manager - CLI for the g8e Gateway, g8e Operator, and platform setup",
		Long: `g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg), g8e Operator (g8eo), and platform setup.`,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

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
	)

	return rootCmd
}

func ExecuteWithVersionInfo(version, buildID, buildTime, platform string) {
	rootCmd := NewRootCmd(version)
	rootCmd.SetVersionTemplate(`{{with .Version}}{{printf "g8e version %s\n" .}}{{end}}`)

	// Store version info globally for serve commands
	versionInfo = serve.VersionInfo{
		Version:   version,
		BuildID:   buildID,
		BuildTime: buildTime,
		Platform:  platform,
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

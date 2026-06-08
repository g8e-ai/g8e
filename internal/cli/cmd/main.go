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
)

func Execute() {
	rootCmd := &cobra.Command{
		Use:   "g8e",
		Short: "g8e Platform Manager - CLI for the g8e Gateway and g8e Operator",
		Long: `g8e is a zero-trust execution platform for agentic infrastructure.
The CLI manages the g8e Gateway (g8eg) and g8e Operator (g8eo).`,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	rootCmd.AddCommand(
		gatewayCmd(),
		mcpCmd(),
		operatorCmd(),
		agentCmd(),
		claudeCmd(),
		vaultCmd(),
		testCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func claudeCmd() *cobra.Command {
	return agentClaudeCmd()
}

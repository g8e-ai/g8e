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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/spf13/cobra"
)

func operatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operator",
		Short: "Manage Operator instances",
		Long:  `Manage and view g8e Operator instances connected to the Gateway.`,
	}

	cmd.AddCommand(
		operatorListCmd(),
		operatorCpCmd(),
	)

	return cmd
}

func operatorListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Operator instances",
		Long:  `List all Operator instances currently connected to the Gateway.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			resp, err := client.Get("/api/operators")
			if err != nil {
				return err
			}

			var operators []Operator
			if err := json.Unmarshal(resp, &operators); err != nil {
				return fmt.Errorf("failed to parse response: %w", err)
			}

			if len(operators) == 0 {
				cmd.Println("No operators found")
				return nil
			}

			cmd.Printf("Operators (%d total)\n", len(operators))
			cmd.Println(strings.Repeat("=", 90))
			cmd.Printf("  %-36s  %-20s  %-15s\n", "ID", "Type", "Status")
			cmd.Println(strings.Repeat("-", 90))
			for _, op := range operators {
				cmd.Printf("  %-36s  %-20s  %-15s\n", op.ID, op.CloudSubtype, op.Status)
			}

			return nil
		},
	}
	return cmd
}

func operatorCpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cp <target>",
		Short: "Copy the operator binary to a target location",
		Long:  `Copy the g8e operator binary to a specified directory or file. If a directory is provided, the binary will be copied with its default name. If a filename is provided, the binary will be copied with that name.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]

			cfg, err := config.Load("")
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			sourceBinary := filepath.Join(cfg.ProjectRoot, "bin", fmt.Sprintf("g8e-%s-%s", runtime.GOOS, runtime.GOARCH))
			if runtime.GOOS == "windows" {
				sourceBinary += ".exe"
			}

			if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
				return fmt.Errorf("operator binary not found at %s. Run 'make build' first", sourceBinary)
			}

			targetInfo, err := os.Stat(target)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to stat target: %w", err)
			}

			var destPath string
			if err == nil && targetInfo.IsDir() {
				basename := filepath.Base(sourceBinary)
				destPath = filepath.Join(target, basename)
			} else {
				destPath = target
			}

			if err := copyFile(sourceBinary, destPath); err != nil {
				return fmt.Errorf("failed to copy binary: %w", err)
			}

			cmd.Printf("Copied operator binary to %s\n", destPath)
			return nil
		},
	}
	return cmd
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}

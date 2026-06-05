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
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
		operatorScpCmd(),
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

			sourceBinary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to get running binary path: %w", err)
			}

			if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
				return fmt.Errorf("operator binary not found at %s", sourceBinary)
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

func operatorScpCmd() *cobra.Command {
	var port int
	var identityFile string
	var recursive bool
	var preserve bool
	var verbose bool
	var compression bool
	var prompt bool

	cmd := &cobra.Command{
		Use:   "scp <user@host:path>",
		Short: "Copy the operator binary to a remote host using scp",
		Long:  `Copy the g8e operator binary to a remote host using scp. Supports common scp flags. If the target path is a directory, the binary will be copied with its default name. Use --prompt to interactively configure scp options.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]

			sourceBinary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to get running binary path: %w", err)
			}

			if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
				return fmt.Errorf("operator binary not found at %s", sourceBinary)
			}

			if prompt {
				if err := promptForScpOptions(cmd, &port, &identityFile, &recursive, &preserve, &verbose, &compression); err != nil {
					return err
				}
			}

			scpArgs := buildScpArgs(port, identityFile, recursive, preserve, verbose, compression, sourceBinary, target)

			cmd.Printf("Copying operator binary to %s\n", target)
			if verbose {
				cmd.Printf("Command: scp %s\n", strings.Join(scpArgs, " "))
			}

			scpCmd := exec.Command("scp", scpArgs...)
			scpCmd.Stdout = cmd.OutOrStdout()
			scpCmd.Stderr = cmd.ErrOrStderr()
			scpCmd.Stdin = cmd.InOrStdin()

			if err := scpCmd.Run(); err != nil {
				return fmt.Errorf("scp failed: %w", err)
			}

			cmd.Printf("Successfully copied operator binary to %s\n", target)
			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "P", 0, "Port to connect to on the remote host")
	cmd.Flags().StringVarP(&identityFile, "identity", "i", "", "Selects the file from which the identity (private key) for public key authentication is read")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Recursive copy (not applicable for single file, but included for compatibility)")
	cmd.Flags().BoolVarP(&preserve, "preserve", "p", false, "Preserves modification times, access times, and modes from the source file")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose mode")
	cmd.Flags().BoolVarP(&compression, "compression", "C", false, "Enable compression")
	cmd.Flags().BoolVar(&prompt, "prompt", false, "Prompt for scp options interactively")

	return cmd
}

func promptForScpOptions(cmd *cobra.Command, port *int, identityFile *string, recursive, preserve, verbose, compression *bool) error {
	reader := bufio.NewReader(cmd.InOrStdin())

	cmd.Println("\nSCP Configuration (press Enter to use default/skip):")

	if *port == 0 {
		cmd.Print("SSH Port [default: 22]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			var p int
			if _, err := fmt.Sscanf(input, "%d", &p); err == nil {
				*port = p
			}
		}
	}

	if *identityFile == "" {
		cmd.Print("Identity file path (SSH private key) [default: none]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			*identityFile = input
		}
	}

	cmd.Print("Preserve file attributes (times, modes) [y/N]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "y" || input == "Y" {
		*preserve = true
	}

	cmd.Print("Enable compression [y/N]: ")
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "y" || input == "Y" {
		*compression = true
	}

	cmd.Print("Verbose output [y/N]: ")
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "y" || input == "Y" {
		*verbose = true
	}

	cmd.Println()
	return nil
}

func buildScpArgs(port int, identityFile string, recursive, preserve, verbose, compression bool, source, target string) []string {
	args := []string{}

	if port != 0 {
		args = append(args, "-P", fmt.Sprintf("%d", port))
	}

	if identityFile != "" {
		args = append(args, "-i", identityFile)
	}

	if recursive {
		args = append(args, "-r")
	}

	if preserve {
		args = append(args, "-p")
	}

	if verbose {
		args = append(args, "-v")
	}

	if compression {
		args = append(args, "-C")
	}

	args = append(args, source, target)
	return args
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

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
	"time"

	"github.com/g8e-ai/g8e/internal/cli/api"
	"github.com/g8e-ai/g8e/internal/cli/auth"
	"github.com/g8e-ai/g8e/internal/cli/serve"
	"github.com/g8e-ai/g8e/internal/cli/stream"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
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
		operatorRunCmd(),
		operatorCpCmd(),
		operatorScpCmd(),
		operatorDeployCmd(),
		operatorStreamCmd(),
	)

	return cmd
}

func operatorListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Operator instances",
		Long:  `List all Operator instances currently connected to the Gateway.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return err
			}

			client, err := api.NewClient(cfg)
			if err != nil {
				return err
			}

			resp, err := client.Get("/api/operators")
			if err != nil {
				return err
			}

			var operators []models.OperatorDocumentGo
			if err := json.Unmarshal(resp, &operators); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
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

func operatorRunCmd() *cobra.Command {
	var key string
	var clientCert string
	var trustBundle string
	var workingDir string
	var cloud bool
	var provider string
	var executionVault bool
	var noGit bool
	var logLevel string
	var heartbeatInterval int

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the g8e Operator in foreground (worker mode)",
		Long:  `Run the g8e Operator in foreground as a worker. This connects to the Gateway and executes commands. This is the re-exec target for remote deployment and can also be run directly for debugging.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, _ := cmd.Flags().GetString("endpoint")
			opts := serve.ServeOperatorOptions{
				LogLevel:          logLevel,
				Endpoint:          endpoint,
				TrustBundlePath:   trustBundle,
				PrivateKey:        key,
				ClientCert:        clientCert,
				WorkingDir:        workingDir,
				LaunchDir:         workingDir,
				CloudMode:         cloud,
				CloudProvider:     provider,
				ExecutionVault:    executionVault,
				NoGit:             noGit,
				HeartbeatInterval: time.Duration(heartbeatInterval) * time.Second,
			}

			// Run operator (this blocks until shutdown)
			serve.RunOperator(opts, versionInfo)
			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Path to operator private key")
	cmd.Flags().StringVar(&clientCert, "cert", "", "Path to operator client certificate")
	cmd.Flags().StringVar(&trustBundle, "trust-bundle", "", "Path to CA trust bundle")
	cmd.Flags().StringVar(&workingDir, "working-dir", "", "Working directory for command execution")
	cmd.Flags().BoolVarP(&cloud, "cloud", "c", false, "Cloud operator mode")
	cmd.Flags().StringVarP(&provider, "provider", "p", "", "Cloud provider (aws, gcp, azure)")
	cmd.Flags().BoolVarP(&executionVault, "execution-vault", "s", true, "Enable execution vault (data stays in working directory)")
	cmd.Flags().BoolVarP(&noGit, "no-git", "G", false, "Disable Git integration")
	cmd.Flags().StringVarP(&logLevel, "log", "l", "info", "Log level: info, error, debug")
	cmd.Flags().IntVar(&heartbeatInterval, "heartbeat-interval", 30, "Heartbeat interval in seconds")

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
				return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
			}

			if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", constants.ErrPathNotFound, sourceBinary)
			}

			targetInfo, err := os.Stat(target)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
			}

			var destPath string
			if err == nil && targetInfo.IsDir() {
				basename := filepath.Base(sourceBinary)
				destPath = filepath.Join(target, basename)
			} else {
				destPath = target
			}

			if err := copyFile(sourceBinary, destPath); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrPathValidation, err)
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
				return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
			}

			if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", constants.ErrPathNotFound, sourceBinary)
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
				return fmt.Errorf("%w: %w", constants.ErrMCPRunShellCommandSSHDial, err)
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

func operatorDeployCmd() *cobra.Command {
	var hosts string
	var port int
	var identityFile string
	var background bool

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy the operator binary to remote hosts and start it",
		Long:  `Deploy the g8e operator binary to remote hosts via SSH and start it in the background. Uses your existing SSH config for authentication. Requires './g8e auth enroll' first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig("")
			if err != nil {
				return err
			}

			creds, err := auth.LoadCredentials(cfg)
			if err != nil || creds == nil {
				return fmt.Errorf("%w: Please run './g8e auth enroll' first", constants.ErrNotAuthenticated)
			}

			if hosts == "" {
				return fmt.Errorf("%w: --hosts flag is required (comma-separated list of hosts)", constants.ErrMissingRequiredField)
			}

			hostList := strings.Split(hosts, ",")
			for i := range hostList {
				hostList[i] = strings.TrimSpace(hostList[i])
			}

			sourceBinary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
			}

			if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", constants.ErrPathNotFound, sourceBinary)
			}

			cmd.Printf("Deploying operator to %d hosts: %s\n", len(hostList), strings.Join(hostList, ", "))

			httpPort := constants.Ports.OperatorHttp
			httpsPort := constants.Ports.OperatorHttps

			for _, host := range hostList {
				cmd.Printf("\nDeploying to %s...\n", host)

				remotePath := "~/g8e"
				scpTarget := fmt.Sprintf("%s:%s", host, remotePath)

				scpArgs := []string{}
				if port != 0 {
					scpArgs = append(scpArgs, "-P", fmt.Sprintf("%d", port))
				}
				if identityFile != "" {
					scpArgs = append(scpArgs, "-i", identityFile)
				}
				scpArgs = append(scpArgs, sourceBinary, scpTarget)

				scpCmd := exec.Command("scp", scpArgs...)
				scpCmd.Stdout = cmd.OutOrStdout()
				scpCmd.Stderr = cmd.ErrOrStderr()

				if err := scpCmd.Run(); err != nil {
					cmd.Printf("Failed to copy to %s: %v\n", host, err)
					continue
				}

				cmd.Printf("Copied binary to %s\n", host)

				sshArgs := []string{}
				if port != 0 {
					sshArgs = append(sshArgs, "-p", fmt.Sprintf("%d", port))
				}
				if identityFile != "" {
					sshArgs = append(sshArgs, "-i", identityFile)
				}
				sshArgs = append(sshArgs, host, "chmod +x ~/g8e")

				chmodCmd := exec.Command("ssh", sshArgs...)
				chmodCmd.Stdout = cmd.OutOrStdout()
				chmodCmd.Stderr = cmd.ErrOrStderr()

				if err := chmodCmd.Run(); err != nil {
					cmd.Printf("Failed to chmod on %s: %v\n", host, err)
					continue
				}

				if background {
					sshArgs = []string{}
					if port != 0 {
						sshArgs = append(sshArgs, "-p", fmt.Sprintf("%d", port))
					}
					if identityFile != "" {
						sshArgs = append(sshArgs, "-i", identityFile)
					}
					startCommand := fmt.Sprintf("nohup ~/g8e gw start --http-port %d --https-port %d > /dev/null 2>&1 &", httpPort, httpsPort)
					sshArgs = append(sshArgs, host, startCommand)

					startCmd := exec.Command("ssh", sshArgs...)
					startCmd.Stdout = cmd.OutOrStdout()
					startCmd.Stderr = cmd.ErrOrStderr()

					if err := startCmd.Run(); err != nil {
						cmd.Printf("Failed to start operator on %s: %v\n", host, err)
						continue
					}

					cmd.Printf("Started operator in background on %s\n", host)
				} else {
					cmd.Printf("Operator deployed to %s (use --background to auto-start)\n", host)
				}
			}

			cmd.Println("\nDeployment complete")
			return nil
		},
	}

	cmd.Flags().StringVar(&hosts, "hosts", "", "Comma-separated list of hosts to deploy to (required)")
	cmd.Flags().IntVarP(&port, "port", "P", 0, "SSH port to connect to on remote hosts")
	cmd.Flags().StringVarP(&identityFile, "identity", "i", "", "SSH identity file (private key)")
	cmd.Flags().BoolVar(&background, "background", false, "Start operator in background after deployment")

	return cmd
}

func operatorStreamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stream [host...] [flags]",
		Short: "Stream and execute the operator on remote hosts via SSH",
		Long:  `Stream the g8e operator binary via native Go crypto/ssh and execute it directly on remote hosts. Supports concurrent streaming, structured JSON output, and advanced SSH configuration. This is the canonical stream implementation (replaces the old exec.Command version).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Convert cobra args to the format expected by RunStream
			// RunStream expects args like ["host1", "host2", "--arch", "amd64", "--endpoint", "..."]
			stream.RunStream(args)
			return nil
		},
	}

	// Flags that match the native stream implementation
	cmd.Flags().String("arch", "amd64", "Target architecture: amd64, arm64, 386")
	cmd.Flags().String("hosts", "", "File of hosts (one per line) or - for stdin")
	cmd.Flags().Int("concurrency", 50, "Max parallel SSH sessions")
	cmd.Flags().Int("timeout", 60, "Per-host dial+inject timeout in seconds")
	cmd.Flags().Bool("no-git", false, "Disable ledger")
	cmd.Flags().String("ssh-config", "", "Path to SSH config file (default: ~/.ssh/config)")
	cmd.Flags().String("known-hosts", "", "Path to SSH known_hosts file (default: ~/.ssh/known_hosts)")
	cmd.Flags().String("binary-dir", "", "Directory containing arch-specific Operator builds")
	cmd.Flags().String("ssh-identity-file", "", "SSH identity file path")
	cmd.Flags().String("ssh-user", "", "SSH username")
	cmd.Flags().String("ssh-passphrase", "", "Passphrase for encrypted SSH private keys")
	cmd.Flags().Bool("preflight", false, "Enable pre-flight SSH connectivity check before binary transfer")

	return cmd
}

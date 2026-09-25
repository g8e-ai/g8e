// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/cli/stream"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/ollama"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/inference"
	"github.com/spf13/cobra"
)

type operatorListEntry struct {
	OperatorID                      string                   `json:"operator_id"`
	OperatorSessionID               string                   `json:"operator_session_id"`
	OperatorType                    constants.OperatorType   `json:"operator_type"`
	Status                          constants.OperatorStatus `json:"status"`
	Component                       constants.ComponentName  `json:"component"`
	Hostname                        string                   `json:"hostname,omitempty"`
	Name                            string                   `json:"name,omitempty"`
	InferenceEnabled                *bool                    `json:"inference_enabled,omitempty"`
	InferenceOllamaEndpoint         string                   `json:"inference_ollama_endpoint,omitempty"`
	ProviderBoundaryObserverEnabled *bool                    `json:"provider_boundary_observer_enabled,omitempty"`
	ProvenanceOperatorEnabled       *bool                    `json:"provenance_operator_enabled,omitempty"`
	ProvenanceModelStorageRoot      string                   `json:"provenance_operator_model_storage_root,omitempty"`
}

type operatorListOutput struct {
	Operators []operatorListEntry `json:"operators"`
}

func operatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "operator",
		Aliases: []string{"operators"},
		Short:   "Manage Operator instances",
		Long:    `Manage and view g8e Operator instances connected to the Gateway.`,
	}

	cmd.AddCommand(
		operatorListCmd(),
		operatorShowCmd(),
		operatorBindCmd(),
		operatorRunCmd(),
		operatorStopCmd(),
		operatorStartCmd(),
		operatorCpCmd(),
		operatorScpCmd(),
		operatorDeployCmd(),
		operatorStreamCmd(),
		operatorModelCmd(),
	)

	return cmd
}

func boolPointer(value bool) *bool {
	return &value
}

func operatorModelCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "model", Hidden: true}
	cmd.AddCommand(
		operatorModelPullCmd(),
		operatorModelCopyCmd(),
		operatorModelInventoryCmd(),
		operatorModelResidencyCmd(),
		operatorModelReleaseCmd(),
	)
	return cmd
}

func operatorOllamaEndpoint() (string, error) {
	endpoint := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	if endpoint == "" {
		return "", fmt.Errorf("operator: model maintenance: %w", constants.ErrInferenceEndpointInvalid)
	}
	return endpoint, nil
}

func operatorModelPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "pull <model>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, err := operatorOllamaEndpoint()
			if err != nil {
				return err
			}
			base, err := url.Parse(endpoint)
			if err != nil {
				return fmt.Errorf("operator: pull model: %w", err)
			}
			client := ollama.NewClient(base, &http.Client{Timeout: inference.ProviderRequestTimeout})
			if err := client.Pull(cmd.Context(), args[0], nil); err != nil {
				return fmt.Errorf("operator: pull model: %w", err)
			}
			return nil
		},
	}
}

func operatorModelCopyCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "copy <source> <destination>",
		Hidden: true,
		Args:   cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, err := operatorOllamaEndpoint()
			if err != nil {
				return err
			}
			base, err := url.Parse(endpoint)
			if err != nil {
				return fmt.Errorf("operator: copy model: %w", err)
			}
			client := ollama.NewClient(base, &http.Client{Timeout: inference.ProviderRequestTimeout})
			if err := client.Copy(cmd.Context(), args[0], args[1]); err != nil {
				return fmt.Errorf("operator: copy model: %w", err)
			}
			return nil
		},
	}
}

func operatorModelInventoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "inventory",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, err := operatorOllamaEndpoint()
			if err != nil {
				return err
			}
			backend, err := inference.NewOllamaBackend(endpoint, slog.Default())
			if err != nil {
				return fmt.Errorf("operator: model inventory: %w", err)
			}
			entries, err := backend.ListProviderModelInventory(cmd.Context())
			if err != nil {
				return fmt.Errorf("operator: model inventory: %w", err)
			}
			payload, err := json.Marshal(entries)
			if err != nil {
				return fmt.Errorf("operator: model inventory: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			return err
		},
	}
}

func operatorModelResidencyCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "residency",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, err := operatorOllamaEndpoint()
			if err != nil {
				return err
			}
			residency, err := inference.ReadProviderResidency(cmd.Context(), inference.ProviderResidencyOptions{Endpoint: endpoint})
			if err != nil {
				return fmt.Errorf("operator: model residency: %w", err)
			}
			payload, err := json.Marshal(residency)
			if err != nil {
				return fmt.Errorf("operator: model residency: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(payload))
			return err
		},
	}
}

func operatorModelReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "release <model>",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, err := operatorOllamaEndpoint()
			if err != nil {
				return err
			}
			backend, err := inference.NewOllamaBackend(endpoint, slog.Default())
			if err != nil {
				return fmt.Errorf("operator: release model: %w", err)
			}
			if err := backend.ReleaseModel(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("operator: release model: %w", err)
			}
			return nil
		},
	}
}

func operatorListCmd() *cobra.Command {
	return operatorListCmdWithConfig(loadConfig, defaultAPIClientFactory, newFileSvc)
}

func operatorListCmdWithConfig(configLoader func(string) (*config.Config, error), clientFactory apiClientFactory, fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all Operator instances",
		Long:  `List all Operator instances currently connected to the Gateway.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			creds, err := auth.LoadCredentials(fileSvc, cfg)
			if err != nil || creds == nil {
				return fmt.Errorf("%w: Please run './g8e auth enroll user' first", constants.ErrNotAuthenticated)
			}

			client, err := clientFactory(fileSvc, cfg)
			if err != nil {
				return err
			}

			resp, err := client.Get(constants.APIPaths.Operators + "?user_id=" + creds.UserID)
			if err != nil {
				return err
			}

			var slotResp models.OperatorSlotResponse
			if err := json.Unmarshal(resp, &slotResp); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
			}

			operators := slotResp.Operators
			if len(operators) == 0 {
				if output.JSONEnabled(cmd) {
					return output.WriteJSON(cmd.OutOrStdout(), operatorListOutput{Operators: []operatorListEntry{}})
				}
				cmd.Println("No operators found")
				return nil
			}

			if output.JSONEnabled(cmd) {
				entries := make([]operatorListEntry, 0, len(operators))
				for _, op := range operators {
					entry := operatorListEntry{
						OperatorID:        op.ID,
						OperatorSessionID: op.OperatorSessionID,
						OperatorType:      op.OperatorType,
						Status:            op.Status,
						Component:         op.Component,
						Hostname:          operatorHostnameValue(op),
						Name:              op.Name,
					}
					if op.RuntimeConfig != nil {
						entry.InferenceEnabled = boolPointer(op.RuntimeConfig.InferenceEnabled)
						entry.InferenceOllamaEndpoint = op.RuntimeConfig.InferenceOllamaEndpoint
						entry.ProviderBoundaryObserverEnabled = boolPointer(op.RuntimeConfig.ProviderBoundaryObserverEnabled)
						entry.ProvenanceOperatorEnabled = boolPointer(op.RuntimeConfig.ProvenanceOperatorEnabled)
						entry.ProvenanceModelStorageRoot = op.RuntimeConfig.ProvenanceOperatorModelStorageRoot
					}
					entries = append(entries, entry)
				}
				return output.WriteJSON(cmd.OutOrStdout(), operatorListOutput{Operators: entries})
			}

			cmd.Printf("Operators (%d total)\n", len(operators))
			cmd.Println(strings.Repeat("=", 140))
			cmd.Printf("  %-36s  %-12s  %-24s  %-36s  %-15s\n", "ID", "Type", "Hostname", "Session ID", "Status")
			cmd.Println(strings.Repeat("-", 140))
			for _, op := range operators {
				cmd.Printf("  %-36s  %-12s  %-24s  %-36s  %-15s\n", op.ID, op.OperatorType, operatorHostnameDisplay(op), op.OperatorSessionID, op.Status)
			}

			return nil
		},
	}
	return cmd
}

func operatorStartCmd() *cobra.Command {
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
	var latticeEndpoint string
	var latticeClientID string
	var latticeClientSecret string
	var latticeSandboxesToken string
	var latticeEntityName string
	var latticePostureFloor string
	var inferenceEnabled bool
	var inferenceOllamaEndpoint string
	var inferencePrimaryModel string
	var inferenceAssistantModel string
	var inferenceLiteModel string
	var inferenceKeepAlive string
	var inferenceCampaignID string
	var inferenceModelRegistryDigest string
	var providerBoundaryObserverEnabled bool
	var providerBoundaryObserverID string
	var provenanceOperatorEnabled bool
	var provenanceOperatorID string
	var provenanceOperatorModelStorageRoot string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the g8e Operator in foreground (worker mode)",
		Long:  `Start the g8e Operator in foreground as a worker. This connects to the Gateway and executes commands. This is the re-exec target for remote deployment and can also be run directly for debugging.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			endpoint, _ := cmd.Flags().GetString("endpoint")
			opts := serve.ServeOperatorOptions{
				LogLevel:                        logLevel,
				Endpoint:                        endpoint,
				TrustBundlePath:                 trustBundle,
				PrivateKey:                      key,
				ClientCert:                      clientCert,
				WorkingDir:                      workingDir,
				LaunchDir:                       workingDir,
				CloudMode:                       cloud,
				CloudProvider:                   provider,
				ExecutionVault:                  executionVault,
				NoGit:                           noGit,
				HeartbeatInterval:               time.Duration(heartbeatInterval) * time.Second,
				InferenceEnabled:                inferenceEnabled,
				InferenceOllamaEndpoint:         inferenceOllamaEndpoint,
				InferencePrimaryModel:           inferencePrimaryModel,
				InferenceAssistantModel:         inferenceAssistantModel,
				InferenceLiteModel:              inferenceLiteModel,
				InferenceKeepAlive:              inferenceKeepAlive,
				InferenceCampaignID:             inferenceCampaignID,
				InferenceModelRegistryDigest:    inferenceModelRegistryDigest,
				ProviderBoundaryObserverEnabled: providerBoundaryObserverEnabled,
				ProviderBoundaryObserverID:      providerBoundaryObserverID,

				ProvenanceOperatorEnabled:          provenanceOperatorEnabled,
				ProvenanceOperatorID:               provenanceOperatorID,
				ProvenanceOperatorModelStorageRoot: provenanceOperatorModelStorageRoot,
			}

			// Run operator (this blocks until shutdown)
			serve.RunOperator(opts, versionInfoFromCmd(cmd))
			return nil
		},
	}

	cmd.Flags().StringVarP(&key, "key", "k", "", "Path to operator private key")
	cmd.Flags().StringVar(&clientCert, "cert", "", "Path to operator client certificate")
	cmd.Flags().StringVar(&trustBundle, "trust-bundle", "", "Path to CA trust bundle")
	cmd.Flags().StringVar(&workingDir, "working-dir", "", "Working directory for command execution")
	cmd.Flags().BoolVarP(&cloud, "cloud", "c", false, "Cloud operator mode")
	cmd.Flags().StringVar(&provider, "provider", "", "Cloud provider (aws, gcp, azure)")
	cmd.Flags().BoolVarP(&executionVault, "execution-vault", "s", true, "Enable execution vault (data stays in working directory)")
	cmd.Flags().BoolVarP(&noGit, "no-git", "G", false, "Disable Git integration")
	cmd.Flags().StringVarP(&logLevel, "log", "l", "info", "Log level: info, error, debug")
	cmd.Flags().IntVar(&heartbeatInterval, "heartbeat-interval", 30, "Heartbeat interval in seconds")
	cmd.Flags().StringVar(&latticeEndpoint, "lattice-endpoint", "", "Lattice gRPC endpoint URL")
	cmd.Flags().StringVar(&latticeClientID, "lattice-client-id", "", "OAuth2 client ID")
	cmd.Flags().StringVar(&latticeClientSecret, "lattice-client-secret", "", "OAuth2 client secret")
	cmd.Flags().StringVar(&latticeSandboxesToken, "lattice-sandboxes-token", "", "Sandbox authorization token")
	cmd.Flags().StringVar(&latticeEntityName, "lattice-entity-name", "", "Entity display name")
	cmd.Flags().StringVar(&latticePostureFloor, "lattice-posture-floor", "consensus", "Minimum governance posture")

	// Inference (g8ellama) flags. Enable when the operator runs as an
	// Inference Node calling the configured remote Ollama provider.
	cmd.Flags().BoolVar(&inferenceEnabled, "inference-enabled", false, "Enable governed LLM inference backend (g8ellama)")
	cmd.Flags().StringVar(&inferenceOllamaEndpoint, "inference-ollama-endpoint", "", "Remote Ollama provider endpoint (default: http://127.0.0.1:11434)")
	cmd.Flags().StringVar(&inferencePrimaryModel, "inference-primary-model", "", "Ollama model name for the Primary chat tier")
	cmd.Flags().StringVar(&inferenceAssistantModel, "inference-assistant-model", "", "Ollama model name for the Assistant chat tier")
	cmd.Flags().StringVar(&inferenceLiteModel, "inference-lite-model", "", "Ollama model name for the Lite chat tier")
	cmd.Flags().StringVar(&inferenceKeepAlive, "inference-keep-alive", "", "Ollama keep-alive duration (default: -1 for infinite)")
	cmd.Flags().StringVar(&inferenceCampaignID, "inference-campaign-id", "", "Frozen evaluation campaign authorized by this inference operator")
	cmd.Flags().StringVar(&inferenceModelRegistryDigest, "inference-model-registry-digest", "", "SHA-256 digest of the frozen campaign model registry")
	cmd.Flags().BoolVar(&providerBoundaryObserverEnabled, "provider-boundary-observer-enabled", false, "Enable read-only provider-boundary hardware observation on the approved provider host")
	cmd.Flags().StringVar(&providerBoundaryObserverID, "provider-boundary-observer-id", "", "Stable observer identity pseudonym")
	cmd.Flags().BoolVar(&provenanceOperatorEnabled, "provenance-operator-enabled", false, "Enable storage-side model provenance attestation at the model file site")
	cmd.Flags().StringVar(&provenanceOperatorID, "provenance-operator-id", "", "Stable provenance operator identity pseudonym")
	cmd.Flags().StringVar(&provenanceOperatorModelStorageRoot, "model-storage-root", "", "Root directory containing content-addressed model weight blobs (for example ~/.ollama/models)")

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
	cmd.Flags().BoolVar(&preserve, "preserve", false, "Preserves modification times, access times, and modes from the source file")
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
	return operatorDeployCmdWithConfig(loadConfig, newFileSvc)
}

func operatorDeployCmdWithConfig(configLoader func(string) (*config.Config, error), fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var hosts string
	var port int
	var identityFile string
	var background bool

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy the operator binary to remote hosts and start it",
		Long:  `Deploy the g8e operator binary to remote hosts via SSH and start it in the background. Uses your existing SSH config for authentication. Requires './g8e auth enroll user' first.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			creds, err := auth.LoadCredentials(fileSvc, cfg)
			if err != nil || creds == nil {
				return fmt.Errorf("%w: Please run './g8e auth enroll user' first", constants.ErrNotAuthenticated)
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

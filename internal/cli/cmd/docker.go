// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 0.0.

package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// dockerComposePath resolves the root unified-stack compose file relative to
// the current working directory. The root compose file deploys the full
// platform stack (gateway, operator, ensemble, dashboard).
func dockerComposePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	return filepath.Join(cwd, constants.DockerComposeFile), nil
}

// checkDockerComposeFileExists verifies the root compose file is present in
// the current working directory.
func checkDockerComposeFileExists() error {
	composePath, err := dockerComposePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(composePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s (run this command from the repository root)", constants.ErrNotFound, constants.DockerComposeFile)
		}
		return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
	}
	return nil
}

// prepareDockerHostRuntime ensures the host-side .g8e tree exists and is
// writable before Docker Compose starts the gateway container.
func prepareDockerHostRuntime(ctx context.Context, fileSvc fs.RuntimeFileService) error {
	if err := fs.EnsureDockerHostRuntimeLayout(ctx, fileSvc); err != nil {
		return fmt.Errorf("docker: prepare host runtime: %w", err)
	}
	return nil
}

// runDockerCompose builds and runs a `docker compose` command against the root
// compose file, streaming stdout/stderr to the console. The optional profiles
// activate compose profiles (e.g. bootstrapped, cross-enrollment) so multiple
// workloads can be started together.
func runDockerCompose(args []string, profiles ...string) error {
	composePath, err := dockerComposePath()
	if err != nil {
		return err
	}
	if err := checkDockerAvailable(); err != nil {
		return err
	}
	fullArgs := []string{"compose", "-f", toDockerPath(composePath)}
	for _, profile := range profiles {
		if profile != "" {
			fullArgs = append(fullArgs, "--profile", profile)
		}
	}
	fullArgs = append(fullArgs, args...)

	c := exec.Command("docker", fullArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// runDockerComposeOutput runs a `docker compose` command against the root
// compose file and returns its combined stdout/stderr output. Unlike
// runDockerCompose it does not stream to the console, so callers can embed the
// output in a larger status view (e.g. `g8e gw status`). The optional profiles
// activate compose profiles.
func runDockerComposeOutput(args []string, profiles ...string) (string, error) {
	composePath, err := dockerComposePath()
	if err != nil {
		return "", err
	}
	if err := checkDockerAvailable(); err != nil {
		return "", err
	}
	fullArgs := []string{"compose", "-f", toDockerPath(composePath)}
	for _, profile := range profiles {
		if profile != "" {
			fullArgs = append(fullArgs, "--profile", profile)
		}
	}
	fullArgs = append(fullArgs, args...)

	c := exec.Command("docker", fullArgs...)
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}
	return string(out), nil
}

// printDockerStackStatus writes a "Docker Compose Stack" section to w showing
// the output of `docker compose ps`. It prints a friendly in-section note and
// returns the error when Docker is not available or the root compose file is
// missing, so callers can decide whether to abort (docker status) or continue
// (gw status, which reports both localhost and Docker status in one view).
func printDockerStackStatus(w io.Writer, profile string) error {
	fmt.Fprintln(w, "Docker Compose Stack")
	fmt.Fprintln(w, "---------------------")
	if err := checkDockerComposeFileExists(); err != nil {
		if errors.Is(err, constants.ErrNotFound) {
			fmt.Fprintln(w, "No docker-compose.yml found in current directory")
		} else {
			fmt.Fprintf(w, "Docker status unavailable: %v\n", err)
		}
		return err
	}
	if err := checkDockerAvailable(); err != nil {
		fmt.Fprintln(w, "Docker not available (install Docker or start the daemon)")
		return err
	}
	out, err := runDockerComposeOutput([]string{"ps"}, profile)
	if err != nil {
		fmt.Fprintf(w, "Docker status unavailable: %v\n", err)
		return err
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		fmt.Fprintln(w, "No running containers")
		return nil
	}
	fmt.Fprintln(w, trimmed)
	return nil
}

func dockerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docker",
		Short: "Manage the Docker Compose unified stack",
		Long: `Manage the root Docker Compose unified stack (gateway, operator, ensemble, dashboard).

Use ` + "`" + `g8e docker init` + "`" + ` to build images and bring the full evaluation stack online in one
command (owner enrollment, platform approvals, readiness checks). See
docs/guides/unified_stack.md.

The root ` + "`" + `docker-compose.yml` + "`" + ` deploys the full platform. Only the gateway starts
by default; the operator, ensemble, and dashboard are gated behind the
` + "`" + `bootstrapped` + "`" + ` profile and require owner enrollment before they can start. Pass
--profile bootstrapped (or --full) to bring up the workloads after enrolling.

Run these commands from the repository root where ` + "`" + `docker-compose.yml` + "`" + ` lives.`,
	}
	cmd.AddCommand(
		dockerInitCmd(),
		dockerStartCmd(),
		dockerStopCmd(),
		dockerStatusCmd(),
		dockerBuildCmd(),
		dockerCleanCmd(),
		dockerResetCmd(),
		dockerRebuildCmd(),
		dockerLogsCmd(),
	)
	return cmd
}

// dockerFullStackProfiles returns the compose profiles required for the full
// evaluation topology (data operator, inference operator, ensemble, dashboard).
func dockerFullStackProfiles() []string {
	return []string{
		constants.DockerBootstrappedProfile,
		constants.DockerEvaluationProfile,
	}
}

// resolveDockerProfile returns the bootstrapped profile name when full is true,
// or the explicit profile override when set, otherwise the empty string
// (gateway-only startup).
func resolveDockerProfile(full bool, profile string) string {
	if profile != "" {
		return profile
	}
	if full {
		return constants.DockerBootstrappedProfile
	}
	return ""
}

func dockerInitCmd() *cobra.Command {
	return dockerInitCmdWithConfig(loadConfig, newFileSvc, defaultAPIClientFactory, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
}

func dockerInitCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	clientFactory apiClientFactory,
	checkOperatorRunning func(*config.Config) error,
	enrollerFactory enrollerFactory,
) *cobra.Command {
	var (
		skipBuild      bool
		skipEnroll     bool
		skipApprovals  bool
		noCache        bool
		clean          bool
		headlessEnroll bool
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Build, bootstrap, and bring up the full unified stack",
		Long: `Build images and bring the full unified Docker stack online in one flow.

This command performs the standard bootstrap workflow documented in
docs/guides/unified_stack.md:

  1. Validate repository-root .env evaluation settings.
  2. Prepare the host .g8e runtime tree for CLI enrollment (PKI lives on host).
  3. Build Docker images for the unified stack (unless --skip-build).
  4. Start the gateway and wait for it to become healthy.
  5. Enroll the CLI owner (unless --skip-enroll).
  6. Start bootstrapped + evaluation workloads (operator, inference operator,
     ensemble, and dashboard).
  7. Auto-approve pending platform enrollment requests in the documented order
     (data operator, dashboard, ensemble, inference operator) unless
     --skip-approvals is set.
  8. Wait for the ensemble health endpoint to respond.

Use --clean to wipe containers, volumes, and networks before init.
That destroys the trust domain and repeats owner enrollment from scratch.

By default, owner enrollment runs the browser passkey ceremony (same as
'g8e auth enroll user'). Pass --headless to opt into an mTLS-only CLI identity
without passkey registration or OS trust installation (same as
'g8e auth enroll user --headless'). On a cold start that bootstraps a fresh
gateway, headless completes immediately; on an already-bootstrapped gateway it
prints 'g8e auth approve-recovery <token>' and waits for approval from an
already-enrolled CLI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			if err := checkDockerAvailable(); err != nil {
				return err
			}
			if err := checkDockerInitEnv(); err != nil {
				return err
			}

			if clean {
				cmd.Println("Cleaning existing Docker Compose stack before init...")
				if err := runDockerCompose([]string{"down", "-v", "--remove-orphans", "-t", "0"}, dockerTeardownProfiles("")...); err != nil {
					cmd.Printf("Warning: compose down had issues: %v\n", err)
				}
				forceRemoveLeftovers(cmd, constants.DockerProjectPrefix)
			}

			cfg, err := configLoader("")
			if err != nil {
				return err
			}
			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			if err := prepareDockerHostRuntime(cmd.Context(), fileSvc); err != nil {
				return err
			}

			if !skipBuild {
				buildArgs, err := dockerBuildArgs(versionInfoFromCmd(cmd), noCache)
				if err != nil {
					return fmt.Errorf("docker init: build arguments: %w", err)
				}
				cmd.Println("Building Docker images for the unified stack...")
				if err := runDockerCompose(buildArgs, dockerFullStackProfiles()...); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
				}
				cmd.Println("Docker images built successfully.")
			}

			cmd.Println("Starting gateway...")
			if err := runDockerCompose([]string{"up", "-d"}); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			if err := waitForDockerGatewayHealthy(cmd); err != nil {
				return err
			}

			if !skipEnroll {
				cmd.Println()
				cmd.Println("Enrolling CLI owner with the gateway...")
				if err := checkOperatorRunning(cfg); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrDockerInitEnrollmentFailed, err)
				}
				coordinator := enrollerFactory(func(format string, a ...any) {
					cmd.Printf(format+"\n", a...)
				}, fileSvc, cfg)
				result, err := coordinator.Enroll(cmd.Context(), dockerOwnerEnrollmentOptions(headlessEnroll))
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrDockerInitEnrollmentFailed, err)
				}
				if result.Reused {
					cmd.Printf("Reusing existing CLI identity (User ID: %s)\n", result.UserID)
				} else if headlessEnroll {
					cmd.Printf("Headless CLI enrollment complete (User ID: %s, Session: %s)\n", result.UserID, result.CLISessionID)
					cmd.Println("Identity is mTLS-only; no passkey was registered.")
				} else {
					cmd.Printf("CLI enrollment complete (User ID: %s, Session: %s)\n", result.UserID, result.CLISessionID)
				}
			}

			cmd.Println()
			cmd.Println("Starting full stack (bootstrapped + evaluation profiles)...")
			if err := runDockerCompose([]string{"up", "-d"}, dockerFullStackProfiles()...); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}

			if skipApprovals {
				cmd.Println()
				cmd.Println("--skip-approvals set: workloads are started but will block waiting")
				cmd.Println("for manual platform enrollment approval. Run:")
				cmd.Println("  g8e auth pending")
				cmd.Println("  g8e auth approve-platform-enrollment <request-id> --yes")
				return nil
			}

			client, err := clientFactory(fileSvc, cfg)
			if err != nil {
				return fmt.Errorf("%w: create API client: %w", constants.ErrDockerInitApprovalFailed, err)
			}
			cmd.Println()
			cmd.Println("Approving pending platform enrollment requests...")
			if err := runDockerInitApprovals(cmd, client); err != nil {
				return err
			}

			cmd.Println()
			cmd.Println("Waiting for ensemble to become healthy...")
			if err := waitForDockerEnsembleHealthy(cmd); err != nil {
				return err
			}

			if err := reportDockerPublicSpectatorReady(cmd); err != nil {
				cmd.Printf("Warning: public spectator bootstrap check failed: %v\n", err)
				cmd.Println("Campaign publish may still work once the gateway exports the first batch.")
			}

			cmd.Println()
			cmd.Println("Unified stack init complete.")
			printDockerSpectatorEndpoints(cmd)
			cmd.Println("Run 'g8e docker status' to check service status.")
			cmd.Println("Run 'g8e operator list' and 'g8e eval inference status --json' to verify operators.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipBuild, "skip-build", false, "Skip building Docker images before startup")
	cmd.Flags().BoolVar(&skipEnroll, "skip-enroll", false, "Skip CLI owner enrollment (reuse an existing enrolled CLI identity)")
	cmd.Flags().BoolVar(&skipApprovals, "skip-approvals", false, "Start workloads without auto-approving platform enrollment requests")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Build Docker images without using the cache")
	cmd.Flags().BoolVar(&clean, "clean", false, "Remove containers, volumes, and networks before init (destructive)")
	cmd.Flags().BoolVar(&headlessEnroll, "headless", false, "Enroll an mTLS-only CLI owner without the browser passkey ceremony (same as 'g8e auth enroll user --headless')")
	return cmd
}

// dockerOwnerEnrollmentOptions returns EnrollmentOptions for docker init/start
// owner enrollment. Default (headless=false) runs the browser passkey ceremony.
// Headless opts into mTLS-only identity and skips OS trust installation, matching
// 'g8e auth enroll user --headless'.
func dockerOwnerEnrollmentOptions(headless bool) auth.EnrollmentOptions {
	return auth.EnrollmentOptions{
		NoSystemTrust: headless,
		Headless:      headless,
	}
}

// checkDockerInitEnv verifies repository-root .env contains the settings required
// for the evaluation profile inference operator.
func checkDockerInitEnv() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	envPath := filepath.Join(cwd, ".env")
	values, err := readDotEnvFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: copy .env.example to .env and set G8E_OLLAMA_ENDPOINT, G8E_INFERENCE_CAMPAIGN_ID, and G8E_INFERENCE_MODEL_REGISTRY_DIGEST", constants.ErrDockerInitEnvRequired)
		}
		return fmt.Errorf("%w: read .env: %w", constants.ErrDockerInitEnvRequired, err)
	}
	required := []string{
		"G8E_OLLAMA_ENDPOINT",
		"G8E_INFERENCE_CAMPAIGN_ID",
		"G8E_INFERENCE_MODEL_REGISTRY_DIGEST",
	}
	var missing []string
	for _, key := range required {
		if strings.TrimSpace(values[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: set %s in .env (see docs/guides/unified_stack.md)", constants.ErrDockerInitEnvRequired, strings.Join(missing, ", "))
	}
	return nil
}

// readDotEnvFile parses KEY=VALUE lines from a dotenv file. It ignores blank
// lines and # comments and does not perform shell expansion.
func readDotEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		values[key] = value
	}
	return values, nil
}

// dockerTeardownProfiles returns compose profiles for stop/clean/reset down
// operations that must remove every optional unified-stack service. The
// evaluation profile carries g8e-inference-operator; omitting it leaves that
// container running and its volumes in use.
func dockerTeardownProfiles(explicit string) []string {
	if explicit != "" {
		return []string{explicit}
	}
	return []string{
		constants.DockerBootstrappedProfile,
		constants.DockerEvaluationProfile,
	}
}

func dockerStartCmd() *cobra.Command {
	return dockerStartCmdWithConfig(loadConfig, newFileSvc, defaultAPIClientFactory, auth.CheckOperatorRunning, newDefaultEnrollmentCoordinator)
}

// dockerStartDeps holds the injectable dependencies for the interactive docker
// start walkthrough. Production wires real factories; tests wire stubs. The
// cfg and fileSvc fields are pre-resolved by the command RunE before containers
// start so a factory failure aborts early.
type dockerStartDeps struct {
	clientFactory        apiClientFactory
	checkOperatorRunning func(*config.Config) error
	enrollerFactory      enrollerFactory
	waitGatewayHealthy   func(*cobra.Command) error
	cfg                  *config.Config
	fileSvc              fs.RuntimeFileService
}

func dockerStartCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	clientFactory apiClientFactory,
	checkOperatorRunning func(*config.Config) error,
	enrollerFactory enrollerFactory,
) *cobra.Command {
	var full bool
	var profile string
	var skipEnroll bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Docker Compose unified stack",
		Long: `Start the Docker Compose unified stack in the background.

By default only the gateway starts. Pass --full (or --profile bootstrapped) to
also start the operator, ensemble, and dashboard workloads, which require
owner enrollment before they become ready.

When --full is set, the command walks the owner through interactive enrollment:
  1. Enrolls the CLI user (the first owner) with the gateway.
  2. Prompts to approve the Ensemble platform enrollment request.
  3. Prompts to approve the Dashboard platform enrollment request.
  4. Prompts to approve the Operator platform enrollment request.

Each component prompt accepts y to approve or n (or any other input) to skip.
Use --skip-enroll to start the bootstrapped profile without the interactive
walkthrough (the workloads will block waiting for manual approval).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			resolved := resolveDockerProfile(full, profile)
			scope := "gateway"
			if resolved != "" {
				scope = fmt.Sprintf("full stack (profile %s)", resolved)
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			if err := prepareDockerHostRuntime(cmd.Context(), fileSvc); err != nil {
				return err
			}

			var walkthroughDeps *dockerStartDeps
			if resolved != "" && !skipEnroll {
				cfg, err := configLoader("")
				if err != nil {
					return err
				}
				walkthroughDeps = &dockerStartDeps{
					clientFactory:        clientFactory,
					checkOperatorRunning: checkOperatorRunning,
					enrollerFactory:      enrollerFactory,
					waitGatewayHealthy:   waitForDockerGatewayHealthy,
					cfg:                  cfg,
					fileSvc:              fileSvc,
				}
			}

			cmd.Printf("Starting Docker Compose %s...\n", scope)
			if err := runDockerCompose([]string{"up", "-d"}, resolved); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Printf("\nDocker Compose %s started successfully.\n", scope)
			cmd.Println("Run 'g8e docker status' to check service status.")
			cmd.Println("Run 'g8e docker logs' to follow logs.")

			if resolved == "" {
				cmd.Println()
				cmd.Println("To bring up the operator, ensemble, and dashboard after enrolling")
				cmd.Println("the first owner, run 'g8e docker start --full'.")
				return nil
			}

			if skipEnroll {
				cmd.Println()
				cmd.Println("--skip-enroll set: workloads are started but will block waiting")
				cmd.Println("for manual platform enrollment approval. Run:")
				cmd.Println("  g8e auth pending")
				cmd.Println("  g8e auth approve-platform-enrollment <request-id>")
				return nil
			}

			return runDockerStartWalkthrough(cmd, *walkthroughDeps)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Start the full stack (gateway + operator + ensemble + dashboard)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to start (e.g. bootstrapped)")
	cmd.Flags().BoolVar(&skipEnroll, "skip-enroll", false, "Start the bootstrapped profile without the interactive enrollment walkthrough")
	return cmd
}

// runDockerStartWalkthrough drives the interactive enrollment walkthrough after
// the bootstrapped profile containers are up. It enrolls the CLI owner, waits for
// the gateway to be reachable, then prompts the owner to approve each
// component's platform enrollment request in order: ensemble, dashboard,
// operator. Each prompt is skippable (any answer other than y skips that
// component without aborting the walkthrough).
func runDockerStartWalkthrough(cmd *cobra.Command, deps dockerStartDeps) error {
	ctx := cmd.Context()

	cmd.Println()
	cmd.Println("=== Interactive enrollment walkthrough ===")
	cmd.Println()

	waitHealthy := deps.waitGatewayHealthy
	if waitHealthy == nil {
		waitHealthy = waitForDockerGatewayHealthy
	}
	if err := waitHealthy(cmd); err != nil {
		return err
	}

	cfg := deps.cfg
	fileSvc := deps.fileSvc

	cmd.Println("Step 1: Enroll the CLI owner with the gateway.")
	cmd.Println("  This creates the first user/session and registers a passkey.")
	if err := deps.checkOperatorRunning(cfg); err != nil {
		cmd.Printf("  Gateway not reachable for enrollment: %v\n", err)
		return fmt.Errorf("%w: %w", constants.ErrDockerStartEnrollmentFailed, err)
	}

	coordinator := deps.enrollerFactory(func(format string, a ...any) {
		cmd.Printf(format+"\n", a...)
	}, fileSvc, cfg)
	result, err := coordinator.Enroll(ctx, auth.EnrollmentOptions{})
	if err != nil {
		cmd.Printf("  Owner enrollment failed: %v\n", err)
		return fmt.Errorf("%w: %w", constants.ErrDockerStartEnrollmentFailed, err)
	}
	if result.Reused {
		cmd.Printf("  Reusing existing CLI identity (no new certificate issued).\n")
	} else {
		cmd.Printf("  CLI session %s complete\n", result.Source)
	}
	cmd.Printf("  User ID: %s\n", result.UserID)
	cmd.Printf("  CLI Session ID: %s\n", result.CLISessionID)
	cmd.Println()

	client, err := deps.clientFactory(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("%w: create API client: %w", constants.ErrDockerStartApprovalFailed, err)
	}

	components := []models.PlatformComponentKind{
		models.PlatformComponentEnsemble,
		models.PlatformComponentDashboard,
		models.PlatformComponentOperator,
	}
	for i, component := range components {
		step := i + 2
		cmd.Printf("Step %d: Approve the %s platform enrollment request.\n", step, component)
		if err := promptApproveComponent(cmd, ctx, client, component); err != nil {
			cmd.Printf("  %s enrollment step failed: %v\n", component, err)
		}
		cmd.Println()
	}

	cmd.Println("Interactive enrollment walkthrough complete.")
	cmd.Println("Run 'g8e docker status' to check service status.")
	cmd.Println("Run 'g8e docker logs' to follow logs.")
	return nil
}

// reportDockerPublicSpectatorReady verifies the gateway-owned public mirror
// bootstrap endpoint responds after init. Host campaigns do not require
// `g8e public init`; the gateway initializes feed state on startup.
func reportDockerPublicSpectatorReady(cmd *cobra.Command) error {
	bootstrap, err := fetchPublicMirrorBootstrap(cmd.Context())
	if err != nil {
		return err
	}
	cmd.Printf("Public mirror bootstrap ready (high_water_sequence=%d).\n", bootstrap.Snapshot.HighWaterSequence)
	return nil
}

// printDockerSpectatorEndpoints prints the acceptance URLs for the embedded
// evaluation explorer and public mirror after docker init.
func printDockerSpectatorEndpoints(cmd *cobra.Command) {
	cmd.Printf("Evaluation explorer: http://127.0.0.1:%d/#/\n", constants.EvalExplorerDefaultPort)
	cmd.Printf("Public mirror bootstrap: http://127.0.0.1:%d/bootstrap\n", constants.PublicSpectatorPublicPort)
	cmd.Println("Campaigns publish through the gateway-owned public spectator automatically.")
}

// waitForDockerGatewayHealthy polls the gateway HTTP health endpoint until it
// responds 200 or the timeout elapses. The gateway container publishes 8080.
func waitForDockerGatewayHealthy(cmd *cobra.Command) error {
	cmd.Println("Waiting for the gateway container to become healthy...")
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", constants.Ports.OperatorHttp)
	plainClient := &http.Client{Timeout: 2 * time.Second} //nolint:gosec
	const (
		maxAttempts  = 60
		pollInterval = 500 * time.Millisecond
	)
	for i := 0; i < maxAttempts; i++ {
		resp, err := plainClient.Get(healthURL) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				cmd.Println("Gateway is healthy.")
				return nil
			}
		}
		if i == maxAttempts-1 {
			return fmt.Errorf("%w: gateway did not become healthy after %v",
				constants.ErrGatewayNotReady, time.Duration(maxAttempts)*pollInterval)
		}
		time.Sleep(pollInterval)
	}
	return nil
}

// promptApproveComponent fetches pending platform enrollment requests, finds
// the one matching the given component kind, displays it, and prompts the owner
// to approve (y) or skip (any other input). When approved it posts the
// decision. A missing pending request is reported but not fatal — the
// component may already be enrolled, or its container may not have submitted
// its request yet.
func promptApproveComponent(cmd *cobra.Command, ctx context.Context, client apiClient, component models.PlatformComponentKind) error {
	pendingBody, err := client.Get(constants.APIPaths.AuthPlatformEnrollmentPending)
	if err != nil {
		return fmt.Errorf("fetch pending list: %w", err)
	}

	var pendingResp models.PlatformEnrollmentPendingResponse
	if err := json.Unmarshal(pendingBody, &pendingResp); err != nil {
		return fmt.Errorf("parse pending list: %w", err)
	}

	req := findPendingRequestByComponent(pendingResp.Requests, component)
	if req == nil {
		cmd.Printf("  No pending %s enrollment request found (it may already be enrolled\n", component)
		cmd.Printf("  or its container has not submitted a request yet). Skipping.\n")
		return nil
	}

	printPlatformEnrollmentRequestDetails(cmd, req)

	if !confirmAction(cmd, fmt.Sprintf("Approve this %s enrollment request?", component)) {
		cmd.Printf("  Skipped %s enrollment. You can approve it later with:\n", component)
		cmd.Printf("  g8e auth approve-platform-enrollment %s\n", req.RequestID)
		return nil
	}

	decisionReq := models.PlatformEnrollmentDecisionRequest{
		RequestID: req.RequestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	}
	resp, err := approvePlatformEnrollmentDecision(client, decisionReq)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDockerStartApprovalFailed, err)
	}

	cmd.Printf("  %s enrollment request %s.\n", component, string(resp.State))
	return nil
}

// approvePlatformEnrollmentDecision posts an owner decision for a pending platform
// enrollment request and returns the gateway response.
func approvePlatformEnrollmentDecision(client apiClient, decisionReq models.PlatformEnrollmentDecisionRequest) (*models.PlatformEnrollmentDecisionResponse, error) {
	if err := decisionReq.Validate(); err != nil {
		return nil, fmt.Errorf("validate decision: %w", err)
	}
	respBody, err := client.Post(constants.APIPaths.AuthPlatformEnrollmentDecision, decisionReq)
	if err != nil {
		return nil, fmt.Errorf("post decision: %w", err)
	}
	var resp models.PlatformEnrollmentDecisionResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse decision response: %w", err)
	}
	return &resp, nil
}

// fetchPendingPlatformEnrollments returns the current pending platform enrollment
// requests from the gateway.
func fetchPendingPlatformEnrollments(client apiClient) ([]models.PlatformEnrollmentPendingRequest, error) {
	pendingBody, err := client.Get(constants.APIPaths.AuthPlatformEnrollmentPending)
	if err != nil {
		return nil, fmt.Errorf("fetch pending list: %w", err)
	}
	var pendingResp models.PlatformEnrollmentPendingResponse
	if err := json.Unmarshal(pendingBody, &pendingResp); err != nil {
		return nil, fmt.Errorf("parse pending list: %w", err)
	}
	return pendingResp.Requests, nil
}

// isInferenceOperatorPendingRequest reports whether a pending operator enrollment
// belongs to the dedicated inference operator workload.
func isInferenceOperatorPendingRequest(req *models.PlatformEnrollmentPendingRequest) bool {
	if req == nil || req.ComponentKind != models.PlatformComponentOperator {
		return false
	}
	if req.Hostname == "inference-operator" {
		return true
	}
	return strings.Contains(req.InstanceID, "inference-operator")
}

// platformEnrollmentApprovalRank assigns the documented approval order for the
// unified stack: data operator, dashboard, ensemble, inference operator.
func platformEnrollmentApprovalRank(req models.PlatformEnrollmentPendingRequest) int {
	switch req.ComponentKind {
	case models.PlatformComponentOperator:
		if isInferenceOperatorPendingRequest(&req) {
			return 4
		}
		return 1
	case models.PlatformComponentDashboard:
		return 2
	case models.PlatformComponentEnsemble:
		return 3
	default:
		return 99
	}
}

// runDockerInitApprovals auto-approves pending platform enrollment requests in
// the documented order until none remain or the readiness timeout elapses.
func runDockerInitApprovals(cmd *cobra.Command, client apiClient) error {
	const (
		maxRounds    = 90
		pollInterval = 2 * time.Second
	)
	approved := make(map[string]struct{})
	for round := 0; round < maxRounds; round++ {
		pending, err := fetchPendingPlatformEnrollments(client)
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrDockerInitApprovalFailed, err)
		}
		var next *models.PlatformEnrollmentPendingRequest
		bestRank := 100
		for i := range pending {
			req := pending[i]
			if _, ok := approved[req.RequestID]; ok {
				continue
			}
			rank := platformEnrollmentApprovalRank(req)
			if rank < bestRank {
				bestRank = rank
				next = &req
			}
		}
		if next == nil {
			if len(pending) == 0 {
				if err := waitForDockerEnsembleHealthy(cmd); err == nil {
					cmd.Println("All platform enrollment requests approved.")
					return nil
				}
			}
			time.Sleep(pollInterval)
			continue
		}

		printPlatformEnrollmentRequestDetails(cmd, next)
		decisionReq := models.PlatformEnrollmentDecisionRequest{
			RequestID: next.RequestID,
			Decision:  models.PlatformEnrollmentDecisionApprove,
		}
		resp, err := approvePlatformEnrollmentDecision(client, decisionReq)
		if err != nil {
			return fmt.Errorf("%w: approve %s (%s): %w", constants.ErrDockerInitApprovalFailed, next.ComponentKind, next.RequestID, err)
		}
		approved[next.RequestID] = struct{}{}
		cmd.Printf("Approved %s enrollment request %s (%s).\n", next.ComponentKind, next.RequestID, resp.State)
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("%w: timed out waiting for platform enrollment approvals to complete", constants.ErrDockerInitApprovalFailed)
}

// waitForDockerEnsembleHealthy polls the ensemble HTTP health endpoint until it
// responds 200 or the timeout elapses.
func waitForDockerEnsembleHealthy(cmd *cobra.Command) error {
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", constants.EnsembleDefaultPort)
	plainClient := &http.Client{Timeout: 2 * time.Second} //nolint:gosec
	const (
		maxAttempts  = 60
		pollInterval = 3 * time.Second
	)
	for i := 0; i < maxAttempts; i++ {
		resp, err := plainClient.Get(healthURL) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				cmd.Println("Ensemble is healthy.")
				return nil
			}
		}
		if i == maxAttempts-1 {
			return fmt.Errorf("%w: ensemble did not become healthy after %v",
				constants.ErrDockerInitReadinessFailed, time.Duration(maxAttempts)*pollInterval)
		}
		time.Sleep(pollInterval)
	}
	return nil
}

// findPendingRequestByComponent returns the first pending request matching the
// given component kind, or nil if none match.
func findPendingRequestByComponent(requests []models.PlatformEnrollmentPendingRequest, component models.PlatformComponentKind) *models.PlatformEnrollmentPendingRequest {
	for i := range requests {
		if requests[i].ComponentKind == component {
			return &requests[i]
		}
	}
	return nil
}

func dockerStopCmd() *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Docker Compose unified stack",
		Long:  `Stop and remove containers for the Docker Compose unified stack, preserving volumes and networks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			cmd.Println("Stopping Docker Compose stack...")
			if err := runDockerCompose([]string{"down"}, dockerTeardownProfiles(profile)...); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
			}
			cmd.Println("\nDocker Compose stack stopped successfully.")
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. bootstrapped)")
	return cmd
}

func dockerStatusCmd() *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status of the Docker Compose unified stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			if err := runDockerCompose([]string{"ps"}, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. bootstrapped)")
	return cmd
}

func dockerBuildArgs(vi serve.VersionInfo, noCache bool) ([]string, error) {
	if !isHex64(vi.SourceTreeStateHash) {
		return nil, constants.ErrSourceTreeHashInvalid
	}
	buildID := strings.TrimSpace(vi.BuildID)
	if buildID == "" {
		buildID = string(constants.SystemHealthUnknown)
	}
	sourceRevision := strings.TrimSpace(vi.SourceRevision)
	if sourceRevision == "" {
		sourceRevision = string(constants.SystemHealthUnknown)
	}
	args := []string{
		"build",
		"--build-arg", "BUILD_ID=" + buildID,
		"--build-arg", "SOURCE_REVISION=" + sourceRevision,
		"--build-arg", "SOURCE_TREE_HASH=" + vi.SourceTreeStateHash,
	}
	if noCache {
		args = append(args, "--no-cache")
	}
	return args, nil
}

func dockerBuildCmd() *cobra.Command {
	var noCache bool
	var profile string

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build Docker images for the unified stack",
		Long:  `Build all Docker images defined in the root docker-compose.yml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			buildArgs, err := dockerBuildArgs(versionInfoFromCmd(cmd), noCache)
			if err != nil {
				return fmt.Errorf("docker: build arguments: %w", err)
			}
			cmd.Println("Building Docker images...")
			if err := runDockerCompose(buildArgs, resolveDockerProfile(true, profile)); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Println("\nDocker images built successfully.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Build without using the Docker cache")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. bootstrapped)")
	return cmd
}

func dockerCleanCmd() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove containers, volumes, and networks for the unified stack",
		Long: `Remove containers, volumes, and networks for the Docker Compose unified stack.

This is a destructive operation that removes all associated Docker volumes and
networks, including the gateway data volume. Use --yes=false to confirm first.

Clean always targets the bootstrapped and evaluation profiles so that operator,
ensemble, dashboard, and inference-operator containers are removed alongside
the gateway, not just the default-profile gateway container.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			if !skipConfirm {
				cmd.Println("WARNING: This will remove ALL containers, volumes, and networks for the unified stack.")
				if !confirmAction(cmd, "Proceed with clean?") {
					cmd.Println("Clean cancelled.")
					return nil
				}
			}
			if err := checkDockerAvailable(); err != nil {
				cmd.Println("Docker not available — nothing to clean.")
				return nil
			}
			cmd.Println("Cleaning Docker Compose stack...")
			// Always pass the bootstrapped and evaluation profiles so operator,
			// ensemble, dashboard, and inference-operator containers are removed
			// together with the gateway. Without them, down only touches
			// default-profile services and profile-gated containers keep running,
			// holding their volumes and the shared network open.
			if err := runDockerCompose([]string{"down", "-v", "--remove-orphans", "-t", "0"}, dockerTeardownProfiles("")...); err != nil {
				cmd.Printf("Warning: compose down had issues: %v\n", err)
			}
			forceRemoveLeftovers(cmd, constants.DockerProjectPrefix)
			cmd.Println("\nDocker Compose stack cleaned successfully.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&skipConfirm, "yes", true, "Skip interactive confirmation (default: true)")
	return cmd
}

func dockerResetCmd() *cobra.Command {
	var full bool
	var profile string

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Clean and restart the Docker Compose unified stack",
		Long:  `Clean (remove containers, volumes, networks) and restart the Docker Compose unified stack.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			cmd.Println("Cleaning Docker Compose stack...")
			if err := runDockerCompose([]string{"down", "-v", "--remove-orphans", "-t", "0"}, dockerTeardownProfiles(profile)...); err != nil {
				cmd.Printf("Warning: compose down had issues: %v\n", err)
			}
			forceRemoveLeftovers(cmd, constants.DockerProjectPrefix)
			fileSvc, err := newFileSvc("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			if err := prepareDockerHostRuntime(cmd.Context(), fileSvc); err != nil {
				return err
			}
			resolved := resolveDockerProfile(full, profile)
			scope := "gateway"
			if resolved != "" {
				scope = fmt.Sprintf("full stack (profile %s)", resolved)
			}
			cmd.Printf("\nStarting Docker Compose %s...\n", scope)
			if err := runDockerCompose([]string{"up", "-d"}, resolved); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Printf("\nDocker Compose %s reset successfully.\n", scope)
			return nil
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Start the full stack (gateway + operator + ensemble + dashboard)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to start (e.g. bootstrapped)")
	return cmd
}

func dockerRebuildCmd() *cobra.Command {
	var noCache bool
	var full bool
	var profile string

	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild images and restart the Docker Compose unified stack",
		Long: `Stop the unified stack, rebuild all Docker images, and start it again.

Use --no-cache=false to reuse the Docker build cache.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			cmd.Println("Stopping Docker Compose stack...")
			if err := runDockerCompose([]string{"down"}, dockerTeardownProfiles(profile)...); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
			}
			buildArgs := []string{"build"}
			if noCache {
				buildArgs = append(buildArgs, "--no-cache")
			}
			cmd.Println("\nRebuilding Docker images...")
			if err := runDockerCompose(buildArgs, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			fileSvc, err := newFileSvc("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			if err := prepareDockerHostRuntime(cmd.Context(), fileSvc); err != nil {
				return err
			}
			resolved := resolveDockerProfile(full, profile)
			scope := "gateway"
			if resolved != "" {
				scope = fmt.Sprintf("full stack (profile %s)", resolved)
			}
			cmd.Printf("\nStarting Docker Compose %s...\n", scope)
			if err := runDockerCompose([]string{"up", "-d"}, resolved); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
			}
			cmd.Printf("\nDocker Compose %s rebuilt and started successfully.\n", scope)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noCache, "no-cache", true, "Rebuild without using the Docker cache")
	cmd.Flags().BoolVar(&full, "full", false, "Start the full stack (gateway + operator + ensemble + dashboard)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to start (e.g. bootstrapped)")
	return cmd
}

func dockerLogsCmd() *cobra.Command {
	var follow bool
	var profile string

	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show logs for the Docker Compose unified stack",
		Long:  `Show logs for the Docker Compose unified stack. Optionally pass a service name to filter, and --follow (-f) to stream.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			logArgs := []string{"logs"}
			if follow {
				logArgs = append(logArgs, "-f")
			}
			if len(args) == 1 {
				logArgs = append(logArgs, args[0])
			}
			if err := runDockerCompose(logArgs, profile); err != nil {
				return fmt.Errorf("%w: %w", constants.ErrInternal, err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile to target (e.g. bootstrapped)")
	return cmd
}

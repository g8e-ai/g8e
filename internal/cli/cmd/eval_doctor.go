// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// evalDoctorDeps carries the injected dependencies for the doctor command.
type evalDoctorDeps struct {
	configLoader   func(string) (*config.Config, error)
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)
	clientFactory  apiClientFactory
	stat           evalFileStat
	runner         evalCommandRunner
	httpClient     *http.Client
}

// evalDoctorCheckStatus is the pass/fail/warn status for a single doctor
// check.
type evalDoctorCheckStatus string

const (
	evalDoctorCheckPass evalDoctorCheckStatus = "pass"
	evalDoctorCheckFail evalDoctorCheckStatus = "fail"
	evalDoctorCheckWarn evalDoctorCheckStatus = "warn"
)

// evalDoctorCheck is a single typed diagnostic result. SafeDetail and
// CorrectiveAction are human-readable and contain no secrets.
type evalDoctorCheck struct {
	ID               string                `json:"id"`
	Status           evalDoctorCheckStatus `json:"status"`
	SafeDetail       string                `json:"safe_detail,omitempty"`
	CorrectiveAction string                `json:"corrective_action,omitempty"`
}

// evalDoctorResult is the typed result emitted by `eval doctor --json`.
type evalDoctorResult struct {
	Scope  string            `json:"scope"`
	Checks []evalDoctorCheck `json:"checks"`
}

// evalDoctorCmd returns the production `eval doctor` command with real
// dependencies.
func evalDoctorCmd() *cobra.Command {
	return evalDoctorCmdWithDeps(evalDoctorDeps{
		configLoader:   config.Load,
		fileSvcFactory: newFileSvc,
		clientFactory:  defaultAPIClientFactory,
		stat:           realEvalFileStat{},
		runner:         realEvalCommandRunner{},
		httpClient:     &http.Client{Timeout: 5 * time.Second},
	})
}

// evalDoctorCmdWithDeps returns the `eval doctor` command wired with the
// supplied dependencies for testability.
func evalDoctorCmdWithDeps(deps evalDoctorDeps) *cobra.Command {
	var scopeLocal, scopeStack, scopeProvider bool
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Read-only environment diagnostics (read-only)",
		Long: `doctor is a read-only diagnostic command. It checks repository
identity, eval project markers, interpreter, engine version, protocol
compatibility, trust bundle, and provider configuration presence.

Scopes:
  --local     No network; checks local files and environment only (default)
  --stack     Adds Gateway/Operator/Ensemble health checks (no inference)
  --provider  Adds provider endpoint/version/inventory checks (no generation)

No check sends a model generation request. Provider configuration is
reported as presence/absence only; values are never printed.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := commandContext(cmd)
			projectRootOverride, _ := cmd.Flags().GetString("project-root")

			scope := "local"
			if scopeStack {
				scope = "stack"
			} else if scopeProvider {
				scope = "provider"
			}

			result, err := runEvalDoctor(ctx, deps, projectRootOverride, scope, cmd.OutOrStdout(), cmd.OutOrStderr())
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalDoctorHuman(cmd.OutOrStdout(), result)
			// Return nonzero if any required check failed.
			for _, check := range result.Checks {
				if check.Status == evalDoctorCheckFail {
					return fmt.Errorf("%w: check %s failed", constants.ErrEvalPlatformUnhealthy, check.ID)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&scopeLocal, "local", false, "Local-only checks (no network); default when no scope is set")
	cmd.Flags().BoolVar(&scopeStack, "stack", false, "Add Gateway/Operator/Ensemble health checks")
	cmd.Flags().BoolVar(&scopeProvider, "provider", false, "Add provider endpoint/version/inventory checks")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

// runEvalDoctor performs the scoped read-only diagnostics.
func runEvalDoctor(ctx context.Context, deps evalDoctorDeps, projectRootOverride, scope string, stdout, stderr io.Writer) (evalDoctorResult, error) {
	projectRoot, err := evalProjectRootFromConfig(deps.configLoader, projectRootOverride)
	if err != nil {
		return evalDoctorResult{}, err
	}

	cfg, err := deps.configLoader(projectRootOverride)
	if err != nil {
		return evalDoctorResult{}, fmt.Errorf("eval: load config: %w", err)
	}

	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return evalDoctorResult{}, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}

	checks := runEvalDoctorLocalChecks(ctx, deps, fileSvc, projectRoot)

	if scope == "stack" || scope == "provider" {
		checks = append(checks, runEvalDoctorStackChecks(ctx, deps, fileSvc, cfg)...)
	}
	if scope == "provider" {
		checks = append(checks, runEvalDoctorProviderChecks(ctx, deps)...)
	}

	return evalDoctorResult{Scope: scope, Checks: checks}, nil
}

// runEvalDoctorLocalChecks performs the no-network local checks.
func runEvalDoctorLocalChecks(ctx context.Context, deps evalDoctorDeps, fileSvc fs.RuntimeFileService, projectRoot string) []evalDoctorCheck {
	var checks []evalDoctorCheck

	checks = append(checks, checkEvalRepositoryIdentity(deps.stat, projectRoot))
	checks = append(checks, checkEvalProjectMarkers(deps.stat, projectRoot))
	checks = append(checks, checkEvalInterpreter(deps.stat, projectRoot))
	checks = append(checks, checkEvalEngineVersion(ctx, deps, projectRoot))
	checks = append(checks, checkEvalProtocolCompatibility())
	checks = append(checks, checkEvalTrustBundle(ctx, fileSvc))
	checks = append(checks, checkEvalProviderConfigPresence())

	return checks
}

// runEvalDoctorStackChecks performs Gateway/Operator/Ensemble health
// checks without provider inference. Gateway and Ensemble expose
// unauthenticated health endpoints on their published HTTP ports. The
// Operator is outbound-only with no published inbound port, so its health
// is determined by querying the Gateway's operator session registry over
// the authenticated mTLS API.
func runEvalDoctorStackChecks(ctx context.Context, deps evalDoctorDeps, fileSvc fs.RuntimeFileService, cfg *config.Config) []evalDoctorCheck {
	var checks []evalDoctorCheck
	checks = append(checks, checkEvalComponentHealth(ctx, deps.httpClient, "gateway", evalGatewayHealthURL(cfg)))
	checks = append(checks, checkEvalOperatorSession(deps, fileSvc, cfg))
	checks = append(checks, checkEvalComponentHealth(ctx, deps.httpClient, "ensemble", evalEnsembleHealthURL(cfg)))
	return checks
}

// runEvalDoctorProviderChecks performs provider endpoint checks. No
// generation request is sent.
func runEvalDoctorProviderChecks(ctx context.Context, deps evalDoctorDeps) []evalDoctorCheck {
	var checks []evalDoctorCheck
	endpoint := os.Getenv("OLLAMA_HOST")
	if endpoint == "" {
		endpoint = os.Getenv("OPENAI_BASE_URL")
	}
	if endpoint == "" {
		checks = append(checks, evalDoctorCheck{
			ID:         "provider_endpoint",
			Status:     evalDoctorCheckWarn,
			SafeDetail: "No provider endpoint configured (OLLAMA_HOST or OPENAI_BASE_URL)",
		})
		return checks
	}
	checks = append(checks, checkEvalComponentHealth(ctx, deps.httpClient, "provider", endpoint))
	return checks
}

// checkEvalRepositoryIdentity validates the root markers.
func checkEvalRepositoryIdentity(stat evalFileStat, projectRoot string) evalDoctorCheck {
	for _, marker := range []string{constants.EvalRootVersion, constants.EvalRootMakefile} {
		markerPath := projectRoot + string(os.PathSeparator) + marker
		if _, err := stat.Stat(markerPath); err != nil {
			return evalDoctorCheck{
				ID:               "repository_identity",
				Status:           evalDoctorCheckFail,
				SafeDetail:       fmt.Sprintf("Missing root marker: %s", marker),
				CorrectiveAction: "Run from the g8e repository root or supply --project-root",
			}
		}
	}
	return evalDoctorCheck{ID: "repository_identity", Status: evalDoctorCheckPass}
}

// checkEvalProjectMarkers validates the eval project markers.
func checkEvalProjectMarkers(stat evalFileStat, projectRoot string) evalDoctorCheck {
	evalProject := projectRoot + string(os.PathSeparator) + constants.EvalProjectDir
	for _, marker := range []string{constants.EvalProjectPyproject, constants.EvalProjectLockfile} {
		markerPath := evalProject + string(os.PathSeparator) + marker
		if _, err := stat.Stat(markerPath); err != nil {
			return evalDoctorCheck{
				ID:               "eval_project",
				Status:           evalDoctorCheckFail,
				SafeDetail:       fmt.Sprintf("Missing eval project marker: %s", marker),
				CorrectiveAction: "Ensure the g8e source checkout is complete",
			}
		}
	}
	return evalDoctorCheck{ID: "eval_project", Status: evalDoctorCheckPass}
}

// checkEvalInterpreter checks the project-local interpreter exists.
func checkEvalInterpreter(stat evalFileStat, projectRoot string) evalDoctorCheck {
	evalProject := projectRoot + string(os.PathSeparator) + constants.EvalProjectDir
	interpreterPath := projectInterpreterPath(evalProject)
	if _, err := stat.Stat(interpreterPath); err != nil {
		return evalDoctorCheck{
			ID:               "interpreter",
			Status:           evalDoctorCheckFail,
			SafeDetail:       "Project-local interpreter not found",
			CorrectiveAction: "Run: ./g8e eval setup",
		}
	}
	return evalDoctorCheck{ID: "interpreter", Status: evalDoctorCheckPass}
}

// checkEvalEngineVersion runs the self-check to verify the engine
// imports and reports the version. If the interpreter is missing, this
// check is skipped (the interpreter check already reports the failure).
func checkEvalEngineVersion(ctx context.Context, deps evalDoctorDeps, projectRoot string) evalDoctorCheck {
	evalProject := projectRoot + string(os.PathSeparator) + constants.EvalProjectDir
	interpreterPath := projectInterpreterPath(evalProject)
	if _, err := deps.stat.Stat(interpreterPath); err != nil {
		return evalDoctorCheck{ID: "engine_version", Status: evalDoctorCheckWarn, SafeDetail: "Skipped: interpreter not set up"}
	}
	var out strings.Builder
	if err := deps.runner.Run(ctx, interpreterPath, []string{"-c", constants.EvalSelfCheckModules}, &out, io.Discard); err != nil {
		return evalDoctorCheck{
			ID:               "engine_version",
			Status:           evalDoctorCheckFail,
			SafeDetail:       "Engine self-check failed (import error)",
			CorrectiveAction: "Run: ./g8e eval setup",
		}
	}
	version := strings.TrimSpace(out.String())
	return evalDoctorCheck{ID: "engine_version", Status: evalDoctorCheckPass, SafeDetail: version}
}

// checkEvalProtocolCompatibility verifies the Go-side schema version
// constant is defined. The Python-side parity check is added in U4 with
// a contract test.
func checkEvalProtocolCompatibility() evalDoctorCheck {
	if models.EvalEngineRequestSchemaVersion == "" {
		return evalDoctorCheck{ID: "protocol_compatibility", Status: evalDoctorCheckFail, SafeDetail: "Go schema version constant is empty"}
	}
	return evalDoctorCheck{
		ID:         "protocol_compatibility",
		Status:     evalDoctorCheckPass,
		SafeDetail: fmt.Sprintf("Go schema version: %s", models.EvalEngineRequestSchemaVersion),
	}
}

// checkEvalTrustBundle checks the canonical trust bundle exists in the
// .g8e/ runtime tree.
func checkEvalTrustBundle(ctx context.Context, fileSvc fs.RuntimeFileService) evalDoctorCheck {
	relPath := constants.PkiDirname + "/" + constants.PkiSubdirTrust + "/" + constants.PkiFileGatewayBundle
	exists, err := fileSvc.FileExists(ctx, relPath)
	if err != nil {
		return evalDoctorCheck{ID: "trust_bundle", Status: evalDoctorCheckWarn, SafeDetail: fmt.Sprintf("Unable to check trust bundle: %v", err)}
	}
	if !exists {
		return evalDoctorCheck{
			ID:               "trust_bundle",
			Status:           evalDoctorCheckWarn,
			SafeDetail:       "Trust bundle not found",
			CorrectiveAction: "Run: ./g8e gw start (or initialize PKI)",
		}
	}
	return evalDoctorCheck{ID: "trust_bundle", Status: evalDoctorCheckPass}
}

// checkEvalProviderConfigPresence checks for provider configuration in
// the environment. Only presence is reported; values are never printed.
func checkEvalProviderConfigPresence() evalDoctorCheck {
	keys := []string{"OLLAMA_HOST", "OPENAI_API_KEY", "OPENAI_BASE_URL", "ANTHROPIC_API_KEY"}
	var present []string
	for _, key := range keys {
		if os.Getenv(key) != "" {
			present = append(present, key)
		}
	}
	if len(present) == 0 {
		return evalDoctorCheck{
			ID:         "provider_config",
			Status:     evalDoctorCheckWarn,
			SafeDetail: "No provider configuration detected in environment",
		}
	}
	return evalDoctorCheck{
		ID:         "provider_config",
		Status:     evalDoctorCheckPass,
		SafeDetail: fmt.Sprintf("Provider configuration present: %s", strings.Join(present, ", ")),
	}
}

// checkEvalComponentHealth performs a simple HTTP GET to a component
// health endpoint. It does not send inference or generation requests.
func checkEvalComponentHealth(ctx context.Context, client *http.Client, name, healthURL string) evalDoctorCheck {
	if healthURL == "" {
		return evalDoctorCheck{ID: name + "_health", Status: evalDoctorCheckWarn, SafeDetail: "No endpoint configured"}
	}
	// The health URL is constructed from config-owned host values, not
	// user-supplied input. The doctor scope is read-only and sends only
	// GET /health, never inference or generation requests.
	//nolint:gosec // G704: health URL is config-derived, not user input
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return evalDoctorCheck{ID: name + "_health", Status: evalDoctorCheckFail, SafeDetail: fmt.Sprintf("Invalid endpoint: %v", err)}
	}
	//nolint:gosec // G704: health URL is config-derived, not user input
	resp, err := client.Do(req)
	if err != nil {
		return evalDoctorCheck{ID: name + "_health", Status: evalDoctorCheckFail, SafeDetail: fmt.Sprintf("Unreachable: %v", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return evalDoctorCheck{ID: name + "_health", Status: evalDoctorCheckFail, SafeDetail: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	return evalDoctorCheck{ID: name + "_health", Status: evalDoctorCheckPass}
}

// checkEvalOperatorSession determines Operator health through the
// Gateway's operator session registry. The Operator is outbound-only and
// publishes no inbound port, so there is no Operator health endpoint to
// GET from the host; an Operator is healthy when the Gateway reports at
// least one live (active or bound) session. Requires CLI credentials;
// without them the check warns rather than fails because enrollment is
// not a precondition for local eval operations.
func checkEvalOperatorSession(deps evalDoctorDeps, fileSvc fs.RuntimeFileService, cfg *config.Config) evalDoctorCheck {
	if deps.clientFactory == nil {
		return evalDoctorCheck{ID: "operator_health", Status: evalDoctorCheckWarn, SafeDetail: "Operator session check not configured"}
	}
	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil {
		return evalDoctorCheck{ID: "operator_health", Status: evalDoctorCheckFail, SafeDetail: "Unable to load CLI credentials"}
	}
	if creds == nil {
		return evalDoctorCheck{
			ID:               "operator_health",
			Status:           evalDoctorCheckWarn,
			SafeDetail:       "CLI not authenticated; operator session status unknown",
			CorrectiveAction: "Run: ./g8e auth enroll user",
		}
	}
	client, err := deps.clientFactory(fileSvc, cfg)
	if err != nil {
		return evalDoctorCheck{ID: "operator_health", Status: evalDoctorCheckFail, SafeDetail: "Unable to build platform API client"}
	}
	resp, err := client.Get(constants.APIPaths.Operators + "?user_id=" + creds.UserID)
	if err != nil {
		return evalDoctorCheck{ID: "operator_health", Status: evalDoctorCheckFail, SafeDetail: "Operator session status unreachable"}
	}
	var slotResp models.OperatorSlotResponse
	if err := json.Unmarshal(resp, &slotResp); err != nil {
		return evalDoctorCheck{ID: "operator_health", Status: evalDoctorCheckFail, SafeDetail: "Invalid operator session response"}
	}
	active := 0
	for _, op := range slotResp.Operators {
		if op.Status == constants.OperatorStatusActive || op.Status == constants.OperatorStatusBound {
			active++
		}
	}
	if active == 0 {
		return evalDoctorCheck{ID: "operator_health", Status: evalDoctorCheckFail, SafeDetail: "No active operator session"}
	}
	return evalDoctorCheck{ID: "operator_health", Status: evalDoctorCheckPass, SafeDetail: fmt.Sprintf("%d operator session(s) active", active)}
}

// evalGatewayHealthURL returns the Gateway health endpoint URL on the
// unauthenticated HTTP discovery surface.
func evalGatewayHealthURL(cfg *config.Config) string {
	return cfg.OperatorDiscoveryURL() + constants.APIPaths.Health
}

// evalEnsembleHealthURL returns the Ensemble health endpoint URL.
func evalEnsembleHealthURL(cfg *config.Config) string {
	return evalEnsembleBaseURL(cfg) + "/health"
}

// evalEnsembleBaseURL returns the Ensemble (g8ee) HTTP base URL derived
// from the configured host and the canonical Ensemble port. It is the
// platform-owned value the facade injects into EvalPlatformContext so
// configs never carry the endpoint and the engine never receives an
// empty ensemble URL.
func evalEnsembleBaseURL(cfg *config.Config) string {
	host := "localhost"
	if cfg.Paths != nil && cfg.Paths.Host != "" {
		host = cfg.Paths.Host
	}
	return fmt.Sprintf("http://%s:%d", host, constants.EnsembleDefaultPort)
}

// printEvalDoctorHuman prints a concise human-readable doctor summary.
func printEvalDoctorHuman(w io.Writer, result evalDoctorResult) {
	fmt.Fprintf(w, "Eval doctor (scope: %s)\n", result.Scope)
	for _, check := range result.Checks {
		status := string(check.Status)
		detail := check.SafeDetail
		if detail != "" {
			fmt.Fprintf(w, "  [%s] %s: %s\n", status, check.ID, detail)
		} else {
			fmt.Fprintf(w, "  [%s] %s\n", status, check.ID)
		}
		if check.CorrectiveAction != "" {
			fmt.Fprintf(w, "    -> %s\n", check.CorrectiveAction)
		}
	}
}

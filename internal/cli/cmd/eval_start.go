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
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/buildinfo"
	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// evalLeaseStartVerificationRequest is the typed JSON request the Go
// facade sends to the Python lease start verification module. The module
// loads the config and lease, builds a LeaseVerificationContext from the
// actual runtime state supplied here, and calls verify_lease_for_start.
type evalLeaseStartVerificationRequest struct {
	ConfigPath           string                `json:"config_path"`
	LeaseStoreDir        string                `json:"lease_store_dir"`
	RepositoryRoot       string                `json:"repository_root"`
	Candidate            evalCandidateIdentity `json:"candidate"`
	ModelInventoryDigest string                `json:"model_inventory_digest"`
	CommandFamily        evalCommandFamily     `json:"command_family"`
	CommandVersion       string                `json:"command_version"`
}

// evalLeaseStartVerificationResult is the typed result emitted by the
// Python lease start verification module. On success it carries the
// operation_id, revision, report_root, lease_id, lease_path, and config
// content_hash so the Go facade can construct an EvalEngineRequest
// without a second round-trip. On failure it carries the stable
// LeaseVerificationFailureCode value and a safe detail string.
type evalLeaseStartVerificationResult struct {
	Verified      bool   `json:"verified"`
	FailureCode   string `json:"failure_code,omitempty"`
	FailureDetail string `json:"failure_detail,omitempty"`
	OperationID   string `json:"operation_id,omitempty"`
	Revision      string `json:"revision,omitempty"`
	ReportRoot    string `json:"report_root,omitempty"`
	LeaseID       string `json:"lease_id,omitempty"`
	LeasePath     string `json:"lease_path,omitempty"`
	ContentHash   string `json:"content_hash,omitempty"`
}

// mapLeaseVerificationFailureCode maps a stable LeaseVerificationFailureCode
// value (emitted by the Python verifier) to a typed Go sentinel error. This
// is the single adapter that translates Python verification codes to Go
// sentinels.
func mapLeaseVerificationFailureCode(code, detail string) error {
	switch code {
	case "lease_missing":
		return fmt.Errorf("%w: %s", constants.ErrEvalLeaseMissing, detail)
	case "lease_inactive", "lease_not_yet_valid", "lease_stopped":
		return fmt.Errorf("%w: %s", constants.ErrEvalLeaseInactive, detail)
	case "lease_expired":
		return fmt.Errorf("%w: %s", constants.ErrEvalLeaseExpired, detail)
	case "lease_consumed":
		return fmt.Errorf("%w: %s", constants.ErrEvalLeaseConsumed, detail)
	case "lease_operation_deadline_exceeds_lease":
		return fmt.Errorf("%w: %s", constants.ErrEvalBudgetPreflightFailed, detail)
	case "request_digest_mismatch", "command_family_mismatch",
		"command_version_mismatch", "operation_identity_mismatch",
		"budget_mismatch", "endpoint_mismatch":
		return fmt.Errorf("%w: %s", constants.ErrEvalLeaseMismatched, detail)
	case "candidate_source_tree_drift", "candidate_binary_drift",
		"candidate_image_drift":
		return fmt.Errorf("%w: %s", constants.ErrEvalCandidateDrift, detail)
	case "model_inventory_drift":
		return fmt.Errorf("%w: %s", constants.ErrEvalInventoryDrift, detail)
	case "report_root_reused":
		return fmt.Errorf("%w: %s", constants.ErrEvalReportRootReused, detail)
	case "authority_drift":
		return fmt.Errorf("%w: %s", constants.ErrEvalAuthorityInvalid, detail)
	default:
		return fmt.Errorf("%w: %s", constants.ErrEvalLeaseMismatched, detail)
	}
}

// evalStartResult is the typed result emitted by start commands.
type evalStartResult struct {
	Operation   string `json:"operation"`
	OperationID string `json:"operation_id"`
	Status      string `json:"status"`
	ErrorCode   string `json:"error_code,omitempty"`
	SafeDetail  string `json:"safe_detail,omitempty"`
	LeaseID     string `json:"lease_id,omitempty"`
	LeaseStatus string `json:"lease_status,omitempty"`
}

// runEvalStart is the shared provider-backed start path. It resolves the
// eval environment, verifies the lease, invokes the engine, and transitions
// the lease to a terminal state based on the engine result. Every
// provider-backed start (diagnostic, campaign, controller) routes through
// this function so lease verification cannot be bypassed.
func runEvalStart(
	ctx context.Context,
	deps evalLeaseDeps,
	configPath string,
	operation models.EvalOperation,
	commandFamily evalCommandFamily,
	jsonOutput, verbose bool,
	stdout, stderr io.Writer,
) (evalStartResult, error) {
	// runEvalPreflight is the single fail-closed gate. It resolves the
	// environment, verifies the lease, and runs the provider-free checks
	// exactly once. Start never re-runs lease verification or check, so
	// the gate cannot be duplicated or bypassed.
	preflight, err := runEvalPreflight(ctx, deps, configPath, commandFamily)
	if err != nil {
		return evalStartResult{}, err
	}
	env := preflight.Env
	cfg := preflight.Cfg
	candidate := preflight.Candidate
	verification := preflight.Verification

	// Load the canonical CLI auth identity for the engine request. Missing
	// credentials fail closed here — before engine launch — rather than
	// surfacing as an engine-child auth failure.
	authCtx, err := loadEvalAuthContext(deps, preflight.FileSvc, cfg)
	if err != nil {
		return evalStartResult{}, err
	}

	// Construct the typed engine request. The lease path is the verified
	// lease file; the report root is repository-relative from the config.
	engineReq := models.EvalEngineRequest{
		SchemaVersion: models.EvalEngineRequestSchemaVersion,
		Operation:     operation,
		OperationID:   verification.OperationID,
		Revision:      verification.Revision,
		ConfigPath:    env.ConfigPath,
		LeasePath:     verification.LeasePath,
		ReportRoot:    verification.ReportRoot,
		Platform:      buildEvalPlatformContext(ctx, env, cfg, candidate, authCtx),
		Flags: models.EvalEngineFlags{
			// The engine-to-facade protocol is always JSON so the facade
			// can parse the typed result. The user's --json flag controls
			// facade-to-user rendering, not the engine protocol.
			JSONOutput: true,
			Verbose:    verbose,
		},
	}

	engineResult, err := invokeEngine(ctx, deps, env.InterpreterPath, engineReq, stderr)

	// Transition the lease to a terminal state based on the engine result.
	// The transition is best-effort: a transition failure is logged to
	// stderr but does not override the engine result. Publication retry
	// does not reactivate the terminal lease.
	transition := evalLeaseTransitionStop
	leaseStatus := "stopped"
	if err == nil && engineResult.Status == string(models.EvalEngineStatusSucceeded) {
		transition = evalLeaseTransitionComplete
		leaseStatus = "completed"
	}
	transitionErr := transitionLeaseAfterRun(ctx, deps, env.InterpreterPath, env.ConfigPath, preflight.LeaseStoreDir, transition)
	if transitionErr != nil {
		fmt.Fprintf(stderr, "warning: lease transition failed: %v\n", transitionErr)
	}

	result := evalStartResult{
		Operation:   string(operation),
		OperationID: verification.OperationID,
		LeaseID:     verification.LeaseID,
		LeaseStatus: leaseStatus,
	}
	if err != nil {
		result.Status = string(models.EvalEngineStatusFailed)
		result.SafeDetail = err.Error()
		return result, err
	}
	result.Status = engineResult.Status
	result.ErrorCode = engineResult.ErrorCode
	result.SafeDetail = engineResult.SafeDetail
	return result, nil
}

// invokeLeaseStartVerification invokes the Python lease start verification
// module and returns the parsed verification result.
func invokeLeaseStartVerification(
	ctx context.Context,
	deps evalLeaseDeps,
	interpreterPath string,
	req evalLeaseStartVerificationRequest,
) (evalLeaseStartVerificationResult, error) {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return evalLeaseStartVerificationResult{}, fmt.Errorf("%w: marshal lease verification request: %w", constants.ErrEvalConfigInvalid, err)
	}

	tmpPath, err := deps.tempFileWriter.WriteTempFile("eval-lease-verify-*.json", reqJSON)
	if err != nil {
		return evalLeaseStartVerificationResult{}, err
	}
	defer os.Remove(tmpPath)

	var stdoutBuf strings.Builder
	args := []string{"-m", constants.EvalLeaseStartVerificationModule, tmpPath}
	if err := deps.runner.Run(ctx, interpreterPath, args, &stdoutBuf, os.Stderr); err != nil {
		return evalLeaseStartVerificationResult{}, fmt.Errorf("%w: %w", constants.ErrEvalLeaseMissing, err)
	}

	var result evalLeaseStartVerificationResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdoutBuf.String())), &result); err != nil {
		return evalLeaseStartVerificationResult{}, fmt.Errorf("%w: parse lease verification result: %w", constants.ErrEvalConfigInvalid, err)
	}
	return result, nil
}

// evalEngineResultJSON is the typed JSON result emitted by the Python
// engine. It mirrors models.EvalEngineResult for stdout parsing.
type evalEngineResultJSON struct {
	SchemaVersion string          `json:"schema_version"`
	Operation     string          `json:"operation"`
	OperationID   string          `json:"operation_id"`
	Status        string          `json:"status"`
	ErrorCode     string          `json:"error_code,omitempty"`
	ErrorStage    string          `json:"error_stage,omitempty"`
	SafeDetail    string          `json:"safe_detail,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

func mapEvalEngineErrorCode(code, detail string) error {
	var sentinel error
	switch models.EvalErrorCode(code) {
	case models.EvalErrorCodeEngineNotSetUp:
		sentinel = constants.ErrEvalEngineNotSetUp
	case models.EvalErrorCodeEngineProtocolMismatch:
		sentinel = constants.ErrEvalEngineProtocolMismatch
	case models.EvalErrorCodeConfigInvalid:
		sentinel = constants.ErrEvalConfigInvalid
	case models.EvalErrorCodeAuthorityInvalid:
		sentinel = constants.ErrEvalAuthorityInvalid
	case models.EvalErrorCodeLeaseMissing:
		sentinel = constants.ErrEvalLeaseMissing
	case models.EvalErrorCodeLeaseInactive:
		sentinel = constants.ErrEvalLeaseInactive
	case models.EvalErrorCodeLeaseExpired:
		sentinel = constants.ErrEvalLeaseExpired
	case models.EvalErrorCodeLeaseMismatched:
		sentinel = constants.ErrEvalLeaseMismatched
	case models.EvalErrorCodeLeaseConsumed:
		sentinel = constants.ErrEvalLeaseConsumed
	case models.EvalErrorCodeCandidateDrift:
		sentinel = constants.ErrEvalCandidateDrift
	case models.EvalErrorCodeInventoryDrift:
		sentinel = constants.ErrEvalInventoryDrift
	case models.EvalErrorCodeReportRootReused:
		sentinel = constants.ErrEvalReportRootReused
	case models.EvalErrorCodeEvidenceKeyInvalid:
		sentinel = constants.ErrEvalEvidenceKeyInvalid
	case models.EvalErrorCodePlatformIdentityUnavailable:
		sentinel = constants.ErrEvalPlatformIdentityUnavailable
	case models.EvalErrorCodePlatformUnhealthy:
		sentinel = constants.ErrEvalPlatformUnhealthy
	case models.EvalErrorCodeProviderUnreachable:
		sentinel = constants.ErrEvalProviderUnreachable
	case models.EvalErrorCodeBudgetPreflightFailed:
		sentinel = constants.ErrEvalBudgetPreflightFailed
	case models.EvalErrorCodeChildStartFailed:
		sentinel = constants.ErrEvalChildStartFailed
	case models.EvalErrorCodeChildExitNonZero:
		sentinel = constants.ErrEvalChildExitNonZero
	case models.EvalErrorCodeChildInterrupted:
		sentinel = constants.ErrEvalChildInterrupted
	case models.EvalErrorCodeStatusReconciliationFailed:
		sentinel = constants.ErrEvalStatusReconciliationFailed
	default:
		sentinel = constants.ErrEvalChildExitNonZero
	}
	if detail == "" {
		return sentinel
	}
	return fmt.Errorf("%w: %s", sentinel, detail)
}

// invokeEngine constructs an EvalEngineRequest JSON file, invokes the
// Python engine module, and returns the parsed result. The typed
// platform context carries the auth identity, trust bundle path, and
// build provenance the SUT needs so the child process authenticates to
// the running platform without any G8E_* environment injection.
func invokeEngine(
	ctx context.Context,
	deps evalLeaseDeps,
	interpreterPath string,
	req models.EvalEngineRequest,
	stderr io.Writer,
) (evalEngineResultJSON, error) {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return evalEngineResultJSON{}, fmt.Errorf("%w: marshal engine request: %w", constants.ErrEvalConfigInvalid, err)
	}

	tmpPath, err := deps.tempFileWriter.WriteTempFile("eval-engine-*.json", reqJSON)
	if err != nil {
		return evalEngineResultJSON{}, err
	}
	defer os.Remove(tmpPath)

	var stdoutBuf strings.Builder
	args := []string{"-m", constants.EvalEngineModule, tmpPath}
	runErr := deps.runner.Run(ctx, interpreterPath, args, &stdoutBuf, stderr)

	var result evalEngineResultJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdoutBuf.String())), &result); err != nil {
		if runErr != nil {
			return evalEngineResultJSON{}, fmt.Errorf("%w: %w", constants.ErrEvalChildExitNonZero, runErr)
		}
		return evalEngineResultJSON{}, fmt.Errorf("%w: parse engine result: %w", constants.ErrEvalConfigInvalid, err)
	}
	if runErr != nil {
		if result.Status == string(models.EvalEngineStatusFailed) {
			return result, mapEvalEngineErrorCode(result.ErrorCode, result.SafeDetail)
		}
		return result, fmt.Errorf("%w: %w", constants.ErrEvalChildExitNonZero, runErr)
	}
	return result, nil
}

// transitionLeaseAfterRun transitions the active lease for the config to a
// terminal state after the engine returns. The transition is "complete"
// on success or "stop" on failure/interruption.
func transitionLeaseAfterRun(
	ctx context.Context,
	deps evalLeaseDeps,
	interpreterPath, configPath, leaseStoreDir string,
	transition evalLeaseTransition,
) error {
	operation, err := leaseOperationForTransition(transition)
	if err != nil {
		return err
	}
	req := evalLeaseLifecycleRequest{
		Operation: operation,
		Transition: &evalLeaseTransitionRequest{
			ConfigPath:    configPath,
			LeaseStoreDir: leaseStoreDir,
			Transition:    transition,
		},
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal lease transition request: %w", err)
	}

	tmpPath, err := deps.tempFileWriter.WriteTempFile("eval-lease-transition-*.json", reqJSON)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	var stdoutBuf strings.Builder
	args := []string{"-m", constants.EvalLeaseLifecycleModule, tmpPath}
	if err := deps.runner.Run(ctx, interpreterPath, args, &stdoutBuf, os.Stderr); err != nil {
		return fmt.Errorf("lease transition: %w", err)
	}
	return nil
}

// printEvalStartHuman prints a concise human-readable start result.
func printEvalStartHuman(stdout io.Writer, result evalStartResult) {
	fmt.Fprintf(stdout, "%s: %s\n", result.Operation, result.Status)
	fmt.Fprintf(stdout, "  operation_id: %s\n", result.OperationID)
	fmt.Fprintf(stdout, "  lease_id:     %s\n", result.LeaseID)
	fmt.Fprintf(stdout, "  lease_status: %s\n", result.LeaseStatus)
	if result.ErrorCode != "" {
		fmt.Fprintf(stdout, "  error_code:   %s\n", result.ErrorCode)
	}
	if result.SafeDetail != "" {
		fmt.Fprintf(stdout, "  detail:       %s\n", result.SafeDetail)
	}
}

// loadEvalAuthContext loads the canonical local CLI auth context for an
// engine request. When the persisted credentials lack an operator
// binding, the binding is resolved from the gateway's authoritative CLI
// session record and persisted — the same recovery path `g8e auth
// context` performs. Missing credentials fail closed with a typed
// platform-identity error so they never surface as an engine-child auth
// failure.
func loadEvalAuthContext(deps evalLeaseDeps, fileSvc fs.RuntimeFileService, cfg *config.Config) (*auth.ClientAuthContext, error) {
	if deps.authContextLoader == nil {
		return nil, fmt.Errorf("%w: auth context loader unavailable", constants.ErrEvalPlatformIdentityUnavailable)
	}
	authCtx, err := deps.authContextLoader(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrEvalPlatformIdentityUnavailable, err)
	}
	if authCtx.OperatorSessionID == "" {
		if deps.clientFactory == nil {
			return nil, fmt.Errorf("%w: operator session binding missing; run './g8e auth refresh' or './g8e auth enroll user'", constants.ErrEvalPlatformIdentityUnavailable)
		}
		client, err := deps.clientFactory(fileSvc, cfg)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrEvalPlatformIdentityUnavailable, err)
		}
		if err := resolveClientOperatorContext(client, authCtx); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrEvalPlatformIdentityUnavailable, err)
		}
		if err := persistClientOperatorContext(fileSvc, cfg, authCtx); err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrEvalPlatformIdentityUnavailable, err)
		}
	}
	return authCtx, nil
}

// evalBuildStamp returns the running binary's build stamp — the same
// version info and VCS identity `g8e version --json` reads.
func evalBuildStamp(ctx context.Context) (serve.VersionInfo, string) {
	vi, _ := ctx.Value(versionInfoKey{}).(serve.VersionInfo)
	return vi, effectiveSourceRevision(vi, buildinfo.ReadVCSStamp())
}

// buildEvalPlatformContext constructs the platform-owned context the
// facade injects into every engine request. authCtx may be nil for
// read-only lifecycle operations that never authenticate to the
// platform; the auth fields are then empty and the engine handlers that
// require them fail closed on the missing identity.
func buildEvalPlatformContext(ctx context.Context, env evalLifecycleEnvironment, cfg *config.Config, candidate evalCandidateIdentity, authCtx *auth.ClientAuthContext) models.EvalPlatformContext {
	binaryPath, _ := os.Executable()
	vi, sourceRevision := evalBuildStamp(ctx)
	platform := models.EvalPlatformContext{
		RepositoryRoot:      env.RepositoryRoot,
		EvalProject:         env.EvalProject,
		G8EBinaryPath:       binaryPath,
		G8EBinarySHA256:     candidate.BinarySHA256,
		PlatformVersion:     vi.Version,
		AuthProjectRoot:     cfg.ProjectRoot,
		RuntimeDir:          cfg.RuntimeDir,
		TrustBundlePath:     cfg.ResolvedTrustBundlePath(),
		GatewayHTTPURL:      cfg.OperatorDiscoveryURL(),
		GatewayHTTPSURL:     cfg.OperatorPublicURL(),
		EnsembleURL:         evalEnsembleBaseURL(cfg),
		SourceRevision:      sourceRevision,
		SourceTreeStateHash: candidate.SourceTreeHash,
	}
	if authCtx != nil {
		platform.CLICertPath = authCtx.ClientCert
		platform.CLIKeyPath = authCtx.ClientKey
		platform.OperatorSessionID = authCtx.OperatorSessionID
		platform.CLISessionID = authCtx.CLISessionID
		platform.UserID = authCtx.UserID
		platform.OperatorID = authCtx.OperatorID
	}
	return platform
}

// evalStartDepsFromLeaseDeps returns the evalLeaseDeps needed for start
// commands. Start commands reuse the lease deps structure since they need
// the same config loader, file service factory, stat, runner, temp file
// writer, candidate resolver, and model inventory resolver.
func evalStartDepsFromLeaseDeps() evalLeaseDeps {
	return evalLeaseDeps{
		configLoader:           config.Load,
		fileSvcFactory:         newFileSvc,
		clientFactory:          defaultAPIClientFactory,
		stat:                   realEvalFileStat{},
		runner:                 realEvalCommandRunner{},
		tempFileWriter:         realEvalTempFileWriter{},
		candidateResolver:      realEvalCandidateResolver{fileReader: realEvalFileReader{}},
		modelInventoryResolver: realEvalModelInventoryResolver{fileReader: realEvalFileReader{}},
		httpClient:             &http.Client{Timeout: 5 * time.Second},
		authContextLoader:      auth.LoadClientAuthContext,
	}
}

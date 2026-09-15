// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// evalCandidateIdentity is the Go mirror of the Python CandidateIdentity
// model. The Go facade computes these values from the actual runtime
// state and passes them to the Python lease lifecycle module.
type evalCandidateIdentity struct {
	SourceTreeHash              string   `json:"source_tree_hash"`
	ExecutionSourceManifestHash string   `json:"execution_source_manifest_hash"`
	BinarySHA256                string   `json:"binary_sha256"`
	ImageIDs                    []string `json:"image_ids"`
}

// evalModelInventoryResolver abstracts model inventory digest computation
// so Tier 1 tests do not touch the provider. The digest is computed from
// the operation config file's SHA-256, which binds to the exact models,
// arms, cohorts, and provider endpoint declared in the config. The same
// config file always produces the same digest; a changed config file
// produces a different digest, providing actual drift protection.
type evalModelInventoryResolver interface {
	Digest(ctx context.Context, configPath string) (string, error)
}

// realEvalModelInventoryResolver computes the model inventory digest from
// the operation config file's SHA-256. This is an actual stable digest
// that binds to the configured model inventory: the same config file
// always produces the same digest, and any change to the config (which
// may change the declared models, arms, cohorts, or provider endpoint)
// produces a different digest. This replaces the prior fixed placeholder
// string that provided no drift protection.
type realEvalModelInventoryResolver struct {
	fileReader evalFileReader
}

func (r realEvalModelInventoryResolver) Digest(ctx context.Context, configPath string) (string, error) {
	bytes, err := r.fileReader.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("eval: read config for inventory digest: %w", err)
	}
	h := sha256.Sum256(bytes)
	return hex.EncodeToString(h[:]), nil
}

// evalCandidateResolver abstracts candidate identity computation so Tier 1
// tests do not touch the filesystem or hash binaries.
type evalCandidateResolver interface {
	Resolve(ctx context.Context, repositoryRoot string) (evalCandidateIdentity, error)
}

// realEvalCandidateResolver computes the candidate identity from the actual
// runtime state: the g8e binary SHA-256, the lockfile SHA-256 as the source
// tree hash, and an empty image list for source-checkout execution.
type realEvalCandidateResolver struct {
	fileReader evalFileReader
}

func (r realEvalCandidateResolver) Resolve(ctx context.Context, repositoryRoot string) (evalCandidateIdentity, error) {
	binaryPath, err := os.Executable()
	if err != nil {
		return evalCandidateIdentity{}, fmt.Errorf("eval: resolve g8e binary: %w", err)
	}
	binaryBytes, err := r.fileReader.ReadFile(binaryPath)
	if err != nil {
		return evalCandidateIdentity{}, fmt.Errorf("eval: read g8e binary: %w", err)
	}
	binaryHash := sha256.Sum256(binaryBytes)

	lockfilePath := repositoryRoot + "/ensemble/evals/uv.lock"
	lockfileBytes, err := r.fileReader.ReadFile(lockfilePath)
	if err != nil {
		return evalCandidateIdentity{}, fmt.Errorf("eval: read lockfile: %w", err)
	}
	lockfileHash := sha256.Sum256(lockfileBytes)

	return evalCandidateIdentity{
		SourceTreeHash:              hex.EncodeToString(lockfileHash[:]),
		ExecutionSourceManifestHash: hex.EncodeToString(lockfileHash[:]),
		BinarySHA256:                hex.EncodeToString(binaryHash[:]),
		ImageIDs:                    []string{},
	}, nil
}

// evalLeaseOperation identifies the lease lifecycle operation the Python
// module dispatches on. It mirrors the operation literal in the Python
// LeaseLifecycleCLIRequest envelope.
type evalLeaseOperation string

const (
	evalLeaseOperationIssue    evalLeaseOperation = "issue"
	evalLeaseOperationInspect  evalLeaseOperation = "inspect"
	evalLeaseOperationStop     evalLeaseOperation = "stop"
	evalLeaseOperationExpire   evalLeaseOperation = "expire"
	evalLeaseOperationComplete evalLeaseOperation = "complete"
)

// evalLeaseTransition is the terminal transition applied to the active
// lease bound to a config. It mirrors the transition literal in the
// Python LeaseTransitionRequest model.
type evalLeaseTransition string

const (
	evalLeaseTransitionStop     evalLeaseTransition = "stop"
	evalLeaseTransitionExpire   evalLeaseTransition = "expire"
	evalLeaseTransitionComplete evalLeaseTransition = "complete"
)

// evalCommandFamily is the command family a lease authorizes. It mirrors
// the Python LeaseCommandFamily enum.
type evalCommandFamily string

const (
	evalCommandFamilyCampaignRun   evalCommandFamily = "campaign_run"
	evalCommandFamilyControllerRun evalCommandFamily = "controller_run"
)

// evalLeaseOperationKind is the Live Operations operation kind a lease
// authorizes. It mirrors the Python OperationKind enum subset the facade
// currently issues.
type evalLeaseOperationKind string

const (
	evalLeaseOperationKindEmbeddedAuthorityDiagnostic evalLeaseOperationKind = "embedded_authority_diagnostic"
)

// evalLeaseDeps carries the injected dependencies for lease commands.
type evalLeaseDeps struct {
	configLoader           func(string) (*config.Config, error)
	fileSvcFactory         func(string, *slog.Logger) (fs.RuntimeFileService, error)
	clientFactory          apiClientFactory
	stat                   evalFileStat
	runner                 evalCommandRunner
	tempFileWriter         evalTempFileWriter
	candidateResolver      evalCandidateResolver
	modelInventoryResolver evalModelInventoryResolver
	httpClient             *http.Client
}

// evalLeaseCmd returns the production `eval lease` command tree.
func evalLeaseCmd() *cobra.Command {
	return evalLeaseCmdWithDeps(evalLeaseDeps{
		configLoader:           config.Load,
		fileSvcFactory:         newFileSvc,
		clientFactory:          defaultAPIClientFactory,
		stat:                   realEvalFileStat{},
		runner:                 realEvalCommandRunner{},
		tempFileWriter:         realEvalTempFileWriter{},
		candidateResolver:      realEvalCandidateResolver{fileReader: realEvalFileReader{}},
		modelInventoryResolver: realEvalModelInventoryResolver{fileReader: realEvalFileReader{}},
		httpClient:             &http.Client{Timeout: 5 * time.Second},
	})
}

// evalLeaseCmdWithDeps returns the `eval lease` command tree wired with
// the supplied dependencies for testability.
func evalLeaseCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lease",
		Short: "Live Operations lease lifecycle (issue, inspect, stop, expire)",
		Long: `lease owns the Live Operations authority lifecycle. A lease
authorizes one provider-backed run against one exact typed request
digest. Only one active lease is permitted at a time.

Every provider-backed diagnostic, campaign, or controller start
requires a valid active lease bound to the exact operation config.
Issue a lease before start; stop or expire it after the run completes,
stops, or safety-fails.`,
	}
	cmd.AddCommand(
		evalLeaseIssueCmdWithDeps(deps),
		evalLeaseInspectCmdWithDeps(deps),
		evalLeaseStopCmdWithDeps(deps),
		evalLeaseExpireCmdWithDeps(deps),
	)
	return cmd
}

// evalLeaseIssueRequest is the typed payload nested under the "issue"
// key of the lease lifecycle request envelope.
type evalLeaseIssueRequest struct {
	ConfigPath                    string                 `json:"config_path"`
	LeaseStoreDir                 string                 `json:"lease_store_dir"`
	CommandFamily                 evalCommandFamily      `json:"command_family"`
	CommandVersion                string                 `json:"command_version"`
	Candidate                     evalCandidateIdentity  `json:"candidate"`
	ModelInventoryDigest          string                 `json:"model_inventory_digest"`
	Endpoint                      string                 `json:"endpoint"`
	AppIdentity                   string                 `json:"app_identity"`
	OperatorSessionIdentity       string                 `json:"operator_session_identity"`
	ExpiresInSeconds              int                    `json:"expires_in_seconds"`
	StartDeadlineSeconds          int                    `json:"start_deadline_seconds"`
	OperationKind                 evalLeaseOperationKind `json:"operation_kind"`
	RequiredRuntimeAuthorityNames []string               `json:"required_runtime_authority_names"`
}

// evalLeaseInspectRequest is the typed payload nested under the
// "inspect" key of the lease lifecycle request envelope.
type evalLeaseInspectRequest struct {
	ConfigPath    string `json:"config_path"`
	LeaseStoreDir string `json:"lease_store_dir"`
}

// evalLeaseTransitionRequest is the typed payload nested under the
// "transition" key of the lease lifecycle request envelope.
type evalLeaseTransitionRequest struct {
	ConfigPath    string              `json:"config_path"`
	LeaseStoreDir string              `json:"lease_store_dir"`
	Transition    evalLeaseTransition `json:"transition"`
}

// evalLeaseLifecycleRequest is the typed request envelope the Go facade
// sends to the Python lease lifecycle module. Exactly one of Issue,
// Inspect, or Transition is populated per Operation; the Python
// LeaseLifecycleCLIRequest model rejects any other combination.
type evalLeaseLifecycleRequest struct {
	Operation  evalLeaseOperation          `json:"operation"`
	Issue      *evalLeaseIssueRequest      `json:"issue,omitempty"`
	Inspect    *evalLeaseInspectRequest    `json:"inspect,omitempty"`
	Transition *evalLeaseTransitionRequest `json:"transition,omitempty"`
}

// evalLeaseIssueResult is the typed result emitted by lease issue.
type evalLeaseIssueResult struct {
	LeaseID                    string `json:"lease_id"`
	LeasePath                  string `json:"lease_path"`
	Status                     string `json:"status"`
	IssuedAt                   string `json:"issued_at"`
	StartDeadline              string `json:"start_deadline"`
	ExpiresAt                  string `json:"expires_at"`
	ContentHash                string `json:"content_hash"`
	RequestDigest              string `json:"request_digest"`
	OperationConfigContentHash string `json:"operation_config_content_hash"`
}

func evalLeaseIssueCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var expiresIn string
	var endpoint, appIdentity, operatorSessionIdentity string
	var jsonOutput, yes bool

	cmd := &cobra.Command{
		Use:   "issue <config>",
		Short: "Issue a new active Live Operations lease (local mutation)",
		Long: `issue creates a new active lease bound to the exact typed
request digest of the supplied operation config. Only one active lease
is permitted at a time; stop or expire the existing lease first.

This command is a local mutation. It requires --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)
			if !yes {
				return fmt.Errorf("%w: --yes is required for lease issue", constants.ErrEvalLeaseMissing)
			}
			duration, err := time.ParseDuration(expiresIn)
			if err != nil {
				return fmt.Errorf("%w: parse --expires-in: %w", constants.ErrEvalConfigInvalid, err)
			}
			if duration <= 0 {
				return fmt.Errorf("%w: --expires-in must be positive", constants.ErrEvalConfigInvalid)
			}
			result, err := runEvalLeaseIssue(ctx, deps, args[0], endpoint, appIdentity, operatorSessionIdentity, int(duration.Seconds()))
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalLeaseIssueHuman(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&expiresIn, "expires-in", "30m", "Lease validity duration (e.g. 30m, 1h)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "Provider endpoint URL (required)")
	cmd.Flags().StringVar(&appIdentity, "app-identity", "spiffe://g8e.local/app/g8ee", "App identity for the lease")
	cmd.Flags().StringVar(&operatorSessionIdentity, "operator-session-identity", "spiffe://g8e.local/operator/org/operator/session", "Operator session identity for the lease")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the local mutation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	_ = cmd.MarkFlagRequired("endpoint")
	return cmd
}

// evalLeaseInspectLease is the typed subset of the stored
// LiveOperationLease document rendered by `eval lease inspect`.
type evalLeaseInspectLease struct {
	LeaseID           string `json:"lease_id"`
	Status            string `json:"status"`
	OperationIdentity string `json:"operation_identity"`
	ReportRoot        string `json:"report_root"`
	IssuedAt          string `json:"issued_at"`
	ExpiresAt         string `json:"expires_at"`
	ContentHash       string `json:"content_hash"`
}

// evalLeaseInspectResult is the typed result emitted by lease inspect.
type evalLeaseInspectResult struct {
	Found bool                   `json:"found"`
	Lease *evalLeaseInspectLease `json:"lease,omitempty"`
}

func evalLeaseInspectCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "inspect <config>",
		Short: "Inspect the lease for a config (read-only)",
		Long: `inspect displays the lease matching the supplied operation
config, if any. Read-only; no mutations.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)
			result, err := runEvalLeaseInspect(ctx, deps, args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalLeaseInspectHuman(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

// evalLeaseTransitionResult is the typed result emitted by lease stop/expire.
type evalLeaseTransitionResult struct {
	LeaseID        string `json:"lease_id"`
	PreviousStatus string `json:"previous_status"`
	NewStatus      string `json:"new_status"`
	TransitionedAt string `json:"transitioned_at"`
}

func evalLeaseStopCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var jsonOutput, yes bool

	cmd := &cobra.Command{
		Use:   "stop <config>",
		Short: "Stop the active lease for a config (local mutation)",
		Long: `stop transitions the active lease matching the supplied
operation config to STOPPED status. Terminal; the lease cannot be
reactivated. Publication retry does not reactivate a stopped lease.

This command is a local mutation. It requires --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)
			if !yes {
				return fmt.Errorf("%w: --yes is required for lease stop", constants.ErrEvalLeaseMissing)
			}
			result, err := runEvalLeaseTransition(ctx, deps, args[0], evalLeaseTransitionStop)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalLeaseTransitionHuman(cmd.OutOrStdout(), "Stopped", result)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the local mutation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

func evalLeaseExpireCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var jsonOutput, yes bool

	cmd := &cobra.Command{
		Use:   "expire <config>",
		Short: "Expire the active lease for a config (local mutation)",
		Long: `expire transitions the active lease matching the supplied
operation config to EXPIRED status. Terminal; the lease cannot be
reactivated.

This command is a local mutation. It requires --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := commandContext(cmd)
			if !yes {
				return fmt.Errorf("%w: --yes is required for lease expire", constants.ErrEvalLeaseMissing)
			}
			result, err := runEvalLeaseTransition(ctx, deps, args[0], evalLeaseTransitionExpire)
			if err != nil {
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalLeaseTransitionHuman(cmd.OutOrStdout(), "Expired", result)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the local mutation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

// runEvalLeaseIssue resolves the environment, computes the candidate
// identity, and invokes the Python lease lifecycle module to issue a
// new active lease.
func runEvalLeaseIssue(ctx context.Context, deps evalLeaseDeps, configPath, endpoint, appIdentity, operatorSessionIdentity string, expiresInSeconds int) (evalLeaseIssueResult, error) {
	projectRoot, err := evalProjectRootFromConfig(deps.configLoader, "")
	if err != nil {
		return evalLeaseIssueResult{}, err
	}
	roots, err := resolveEvalRoots(ctx, projectRoot, deps.stat)
	if err != nil {
		return evalLeaseIssueResult{}, err
	}
	resolver := &evalEnvironmentResolver{stat: deps.stat}
	env, err := resolver.Resolve(ctx, evalProjectRootSpec(roots))
	if err != nil {
		return evalLeaseIssueResult{}, err
	}

	cfg, err := deps.configLoader("")
	if err != nil {
		return evalLeaseIssueResult{}, fmt.Errorf("eval: load config: %w", err)
	}
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return evalLeaseIssueResult{}, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	leaseStoreDir := fileSvc.Resolve(constants.EvalLeaseDirname)

	candidate, err := deps.candidateResolver.Resolve(ctx, roots.RepositoryRoot)
	if err != nil {
		return evalLeaseIssueResult{}, err
	}

	absConfigPath, err := resolveAbsPath(configPath)
	if err != nil {
		return evalLeaseIssueResult{}, err
	}

	modelInventoryDigest, err := deps.modelInventoryResolver.Digest(ctx, absConfigPath)
	if err != nil {
		return evalLeaseIssueResult{}, err
	}

	req := evalLeaseLifecycleRequest{
		Operation: evalLeaseOperationIssue,
		Issue: &evalLeaseIssueRequest{
			ConfigPath:                    absConfigPath,
			LeaseStoreDir:                 leaseStoreDir,
			CommandFamily:                 evalCommandFamilyCampaignRun,
			CommandVersion:                models.EvalEngineRequestSchemaVersion,
			Candidate:                     candidate,
			ModelInventoryDigest:          modelInventoryDigest,
			Endpoint:                      endpoint,
			AppIdentity:                   appIdentity,
			OperatorSessionIdentity:       operatorSessionIdentity,
			ExpiresInSeconds:              expiresInSeconds,
			StartDeadlineSeconds:          300,
			OperationKind:                 evalLeaseOperationKindEmbeddedAuthorityDiagnostic,
			RequiredRuntimeAuthorityNames: []string{"gold_set", "evidence_key"},
		},
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return evalLeaseIssueResult{}, fmt.Errorf("%w: marshal lease request: %w", constants.ErrEvalConfigInvalid, err)
	}

	result, err := invokeLeaseLifecycle(ctx, deps, env.InterpreterPath, reqJSON)
	if err != nil {
		return evalLeaseIssueResult{}, err
	}
	return result, nil
}

// runEvalLeaseInspect invokes the Python lease lifecycle module to
// inspect the lease for a config.
func runEvalLeaseInspect(ctx context.Context, deps evalLeaseDeps, configPath string) (evalLeaseInspectResult, error) {
	projectRoot, err := evalProjectRootFromConfig(deps.configLoader, "")
	if err != nil {
		return evalLeaseInspectResult{}, err
	}
	roots, err := resolveEvalRoots(ctx, projectRoot, deps.stat)
	if err != nil {
		return evalLeaseInspectResult{}, err
	}
	resolver := &evalEnvironmentResolver{stat: deps.stat}
	env, err := resolver.Resolve(ctx, evalProjectRootSpec(roots))
	if err != nil {
		return evalLeaseInspectResult{}, err
	}

	cfg, err := deps.configLoader("")
	if err != nil {
		return evalLeaseInspectResult{}, fmt.Errorf("eval: load config: %w", err)
	}
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return evalLeaseInspectResult{}, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	leaseStoreDir := fileSvc.Resolve(constants.EvalLeaseDirname)

	absConfigPath, err := resolveAbsPath(configPath)
	if err != nil {
		return evalLeaseInspectResult{}, err
	}

	req := evalLeaseLifecycleRequest{
		Operation: evalLeaseOperationInspect,
		Inspect: &evalLeaseInspectRequest{
			ConfigPath:    absConfigPath,
			LeaseStoreDir: leaseStoreDir,
		},
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return evalLeaseInspectResult{}, fmt.Errorf("%w: marshal lease request: %w", constants.ErrEvalConfigInvalid, err)
	}

	_, result, err := invokeLeaseLifecycleRaw(ctx, deps, env.InterpreterPath, reqJSON)
	if err != nil {
		return evalLeaseInspectResult{}, err
	}
	var inspectResult evalLeaseInspectResult
	if err := json.Unmarshal(result, &inspectResult); err != nil {
		return evalLeaseInspectResult{}, fmt.Errorf("%w: parse lease inspect result: %w", constants.ErrEvalConfigInvalid, err)
	}
	return inspectResult, nil
}

// leaseOperationForTransition maps a lease transition to the matching
// envelope operation. The transition and operation literals are distinct
// enums on the Python side; an unknown transition fails closed.
func leaseOperationForTransition(transition evalLeaseTransition) (evalLeaseOperation, error) {
	switch transition {
	case evalLeaseTransitionStop:
		return evalLeaseOperationStop, nil
	case evalLeaseTransitionExpire:
		return evalLeaseOperationExpire, nil
	case evalLeaseTransitionComplete:
		return evalLeaseOperationComplete, nil
	default:
		return "", fmt.Errorf("%w: unknown lease transition %q", constants.ErrEvalConfigInvalid, transition)
	}
}

// runEvalLeaseTransition invokes the Python lease lifecycle module to
// stop or expire the active lease for a config.
func runEvalLeaseTransition(ctx context.Context, deps evalLeaseDeps, configPath string, transition evalLeaseTransition) (evalLeaseTransitionResult, error) {
	projectRoot, err := evalProjectRootFromConfig(deps.configLoader, "")
	if err != nil {
		return evalLeaseTransitionResult{}, err
	}
	roots, err := resolveEvalRoots(ctx, projectRoot, deps.stat)
	if err != nil {
		return evalLeaseTransitionResult{}, err
	}
	resolver := &evalEnvironmentResolver{stat: deps.stat}
	env, err := resolver.Resolve(ctx, evalProjectRootSpec(roots))
	if err != nil {
		return evalLeaseTransitionResult{}, err
	}

	cfg, err := deps.configLoader("")
	if err != nil {
		return evalLeaseTransitionResult{}, fmt.Errorf("eval: load config: %w", err)
	}
	fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
	if err != nil {
		return evalLeaseTransitionResult{}, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
	}
	leaseStoreDir := fileSvc.Resolve(constants.EvalLeaseDirname)

	absConfigPath, err := resolveAbsPath(configPath)
	if err != nil {
		return evalLeaseTransitionResult{}, err
	}

	operation, err := leaseOperationForTransition(transition)
	if err != nil {
		return evalLeaseTransitionResult{}, err
	}
	req := evalLeaseLifecycleRequest{
		Operation: operation,
		Transition: &evalLeaseTransitionRequest{
			ConfigPath:    absConfigPath,
			LeaseStoreDir: leaseStoreDir,
			Transition:    transition,
		},
	}
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return evalLeaseTransitionResult{}, fmt.Errorf("%w: marshal lease request: %w", constants.ErrEvalConfigInvalid, err)
	}

	_, result, err := invokeLeaseLifecycleRaw(ctx, deps, env.InterpreterPath, reqJSON)
	if err != nil {
		return evalLeaseTransitionResult{}, err
	}
	var transitionResult evalLeaseTransitionResult
	if err := json.Unmarshal(result, &transitionResult); err != nil {
		return evalLeaseTransitionResult{}, fmt.Errorf("%w: parse lease transition result: %w", constants.ErrEvalConfigInvalid, err)
	}
	return transitionResult, nil
}

// invokeLeaseLifecycle invokes the Python lease lifecycle module and
// returns the parsed issue result.
func invokeLeaseLifecycle(ctx context.Context, deps evalLeaseDeps, interpreterPath string, requestJSON []byte) (evalLeaseIssueResult, error) {
	stdout, result, err := invokeLeaseLifecycleRaw(ctx, deps, interpreterPath, requestJSON)
	if err != nil {
		return evalLeaseIssueResult{}, err
	}
	var issueResult evalLeaseIssueResult
	if err := json.Unmarshal(result, &issueResult); err != nil {
		return evalLeaseIssueResult{}, fmt.Errorf("%w: parse lease issue result: %w", constants.ErrEvalConfigInvalid, err)
	}
	_ = stdout
	return issueResult, nil
}

// invokeLeaseLifecycleRaw invokes the Python lease lifecycle module and
// returns the raw stdout bytes and parsed JSON result.
func invokeLeaseLifecycleRaw(ctx context.Context, deps evalLeaseDeps, interpreterPath string, requestJSON []byte) (string, []byte, error) {
	tmpPath, err := deps.tempFileWriter.WriteTempFile("eval-lease-*.json", requestJSON)
	if err != nil {
		return "", nil, err
	}
	defer os.Remove(tmpPath)

	var stdoutBuf strings.Builder
	args := []string{"-m", constants.EvalLeaseLifecycleModule, tmpPath}
	if err := deps.runner.Run(ctx, interpreterPath, args, &stdoutBuf, os.Stderr); err != nil {
		return "", nil, fmt.Errorf("%w: %w", constants.ErrEvalLeaseMissing, err)
	}
	raw := strings.TrimSpace(stdoutBuf.String())
	return raw, []byte(raw), nil
}

// resolveAbsPath converts a config path to an absolute path.
func resolveAbsPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%w: config path is empty", constants.ErrEvalConfigInvalid)
	}
	abs, err := absPath(path)
	if err != nil {
		return "", fmt.Errorf("%w: resolve config path: %w", constants.ErrEvalConfigInvalid, err)
	}
	return abs, nil
}

func absPath(path string) (string, error) {
	if strings.HasPrefix(path, "/") {
		return path, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", cwd, path), nil
}

func printEvalLeaseIssueHuman(stdout io.Writer, result evalLeaseIssueResult) {
	fmt.Fprintf(stdout, "Lease issued: %s\n", result.LeaseID)
	fmt.Fprintf(stdout, "  status:        %s\n", result.Status)
	fmt.Fprintf(stdout, "  issued_at:     %s\n", result.IssuedAt)
	fmt.Fprintf(stdout, "  start_deadline: %s\n", result.StartDeadline)
	fmt.Fprintf(stdout, "  expires_at:    %s\n", result.ExpiresAt)
	fmt.Fprintf(stdout, "  content_hash:  %s\n", result.ContentHash)
	fmt.Fprintf(stdout, "  request_digest: %s\n", result.RequestDigest)
	fmt.Fprintf(stdout, "  config_hash:   %s\n", result.OperationConfigContentHash)
	fmt.Fprintf(stdout, "  lease_path:    %s\n", result.LeasePath)
}

func printEvalLeaseInspectHuman(stdout io.Writer, result evalLeaseInspectResult) {
	if !result.Found {
		fmt.Fprintf(stdout, "No lease found for this config.\n")
		return
	}
	if result.Lease == nil {
		fmt.Fprintf(stdout, "No lease found for this config.\n")
		return
	}
	lease := result.Lease
	fmt.Fprintf(stdout, "Lease: %s\n", lease.LeaseID)
	fmt.Fprintf(stdout, "  status:        %s\n", lease.Status)
	fmt.Fprintf(stdout, "  operation:     %s\n", lease.OperationIdentity)
	fmt.Fprintf(stdout, "  report_root:   %s\n", lease.ReportRoot)
	fmt.Fprintf(stdout, "  issued_at:     %s\n", lease.IssuedAt)
	fmt.Fprintf(stdout, "  expires_at:    %s\n", lease.ExpiresAt)
	fmt.Fprintf(stdout, "  content_hash:  %s\n", lease.ContentHash)
}

func printEvalLeaseTransitionHuman(stdout io.Writer, verb string, result evalLeaseTransitionResult) {
	fmt.Fprintf(stdout, "Lease %s: %s\n", verb, result.LeaseID)
	fmt.Fprintf(stdout, "  previous_status: %s\n", result.PreviousStatus)
	fmt.Fprintf(stdout, "  new_status:       %s\n", result.NewStatus)
	fmt.Fprintf(stdout, "  transitioned_at:  %s\n", result.TransitionedAt)
}

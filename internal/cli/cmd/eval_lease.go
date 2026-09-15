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
// so Tier 1 tests do not touch the provider.
type evalModelInventoryResolver interface {
	Digest(ctx context.Context) (string, error)
}

// realEvalModelInventoryResolver returns a fixed digest for source-checkout
// execution where no live model inventory is available at issue time. The
// digest is recomputed at start time by the engine; a mismatch fails closed.
type realEvalModelInventoryResolver struct{}

func (realEvalModelInventoryResolver) Digest(ctx context.Context) (string, error) {
	h := sha256.Sum256([]byte("source-checkout-no-live-inventory"))
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

// evalLeaseDeps carries the injected dependencies for lease commands.
type evalLeaseDeps struct {
	configLoader           func(string) (*config.Config, error)
	fileSvcFactory         func(string, *slog.Logger) (fs.RuntimeFileService, error)
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
		stat:                   realEvalFileStat{},
		runner:                 realEvalCommandRunner{},
		tempFileWriter:         realEvalTempFileWriter{},
		candidateResolver:      realEvalCandidateResolver{fileReader: realEvalFileReader{}},
		modelInventoryResolver: realEvalModelInventoryResolver{},
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

// evalLeaseIssueRequest is the typed JSON request the Go facade sends
// to the Python lease lifecycle module for issue.
type evalLeaseIssueRequest struct {
	Kind                          string                `json:"kind"`
	ConfigPath                    string                `json:"config_path"`
	LeaseStoreDir                 string                `json:"lease_store_dir"`
	CommandFamily                 string                `json:"command_family"`
	CommandVersion                string                `json:"command_version"`
	Candidate                     evalCandidateIdentity `json:"candidate"`
	ModelInventoryDigest          string                `json:"model_inventory_digest"`
	Endpoint                      string                `json:"endpoint"`
	AppIdentity                   string                `json:"app_identity"`
	OperatorSessionIdentity       string                `json:"operator_session_identity"`
	ExpiresInSeconds              int                   `json:"expires_in_seconds"`
	StartDeadlineSeconds          int                   `json:"start_deadline_seconds"`
	OperationKind                 string                `json:"operation_kind"`
	RequiredRuntimeAuthorityNames []string              `json:"required_runtime_authority_names"`
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

// evalLeaseInspectResult is the typed result emitted by lease inspect.
type evalLeaseInspectResult struct {
	Found bool             `json:"found"`
	Lease *json.RawMessage `json:"lease,omitempty"`
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
			result, err := runEvalLeaseTransition(ctx, deps, args[0], "stop")
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
			result, err := runEvalLeaseTransition(ctx, deps, args[0], "expire")
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
	modelInventoryDigest, err := deps.modelInventoryResolver.Digest(ctx)
	if err != nil {
		return evalLeaseIssueResult{}, err
	}

	absConfigPath, err := resolveAbsPath(configPath)
	if err != nil {
		return evalLeaseIssueResult{}, err
	}

	req := evalLeaseIssueRequest{
		Kind:                          "issue",
		ConfigPath:                    absConfigPath,
		LeaseStoreDir:                 leaseStoreDir,
		CommandFamily:                 "campaign_run",
		CommandVersion:                models.EvalEngineRequestSchemaVersion,
		Candidate:                     candidate,
		ModelInventoryDigest:          modelInventoryDigest,
		Endpoint:                      endpoint,
		AppIdentity:                   appIdentity,
		OperatorSessionIdentity:       operatorSessionIdentity,
		ExpiresInSeconds:              expiresInSeconds,
		StartDeadlineSeconds:          300,
		OperationKind:                 "embedded_authority_diagnostic",
		RequiredRuntimeAuthorityNames: []string{"gold_set", "evidence_key"},
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

	req := map[string]any{
		"operation": "inspect",
		"inspect": map[string]any{
			"config_path":     absConfigPath,
			"lease_store_dir": leaseStoreDir,
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

// runEvalLeaseTransition invokes the Python lease lifecycle module to
// stop or expire the active lease for a config.
func runEvalLeaseTransition(ctx context.Context, deps evalLeaseDeps, configPath, transition string) (evalLeaseTransitionResult, error) {
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

	req := map[string]any{
		"operation": transition,
		"transition": map[string]any{
			"config_path":     absConfigPath,
			"lease_store_dir": leaseStoreDir,
			"transition":      transition,
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
	var lease map[string]any
	if err := json.Unmarshal(*result.Lease, &lease); err != nil {
		fmt.Fprintf(stdout, "Lease found but could not be displayed.\n")
		return
	}
	fmt.Fprintf(stdout, "Lease: %s\n", lease["lease_id"])
	fmt.Fprintf(stdout, "  status:        %s\n", lease["status"])
	fmt.Fprintf(stdout, "  operation:     %s\n", lease["operation_identity"])
	fmt.Fprintf(stdout, "  report_root:   %s\n", lease["report_root"])
	fmt.Fprintf(stdout, "  issued_at:     %s\n", lease["issued_at"])
	fmt.Fprintf(stdout, "  expires_at:    %s\n", lease["expires_at"])
	fmt.Fprintf(stdout, "  content_hash:  %s\n", lease["content_hash"])
}

func printEvalLeaseTransitionHuman(stdout io.Writer, verb string, result evalLeaseTransitionResult) {
	fmt.Fprintf(stdout, "Lease %s: %s\n", verb, result.LeaseID)
	fmt.Fprintf(stdout, "  previous_status: %s\n", result.PreviousStatus)
	fmt.Fprintf(stdout, "  new_status:       %s\n", result.NewStatus)
	fmt.Fprintf(stdout, "  transitioned_at:  %s\n", result.TransitionedAt)
}

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
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

type evalOperationLifecycleRequest struct {
	Action         string `json:"action"`
	ConfigPath     string `json:"config_path"`
	RepositoryRoot string `json:"repository_root"`
}

type evalOperationStopConditions struct {
	IdleTimeoutS float64  `json:"idle_timeout_s"`
	MaxDurationS *float64 `json:"max_duration_s,omitempty"`
}

type evalOperationPlan struct {
	OperationKind        string                      `json:"operation_kind"`
	OperationID          string                      `json:"operation_id"`
	Revision             string                      `json:"revision"`
	SelectedModels       []string                    `json:"selected_models"`
	Arms                 []string                    `json:"arms"`
	TaskCount            int                         `json:"task_count"`
	TaskIdentities       []string                    `json:"task_identities"`
	Repetitions          int                         `json:"repetitions"`
	AssignmentCount      int                         `json:"assignment_count"`
	WarmupCalls          int                         `json:"warmup_calls"`
	MaximumProviderCalls int                         `json:"maximum_provider_calls"`
	MaximumTokens        int                         `json:"maximum_tokens"`
	MaximumUSD           float64                     `json:"maximum_usd"`
	MaximumDurationS     *float64                    `json:"maximum_duration_s,omitempty"`
	MinimumFreeDiskGB    *float64                    `json:"minimum_free_disk_gb,omitempty"`
	ScheduleIdentity     string                      `json:"schedule_identity"`
	StopConditions       evalOperationStopConditions `json:"stop_conditions"`
}

type evalOperationCheck struct {
	CheckID    string `json:"check_id"`
	Status     string `json:"status"`
	SafeDetail string `json:"safe_detail"`
}

type evalOperationCheckResult struct {
	OK            bool                 `json:"ok"`
	OperationKind string               `json:"operation_kind"`
	OperationID   string               `json:"operation_id"`
	Checks        []evalOperationCheck `json:"checks"`
}

type evalLifecycleEnvironment struct {
	RepositoryRoot  string
	EvalProject     string
	InterpreterPath string
	ConfigPath      string
}

type evalOperationStatus struct {
	OperationKind        string  `json:"operation_kind"`
	OperationID          string  `json:"operation_id"`
	Revision             string  `json:"revision"`
	Status               string  `json:"status"`
	ProcessState         string  `json:"process_state"`
	ReportRoot           string  `json:"report_root"`
	CompletedAssignments int     `json:"completed_assignments"`
	TotalAssignments     int     `json:"total_assignments"`
	ProviderRequests     int     `json:"provider_requests"`
	Tokens               int     `json:"tokens"`
	SpentUSD             float64 `json:"spent_usd"`
	StopReason           string  `json:"stop_reason,omitempty"`
	VerificationState    string  `json:"verification_state"`
	PublicationState     string  `json:"publication_state"`
	SafeDetail           string  `json:"safe_detail,omitempty"`
}

type evalOperationStopResult struct {
	OperationKind string `json:"operation_kind"`
	OperationID   string `json:"operation_id"`
	Status        string `json:"status"`
	Immediate     bool   `json:"immediate"`
	RequestPath   string `json:"request_path"`
}

type evalOperationVerifyResult struct {
	OperationKind string   `json:"operation_kind"`
	OperationID   string   `json:"operation_id"`
	OK            bool     `json:"ok"`
	CheckedLayers []string `json:"checked_layers"`
	Failures      []string `json:"failures"`
}

func resolveEvalLifecycleEnvironment(ctx context.Context, deps evalLeaseDeps, configPath string) (evalLifecycleEnvironment, error) {
	projectRoot, err := evalProjectRootFromConfig(deps.configLoader, "")
	if err != nil {
		return evalLifecycleEnvironment{}, err
	}
	roots, err := resolveEvalRoots(ctx, projectRoot, deps.stat)
	if err != nil {
		return evalLifecycleEnvironment{}, err
	}
	env, err := (&evalEnvironmentResolver{stat: deps.stat}).Resolve(ctx, evalProjectRootSpec(roots))
	if err != nil {
		return evalLifecycleEnvironment{}, err
	}
	absConfigPath, err := resolveAbsPath(configPath)
	if err != nil {
		return evalLifecycleEnvironment{}, err
	}
	return evalLifecycleEnvironment{RepositoryRoot: roots.RepositoryRoot, EvalProject: roots.EvalProject, InterpreterPath: env.InterpreterPath, ConfigPath: absConfigPath}, nil
}

func invokeEvalOperationLifecycle(ctx context.Context, deps evalLeaseDeps, env evalLifecycleEnvironment, action string) ([]byte, error) {
	requestJSON, err := json.Marshal(evalOperationLifecycleRequest{Action: action, ConfigPath: env.ConfigPath, RepositoryRoot: env.RepositoryRoot})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal lifecycle request: %w", constants.ErrEvalConfigInvalid, err)
	}
	tmpPath, err := deps.tempFileWriter.WriteTempFile("eval-lifecycle-*.json", requestJSON)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpPath)
	var stdout strings.Builder
	if err := deps.runner.Run(ctx, env.InterpreterPath, []string{"-m", constants.EvalOperationLifecycleModule, tmpPath}, &stdout, os.Stderr); err != nil {
		return nil, fmt.Errorf("%w: %s: %w", constants.ErrEvalConfigInvalid, action, err)
	}
	return []byte(strings.TrimSpace(stdout.String())), nil
}

func runEvalOperationPlan(ctx context.Context, deps evalLeaseDeps, configPath string) (evalOperationPlan, error) {
	env, err := resolveEvalLifecycleEnvironment(ctx, deps, configPath)
	if err != nil {
		return evalOperationPlan{}, err
	}
	payload, err := invokeEvalOperationLifecycle(ctx, deps, env, "plan")
	if err != nil {
		return evalOperationPlan{}, err
	}
	var result evalOperationPlan
	if err := json.Unmarshal(payload, &result); err != nil {
		return evalOperationPlan{}, fmt.Errorf("%w: parse plan result: %w", constants.ErrEvalConfigInvalid, err)
	}
	return result, nil
}

func runEvalOperationCheck(ctx context.Context, deps evalLeaseDeps, configPath string) (evalOperationCheckResult, error) {
	preflight, err := runEvalPreflight(ctx, deps, configPath, evalCommandFamilyCampaignRun)
	if err != nil {
		return preflight.CheckResult, err
	}
	return preflight.CheckResult, nil
}

func runEvalEngineLifecycle(ctx context.Context, deps evalLeaseDeps, configPath string, operation models.EvalOperation, flags models.EvalEngineFlags, stderr io.Writer) (evalEngineResultJSON, error) {
	env, err := resolveEvalLifecycleEnvironment(ctx, deps, configPath)
	if err != nil {
		return evalEngineResultJSON{}, err
	}
	cfg, err := deps.configLoader("")
	if err != nil {
		return evalEngineResultJSON{}, fmt.Errorf("eval: load config: %w", err)
	}
	candidate, err := deps.candidateResolver.Resolve(ctx, env.RepositoryRoot)
	if err != nil {
		return evalEngineResultJSON{}, err
	}
	binaryPath, err := os.Executable()
	if err != nil {
		return evalEngineResultJSON{}, fmt.Errorf("eval: resolve g8e binary: %w", err)
	}
	request := models.EvalEngineRequest{
		SchemaVersion: models.EvalEngineRequestSchemaVersion,
		Operation: operation,
		ConfigPath: env.ConfigPath,
		Platform: models.EvalPlatformContext{
			RepositoryRoot: env.RepositoryRoot,
			EvalProject: env.EvalProject,
			G8EBinaryPath: binaryPath,
			G8EBinarySHA256: candidate.BinarySHA256,
			AuthProjectRoot: cfg.ProjectRoot,
			RuntimeDir: cfg.RuntimeDir,
			TrustBundlePath: cfg.ResolvedTrustBundlePath(),
			GatewayHTTPURL: cfg.OperatorDiscoveryURL(),
			GatewayHTTPSURL: cfg.OperatorPublicURL(),
			EnsembleURL: evalEnsembleBaseURL(cfg),
		},
		Flags: flags,
	}
	return invokeEngine(ctx, deps, env.InterpreterPath, request, stderr)
}

func evalDiagnosticPlanCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationPlanCmdWithDeps(deps, "diagnostic")
}

func evalCampaignPlanCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationPlanCmdWithDeps(deps, "campaign")
}

func evalOperationPlanCmdWithDeps(deps evalLeaseDeps, noun string) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "plan <config>", Short: "Print the exact provider-free operation plan (read-only)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runEvalOperationPlan(commandContext(cmd), deps, args[0])
			if err != nil {
				return err
			}
			if result.OperationKind != noun {
				return fmt.Errorf("%w: expected %s config, got %s", constants.ErrEvalConfigInvalid, noun, result.OperationKind)
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalOperationPlan(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

func evalDiagnosticCheckCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationCheckCmdWithDeps(deps, "diagnostic")
}

func evalCampaignCheckCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationCheckCmdWithDeps(deps, "campaign")
}

func evalOperationCheckCmdWithDeps(deps evalLeaseDeps, noun string) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "check <config>", Short: "Run provider-free config, authority, lease, candidate, inventory, evidence, report-root, and disk preflight", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runEvalOperationCheck(commandContext(cmd), deps, args[0])
			if err != nil {
				return err
			}
			if result.OperationKind != noun {
				return fmt.Errorf("%w: expected %s config, got %s", constants.ErrEvalConfigInvalid, noun, result.OperationKind)
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalOperationCheck(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

func evalDiagnosticStatusCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationStatusCmdWithDeps(deps, "diagnostic", models.EvalOperationDiagnosticStatus)
}

func evalCampaignStatusCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationStatusCmdWithDeps(deps, "campaign", models.EvalOperationCampaignStatus)
}

func evalOperationStatusCmdWithDeps(deps evalLeaseDeps, noun string, operation models.EvalOperation) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "status <config>", Short: "Reconcile process, report, budget, and publication state (read-only)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engineResult, err := runEvalEngineLifecycle(commandContext(cmd), deps, args[0], operation, models.EvalEngineFlags{JSONOutput: true}, cmd.OutOrStderr())
			if err != nil {
				return err
			}
			var result evalOperationStatus
			if err := json.Unmarshal(engineResult.Payload, &result); err != nil {
				return fmt.Errorf("%w: parse status payload: %w", constants.ErrEvalStatusReconciliationFailed, err)
			}
			if result.OperationKind != noun {
				return fmt.Errorf("%w: expected %s config, got %s", constants.ErrEvalConfigInvalid, noun, result.OperationKind)
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalOperationStatus(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

func evalDiagnosticStopCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationStopCmdWithDeps(deps, "diagnostic", models.EvalOperationDiagnosticStop)
}

func evalCampaignStopCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationStopCmdWithDeps(deps, "campaign", models.EvalOperationCampaignStop)
}

func evalOperationStopCmdWithDeps(deps evalLeaseDeps, noun string, operation models.EvalOperation) *cobra.Command {
	var immediate, yes, jsonOutput bool
	cmd := &cobra.Command{
		Use: "stop <config>", Short: "Request an identity-bound graceful stop (local mutation)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if immediate && !yes {
				return fmt.Errorf("%w: --immediate requires --yes", constants.ErrEvalConfigInvalid)
			}
			engineResult, err := runEvalEngineLifecycle(commandContext(cmd), deps, args[0], operation, models.EvalEngineFlags{JSONOutput: true, ImmediateStop: immediate}, cmd.OutOrStderr())
			if err != nil {
				return err
			}
			var result evalOperationStopResult
			if err := json.Unmarshal(engineResult.Payload, &result); err != nil {
				return fmt.Errorf("%w: parse stop payload: %w", constants.ErrEvalStatusReconciliationFailed, err)
			}
			if result.OperationKind != noun {
				return fmt.Errorf("%w: expected %s config, got %s", constants.ErrEvalConfigInvalid, noun, result.OperationKind)
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s stop: %s\n", noun, result.Status)
			return nil
		},
	}
	cmd.Flags().BoolVar(&immediate, "immediate", false, "Force termination instead of requesting a graceful stop")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm forced termination")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

func evalDiagnosticVerifyCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationVerifyCmdWithDeps(deps, "diagnostic", models.EvalOperationDiagnosticVerify)
}

func evalCampaignVerifyCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	return evalOperationVerifyCmdWithDeps(deps, "campaign", models.EvalOperationCampaignVerify)
}

func evalOperationVerifyCmdWithDeps(deps evalLeaseDeps, noun string, operation models.EvalOperation) *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "verify <config>", Short: "Run complete offline report verification (read-only)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engineResult, err := runEvalEngineLifecycle(commandContext(cmd), deps, args[0], operation, models.EvalEngineFlags{JSONOutput: true}, cmd.OutOrStderr())
			// Render the typed payload even when the engine returned a
			// failed result so --json does not lose the checked layers or
			// the specific failures. The mapped error is returned after
			// rendering so the nonzero exit classification is preserved.
			if len(engineResult.Payload) > 0 {
				var result evalOperationVerifyResult
				if uerr := json.Unmarshal(engineResult.Payload, &result); uerr == nil && result.OperationKind == noun {
					if jsonOutput {
						if jerr := emitEvalJSON(cmd, result); jerr != nil {
							return jerr
						}
					} else {
						if result.OK {
							fmt.Fprintf(cmd.OutOrStdout(), "%s verify: verified (%s)\n", noun, strings.Join(result.CheckedLayers, ", "))
						} else {
							fmt.Fprintf(cmd.OutOrStdout(), "%s verify: failed (%s)\n", noun, strings.Join(result.CheckedLayers, ", "))
							for _, failure := range result.Failures {
								fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", failure)
							}
						}
					}
				}
			}
			if err != nil {
				return err
			}
			var result evalOperationVerifyResult
			if err := json.Unmarshal(engineResult.Payload, &result); err != nil {
				return fmt.Errorf("%w: parse verify payload: %w", constants.ErrEvalAuthorityInvalid, err)
			}
			if result.OperationKind != noun {
				return fmt.Errorf("%w: expected %s config, got %s", constants.ErrEvalConfigInvalid, noun, result.OperationKind)
			}
			if !result.OK {
				return fmt.Errorf("%w: offline verification failed with %d failure(s)", constants.ErrEvalAuthorityInvalid, len(result.Failures))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

func printEvalOperationStatus(stdout io.Writer, result evalOperationStatus) {
	fmt.Fprintf(stdout, "%s status: %s\n", result.OperationKind, result.Status)
	fmt.Fprintf(stdout, "  operation_id:          %s\n", result.OperationID)
	fmt.Fprintf(stdout, "  process:               %s\n", result.ProcessState)
	fmt.Fprintf(stdout, "  assignments:           %d/%d\n", result.CompletedAssignments, result.TotalAssignments)
	fmt.Fprintf(stdout, "  provider_requests:     %d\n", result.ProviderRequests)
	fmt.Fprintf(stdout, "  tokens:                %d\n", result.Tokens)
	fmt.Fprintf(stdout, "  spent_usd:             %.2f\n", result.SpentUSD)
	if result.StopReason != "" {
		fmt.Fprintf(stdout, "  stop_reason:           %s\n", result.StopReason)
	}
	fmt.Fprintf(stdout, "  verification:          %s\n", result.VerificationState)
	fmt.Fprintf(stdout, "  publication:           %s\n", result.PublicationState)
	if result.SafeDetail != "" {
		fmt.Fprintf(stdout, "  detail:                %s\n", result.SafeDetail)
	}
}

func printEvalOperationPlan(stdout io.Writer, result evalOperationPlan) {
	fmt.Fprintf(stdout, "%s plan: %s\n", result.OperationKind, result.OperationID)
	fmt.Fprintf(stdout, "  revision:               %s\n", result.Revision)
	fmt.Fprintf(stdout, "  selected_models:        %s\n", strings.Join(result.SelectedModels, ", "))
	fmt.Fprintf(stdout, "  arms:                   %s\n", strings.Join(result.Arms, ", "))
	fmt.Fprintf(stdout, "  tasks:                  %d\n", result.TaskCount)
	fmt.Fprintf(stdout, "  task_identities:        %s\n", strings.Join(result.TaskIdentities, ", "))
	fmt.Fprintf(stdout, "  repetitions:            %d\n", result.Repetitions)
	fmt.Fprintf(stdout, "  assignments:            %d\n", result.AssignmentCount)
	fmt.Fprintf(stdout, "  warmup_calls:           %d\n", result.WarmupCalls)
	fmt.Fprintf(stdout, "  maximum_provider_calls: %d\n", result.MaximumProviderCalls)
	fmt.Fprintf(stdout, "  maximum_tokens:         %d\n", result.MaximumTokens)
	fmt.Fprintf(stdout, "  maximum_usd:            %.2f\n", result.MaximumUSD)
	fmt.Fprintf(stdout, "  schedule_identity:      %s\n", result.ScheduleIdentity)
}

func printEvalOperationCheck(stdout io.Writer, result evalOperationCheckResult) {
	fmt.Fprintf(stdout, "%s check: %s\n", result.OperationKind, result.OperationID)
	for _, check := range result.Checks {
		fmt.Fprintf(stdout, "  %s: %s (%s)\n", check.CheckID, check.Status, check.SafeDetail)
	}
}

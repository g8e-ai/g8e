// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
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
	TaskManifestDigest   string                      `json:"task_manifest_digest"`
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
	return runEvalEngineOperation(ctx, deps, configPath, "", operation, flags, nil, stderr)
}

func runEvalEngineOperation(ctx context.Context, deps evalLeaseDeps, configPath, reportRoot string, operation models.EvalOperation, flags models.EvalEngineFlags, parameters map[string]string, stderr io.Writer) (evalEngineResultJSON, error) {
	projectRoot, err := evalProjectRootFromConfig(deps.configLoader, "")
	if err != nil {
		return evalEngineResultJSON{}, err
	}
	roots, err := resolveEvalRoots(ctx, projectRoot, deps.stat)
	if err != nil {
		return evalEngineResultJSON{}, err
	}
	resolved, err := (&evalEnvironmentResolver{stat: deps.stat}).Resolve(ctx, evalProjectRootSpec(roots))
	if err != nil {
		return evalEngineResultJSON{}, err
	}
	absConfigPath := ""
	if configPath != "" {
		absConfigPath, err = resolveAbsPath(configPath)
		if err != nil {
			return evalEngineResultJSON{}, err
		}
	}
	absReportRoot := ""
	if reportRoot != "" {
		absReportRoot, err = resolveAbsPath(reportRoot)
		if err != nil {
			return evalEngineResultJSON{}, err
		}
	}
	cfg, err := deps.configLoader("")
	if err != nil {
		return evalEngineResultJSON{}, fmt.Errorf("eval: load config: %w", err)
	}
	candidate, err := deps.candidateResolver.Resolve(ctx, roots.RepositoryRoot)
	if err != nil {
		return evalEngineResultJSON{}, err
	}
	// Best-effort auth context: lifecycle operations (status, stop,
	// verify) never authenticate to the platform, so missing credentials
	// do not block them. Provider-backed starts enforce auth separately
	// through loadEvalAuthContext.
	var authCtx *auth.ClientAuthContext
	if fileSvc, ferr := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default()); ferr == nil && deps.authContextLoader != nil {
		authCtx, _ = deps.authContextLoader(fileSvc, cfg)
	}
	env := evalLifecycleEnvironment{RepositoryRoot: roots.RepositoryRoot, EvalProject: roots.EvalProject, InterpreterPath: resolved.InterpreterPath, ConfigPath: absConfigPath}
	request := models.EvalEngineRequest{
		SchemaVersion: models.EvalEngineRequestSchemaVersion,
		Operation:     operation,
		ConfigPath:    absConfigPath,
		ReportRoot:    absReportRoot,
		Platform:      buildEvalPlatformContext(ctx, env, cfg, candidate, authCtx),
		Flags:         flags,
		Parameters:    parameters,
	}
	return invokeEngine(ctx, deps, resolved.InterpreterPath, request, stderr)
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

func runEvalUtilityOperation(cmd *cobra.Command, deps evalLeaseDeps, configPath, reportRoot string, operation models.EvalOperation, parameters map[string]string, jsonOutput bool) error {
	result, err := runEvalEngineOperation(commandContext(cmd), deps, configPath, reportRoot, operation, models.EvalEngineFlags{JSONOutput: true}, parameters, cmd.OutOrStderr())
	if len(result.Payload) > 0 {
		if jsonOutput {
			fmt.Fprintln(cmd.OutOrStdout(), string(result.Payload))
		} else {
			var formatted bytes.Buffer
			if indentErr := json.Indent(&formatted, result.Payload, "  ", "  "); indentErr != nil {
				return fmt.Errorf("%w: render engine payload: %w", constants.ErrEvalConfigInvalid, indentErr)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n  %s\n", operation, result.Status, formatted.String())
		}
	}
	return err
}

func evalCampaignSetCmd() *cobra.Command {
	return evalCampaignSetCmdWithDeps(evalStartDepsFromLeaseDeps())
}

func evalCampaignSetCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	cmd := &cobra.Command{Use: "campaign-set", Short: "Plan, validate, and verify campaign sets"}
	var planJSON bool
	planCmd := &cobra.Command{Use: "plan <plan>", Short: "Print a deterministic campaign-set plan (read-only)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return runEvalUtilityOperation(cmd, deps, args[0], "", models.EvalOperationCampaignSetPlan, nil, planJSON)
	}}
	planCmd.Flags().BoolVar(&planJSON, "json", false, "Emit a single canonical JSON object on stdout")
	var index string
	var validateJSON bool
	validateCmd := &cobra.Command{Use: "validate <plan>", Short: "Validate a campaign-set plan and index (read-only)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		absIndex, err := resolveAbsPath(index)
		if err != nil {
			return err
		}
		return runEvalUtilityOperation(cmd, deps, args[0], "", models.EvalOperationCampaignSetValidate, map[string]string{"index": absIndex}, validateJSON)
	}}
	validateCmd.Flags().StringVar(&index, "index", "", "Campaign-set index path")
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Emit a single canonical JSON object on stdout")
	_ = validateCmd.MarkFlagRequired("index")
	var verifyIndex, replacementRule string
	var childDirs []string
	var verifyJSON bool
	verifyCmd := &cobra.Command{Use: "verify <plan>", Short: "Run complete campaign-set verification (read-only)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		absIndex, err := resolveAbsPath(verifyIndex)
		if err != nil {
			return err
		}
		children := make(map[string]string, len(childDirs))
		for _, child := range childDirs {
			id, path, ok := strings.Cut(child, "=")
			if !ok || id == "" || path == "" {
				return fmt.Errorf("%w: --child-dir values must use child-id=report-dir", constants.ErrEvalConfigInvalid)
			}
			absPath, pathErr := resolveAbsPath(path)
			if pathErr != nil {
				return pathErr
			}
			children[id] = absPath
		}
		childJSON, err := json.Marshal(children)
		if err != nil {
			return fmt.Errorf("%w: marshal child directories: %w", constants.ErrEvalConfigInvalid, err)
		}
		parameters := map[string]string{"index": absIndex, "child_dirs": string(childJSON)}
		if replacementRule != "" {
			parameters["replacement_rule"], err = resolveAbsPath(replacementRule)
			if err != nil {
				return err
			}
		}
		return runEvalUtilityOperation(cmd, deps, args[0], "", models.EvalOperationCampaignSetVerify, parameters, verifyJSON)
	}}
	verifyCmd.Flags().StringVar(&verifyIndex, "index", "", "Campaign-set index path")
	verifyCmd.Flags().StringSliceVar(&childDirs, "child-dir", nil, "Child binding in child-id=report-dir form")
	verifyCmd.Flags().StringVar(&replacementRule, "replacement-rule", "", "Replacement rule path")
	verifyCmd.Flags().BoolVar(&verifyJSON, "json", false, "Emit a single canonical JSON object on stdout")
	_ = verifyCmd.MarkFlagRequired("index")
	_ = verifyCmd.MarkFlagRequired("child-dir")
	cmd.AddCommand(planCmd, validateCmd, verifyCmd)
	return cmd
}

func evalBundleCmd() *cobra.Command {
	deps := evalStartDepsFromLeaseDeps()
	var yes, jsonOutput bool
	cmd := &cobra.Command{Use: "bundle <report-dir>", Short: "Create a signed immutable eval bundle (local mutation)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return fmt.Errorf("%w: --yes is required for bundle", constants.ErrEvalConfigInvalid)
		}
		reportRoot, err := resolveAbsPath(args[0])
		if err != nil {
			return err
		}
		cfg, err := deps.configLoader("")
		if err != nil {
			return fmt.Errorf("eval: load config: %w", err)
		}
		fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
		}
		parameters := map[string]string{
			"bundle_dir":       reportRoot + constants.EvalBundleDirectorySuffix,
			"bundle_id":        filepath.Base(reportRoot),
			"signing_key_path": fileSvc.Resolve(constants.EvalBundleSigningKeyPath),
		}
		return runEvalUtilityOperation(cmd, deps, "", reportRoot, models.EvalOperationBundle, parameters, jsonOutput)
	}}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm bundle creation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

func evalVerifyCmd() *cobra.Command {
	deps := evalStartDepsFromLeaseDeps()
	var receipts, jsonOutput bool
	cmd := &cobra.Command{Use: "verify <report-dir>", Short: "Run complete offline bundle verification (read-only)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveAbsPath(args[0])
		if err != nil {
			return err
		}
		cfg, err := deps.configLoader("")
		if err != nil {
			return fmt.Errorf("eval: load config: %w", err)
		}
		fileSvc, err := deps.fileSvcFactory(cfg.ProjectRoot, slog.Default())
		if err != nil {
			return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
		}
		if receipts {
			return runEvalUtilityOperation(cmd, deps, "", root, models.EvalOperationVerifyReceipts, map[string]string{"pki_dir": fileSvc.Resolve(constants.PkiDirname)}, jsonOutput)
		}
		parameters := map[string]string{}
		exists, err := fileSvc.FileExists(commandContext(cmd), constants.EvalBundleTrustStorePath)
		if err != nil {
			return err
		}
		if exists {
			parameters["trust_store"] = fileSvc.Resolve(constants.EvalBundleTrustStorePath)
		}
		return runEvalUtilityOperation(cmd, deps, "", root, models.EvalOperationVerify, parameters, jsonOutput)
	}}
	cmd.Flags().BoolVar(&receipts, "receipts", false, "Run receipt-signature verification only")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	return cmd
}

func evalQualificationCmd() *cobra.Command {
	deps := evalStartDepsFromLeaseDeps()
	cmd := &cobra.Command{Use: "qualification", Short: "Build deterministic collection-candidate qualification evidence"}
	var authority, sourceRoot, authorityRecordPath, hashOutput string
	var hashJSON bool
	hashCmd := &cobra.Command{Use: "hash-source", Short: "Hash an owner-approved source manifest (read-only)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		parameters, err := resolveEvalPathParameters(map[string]string{"authority": authority, "source_root": sourceRoot, "output": hashOutput})
		if err != nil {
			return err
		}
		parameters["authority_record_path"] = authorityRecordPath
		return runEvalUtilityOperation(cmd, deps, "", "", models.EvalOperationQualificationHashSource, parameters, hashJSON)
	}}
	hashCmd.Flags().StringVar(&authority, "authority", "", "Source manifest authority path")
	hashCmd.Flags().StringVar(&sourceRoot, "source-root", "", "Source tree root")
	hashCmd.Flags().StringVar(&authorityRecordPath, "authority-record-path", "", "Authority record path stored in evidence")
	hashCmd.Flags().StringVarP(&hashOutput, "output", "o", "", "Output path")
	hashCmd.Flags().BoolVar(&hashJSON, "json", false, "Emit a single canonical JSON object on stdout")
	for _, name := range []string{"authority", "source-root", "authority-record-path", "output"} {
		_ = hashCmd.MarkFlagRequired(name)
	}
	var fullSource, executionSource, binaryPath, candidateOutput string
	var images []string
	var candidateJSON bool
	candidateCmd := &cobra.Command{Use: "candidate", Short: "Build a typed candidate identity (read-only)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		parameters, err := resolveEvalPathParameters(map[string]string{"full_source": fullSource, "execution_source": executionSource, "binary": binaryPath, "output": candidateOutput})
		if err != nil {
			return err
		}
		parsedImages := make([]evalQualificationImage, 0, len(images))
		for _, image := range images {
			components, imageID, ok := strings.Cut(image, "=")
			if !ok || components == "" || imageID == "" {
				return fmt.Errorf("%w: --image values must use component[,component]=sha256:<digest>", constants.ErrEvalConfigInvalid)
			}
			parsedImages = append(parsedImages, evalQualificationImage{Components: strings.Split(components, ","), ImageID: imageID})
		}
		encoded, err := json.Marshal(parsedImages)
		if err != nil {
			return fmt.Errorf("%w: marshal image identities: %w", constants.ErrEvalConfigInvalid, err)
		}
		parameters["images"] = string(encoded)
		return runEvalUtilityOperation(cmd, deps, "", "", models.EvalOperationQualificationCandidate, parameters, candidateJSON)
	}}
	candidateCmd.Flags().StringVar(&fullSource, "full-source", "", "Full-source manifest result")
	candidateCmd.Flags().StringVar(&executionSource, "execution-source", "", "Execution-source manifest result")
	candidateCmd.Flags().StringVar(&binaryPath, "binary", "", "Candidate binary")
	candidateCmd.Flags().StringSliceVar(&images, "image", nil, "Component image identity")
	candidateCmd.Flags().StringVarP(&candidateOutput, "output", "o", "", "Output path")
	candidateCmd.Flags().BoolVar(&candidateJSON, "json", false, "Emit a single canonical JSON object on stdout")
	for _, name := range []string{"full-source", "execution-source", "binary", "image", "output"} {
		_ = candidateCmd.MarkFlagRequired(name)
	}
	var collectRequest, collectOutput string
	var collectJSON bool
	collectCmd := &cobra.Command{Use: "collect-runtime", Short: "Collect public runtime identity evidence (local mutation)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		parameters, err := resolveEvalPathParameters(map[string]string{"request": collectRequest, "output": collectOutput})
		if err != nil {
			return err
		}
		return runEvalUtilityOperation(cmd, deps, "", "", models.EvalOperationQualificationCollectRuntime, parameters, collectJSON)
	}}
	collectCmd.Flags().StringVar(&collectRequest, "request", "", "Runtime collection request")
	collectCmd.Flags().StringVarP(&collectOutput, "output", "o", "", "Output path")
	collectCmd.Flags().BoolVar(&collectJSON, "json", false, "Emit a single canonical JSON object on stdout")
	_ = collectCmd.MarkFlagRequired("request")
	_ = collectCmd.MarkFlagRequired("output")
	var gateCandidate, gateID, gateOutput string
	var toolVersions []string
	var gateJSON bool
	gateCmd := &cobra.Command{Use: "run-gate -- <command> [args...]", Short: "Run one deterministic candidate-bound gate (local mutation)", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		parameters, err := resolveEvalPathParameters(map[string]string{"candidate": gateCandidate, "output": gateOutput})
		if err != nil {
			return err
		}
		versions := make(map[string]string, len(toolVersions))
		for _, item := range toolVersions {
			name, version, ok := strings.Cut(item, "=")
			if !ok || name == "" || version == "" {
				return fmt.Errorf("%w: --tool-version values must use name=version", constants.ErrEvalConfigInvalid)
			}
			versions[name] = version
		}
		commandJSON, err := json.Marshal(args)
		if err != nil {
			return fmt.Errorf("%w: marshal gate command: %w", constants.ErrEvalConfigInvalid, err)
		}
		versionJSON, err := json.Marshal(versions)
		if err != nil {
			return fmt.Errorf("%w: marshal tool versions: %w", constants.ErrEvalConfigInvalid, err)
		}
		parameters["gate_id"] = gateID
		parameters["command"] = string(commandJSON)
		parameters["tool_versions"] = string(versionJSON)
		return runEvalUtilityOperation(cmd, deps, "", "", models.EvalOperationQualificationRunGate, parameters, gateJSON)
	}}
	gateCmd.Flags().StringVar(&gateCandidate, "candidate", "", "Candidate identity path")
	gateCmd.Flags().StringVar(&gateID, "gate-id", "", "Gate identity")
	gateCmd.Flags().StringSliceVar(&toolVersions, "tool-version", nil, "Tool version in name=version form")
	gateCmd.Flags().StringVarP(&gateOutput, "output", "o", "", "Output path")
	gateCmd.Flags().BoolVar(&gateJSON, "json", false, "Emit a single canonical JSON object on stdout")
	for _, name := range []string{"candidate", "gate-id", "tool-version", "output"} {
		_ = gateCmd.MarkFlagRequired(name)
	}
	var buildInput, buildOutput, buildCheck string
	var buildJSON bool
	buildCmd := &cobra.Command{Use: "build", Short: "Build or reproduce a qualification draft (local mutation)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		parameters, err := resolveEvalPathParameters(map[string]string{"input": buildInput})
		if err != nil {
			return err
		}
		if (buildOutput == "") == (buildCheck == "") {
			return fmt.Errorf("%w: exactly one of --output or --check is required", constants.ErrEvalConfigInvalid)
		}
		optional, err := resolveEvalPathParameters(map[string]string{"output": buildOutput, "check": buildCheck})
		if err != nil {
			return err
		}
		for name, value := range optional {
			parameters[name] = value
		}
		return runEvalUtilityOperation(cmd, deps, "", "", models.EvalOperationQualificationBuild, parameters, buildJSON)
	}}
	buildCmd.Flags().StringVar(&buildInput, "input", "", "Qualification build request")
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Output path")
	buildCmd.Flags().StringVar(&buildCheck, "check", "", "Existing draft to reproduce")
	buildCmd.Flags().BoolVar(&buildJSON, "json", false, "Emit a single canonical JSON object on stdout")
	_ = buildCmd.MarkFlagRequired("input")
	cmd.AddCommand(hashCmd, candidateCmd, collectCmd, gateCmd, buildCmd)
	return cmd
}

type evalQualificationImage struct {
	Components []string `json:"components"`
	ImageID    string   `json:"image_id"`
}

func resolveEvalPathParameters(values map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(values))
	for name, value := range values {
		if value == "" {
			continue
		}
		abs, err := resolveAbsPath(value)
		if err != nil {
			return nil, err
		}
		resolved[name] = abs
	}
	return resolved, nil
}

func evalBenchSyntheticCmd() *cobra.Command {
	deps := evalStartDepsFromLeaseDeps()
	var suite, goldSet, outputDir, preregistration string
	var limit int
	var yes, jsonOutput bool
	cmd := &cobra.Command{Use: "bench-synthetic", Short: "Run a deterministic provider-free synthetic benchmark (local mutation)", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !yes {
			return fmt.Errorf("%w: --yes is required for bench-synthetic", constants.ErrEvalConfigInvalid)
		}
		parameters := map[string]string{"suite": suite}
		for name, value := range map[string]string{"gold_set": goldSet, "preregistration": preregistration} {
			if value != "" {
				abs, err := resolveAbsPath(value)
				if err != nil {
					return err
				}
				parameters[name] = abs
			}
		}
		if limit > 0 {
			parameters["limit"] = fmt.Sprintf("%d", limit)
		}
		return runEvalUtilityOperation(cmd, deps, "", outputDir, models.EvalOperationBenchSynthetic, parameters, jsonOutput)
	}}
	cmd.Flags().StringVar(&suite, "suite", "", "Synthetic suite name")
	cmd.Flags().StringVar(&goldSet, "gold-set", "", "Optional gold-set path")
	cmd.Flags().StringVarP(&outputDir, "out", "o", "reports", "Output directory")
	cmd.Flags().StringVar(&preregistration, "preregistration", "", "Optional preregistration path")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit the number of tasks")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm synthetic benchmark creation")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	_ = cmd.MarkFlagRequired("suite")
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
	fmt.Fprintf(stdout, "  task_manifest_digest:   %s\n", result.TaskManifestDigest)
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

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// evalControllerCmd returns the production `eval controller` command tree
// with real dependencies.
func evalControllerCmd() *cobra.Command {
	return evalControllerCmdWithDeps(evalStartDepsFromLeaseDeps())
}

// evalControllerCmdWithDeps returns the `eval controller` command tree
// wired with the supplied dependencies for testability.
func evalControllerCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "controller",
		Short: "Controller lifecycle (run, status, stop, recover)",
		Long: `controller owns the multi-operation controller lifecycle. A
controller run executes a manifest of operations against a work
directory and produces a coordinated run. Status reconciles durable state,
stop writes an identity-bound request, and recover seals interrupted execution
or retries publication without rerunning inference.`,
	}
	cmd.AddCommand(
		evalControllerRunCmdWithDeps(deps),
		evalControllerStatusCmdWithDeps(deps),
		evalControllerStopCmdWithDeps(deps),
		evalControllerRecoverCmdWithDeps(deps),
	)
	return cmd
}

func evalControllerStatusCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var workDir string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use: "status", Short: "Read durable controller state (read-only)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			absWorkDir, err := resolveAbsPath(workDir)
			if err != nil {
				return err
			}
			return runEvalUtilityOperation(cmd, deps, "", "", models.EvalOperationControllerStatus, map[string]string{"work_dir": absWorkDir}, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&workDir, "work-dir", "", "Controller work directory")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	_ = cmd.MarkFlagRequired("work-dir")
	return cmd
}

func evalControllerStopCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var workDir string
	var immediate, yes, jsonOutput bool
	cmd := &cobra.Command{
		Use: "stop", Short: "Request a durable controller stop (local mutation)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if immediate && !yes {
				return fmt.Errorf("%w: --immediate requires --yes", constants.ErrEvalConfigInvalid)
			}
			absWorkDir, err := resolveAbsPath(workDir)
			if err != nil {
				return err
			}
			result, err := runEvalEngineOperation(commandContext(cmd), deps, "", "", models.EvalOperationControllerStop, models.EvalEngineFlags{JSONOutput: true, ImmediateStop: immediate}, map[string]string{"work_dir": absWorkDir}, cmd.OutOrStderr())
			if len(result.Payload) > 0 {
				if jsonOutput {
					fmt.Fprintln(cmd.OutOrStdout(), string(result.Payload))
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "controller stop: %s\n", result.Status)
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&workDir, "work-dir", "", "Controller work directory")
	cmd.Flags().BoolVar(&immediate, "immediate", false, "Terminate the active child and retain dead evidence")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm forced termination")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	_ = cmd.MarkFlagRequired("work-dir")
	return cmd
}

func evalControllerRecoverCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var workDir string
	var publication, jsonOutput bool
	cmd := &cobra.Command{
		Use: "recover", Short: "Seal interrupted execution or retry durable publication", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			absWorkDir, err := resolveAbsPath(workDir)
			if err != nil {
				return err
			}
			parameters := map[string]string{"work_dir": absWorkDir}
			if publication {
				parameters["publication"] = "true"
			}
			return runEvalUtilityOperation(cmd, deps, "", "", models.EvalOperationControllerRecover, parameters, jsonOutput)
		},
	}
	cmd.Flags().StringVar(&workDir, "work-dir", "", "Controller work directory")
	cmd.Flags().BoolVar(&publication, "publication", false, "Retry durable publication only")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	_ = cmd.MarkFlagRequired("work-dir")
	return cmd
}

// evalControllerRunCmdWithDeps returns the `eval controller run` command
// wired with the supplied dependencies. Run is provider-backed and
// requires a valid active lease; it routes through runEvalStart so lease
// verification cannot be bypassed.
func evalControllerRunCmdWithDeps(deps evalLeaseDeps) *cobra.Command {
	var workDir string
	var jsonOutput, verbose, yes bool

	cmd := &cobra.Command{
		Use:   "run <manifest>",
		Short: "Start a provider-backed controller run (requires lease)",
		Long: `run launches a controller run from a manifest against a work
directory. It is provider-backed: it requires a valid active lease
bound to the exact typed request digest of the supplied manifest.
Issue a lease with 'eval lease issue' before run.

The command verifies the lease before report-root creation or engine
launch. A missing, inactive, expired, consumed, or mismatched lease
fails closed with a typed error. After the engine reaches a terminal
state the lease is transitioned to completed (success) or stopped
(failure/interruption).

This command requires --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("%w: --yes is required for controller run", constants.ErrEvalLeaseMissing)
			}
			ctx := commandContext(cmd)
			result, err := runEvalStart(ctx, deps, args[0], models.EvalOperationControllerRun, evalCommandFamilyControllerRun, jsonOutput, verbose, cmd.OutOrStdout(), cmd.OutOrStderr())
			if err != nil {
				if jsonOutput {
					return emitEvalJSON(cmd, result)
				}
				return err
			}
			if jsonOutput {
				return emitEvalJSON(cmd, result)
			}
			printEvalStartHuman(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&workDir, "work-dir", "", "Work directory for the controller run")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the provider-backed launch")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit a single canonical JSON object on stdout")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Add authority and per-assignment detail to human output")
	_ = cmd.MarkFlagRequired("work-dir")
	return cmd
}

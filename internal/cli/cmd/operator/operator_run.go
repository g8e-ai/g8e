// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/api"
	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/operator"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

type operatorRunJSON struct {
	Results []operator.RunResult `json:"results"`
}

type operatorRunClientFactory func(fs.RuntimeFileService, *config.Config, time.Duration) (authcmd.APIClient, error)

func defaultOperatorRunClientFactory(fileSvc fs.RuntimeFileService, cfg *config.Config, timeout time.Duration) (authcmd.APIClient, error) {
	return api.NewClientWithTimeout(fileSvc, cfg, timeout)
}

func operatorRunCmd() *cobra.Command {
	return operatorRunCmdWithConfig(shared.LoadConfig, defaultOperatorRunClientFactory, shared.NewFileSvc)
}

func operatorRunCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory operatorRunClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var command string
	var timeoutSeconds int

	cmd := &cobra.Command{
		Use:   "run <operator-session-id> [operator-session-id...]",
		Short: "Execute a shell command on one or more remote operators",
		Long: `Execute a governed shell command on remote operator sessions through the gateway.

Provide one or more operator session IDs (from './g8e operator list') and the
command to run with --cmd. Commands are dispatched in parallel to every target
session using the native EXECUTE_BASH gateway path.

Requires './g8e auth enroll user'. Each target session must belong to the
authenticated user and be active.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			command = strings.TrimSpace(command)
			if command == "" {
				return fmt.Errorf("%w: --cmd is required", constants.ErrMissingRequiredField)
			}
			if timeoutSeconds <= 0 {
				timeoutSeconds = constants.DefaultShellCommandTimeout
			}
			if timeoutSeconds > constants.MaxShellCommandTimeout {
				return fmt.Errorf("%w: --timeout cannot exceed %d seconds", constants.ErrMCPRunShellCommandTimeoutExceeded, constants.MaxShellCommandTimeout)
			}

			targetSessionIDs := dedupeOperatorSessionIDs(args)
			if len(targetSessionIDs) == 0 {
				return fmt.Errorf("%w: at least one operator session id is required", constants.ErrGatewayOperatorSessionIDRequired)
			}

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

			client, err := clientFactory(fileSvc, cfg, time.Duration(timeoutSeconds)*time.Second+5*time.Second)
			if err != nil {
				return fmt.Errorf("operator run: create API client: %w", err)
			}

			operators, err := listUserOperators(client, creds.UserID)
			if err != nil {
				return fmt.Errorf("operator run: %w", err)
			}
			targets, err := resolveOperatorRunTargets(operators, targetSessionIDs)
			if err != nil {
				return err
			}

			results := dispatchOperatorRun(client, targets, command, creds.CLISessionID)
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), operatorRunJSON{Results: results})
			}

			failures := writeOperatorRunText(cmd, results)
			if failures > 0 {
				return fmt.Errorf("operator run: %d of %d dispatches failed", failures, len(results))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&command, "cmd", "", "Shell command to execute on each target operator (required)")
	cmd.Flags().IntVar(&timeoutSeconds, "timeout", constants.DefaultShellCommandTimeout, fmt.Sprintf("Per-operator dispatch timeout in seconds (max %d)", constants.MaxShellCommandTimeout))
	return cmd
}

func operatorStopCmd() *cobra.Command {
	return operatorStopCmdWithConfig(shared.LoadConfig, authcmd.DefaultAPIClientFactory, shared.NewFileSvc)
}

func operatorStopCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	return operatorStopCmdWithLocal(configLoader, clientFactory, fileSvcFactory, discoverLocalOperators)
}

func operatorStopCmdWithLocal(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	discover func() ([]localOperatorProcess, error),
) *cobra.Command {
	var reason string
	var grace time.Duration
	cmd := &cobra.Command{
		Use:   "stop [operator-session-id]",
		Short: "Stop operators, terminating local workers if governed shutdown stalls",
		Long: `Request governed shutdown, then wait briefly for local workers to exit.
Local workers that remain running receive TERM, then KILL. With no session ID,
stop all local g8e operator workers owned by the current user, including workers
missing from the gateway registry. Remote-only targets receive governed shutdown.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if grace < 0 {
				return fmt.Errorf("operator stop: --grace must not be negative")
			}
			sessionID := ""
			if len(args) > 0 {
				sessionID = strings.TrimSpace(args[0])
				if sessionID == "" {
					return constants.ErrGatewayOperatorSessionIDRequired
				}
			}
			locals, discoveryErr := discover()
			defer func() {
				for _, p := range locals {
					p.close()
				}
			}()
			if sessionID == "" && discoveryErr != nil {
				return discoveryErr
			}
			if sessionID == "" && len(locals) == 0 {
				if output.JSONEnabled(cmd) {
					return output.WriteJSON(cmd.OutOrStdout(), []operatorStopResult{})
				}
				cmd.Println("No local operator workers running.")
				return nil
			}
			var client authcmd.APIClient
			var operators []*operatorv1.OperatorDocument
			governedErr := func() error {
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
					return constants.ErrNotAuthenticated
				}
				client, err = clientFactory(fileSvc, cfg)
				if err != nil {
					return err
				}
				operators, err = listUserOperators(client, creds.UserID)
				return err
			}()
			if sessionID != "" {
				if governedErr != nil {
					return fmt.Errorf("operator stop: %w", governedErr)
				}
				var target *operatorv1.OperatorDocument
				for i := range operators {
					if operators[i].OperatorSessionID == sessionID {
						target = &operators[i]
						break
					}
				}
				if target == nil {
					return fmt.Errorf("operator stop: no operator found with session id %s for the authenticated user", sessionID)
				}
				if err := operator.CheckStoppable(*target); err != nil {
					return err
				}
				var matched []localOperatorProcess
				for _, p := range locals {
					if p.matches(*target) {
						matched = append(matched, p)
					}
				}
				if len(matched) > 1 {
					return fmt.Errorf("operator stop: ambiguous local workers for session %s; use bare 'g8e operator stop' to stop all local workers", sessionID)
				}
				response, err := requestOperatorStop(client, sessionID, reason)
				if len(matched) == 0 {
					if err != nil {
						return err
					}
					if discoveryErr != nil {
						cmd.PrintErrf("Local process discovery unavailable: %v\n", discoveryErr)
					}
					if output.JSONEnabled(cmd) {
						return output.WriteJSON(cmd.OutOrStdout(), response)
					}
					cmd.Printf("Stop requested for operator session %s (operator %s).\n", response.OperatorSessionID, response.OperatorID)
					return nil
				}
				result := stopLocalOperator(cmd, matched[0], response, err, grace)
				if result.Error != "" {
					return fmt.Errorf("operator stop: %s", result.Error)
				}
				if output.JSONEnabled(cmd) {
					return output.WriteJSON(cmd.OutOrStdout(), result)
				}
				cmd.Printf("Stopped local operator PID %d (%s).\n", result.PID, result.Method)
				return nil
			}
			results := make([]operatorStopResult, 0, len(locals))
			var failures []error
			for _, p := range locals {
				response := models.StopOperatorResponse{}
				err := governedErr
				if err == nil {
					var targets []*operatorv1.OperatorDocument
					for _, op := range operators {
						if op.OperatorType == constants.OperatorTypeRemote && p.matches(op) {
							targets = append(targets, op)
						}
					}
					if len(targets) == 1 {
						response, err = requestOperatorStop(client, targets[0].OperatorSessionID, reason)
					} else {
						err = fmt.Errorf("no unique gateway session for local PID %d", p.pid)
					}
				}
				result := stopLocalOperator(cmd, p, response, err, grace)
				results = append(results, result)
				if result.Error != "" {
					failures = append(failures, fmt.Errorf("PID %d: %s", p.pid, result.Error))
				}
				if !output.JSONEnabled(cmd) {
					cmd.Printf("Local operator PID %d: %s\n", p.pid, result.Method)
				}
			}
			if output.JSONEnabled(cmd) {
				if err := output.WriteJSON(cmd.OutOrStdout(), results); err != nil {
					return err
				}
			}
			return errors.Join(failures...)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Reason recorded with the shutdown request")
	cmd.Flags().DurationVar(&grace, "grace", 2*time.Second, "Time to wait for governed shutdown and again after TERM before KILL")
	return cmd
}

func requestOperatorStop(client authcmd.APIClient, sessionID, reason string) (models.StopOperatorResponse, error) {
	body, err := client.Post(constants.APIPaths.OperatorsStop, operator.NewStopRequest(sessionID, reason))
	if err != nil {
		return models.StopOperatorResponse{}, fmt.Errorf("operator stop: request shutdown: %w", err)
	}
	return operator.DecodeStopResponse(body)
}

func dedupeOperatorSessionIDs(args []string) []string {
	seen := make(map[string]struct{}, len(args))
	ordered := make([]string, 0, len(args))
	for _, arg := range args {
		sessionID := strings.TrimSpace(arg)
		if sessionID == "" {
			continue
		}
		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		ordered = append(ordered, sessionID)
	}
	return ordered
}

func listUserOperators(client authcmd.APIClient, userID string) ([]*operatorv1.OperatorDocument, error) {
	resp, err := client.Get(constants.APIPaths.Operators + "?user_id=" + userID)
	if err != nil {
		return nil, fmt.Errorf("list operators: %w", err)
	}
	var slotResp models.OperatorSlotResponse
	if err := json.Unmarshal(resp, &slotResp); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return slotResp.Operators, nil
}

type operatorRunTarget struct {
	OperatorID        string
	OperatorSessionID string
}

func resolveOperatorRunTargets(operators []*operatorv1.OperatorDocument, sessionIDs []string) ([]operatorRunTarget, error) {
	bySession := make(map[string]*operatorv1.OperatorDocument, len(operators))
	for _, op := range operators {
		if op.OperatorSessionID != "" {
			bySession[op.OperatorSessionID] = op
		}
	}

	targets := make([]operatorRunTarget, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		op, ok := bySession[sessionID]
		if !ok {
			return nil, fmt.Errorf("operator run: no operator found with session id %s for the authenticated user", sessionID)
		}
		if op.Status != constants.OperatorStatusActive {
			return nil, fmt.Errorf("operator run: operator session %s is not active (status=%s)", sessionID, op.Status)
		}
		targets = append(targets, operatorRunTarget{
			OperatorID:        op.ID,
			OperatorSessionID: op.OperatorSessionID,
		})
	}
	return targets, nil
}

func dispatchOperatorRun(client authcmd.APIClient, targets []operatorRunTarget, command, cliSessionID string) []operator.RunResult {
	results := make([]operator.RunResult, len(targets))
	var wg sync.WaitGroup
	for index, target := range targets {
		wg.Add(1)
		go func(i int, t operatorRunTarget) {
			defer wg.Done()
			results[i] = dispatchOperatorRunOnce(client, t, command, cliSessionID)
		}(index, target)
	}
	wg.Wait()
	return results
}

func dispatchOperatorRunOnce(client authcmd.APIClient, target operatorRunTarget, command, cliSessionID string) operator.RunResult {
	result := operator.RunResult{
		OperatorSessionID: target.OperatorSessionID,
		OperatorID:        target.OperatorID,
	}

	requestID, err := uuid.NewString()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	request, err := operator.BuildExecuteBashDispatchRequest(target.OperatorSessionID, command, requestID, cliSessionID)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	raw, err := client.Post(constants.APIPaths.OperatorsCommands, request)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	response, err := operator.DecodeDispatchResponse(raw)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.TransactionID = response.TransactionID
	result.Success = response.Success
	if response.Error != "" {
		result.Error = response.Error
	}
	if !response.Success {
		if result.Error == "" {
			result.Error = "dispatch failed"
		}
		return result
	}

	commandResult, err := operator.ParseCommandResult(response)
	if err != nil {
		result.Error = err.Error()
		result.Success = false
		return result
	}

	result.ExitCode = commandResult.GetReturnCode()
	result.Stdout = commandResult.GetStdout()
	result.Stderr = commandResult.GetStderr()
	if commandResult.GetStatus() != operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
		result.Success = false
		if result.Error == "" {
			result.Error = commandResult.GetError()
		}
	}
	return result
}

func writeOperatorRunText(cmd *cobra.Command, results []operator.RunResult) int {
	failures := 0
	for _, result := range results {
		cmd.Printf("Operator session %s (operator %s)\n", result.OperatorSessionID, result.OperatorID)
		if result.TransactionID != "" {
			cmd.Printf("  transaction: %s\n", result.TransactionID)
		}
		if !result.Success {
			failures++
			if result.Error != "" {
				cmd.Printf("  error: %s\n", result.Error)
			} else {
				cmd.Printf("  error: dispatch failed\n")
			}
			continue
		}
		cmd.Printf("  exit: %d\n", result.ExitCode)
		if result.Stdout != "" {
			cmd.Printf("  stdout:\n%s\n", indentOperatorRunOutput(result.Stdout))
		}
		if result.Stderr != "" {
			cmd.Printf("  stderr:\n%s\n", indentOperatorRunOutput(result.Stderr))
		}
		cmd.Println()
	}
	return failures
}

func indentOperatorRunOutput(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "    " + line
	}
	return strings.Join(lines, "\n")
}

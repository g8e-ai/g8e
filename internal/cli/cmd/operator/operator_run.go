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

// defaultOperatorCommandConcurrency bounds bulk run dispatches and local stops.
const defaultOperatorCommandConcurrency = 64

type operatorRunJSON struct {
	Results []operator.RunResult `json:"results"`
	Summary operator.RunSummary  `json:"summary"`
}

type operatorRunClientFactory func(fs.RuntimeFileService, *config.Config, api.ClientOptions) (authcmd.APIClient, error)

func defaultOperatorRunClientFactory(fileSvc fs.RuntimeFileService, cfg *config.Config, opts api.ClientOptions) (authcmd.APIClient, error) {
	return api.NewClientWithOptions(fileSvc, cfg, opts)
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
	var concurrency int
	var allActive bool

	cmd := &cobra.Command{
		Use:   "run (<operator-session-id> [operator-session-id...] | --all-active)",
		Short: "Execute a shell command on one or more remote operators",
		Long: `Execute a governed shell command on remote operator sessions through the gateway.

Provide one or more operator session IDs (from './g8e operator list') and the
command to run with --cmd, or pass --all-active to target every active
operator session owned by the authenticated user. Commands are dispatched in
parallel using the native EXECUTE_BASH gateway path, with at most --concurrency
requests in flight (default: the number of targets, capped at 64).

With --json the output also carries a summary of dispatch latency
(p50/p95/p99/max over successful dispatches) and the wall time of the run.

Requires './g8e auth enroll user'. Each target session must belong to the
authenticated user and be active.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if allActive == (len(args) > 0) {
				return constants.ErrOperatorRunTargetSelection
			}
			if cmd.Flags().Changed("concurrency") && concurrency < 1 {
				return fmt.Errorf("%w: got %d", constants.ErrOperatorRunInvalidConcurrency, concurrency)
			}
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
			if !allActive && len(targetSessionIDs) == 0 {
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

			poolSize := concurrency
			if poolSize < 1 {
				poolSize = defaultOperatorCommandConcurrency
			}
			client, err := clientFactory(fileSvc, cfg, api.ClientOptions{
				Timeout:             time.Duration(timeoutSeconds)*time.Second + 5*time.Second,
				MaxIdleConnsPerHost: poolSize,
			})
			if err != nil {
				return fmt.Errorf("operator run: create API client: %w", err)
			}

			operators, err := listUserOperators(client, creds.UserID)
			if err != nil {
				return fmt.Errorf("operator run: %w", err)
			}
			var targets []operatorRunTarget
			if allActive {
				targets, err = activeOperatorRunTargets(operators)
			} else {
				targets, err = resolveOperatorRunTargets(operators, targetSessionIDs)
			}
			if err != nil {
				return err
			}

			effective := min(poolSize, len(targets))
			started := time.Now()
			results := dispatchOperatorRun(client, targets, command, creds.CLISessionID, effective)
			if output.JSONEnabled(cmd) {
				return output.WriteJSON(cmd.OutOrStdout(), operatorRunJSON{
					Results: results,
					Summary: operator.SummarizeRun(results, time.Since(started), effective),
				})
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
	cmd.Flags().IntVar(&concurrency, "concurrency", 0, fmt.Sprintf("Maximum in-flight dispatches (default: number of targets, capped at %d)", defaultOperatorCommandConcurrency))
	cmd.Flags().BoolVar(&allActive, "all-active", false, "Target every active operator session owned by the authenticated user (cannot be combined with session IDs)")
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
		Use:          "stop [operator-session-id]",
		Short:        "Stop operators, terminating local workers if governed shutdown stalls",
		SilenceUsage: true,
		Long: `Request governed shutdown, then wait briefly for local workers to exit.
Local workers that remain running receive TERM, then KILL. Bulk stops process at most
64 workers concurrently and report each outcome. With no session ID,
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
					if operators[i].OperatorSessionId == sessionID {
						target = operators[i]
						break
					}
				}
				if target == nil {
					return fmt.Errorf("operator stop: no operator found with session id %s for the authenticated user", sessionID)
				}
				if err := operator.CheckStoppable(target); err != nil {
					return err
				}
				var matched []localOperatorProcess
				for _, p := range locals {
					if p.matches(target) {
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
				if result.GovernedError != "" {
					cmd.PrintErrf("PID %d: governed shutdown unavailable (%s).\n", result.PID, result.GovernedError)
				}
				if result.Error != "" {
					return fmt.Errorf("operator stop: %s", result.Error)
				}
				if output.JSONEnabled(cmd) {
					return output.WriteJSON(cmd.OutOrStdout(), result)
				}
				cmd.Printf("Stopped local operator PID %d (%s).\n", result.PID, result.Method)
				return nil
			}
			results := make([]operatorStopResult, len(locals))
			jobs := make(chan int)
			var wg sync.WaitGroup
			for range min(defaultOperatorCommandConcurrency, len(locals)) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := range jobs {
						p := locals[i]
						if err := cmd.Context().Err(); err != nil {
							results[i] = operatorStopResult{PID: p.pid, Method: "failed", Error: err.Error()}
							continue
						}
						response := models.StopOperatorResponse{}
						err := governedErr
						if err == nil {
							var targets []*operatorv1.OperatorDocument
							for _, op := range operators {
								if constants.OperatorType(op.OperatorType) == constants.OperatorTypeRemote && p.matches(op) {
									targets = append(targets, op)
								}
							}
							if len(targets) == 1 {
								response, err = requestOperatorStop(client, targets[0].OperatorSessionId, reason)
							} else {
								err = fmt.Errorf("no unique gateway session for local PID %d", p.pid)
							}
						}
						results[i] = stopLocalOperator(cmd, p, response, err, grace)
					}
				}()
			}
			for i := range locals {
				jobs <- i
			}
			close(jobs)
			wg.Wait()
			var failures []error
			for _, result := range results {
				if result.GovernedError != "" {
					cmd.PrintErrf("PID %d: governed shutdown unavailable (%s).\n", result.PID, result.GovernedError)
				}
				if result.Error != "" {
					failures = append(failures, fmt.Errorf("PID %d: %s", result.PID, result.Error))
				}
				if !output.JSONEnabled(cmd) {
					cmd.Printf("Local operator PID %d: %s\n", result.PID, result.Method)
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
		if op.OperatorSessionId != "" {
			bySession[op.OperatorSessionId] = op
		}
	}

	targets := make([]operatorRunTarget, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		op, ok := bySession[sessionID]
		if !ok {
			return nil, fmt.Errorf("operator run: no operator found with session id %s for the authenticated user", sessionID)
		}
		if constants.OperatorStatus(op.Status) != constants.OperatorStatusActive {
			return nil, fmt.Errorf("operator run: operator session %s is not active (status=%s)", sessionID, op.Status)
		}
		targets = append(targets, operatorRunTarget{
			OperatorID:        op.Id,
			OperatorSessionID: op.OperatorSessionId,
		})
	}
	return targets, nil
}

// activeOperatorRunTargets selects every active operator session in the list.
func activeOperatorRunTargets(operators []*operatorv1.OperatorDocument) ([]operatorRunTarget, error) {
	var targets []operatorRunTarget
	for _, op := range operators {
		if op.OperatorSessionId == "" || constants.OperatorStatus(op.Status) != constants.OperatorStatusActive {
			continue
		}
		targets = append(targets, operatorRunTarget{OperatorID: op.Id, OperatorSessionID: op.OperatorSessionId})
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("operator run: %w", constants.ErrOperatorRunNoActiveOperators)
	}
	return targets, nil
}

// dispatchOperatorRun dispatches to every target with at most concurrency
// requests in flight. Results keep target order.
func dispatchOperatorRun(client authcmd.APIClient, targets []operatorRunTarget, command, cliSessionID string, concurrency int) []operator.RunResult {
	results := make([]operator.RunResult, len(targets))
	if concurrency < 1 {
		concurrency = 1
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(concurrency, len(targets)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = dispatchOperatorRunOnce(client, targets[i], command, cliSessionID)
			}
		}()
	}
	for i := range targets {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func dispatchOperatorRunOnce(client authcmd.APIClient, target operatorRunTarget, command, cliSessionID string) (result operator.RunResult) {
	result = operator.RunResult{
		OperatorSessionID: target.OperatorSessionID,
		OperatorID:        target.OperatorID,
		StartedAt:         time.Now(),
	}
	defer func() { result.DurationMs = operator.DurationMs(time.Since(result.StartedAt)) }()

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

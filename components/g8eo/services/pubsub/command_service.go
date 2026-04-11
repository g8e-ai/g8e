// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/services/system"
	vault "github.com/g8e-ai/g8e/components/g8eo/services/vault"
)

// sentinelVerdict carries the outcome of a pre-execution sentinel analysis.
type sentinelVerdict struct {
	blocked       bool
	blockedResult *models.ExecutionResultsPayload
	blockedEvent  *storage.Event
}

// StatusUpdateInterval is the interval between periodic status updates during long-running commands.
const StatusUpdateInterval = 10 * time.Second

// CommandService owns command execution and cancellation handling.
type CommandService struct {
	config         *config.Config
	logger         *slog.Logger
	execution      *execution.ExecutionService
	results        ResultsPublisher
	sentinel       *sentinel.Sentinel
	vaultWriter    *VaultWriter
	auditVault     *storage.AuditVaultService
	localStore     *storage.LocalStoreService
	rawVault       *storage.RawVaultService
	encryption     *vault.Vault
	ledger         *storage.LedgerService
	historyHandler *storage.HistoryHandler
}

// NewCommandService creates a new CommandService.
func NewCommandService(cfg *config.Config, logger *slog.Logger, execSvc *execution.ExecutionService) *CommandService {
	return &CommandService{
		config:    cfg,
		logger:    logger,
		execution: execSvc,
	}
}

// SetResultsPublisher sets the results publisher for the CommandService.
func (cs *CommandService) SetResultsPublisher(results ResultsPublisher) {
	cs.results = results
}

// SetLocalStoreService sets the local store for the CommandService.
func (cs *CommandService) SetLocalStoreService(ls *storage.LocalStoreService) {
	cs.localStore = ls
	if cs.vaultWriter != nil {
		cs.vaultWriter.localStore = ls
	}
}

// SetRawVaultService sets the raw vault for the CommandService.
func (cs *CommandService) SetRawVaultService(rv *storage.RawVaultService) {
	cs.rawVault = rv
	if cs.vaultWriter != nil {
		cs.vaultWriter.rawVault = rv
	}
}

// SetAuditVaultService sets the audit vault for the CommandService.
func (cs *CommandService) SetAuditVaultService(av *storage.AuditVaultService) {
	cs.auditVault = av
}

// SetLedgerService sets the ledger service for the CommandService.
func (cs *CommandService) SetLedgerService(l *storage.LedgerService) {
	cs.ledger = l
}

// SetHistoryHandler sets the history handler for the CommandService.
func (cs *CommandService) SetHistoryHandler(h *storage.HistoryHandler) {
	cs.historyHandler = h
}

// SetSentinel sets the sentinel for the CommandService.
func (cs *CommandService) SetSentinel(s *sentinel.Sentinel) {
	cs.sentinel = s
}

// HandleExecutionRequest processes an inbound command execution request.
func (cs *CommandService) HandleExecutionRequest(ctx context.Context, msg PubSubCommandMessage) {
	var p models.CommandRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		cs.logger.Error("Failed to decode command payload", "error", err)
		return
	}
	command := p.Command
	if command == "" {
		cs.logger.Warn("Command execution request without command payload")
		return
	}

	justification := p.Justification
	if justification == "" {
		justification = "No justification provided"
	}

	vaultMode := p.SentinelMode
	if vaultMode == "" {
		vaultMode = constants.Status.VaultMode.Raw
	}

	cs.logger.Info("Command execution requested",
		"command", command,
		"justification", justification,
		"sentinel_mode", vaultMode)

	execReq, err := payloadToExecutionRequest(msg)
	if err != nil {
		cs.logger.Error("Failed to create execution request", "error", err)
		return
	}

	if verdict := cs.runSentinelGuard(execReq); verdict.blocked {
		if verdict.blockedEvent != nil && cs.auditVault != nil && cs.auditVault.IsEnabled() {
			if _, err := cs.auditVault.RecordEvent(verdict.blockedEvent); err != nil {
				cs.logger.Warn("Failed to record sentinel blocked event in audit vault", "error", err)
			}
		}
		if cs.results != nil {
			if err := cs.results.PublishExecutionResult(ctx, verdict.blockedResult, msg); err != nil {
				cs.logger.Error("Failed to publish blocked result", "error", err)
			}
		}
		return
	}

	done := make(chan struct{})
	var result *models.ExecutionResultsPayload
	var execErr error
	startTime := time.Now().UTC()

	go func() {
		defer close(done)
		result, execErr = cs.execution.ExecuteCommand(ctx, execReq)
	}()

	statusUpdateCount := cs.runStatusTicker(ctx, execReq, msg, command, startTime, done)

	// runStatusTicker may have returned via ctx.Done() before the execution goroutine
	// finished writing result and execErr. Drain done to guarantee the write is visible.
	<-done

	if execErr != nil {
		result = &models.ExecutionResultsPayload{
			ExecutionID:     execReq.ExecutionID,
			CaseID:          execReq.CaseID,
			TaskID:          execReq.TaskID,
			InvestigationID: execReq.InvestigationID,
			Command:         execReq.Command,
			Args:            execReq.Args,
			Status:          constants.ExecutionStatusFailed,
			ErrorMessage:    system.StringPtr(execErr.Error()),
			ErrorType:       system.StringPtr("execution_error"),
		}
	} else if result == nil {
		result = &models.ExecutionResultsPayload{
			ExecutionID:  execReq.ExecutionID,
			CaseID:       execReq.CaseID,
			TaskID:       execReq.TaskID,
			Status:       constants.ExecutionStatusFailed,
			ErrorMessage: system.StringPtr("execution returned no result"),
			ErrorType:    system.StringPtr("execution_error"),
		}
	} else {
		cs.logger.Info("Command execution completed",
			"command", command,
			"status", result.Status,
			"status_updates", statusUpdateCount,
			"total_elapsed", fmt.Sprintf("%.1fs", time.Since(startTime).Seconds()))
	}

	rawStdoutSize := len(result.Stdout)
	rawStderrSize := len(result.Stderr)

	if cs.vaultWriter != nil {
		taskID := ""
		if result.TaskID != nil {
			taskID = *result.TaskID
		}
		cs.vaultWriter.WriteExecution(executionWriteParams{
			id:              result.ExecutionID,
			command:         cs.execution.BuildCommandString(result.Command, result.Args),
			exitCode:        result.ReturnCode,
			durationMs:      int64(result.DurationSeconds * 1000),
			stdout:          result.Stdout,
			stderr:          result.Stderr,
			stdoutSize:      rawStdoutSize,
			stderrSize:      rawStderrSize,
			caseID:          result.CaseID,
			taskID:          taskID,
			investigationID: result.InvestigationID,
			vaultMode:       vaultMode,
		})
	}

	if cs.sentinel != nil && cs.sentinel.IsEnabled() {
		cs.logger.Info("sentinel.Sentinel scrubbing execution output",
			"execution_id", result.ExecutionID,
			"raw_stdout_size", rawStdoutSize,
			"raw_stderr_size", rawStderrSize)
		result.Stdout = cs.sentinel.ScrubText(result.Stdout)
		result.Stderr = cs.sentinel.ScrubText(result.Stderr)
	}

	if cs.auditVault != nil && cs.auditVault.IsEnabled() {
		event := &storage.Event{
			OperatorSessionID:   cs.config.OperatorSessionId,
			Timestamp:           time.Now().UTC(),
			Type:                storage.EventTypeCmdExec,
			ContentText:         justification,
			CommandRaw:          command,
			CommandExitCode:     result.ReturnCode,
			CommandStdout:       result.Stdout,
			CommandStderr:       result.Stderr,
			ExecutionDurationMs: int64(result.DurationSeconds * 1000),
		}

		if _, err := cs.auditVault.RecordEvent(event); err != nil {
			cs.logger.Warn("Failed to record command event in audit vault", "error", err)
		} else {
			cs.logger.Info("Scrubbed command event recorded in audit vault (LFAA)",
				"execution_id", result.ExecutionID,
				"operator_session_id", cs.config.OperatorSessionId)
		}
	}

	if cs.results != nil {
		if err := cs.results.PublishExecutionResult(ctx, result, msg); err != nil {
			cs.logger.Error("Failed to publish execution result", "error", err)
		}
	}
}

// runSentinelGuard performs pre-execution threat analysis on the command.
// Returns a sentinelVerdict — if blocked is true the caller must publish the blocked result
// and audit event, then return without executing the command.
func (cs *CommandService) runSentinelGuard(execReq *models.ExecutionRequestPayload) sentinelVerdict {
	if cs.sentinel == nil {
		return sentinelVerdict{}
	}

	fullCommand := cs.execution.BuildCommandString(execReq.Command, execReq.Args)
	analysis := cs.sentinel.AnalyzeCommand(fullCommand)

	if !analysis.Safe {
		cs.logger.Error("SENTINEL BLOCKED: Command failed pre-execution threat analysis",
			"threat_level", analysis.ThreatLevel,
			"risk_score", analysis.RiskScore,
			"block_reason", analysis.BlockReason,
			"threat_count", len(analysis.ThreatSignals),
			"command_scrubbed", analysis.Command)

		exitCode := 126
		return sentinelVerdict{
			blocked: true,
			blockedResult: &models.ExecutionResultsPayload{
				ExecutionID:     execReq.ExecutionID,
				CaseID:          execReq.CaseID,
				TaskID:          execReq.TaskID,
				InvestigationID: execReq.InvestigationID,
				Command:         execReq.Command,
				Args:            execReq.Args,
				Status:          constants.ExecutionStatusFailed,
				ReturnCode:      system.IntPtr(126),
				Stderr:          fmt.Sprintf("SENTINEL BLOCKED: %s", analysis.BlockReason),
				ErrorMessage:    system.StringPtr(fmt.Sprintf("Command blocked by sentinel.Sentinel: %s", analysis.BlockReason)),
				ErrorType:       system.StringPtr("sentinel_blocked"),
			},
			blockedEvent: &storage.Event{
				OperatorSessionID: cs.config.OperatorSessionId,
				Timestamp:         time.Now().UTC(),
				Type:              storage.EventTypeCmdExec,
				ContentText:       fmt.Sprintf("SENTINEL BLOCKED: %s (threat_level=%s, risk_score=%d)", analysis.BlockReason, analysis.ThreatLevel, analysis.RiskScore),
				CommandRaw:        analysis.Command,
				CommandExitCode:   &exitCode,
				CommandStderr:     fmt.Sprintf("Blocked by sentinel.Sentinel pre-execution analysis: %s", analysis.BlockReason),
			},
		}
	}

	if len(analysis.ThreatSignals) > 0 {
		cs.logger.Warn("sentinel.Sentinel detected potential threats in command (allowing with review)",
			"threat_level", analysis.ThreatLevel,
			"risk_score", analysis.RiskScore,
			"threat_count", len(analysis.ThreatSignals),
			"requires_approval", analysis.RequiresApproval)
	}

	return sentinelVerdict{}
}

// runStatusTicker drives the periodic status update loop while a command executes.
// It returns the number of status updates sent, which is used in completion logging.
func (cs *CommandService) runStatusTicker(
	ctx context.Context,
	execReq *models.ExecutionRequestPayload,
	msg PubSubCommandMessage,
	command string,
	startTime time.Time,
	done <-chan struct{},
) int {
	ticker := time.NewTicker(StatusUpdateInterval)
	defer ticker.Stop()

	updateCount := 0

	for {
		select {
		case <-done:
			return updateCount

		case <-ticker.C:
			updateCount++
			elapsed := time.Since(startTime).Seconds()

			if cs.results != nil {
				statusUpdate := &ExecutionStatusUpdate{
					ExecutionID:       execReq.ExecutionID,
					CaseID:            execReq.CaseID,
					TaskID:            execReq.TaskID,
					InvestigationID:   execReq.InvestigationID,
					OperatorSessionID: msg.OperatorSessionID,
					Command:           command,
					Status:            constants.ExecutionStatusExecuting,
					ProcessAlive:      true,
					ElapsedSeconds:    elapsed,
					Message:           fmt.Sprintf("Command still executing (%.0fs elapsed)", elapsed),
				}
				if err := cs.results.PublishExecutionStatus(ctx, statusUpdate); err != nil {
					cs.logger.Warn("Failed to publish status update", "error", err)
				} else {
					cs.logger.Info("Execution status update sent",
						"execution_id", execReq.ExecutionID,
						"elapsed", fmt.Sprintf("%.0fs", elapsed),
						"update_count", updateCount)
				}
			}

		case <-ctx.Done():
			cs.logger.Warn("Command execution context cancelled")
			return updateCount
		}
	}
}

// HandleCancelRequest processes an inbound command cancellation request.
func (cs *CommandService) HandleCancelRequest(ctx context.Context, msg PubSubCommandMessage) {
	var p models.CommandCancelRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		cs.logger.Error("Failed to decode cancel payload", "error", err)
		return
	}
	executionID := p.ExecutionID
	if executionID == "" {
		cs.logger.Warn("Cancel request without execution_id")
		return
	}

	cs.logger.Info("Command cancellation requested by user", "execution_id", executionID)

	err := cs.execution.CancelExecution(executionID)

	now := time.Now().UTC()
	var result *models.ExecutionResultsPayload

	if err != nil {
		cs.logger.Warn("Failed to cancel execution (may have already completed)", "error", err)
		result = &models.ExecutionResultsPayload{
			ExecutionID:  executionID,
			CaseID:       msg.CaseID,
			Status:       constants.ExecutionStatusFailed,
			StartTime:    &now,
			ErrorMessage: system.StringPtr(fmt.Sprintf("Cancel failed: %v", err)),
			ErrorType:    system.StringPtr("cancel_failed"),
		}
	} else {
		cs.logger.Info("Command cancelled successfully", "execution_id", executionID)
		result = &models.ExecutionResultsPayload{
			ExecutionID:  executionID,
			CaseID:       msg.CaseID,
			Status:       constants.ExecutionStatusCancelled,
			StartTime:    &now,
			ErrorMessage: system.StringPtr("Command cancelled by user"),
			ErrorType:    system.StringPtr("user_cancelled"),
		}
	}

	if cs.results != nil {
		if err := cs.results.PublishCancellationResult(ctx, result, msg); err != nil {
			cs.logger.Error("Failed to publish cancellation result", "error", err)
		}
	}
}

// payloadToExecutionRequest is a package-level helper shared by CommandService and tests.
func payloadToExecutionRequest(msg PubSubCommandMessage) (*models.ExecutionRequestPayload, error) {
	var p models.CommandRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("failed to decode command payload: %w", err)
	}
	if p.Command == "" {
		return nil, fmt.Errorf("missing command in payload")
	}

	executionID := msg.ID
	if p.ExecutionID != "" {
		executionID = p.ExecutionID
	}

	timeoutSeconds := 300
	if p.TimeoutSeconds > 0 {
		timeoutSeconds = p.TimeoutSeconds
	}

	return &models.ExecutionRequestPayload{
		ExecutionID:     executionID,
		CaseID:          msg.CaseID,
		TaskID:          msg.TaskID,
		InvestigationID: msg.InvestigationID,
		Command:         p.Command,
		TimeoutSeconds:  timeoutSeconds,
		RequestedBy:     "g8e-system",
	}, nil
}

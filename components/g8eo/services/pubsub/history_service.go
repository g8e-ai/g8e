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
	"github.com/g8e-ai/g8e/components/g8eo/services/sqliteutil"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
)

// HistoryService owns log retrieval, execution history, file history, file restore,
type HistoryService struct {
	config         *config.Config
	logger         *slog.Logger
	client         PubSubClient
	localStore     *storage.LocalStoreService
	rawVault       *storage.RawVaultService
	historyHandler *storage.HistoryHandler
}

// NewHistoryService creates a new HistoryService.
func NewHistoryService(cfg *config.Config, logger *slog.Logger, client PubSubClient) *HistoryService {
	return &HistoryService{
		config: cfg,
		logger: logger,
		client: client,
	}
}

// HandleFetchLogsRequest processes a fetch logs request, routing to raw or scrubbed vault.
func (hs *HistoryService) HandleFetchLogsRequest(ctx context.Context, msg PubSubCommandMessage) {
	var flrp models.FetchLogsRequestPayload
	if err := json.Unmarshal(msg.Payload, &flrp); err != nil {
		hs.logger.Error("Failed to decode fetch logs payload", "error", err)
		hs.publishFetchLogsError(ctx, msg, "", "invalid request payload")
		return
	}
	executionID := flrp.ExecutionID
	if executionID == "" {
		hs.logger.Warn("Fetch logs request without execution_id")
		hs.publishFetchLogsError(ctx, msg, "", "missing execution_id in request")
		return
	}

	vaultMode := flrp.SentinelMode
	if vaultMode == "" {
		vaultMode = constants.Status.VaultMode.Raw
	}

	hs.logger.Info("Fetch logs requested (Dual-Vault)",
		"execution_id", executionID,
		"sentinel_mode", vaultMode)

	if vaultMode == constants.Status.VaultMode.Scrubbed {
		hs.handleFetchFromScrubbedVault(ctx, msg, executionID)
	} else {
		hs.handleFetchFromRawVault(ctx, msg, executionID)
	}
}

func (hs *HistoryService) handleFetchFromRawVault(ctx context.Context, msg PubSubCommandMessage, executionID string) {
	if hs.rawVault == nil || !hs.rawVault.IsEnabled() {
		hs.logger.Warn("Raw vault not available, falling back to scrubbed vault")
		hs.handleFetchFromScrubbedVault(ctx, msg, executionID)
		return
	}

	record, err := hs.rawVault.GetRawExecution(executionID)
	if err != nil {
		hs.logger.Error("Failed to retrieve execution from raw vault", "error", err)
		hs.publishFetchLogsError(ctx, msg, executionID, fmt.Sprintf("failed to retrieve execution: %v", err))
		return
	}

	if record == nil {
		hs.logger.Warn("Execution not found in raw vault", "execution_id", executionID)
		hs.publishFetchLogsError(ctx, msg, executionID, "execution not found in raw vault")
		return
	}

	hs.publishFetchLogsResultFromRaw(ctx, msg, record)
}

func (hs *HistoryService) handleFetchFromScrubbedVault(ctx context.Context, msg PubSubCommandMessage, executionID string) {
	if hs.localStore == nil || !hs.localStore.IsEnabled() {
		hs.logger.Warn("Scrubbed vault not available for fetch logs request")
		hs.publishFetchLogsError(ctx, msg, executionID, "scrubbed vault is not enabled on this operator")
		return
	}

	record, err := hs.localStore.GetExecution(executionID)
	if err != nil {
		hs.logger.Error("Failed to retrieve execution from scrubbed vault", "error", err)
		hs.publishFetchLogsError(ctx, msg, executionID, fmt.Sprintf("failed to retrieve execution: %v", err))
		return
	}

	if record == nil {
		hs.logger.Warn("Execution not found in scrubbed vault", "execution_id", executionID)
		hs.publishFetchLogsError(ctx, msg, executionID, "execution not found in scrubbed vault")
		return
	}

	hs.publishFetchLogsResult(ctx, msg, record)
}

func (hs *HistoryService) publishFetchLogsResultFromRaw(ctx context.Context, msg PubSubCommandMessage, record *storage.RawExecutionRecord) {
	hs.publishFetchLogsPayload(ctx, msg, models.FetchLogsResultPayload{
		ExecutionID:       record.ID,
		Command:           record.Command,
		ExitCode:          record.ExitCode,
		DurationMs:        record.DurationMs,
		Stdout:            string(record.StdoutCompressed),
		Stderr:            string(record.StderrCompressed),
		StdoutSize:        record.StdoutSize,
		StderrSize:        record.StderrSize,
		Timestamp:         record.TimestampUTC.Format(time.RFC3339Nano),
		OperatorID:        hs.config.OperatorID,
		OperatorSessionID: hs.config.OperatorSessionId,
		SentinelMode:      constants.Status.VaultMode.Raw,
	})
	hs.logger.Info("Fetch logs result transmitted (Raw Vault)",
		"execution_id", record.ID,
		"stdout_size", record.StdoutSize,
		"stderr_size", record.StderrSize)
}

func (hs *HistoryService) publishFetchLogsResult(ctx context.Context, msg PubSubCommandMessage, record *storage.ExecutionRecord) {
	hs.publishFetchLogsPayload(ctx, msg, models.FetchLogsResultPayload{
		ExecutionID:       record.ID,
		Command:           record.Command,
		ExitCode:          record.ExitCode,
		DurationMs:        record.DurationMs,
		Stdout:            string(record.StdoutCompressed),
		Stderr:            string(record.StderrCompressed),
		StdoutSize:        record.StdoutSize,
		StderrSize:        record.StderrSize,
		Timestamp:         record.TimestampUTC.Format(time.RFC3339Nano),
		OperatorID:        hs.config.OperatorID,
		OperatorSessionID: hs.config.OperatorSessionId,
		SentinelMode:      constants.Status.VaultMode.Scrubbed,
	})
	hs.logger.Info("Fetch logs result transmitted",
		"execution_id", record.ID,
		"stdout_size", record.StdoutSize,
		"stderr_size", record.StderrSize)
}

func (hs *HistoryService) publishFetchLogsPayload(ctx context.Context, msg PubSubCommandMessage, payload models.FetchLogsResultPayload) {
	resultMsg, err := models.NewG8eMessage(
		msg.ID, constants.Event.Operator.FetchLogs.Completed, msg.CaseID,
		hs.config.OperatorID, hs.config.OperatorSessionId, hs.config.SystemFingerprint, payload,
	)
	if err != nil {
		hs.logger.Error("Failed to build fetch logs result message", "error", err)
		return
	}
	resultMsg.TaskID = msg.TaskID
	resultMsg.InvestigationID = msg.InvestigationID
	resultMsg.OperatorSessionID = msg.OperatorSessionID

	data, err := resultMsg.Marshal()
	if err != nil {
		hs.logger.Error("Failed to marshal fetch logs result", "error", err)
		return
	}

	channelName := constants.ResultsChannel(hs.config.OperatorID, hs.config.OperatorSessionId)
	if err := hs.client.Publish(ctx, channelName, data); err != nil {
		hs.logger.Error("Failed to publish fetch logs result", "error", err)
	}
}

func (hs *HistoryService) publishFetchLogsError(ctx context.Context, msg PubSubCommandMessage, executionID, errorMsg string) {
	payload := models.FetchLogsResultPayload{
		ExecutionID:       executionID,
		Error:             errorMsg,
		OperatorID:        hs.config.OperatorID,
		OperatorSessionID: hs.config.OperatorSessionId,
	}

	resultMsg, err := models.NewG8eMessage(
		msg.ID, constants.Event.Operator.FetchLogs.Failed, msg.CaseID,
		hs.config.OperatorID, hs.config.OperatorSessionId, hs.config.SystemFingerprint, payload,
	)
	if err != nil {
		hs.logger.Error("Failed to build fetch logs error message", "error", err)
		return
	}
	resultMsg.TaskID = msg.TaskID
	resultMsg.InvestigationID = msg.InvestigationID
	resultMsg.OperatorSessionID = msg.OperatorSessionID

	data, err := resultMsg.Marshal()
	if err != nil {
		hs.logger.Error("Failed to marshal fetch logs error", "error", err)
		return
	}

	channelName := constants.ResultsChannel(hs.config.OperatorID, hs.config.OperatorSessionId)
	if err := hs.client.Publish(ctx, channelName, data); err != nil {
		hs.logger.Error("Failed to publish fetch logs error", "error", err)
	}
}

// HandleFetchHistoryRequest processes a fetch history request.
func (hs *HistoryService) HandleFetchHistoryRequest(ctx context.Context, msg PubSubCommandMessage) {
	hs.logger.Info("FETCH_HISTORY requested (LFAA)")

	if hs.historyHandler == nil || !hs.historyHandler.IsEnabled() {
		hs.logger.Warn("History handler not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchHistory.Failed,
			"history handler not available on this operator")
		return
	}

	payload, err := hs.historyHandler.HandleFetchHistory(msg.Payload)
	if err != nil {
		hs.logger.Error("History handler failed", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchHistory.Failed,
			fmt.Sprintf("failed to fetch history: %v", err))
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchHistory.Completed, payload)
}

// HandleFetchFileHistoryRequest processes a fetch file history request.
func (hs *HistoryService) HandleFetchFileHistoryRequest(ctx context.Context, msg PubSubCommandMessage) {
	var ffhp models.FetchFileHistoryRequestPayload
	if err := json.Unmarshal(msg.Payload, &ffhp); err != nil {
		hs.logger.Error("Failed to decode fetch file history payload", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed, "invalid request payload")
		return
	}
	hs.logger.Info("FETCH_FILE_HISTORY requested (LFAA)", "file_path", ffhp.FilePath)

	if hs.historyHandler == nil || !hs.historyHandler.IsEnabled() {
		hs.logger.Warn("History handler not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed,
			"history handler not available on this operator")
		return
	}

	payload, err := hs.historyHandler.HandleFetchFileHistory(msg.Payload)
	if err != nil {
		hs.logger.Error("File history handler failed", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed,
			fmt.Sprintf("failed to fetch file history: %v", err))
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Completed, payload)
}

// HandleRestoreFileRequest processes a file restore request.
func (hs *HistoryService) HandleRestoreFileRequest(ctx context.Context, msg PubSubCommandMessage) {
	var rfp models.RestoreFileRequestPayload
	if err := json.Unmarshal(msg.Payload, &rfp); err != nil {
		hs.logger.Error("Failed to decode restore file payload", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed, "invalid request payload")
		return
	}
	hs.logger.Info("RESTORE_FILE requested (LFAA)", "file_path", rfp.FilePath, "commit_hash", rfp.CommitHash)

	if hs.historyHandler == nil || !hs.historyHandler.IsEnabled() {
		hs.logger.Warn("History handler not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed,
			"history handler not available on this operator")
		return
	}

	payload, err := hs.historyHandler.HandleRestoreFile(msg.Payload)
	if err != nil {
		hs.logger.Error("Restore file handler failed", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed,
			fmt.Sprintf("failed to restore file: %v", err))
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Completed, payload)
}

// HandleFetchFileDiffRequest processes a fetch file diff request.
func (hs *HistoryService) HandleFetchFileDiffRequest(ctx context.Context, msg PubSubCommandMessage) {
	hs.logger.Info("FETCH_FILE_DIFF requested (LFAA)")

	if hs.localStore == nil || !hs.localStore.IsEnabled() {
		hs.logger.Warn("Local store (scrubbed vault) not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
			"local storage not available on this operator")
		return
	}

	var ffdp models.FetchFileDiffRequestPayload
	if err := json.Unmarshal(msg.Payload, &ffdp); err != nil {
		hs.logger.Error("Failed to decode fetch file diff payload", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed, "invalid request payload")
		return
	}
	diffID := ffdp.DiffID
	operatorSessionID := ffdp.OperatorSessionID
	filePath := ffdp.FilePath
	limit := ffdp.Limit
	if limit <= 0 {
		limit = 50
	}

	if diffID != "" {
		record, err := hs.localStore.GetFileDiff(diffID)
		if err != nil {
			hs.logger.Error("Failed to fetch file diff", "diff_id", diffID, "error", err)
			publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
				fmt.Sprintf("failed to fetch file diff: %v", err))
			return
		}
		if record == nil {
			publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
				fmt.Sprintf("file diff not found: %s", diffID))
			return
		}

		diffEntry := models.FileDiffEntry{
			ID:                record.ID,
			Timestamp:         sqliteutil.FormatTimestamp(record.TimestampUTC),
			FilePath:          record.FilePath,
			Operation:         record.Operation,
			LedgerHashBefore:  record.LedgerHashBefore,
			LedgerHashAfter:   record.LedgerHashAfter,
			DiffStat:          record.DiffStat,
			DiffContent:       string(record.DiffCompressed),
			DiffSize:          record.DiffSize,
			OperatorSessionID: record.OperatorSessionID,
		}
		publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Completed,
			models.FetchFileDiffResultPayload{
				Success: true,
				Diff:    &diffEntry,
			})
		return
	}

	if operatorSessionID != "" {
		records, err := hs.localStore.GetFileDiffsBySession(operatorSessionID, limit)
		if err != nil {
			hs.logger.Error("Failed to fetch file diffs by session", "operator_session_id", operatorSessionID, "error", err)
			publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
				fmt.Sprintf("failed to fetch file diffs: %v", err))
			return
		}

		diffs := make([]models.FileDiffEntry, 0, len(records))
		for _, record := range records {
			if filePath != "" && record.FilePath != filePath {
				continue
			}
			diffs = append(diffs, models.FileDiffEntry{
				ID:               record.ID,
				Timestamp:        sqliteutil.FormatTimestamp(record.TimestampUTC),
				FilePath:         record.FilePath,
				Operation:        record.Operation,
				LedgerHashBefore: record.LedgerHashBefore,
				LedgerHashAfter:  record.LedgerHashAfter,
				DiffStat:         record.DiffStat,
				DiffSize:         record.DiffSize,
			})
		}

		total := len(diffs)
		publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Completed,
			models.FetchFileDiffResultPayload{
				Success:           true,
				Diffs:             diffs,
				Total:             &total,
				OperatorSessionID: operatorSessionID,
			})
		return
	}

	publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
		"either diff_id or operator_session_id is required")
}

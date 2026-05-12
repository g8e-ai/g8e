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
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/services/sqliteutil"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"google.golang.org/protobuf/proto"
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
	var protoFetch operatorv1.FetchLogsRequested
	if err := proto.Unmarshal(msg.Payload, &protoFetch); err != nil {
		hs.logger.Error("Failed to decode fetch logs payload as protobuf FetchLogsRequested", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "invalid request payload")
		return
	}

	executionID := protoFetch.ExecutionId
	if executionID == "" {
		hs.logger.Warn("Fetch logs request without execution_id")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "missing execution_id in request")
		return
	}

	vaultMode := protoFetch.SentinelMode
	if vaultMode == "" {
		vaultMode = constants.Status.VaultMode.Raw
	}

	hs.logger.Info("Fetch logs requested (Dual-Vault, via Protobuf)",
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
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, fmt.Sprintf("failed to retrieve execution: %v", err))
		return
	}

	if record == nil {
		hs.logger.Warn("Execution not found in raw vault", "execution_id", executionID)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "execution not found in raw vault")
		return
	}

	hs.publishFetchLogsResultFromRaw(ctx, msg, record)
}

func (hs *HistoryService) handleFetchFromScrubbedVault(ctx context.Context, msg PubSubCommandMessage, executionID string) {
	if hs.localStore == nil || !hs.localStore.IsEnabled() {
		hs.logger.Warn("Scrubbed vault not available for fetch logs request")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "scrubbed vault is not enabled on this operator")
		return
	}

	record, err := hs.localStore.GetExecution(executionID)
	if err != nil {
		hs.logger.Error("Failed to retrieve execution from scrubbed vault", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, fmt.Sprintf("failed to retrieve execution: %v", err))
		return
	}

	if record == nil {
		hs.logger.Warn("Execution not found in scrubbed vault", "execution_id", executionID)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "execution not found in scrubbed vault")
		return
	}

	hs.publishFetchLogsResult(ctx, msg, record)
}

func (hs *HistoryService) publishFetchLogsResultFromRaw(ctx context.Context, msg PubSubCommandMessage, record *storage.RawExecutionRecord) {
	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Completed,
		&operatorv1.FetchLogsResult{
			ExecutionId:  record.ID,
			Command:      record.Command,
			ReturnCode:   int32(*record.ExitCode),
			DurationMs:   record.DurationMs,
			Stdout:       string(record.StdoutCompressed),
			Stderr:       string(record.StderrCompressed),
			StdoutSize:   int32(record.StdoutSize),
			StderrSize:   int32(record.StderrSize),
			Timestamp:    record.TimestampUTC.Format(time.RFC3339Nano),
			SentinelMode: constants.Status.VaultMode.Raw,
		})
	hs.logger.Info("Fetch logs result transmitted (Raw Vault)",
		"execution_id", record.ID,
		"stdout_size", record.StdoutSize,
		"stderr_size", record.StderrSize)
}

func (hs *HistoryService) publishFetchLogsResult(ctx context.Context, msg PubSubCommandMessage, record *storage.ExecutionRecord) {
	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Completed,
		&operatorv1.FetchLogsResult{
			ExecutionId:  record.ID,
			Command:      record.Command,
			ReturnCode:   int32(*record.ExitCode),
			DurationMs:   record.DurationMs,
			Stdout:       string(record.StdoutCompressed),
			Stderr:       string(record.StderrCompressed),
			StdoutSize:   int32(record.StdoutSize),
			StderrSize:   int32(record.StderrSize),
			Timestamp:    record.TimestampUTC.Format(time.RFC3339Nano),
			SentinelMode: constants.Status.VaultMode.Scrubbed,
		})
	hs.logger.Info("Fetch logs result transmitted",
		"execution_id", record.ID,
		"stdout_size", record.StdoutSize,
		"stderr_size", record.StderrSize)
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
	var protoFetch operatorv1.FetchFileHistoryRequested
	if err := proto.Unmarshal(msg.Payload, &protoFetch); err != nil {
		hs.logger.Error("Failed to decode fetch file history payload as protobuf FetchFileHistoryRequested", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed, "invalid request payload")
		return
	}
	hs.logger.Info("FETCH_FILE_HISTORY requested (LFAA, via Protobuf)", "file_path", protoFetch.FilePath)

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
	var protoRestore operatorv1.RestoreFileRequested
	if err := proto.Unmarshal(msg.Payload, &protoRestore); err != nil {
		hs.logger.Error("Failed to decode restore file payload as protobuf RestoreFileRequested", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed, "invalid request payload")
		return
	}
	hs.logger.Info("RESTORE_FILE requested (LFAA, via Protobuf)", "file_path", protoRestore.FilePath, "commit_hash", protoRestore.CommitHash)

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
	hs.logger.Info("FETCH_FILE_DIFF requested (LFAA, via Protobuf)")

	if hs.localStore == nil || !hs.localStore.IsEnabled() {
		hs.logger.Warn("Local store (scrubbed vault) not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
			"local storage not available on this operator")
		return
	}

	var protoDiff operatorv1.FetchFileDiffRequested
	if err := proto.Unmarshal(msg.Payload, &protoDiff); err != nil {
		hs.logger.Error("Failed to decode fetch file diff payload as protobuf FetchFileDiffRequested", "error", err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed, "invalid request payload")
		return
	}
	diffID := protoDiff.DiffId
	operatorSessionID := protoDiff.OperatorSessionId
	filePath := protoDiff.FilePath
	limit := protoDiff.Limit
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

		diffEntry := &operatorv1.FileDiffEntry{
			Id:                record.ID,
			Timestamp:         sqliteutil.FormatTimestamp(record.TimestampUTC),
			FilePath:          record.FilePath,
			Operation:         record.Operation,
			LedgerHashBefore:  record.LedgerHashBefore,
			LedgerHashAfter:   record.LedgerHashAfter,
			DiffStat:          record.DiffStat,
			DiffContent:       string(record.DiffCompressed),
			DiffSize:          int32(record.DiffSize),
			OperatorSessionId: record.OperatorSessionID,
		}
		publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Completed,
			&operatorv1.FetchFileDiffResult{
				Success: true,
				Diff:    diffEntry,
			})
		return
	}

	if operatorSessionID != "" {
		records, err := hs.localStore.GetFileDiffsBySession(operatorSessionID, int(limit))
		if err != nil {
			hs.logger.Error("Failed to fetch file diffs by session", "operator_session_id", operatorSessionID, "error", err)
			publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
				fmt.Sprintf("failed to fetch file diffs: %v", err))
			return
		}

		diffs := make([]*operatorv1.FileDiffEntry, 0, len(records))
		for _, record := range records {
			if filePath != "" && record.FilePath != filePath {
				continue
			}
			diffs = append(diffs, &operatorv1.FileDiffEntry{
				Id:               record.ID,
				Timestamp:        sqliteutil.FormatTimestamp(record.TimestampUTC),
				FilePath:         record.FilePath,
				Operation:        record.Operation,
				LedgerHashBefore: record.LedgerHashBefore,
				LedgerHashAfter:  record.LedgerHashAfter,
				DiffStat:         record.DiffStat,
				DiffSize:         int32(record.DiffSize),
			})
		}

		total := int32(len(diffs))
		publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Completed,
			&operatorv1.FetchFileDiffResult{
				Success:           true,
				Diffs:             diffs,
				Total:             total,
				OperatorSessionId: operatorSessionID,
			})
		return
	}

	publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
		"either diff_id or operator_session_id is required")
}

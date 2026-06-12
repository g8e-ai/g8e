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

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/interfaces"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	storage "github.com/g8e-ai/g8e/internal/services/storage"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
)

// HistoryService owns log retrieval, execution history, file history, file restore,
type HistoryService struct {
	config         *config.Config
	logger         *slog.Logger
	client         PubSubClient
	executionVault interfaces.ExecutionVault
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

// HandleFetchLogsRequest processes a fetch logs request from the consolidated execution vault.
func (hs *HistoryService) HandleFetchLogsRequest(ctx context.Context, msg *PubSubCommandMessage) {
	var protoFetch operatorv1.FetchLogsRequested
	if err := proto.Unmarshal(msg.Payload, &protoFetch); err != nil {
		hs.logger.Error("Failed to decode fetch logs payload as protobuf FetchLogsRequested", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "invalid request payload")
		return
	}

	executionID := protoFetch.ExecutionId
	if executionID == "" {
		hs.logger.Warn("Fetch logs request without execution_id")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "missing execution_id in request")
		return
	}

	hs.logger.Info("Fetch logs requested (Consolidated Execution Vault, via Protobuf)",
		"execution_id", executionID)

	hs.handleFetchFromConsolidatedVault(ctx, msg, executionID)
}

func (hs *HistoryService) handleFetchFromConsolidatedVault(ctx context.Context, msg *PubSubCommandMessage, executionID string) {
	if hs.executionVault == nil {
		hs.logger.Warn("Consolidated execution vault not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "consolidated execution vault is not available on this operator")
		return
	}

	record, err := hs.executionVault.GetExecution(ctx, executionID)
	if err != nil {
		hs.logger.Error("Failed to retrieve execution from consolidated vault", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, fmt.Sprintf("failed to retrieve execution: %v", err))
		return
	}

	if record == nil {
		hs.logger.Warn("Execution not found in consolidated vault", "execution_id", executionID)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "execution not found in consolidated vault")
		return
	}

	hs.publishFetchLogsResult(ctx, msg, record)
}

func (hs *HistoryService) publishFetchLogsResult(ctx context.Context, msg *PubSubCommandMessage, record *models.ExecutionRecord) {
	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Completed,
		&operatorv1.FetchLogsResult{
			ExecutionId: record.ID,
			Command:     record.Command,
			ReturnCode:  int32(*record.ExitCode), //nolint:gosec // exit codes are 0-255
			DurationMs:  record.DurationMs,
			Stdout:      string(record.StdoutCompressed),
			Stderr:      string(record.StderrCompressed),
			StdoutSize:  int32(record.StdoutSize), //nolint:gosec // bounded by storage limits
			StderrSize:  int32(record.StderrSize), //nolint:gosec // bounded by storage limits
			Timestamp:   record.TimestampUTC.Format(time.RFC3339Nano),
		})
	hs.logger.Info("Fetch logs result transmitted (Consolidated Execution Vault)",
		"execution_id", record.ID,
		"stdout_size", record.StdoutSize,
		"stderr_size", record.StderrSize)
}

// HandleFetchHistoryRequest processes a fetch history request.
func (hs *HistoryService) HandleFetchHistoryRequest(ctx context.Context, msg *PubSubCommandMessage) {
	hs.logger.Info("FETCH_HISTORY requested (LFAA)")

	if hs.historyHandler == nil {
		hs.logger.Warn("History handler not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchHistory.Failed,
			"history handler not available on this operator")
		return
	}

	payload, err := hs.historyHandler.HandleFetchHistory(msg.Payload)
	if err != nil {
		hs.logger.Error("History handler failed", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchHistory.Failed,
			fmt.Sprintf("failed to fetch history: %v", err))
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchHistory.Completed, payload)
}

// HandleFetchFileHistoryRequest processes a fetch file history request.
func (hs *HistoryService) HandleFetchFileHistoryRequest(ctx context.Context, msg *PubSubCommandMessage) {
	var protoFetch operatorv1.FetchFileHistoryRequested
	if err := proto.Unmarshal(msg.Payload, &protoFetch); err != nil {
		hs.logger.Error("Failed to decode fetch file history payload as protobuf FetchFileHistoryRequested", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed, "invalid request payload")
		return
	}
	hs.logger.Info("FETCH_FILE_HISTORY requested (LFAA, via Protobuf)", "file_path", protoFetch.FilePath)

	if hs.historyHandler == nil {
		hs.logger.Warn("History handler not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed,
			"history handler not available on this operator")
		return
	}

	payload, err := hs.historyHandler.HandleFetchFileHistory(msg.Payload)
	if err != nil {
		hs.logger.Error("File history handler failed", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed,
			fmt.Sprintf("failed to fetch file history: %v", err))
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Completed, payload)
}

// HandleRestoreFileRequest processes a file restore request.
func (hs *HistoryService) HandleRestoreFileRequest(ctx context.Context, msg *PubSubCommandMessage) {
	var protoRestore operatorv1.RestoreFileRequested
	if err := proto.Unmarshal(msg.Payload, &protoRestore); err != nil {
		hs.logger.Error("Failed to decode restore file payload as protobuf RestoreFileRequested", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed, "invalid request payload")
		return
	}
	hs.logger.Info("RESTORE_FILE requested (LFAA, via Protobuf)", "file_path", protoRestore.FilePath, "commit_hash", protoRestore.CommitHash)

	if hs.historyHandler == nil {
		hs.logger.Warn("History handler not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed,
			"history handler not available on this operator")
		return
	}

	payload, err := hs.historyHandler.HandleRestoreFile(msg.Payload)
	if err != nil {
		hs.logger.Error("Restore file handler failed", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed,
			fmt.Sprintf("failed to restore file: %v", err))
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Completed, payload)
}

// HandleFetchFileDiffRequest processes a fetch file diff request.
func (hs *HistoryService) HandleFetchFileDiffRequest(ctx context.Context, msg *PubSubCommandMessage) {
	hs.logger.Info("FETCH_FILE_DIFF requested (LFAA, via Protobuf)")

	if hs.executionVault == nil {
		hs.logger.Warn("Execution vault not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
			"execution vault not available on this operator")
		return
	}

	var protoDiff operatorv1.FetchFileDiffRequested
	if err := proto.Unmarshal(msg.Payload, &protoDiff); err != nil {
		hs.logger.Error("Failed to decode fetch file diff payload as protobuf FetchFileDiffRequested", string(constants.ConnectionStateError), err)
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
		record, err := hs.executionVault.GetFileDiff(ctx, diffID)
		if err != nil {
			hs.logger.Error("Failed to fetch file diff", "diff_id", diffID, string(constants.ConnectionStateError), err)
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
			DiffSize:          int32(record.DiffSize), //nolint:gosec // bounded by file size
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
		records, err := hs.executionVault.GetFileDiffsBySession(ctx, operatorSessionID, int(limit))
		if err != nil {
			hs.logger.Error("Failed to fetch file diffs by session", "operator_session_id", operatorSessionID, string(constants.ConnectionStateError), err)
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
				DiffSize:         int32(record.DiffSize), //nolint:gosec // bounded by file size
			})
		}

		total := int32(len(diffs)) //nolint:gosec // bounded by query limits
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

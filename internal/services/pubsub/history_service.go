// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/scrubbing"
	storage "github.com/g8e-ai/g8e/v2/internal/services/storage"
	"github.com/g8e-ai/g8e/v2/internal/timesvc"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/proto"
)

// HistoryService owns log retrieval, execution history, file history, file restore,
type HistoryService struct {
	config         *config.Config
	logger         *slog.Logger
	client         PubSubClient
	executionVault storage.ExecutionVault
	historyHandler *storage.HistoryHandler
	auditStore     AuditEventRecorder // *storage.SQLAuditStore - optional for observed-state content evidence
	scrubbing      *scrubbing.ScrubbingService
}

// NewHistoryService creates a new HistoryService.
func NewHistoryService(cfg *config.Config, logger *slog.Logger, client PubSubClient) *HistoryService {
	return &HistoryService{
		config: cfg,
		logger: logger,
		client: client,
	}
}

// SetAuditStore sets the audit store for observed-state content evidence.
func (hs *HistoryService) SetAuditStore(auditStore AuditEventRecorder) {
	hs.auditStore = auditStore
}

// SetScrubbingService sets the scrubbing service for observed-state content evidence.
func (hs *HistoryService) SetScrubbingService(scrubbingSvc *scrubbing.ScrubbingService) {
	hs.scrubbing = scrubbingSvc
}

// HandleFetchLogsRequest processes a fetch logs request from the consolidated execution vault.
func (hs *HistoryService) HandleFetchLogsRequest(ctx context.Context, msg *PubSubCommandMessage) {
	var protoFetch operatorv1.FetchLogsRequested
	if err := proto.Unmarshal(msg.Payload, &protoFetch); err != nil {
		hs.logger.Error("Failed to decode fetch logs payload as protobuf FetchLogsRequested", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "invalid request payload", hs.auditStore, hs.scrubbing)
		return
	}

	executionID := protoFetch.ExecutionId
	if executionID == "" {
		hs.logger.Warn("Fetch logs request without execution_id")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "missing execution_id in request", hs.auditStore, hs.scrubbing)
		return
	}

	hs.logger.Info("Fetch logs requested (Consolidated Execution Vault, via Protobuf)",
		"execution_id", executionID)

	hs.handleFetchFromConsolidatedVault(ctx, msg, executionID)
}

func (hs *HistoryService) handleFetchFromConsolidatedVault(ctx context.Context, msg *PubSubCommandMessage, executionID string) {
	if hs.executionVault == nil {
		hs.logger.Warn("Consolidated execution vault not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "consolidated execution vault is not available on this operator", hs.auditStore, hs.scrubbing)
		return
	}

	record, err := hs.executionVault.GetExecution(ctx, executionID)
	if err != nil {
		hs.logger.Error("Failed to retrieve execution from consolidated vault", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, fmt.Sprintf("failed to retrieve execution: %v", err), hs.auditStore, hs.scrubbing)
		return
	}

	if record == nil {
		hs.logger.Warn("Execution not found in consolidated vault", "execution_id", executionID)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Failed, "execution not found in consolidated vault", hs.auditStore, hs.scrubbing)
		return
	}

	hs.publishFetchLogsResult(ctx, msg, record)
}

func (hs *HistoryService) publishFetchLogsResult(ctx context.Context, msg *PubSubCommandMessage, record *models.ExecutionRecord) {
	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchLogs.Completed,
		&operatorv1.FetchLogsResult{
			ExecutionId: record.ID,
			Command:     record.Command,
			ReturnCode:  int32(record.ExitCode), //nolint:gosec // exit codes are 0-255
			DurationMs:  record.DurationMs,
			Stdout:      string(record.StdoutCompressed),
			Stderr:      string(record.StderrCompressed),
			StdoutSize:  int32(record.StdoutSize), //nolint:gosec // bounded by storage limits
			StderrSize:  int32(record.StderrSize), //nolint:gosec // bounded by storage limits
			Timestamp:   timesvc.FormatTimestamp(record.TimestampUTC),
		}, hs.auditStore, hs.scrubbing)
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
			"history handler not available on this operator", hs.auditStore, hs.scrubbing)
		return
	}

	payload, err := hs.historyHandler.HandleFetchHistory(msg.Payload)
	if err != nil {
		hs.logger.Error("History handler failed", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchHistory.Failed,
			fmt.Sprintf("failed to fetch history: %v", err), hs.auditStore, hs.scrubbing)
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchHistory.Completed, payload, hs.auditStore, hs.scrubbing)
}

// HandleFetchFileHistoryRequest processes a fetch file history request.
func (hs *HistoryService) HandleFetchFileHistoryRequest(ctx context.Context, msg *PubSubCommandMessage) {
	var protoFetch operatorv1.FetchFileHistoryRequested
	if err := proto.Unmarshal(msg.Payload, &protoFetch); err != nil {
		hs.logger.Error("Failed to decode fetch file history payload as protobuf FetchFileHistoryRequested", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed, "invalid request payload", hs.auditStore, hs.scrubbing)
		return
	}
	hs.logger.Info("FETCH_FILE_HISTORY requested (LFAA, via Protobuf)", "file_path", protoFetch.FilePath)

	if hs.historyHandler == nil {
		hs.logger.Warn("History handler not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed,
			"history handler not available on this operator", hs.auditStore, hs.scrubbing)
		return
	}

	payload, err := hs.historyHandler.HandleFetchFileHistory(msg.Payload)
	if err != nil {
		hs.logger.Error("File history handler failed", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Failed,
			fmt.Sprintf("failed to fetch file history: %v", err), hs.auditStore, hs.scrubbing)
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileHistory.Completed, payload, hs.auditStore, hs.scrubbing)
}

// HandleRestoreFileRequest processes a file restore request.
func (hs *HistoryService) HandleRestoreFileRequest(ctx context.Context, msg *PubSubCommandMessage) {
	var protoRestore operatorv1.RestoreFileRequested
	if err := proto.Unmarshal(msg.Payload, &protoRestore); err != nil {
		hs.logger.Error("Failed to decode restore file payload as protobuf RestoreFileRequested", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed, "invalid request payload", hs.auditStore, hs.scrubbing)
		return
	}
	hs.logger.Info("RESTORE_FILE requested (LFAA, via Protobuf)", "file_path", protoRestore.FilePath, "commit_hash", protoRestore.CommitHash)

	if hs.historyHandler == nil {
		hs.logger.Warn("History handler not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed,
			"history handler not available on this operator", hs.auditStore, hs.scrubbing)
		return
	}

	payload, err := hs.historyHandler.HandleRestoreFile(msg.Payload)
	if err != nil {
		hs.logger.Error("Restore file handler failed", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Failed,
			fmt.Sprintf("failed to restore file: %v", err), hs.auditStore, hs.scrubbing)
		return
	}

	publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.RestoreFile.Completed, payload, hs.auditStore, hs.scrubbing)
}

// HandleFetchFileDiffRequest processes a fetch file diff request.
func (hs *HistoryService) HandleFetchFileDiffRequest(ctx context.Context, msg *PubSubCommandMessage) {
	hs.logger.Info("FETCH_FILE_DIFF requested (LFAA, via Protobuf)")

	if hs.executionVault == nil {
		hs.logger.Warn("Execution vault not available")
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
			"execution vault not available on this operator", hs.auditStore, hs.scrubbing)
		return
	}

	var protoDiff operatorv1.FetchFileDiffRequested
	if err := proto.Unmarshal(msg.Payload, &protoDiff); err != nil {
		hs.logger.Error("Failed to decode fetch file diff payload as protobuf FetchFileDiffRequested", string(constants.ConnectionStateError), err)
		publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed, "invalid request payload", hs.auditStore, hs.scrubbing)
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
				fmt.Sprintf("failed to fetch file diff: %v", err), hs.auditStore, hs.scrubbing)
			return
		}
		if record == nil {
			publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
				fmt.Sprintf("file diff not found: %s", diffID), hs.auditStore, hs.scrubbing)
			return
		}

		diffEntry := &operatorv1.FileDiffEntry{
			Id:                record.ID,
			Timestamp:         timesvc.FormatTimestamp(record.TimestampUTC),
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
			}, hs.auditStore, hs.scrubbing)
		return
	}

	if operatorSessionID != "" {
		records, err := hs.executionVault.GetFileDiffsBySession(ctx, operatorSessionID, int(limit))
		if err != nil {
			hs.logger.Error("Failed to fetch file diffs by session", "operator_session_id", operatorSessionID, string(constants.ConnectionStateError), err)
			publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
				fmt.Sprintf("failed to fetch file diffs: %v", err), hs.auditStore, hs.scrubbing)
			return
		}

		diffs := make([]*operatorv1.FileDiffEntry, 0, len(records))
		for _, record := range records {
			if filePath != "" && record.FilePath != filePath {
				continue
			}
			diffs = append(diffs, &operatorv1.FileDiffEntry{
				Id:                record.ID,
				Timestamp:         timesvc.FormatTimestamp(record.TimestampUTC),
				FilePath:          record.FilePath,
				Operation:         record.Operation,
				LedgerHashBefore:  record.LedgerHashBefore,
				LedgerHashAfter:   record.LedgerHashAfter,
				DiffStat:          record.DiffStat,
				DiffSize:          int32(record.DiffSize), //nolint:gosec // bounded by file size
				OperatorSessionId: operatorSessionID,
			})
		}

		total := int32(len(diffs)) //nolint:gosec // bounded by query limits
		publishLFAATypedResponseTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Completed,
			&operatorv1.FetchFileDiffResult{
				Success:           true,
				Diffs:             diffs,
				Total:             total,
				OperatorSessionId: operatorSessionID,
			}, hs.auditStore, hs.scrubbing)
		return
	}

	publishLFAAErrorTo(ctx, hs.client, hs.config, hs.logger, msg, constants.Event.Operator.FetchFileDiff.Failed,
		"either diff_id or operator_session_id is required", hs.auditStore, hs.scrubbing)
}

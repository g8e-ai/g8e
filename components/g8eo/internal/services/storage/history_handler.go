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

package storage

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/components/g8eo/internal/protocol/proto/operatorv1"
	"google.golang.org/protobuf/proto"
)

type HistoryHandler struct {
	auditVault *AuditVaultService
	ledger     *LedgerService
	logger     *slog.Logger
}

type fetchHistoryRequest struct {
	OperatorSessionID string `json:"operator_session_id"`
	Limit             int    `json:"limit"`
	Offset            int    `json:"offset"`
}

type fetchFileHistoryRequest struct {
	FilePath string `json:"file_path"`
	Limit    int    `json:"limit"`
}

type restoreFileRequest struct {
	FilePath          string `json:"file_path"`
	CommitHash        string `json:"commit_hash"`
	OperatorSessionID string `json:"operator_session_id"`
}

func NewHistoryHandler(auditVault *AuditVaultService, ledger *LedgerService, logger *slog.Logger) *HistoryHandler {
	return &HistoryHandler{
		auditVault: auditVault,
		ledger:     ledger,
		logger:     logger,
	}
}

func (hh *HistoryHandler) HandleFetchHistory(requestJSON []byte) (*operatorv1.FetchHistoryResult, error) {
	var request operatorv1.FetchHistoryRequested
	if err := proto.Unmarshal(requestJSON, &request); err != nil {
		return hh.fetchHistoryError(fmt.Errorf("invalid request format as protobuf FetchHistoryRequested: %w", err).Error()), nil
	}

	if request.OperatorSessionId == "" {
		return hh.fetchHistoryError("operator_session_id is required"), nil
	}

	limit := int(request.Limit)
	if limit <= 0 {
		limit = 50
	}

	offset := int(request.Offset)

	hh.logger.Info("Handling FETCH_HISTORY request (via Protobuf)",
		"operator_session_id", request.OperatorSessionId,
		"limit", limit,
		"offset", offset)

	session, err := hh.auditVault.GetSession(request.OperatorSessionId)
	if err != nil {
		return hh.fetchHistoryError(fmt.Errorf("failed to get session: %w", err).Error()), nil
	}

	events, err := hh.auditVault.GetEvents(request.OperatorSessionId, limit, offset)
	if err != nil {
		return hh.fetchHistoryError(fmt.Errorf("failed to get events: %w", err).Error()), nil
	}

	result := &operatorv1.FetchHistoryResult{
		Success:           true,
		OperatorSessionId: request.OperatorSessionId,
		Events:            make([]*operatorv1.AuditEvent, 0, len(events)),
		Limit:             int32(limit),
		Offset:            int32(offset),
	}

	if session != nil {
		result.WebSession = &operatorv1.AuditWebSession{
			Id:           session.ID,
			Title:        session.Title,
			CreatedAt:    session.CreatedAt.UTC().Format(time.RFC3339Nano),
			UserIdentity: session.UserIdentity,
		}
	}

	for _, event := range events {
		auditEvent := &operatorv1.AuditEvent{
			Id:                  event.ID,
			OperatorSessionId:   event.OperatorSessionID,
			Timestamp:           sqliteutil.FormatTimestamp(event.Timestamp),
			Type:                string(event.Type),
			ContentText:         event.ContentText,
			CommandRaw:          event.CommandRaw,
			CommandExitCode:     0, // Will be set below if not nil
			CommandStdout:       event.CommandStdout,
			CommandStderr:       event.CommandStderr,
			ExecutionDurationMs: event.ExecutionDurationMs,
			StoredLocally:       event.StoredLocally,
			StdoutTruncated:     event.StdoutTruncated,
			StderrTruncated:     event.StderrTruncated,
			FileMutations:       []*operatorv1.AuditFileMutation{},
		}
		if event.CommandExitCode != nil {
			auditEvent.CommandExitCode = int32(*event.CommandExitCode)
		}

		if event.Type == EventTypeFileMutation {
			mutations, err := hh.auditVault.GetFileMutations(event.ID)
			if err != nil {
				hh.logger.Warn("Failed to get file mutations", "event_id", event.ID, "error", err)
			} else {
				for _, m := range mutations {
					auditEvent.FileMutations = append(auditEvent.FileMutations, &operatorv1.AuditFileMutation{
						Id:               m.ID,
						Filepath:         m.Filepath,
						Operation:        string(m.Operation),
						LedgerHashBefore: m.LedgerHashBefore,
						LedgerHashAfter:  m.LedgerHashAfter,
						DiffStat:         m.DiffStat,
					})
				}
			}
		}

		result.Events = append(result.Events, auditEvent)
	}

	result.Total = int32(len(result.Events))
	return result, nil
}

func (hh *HistoryHandler) HandleFetchFileHistory(requestJSON []byte) (*operatorv1.FetchFileHistoryResult, error) {
	var request operatorv1.FetchFileHistoryRequested
	if err := proto.Unmarshal(requestJSON, &request); err != nil {
		return hh.fetchFileHistoryError(fmt.Errorf("invalid request format as protobuf FetchFileHistoryRequested: %w", err).Error()), nil
	}

	if request.FilePath == "" {
		return hh.fetchFileHistoryError("file_path is required"), nil
	}

	limit := int(request.Limit)
	if limit <= 0 {
		limit = 50
	}

	hh.logger.Info("Handling FETCH_FILE_HISTORY request (via Protobuf)",
		"file_path", request.FilePath,
		"limit", limit,
		"operator_session_id", request.OperatorSessionId)

	history, err := hh.ledger.GetFileHistory(request.FilePath, limit, request.OperatorSessionId)
	if err != nil {
		return hh.fetchFileHistoryError(fmt.Errorf("failed to get file history: %w", err).Error()), nil
	}

	result := &operatorv1.FetchFileHistoryResult{
		Success:  true,
		FilePath: request.FilePath,
		History:  make([]*operatorv1.FileHistoryEntry, 0, len(history)),
	}

	for _, entry := range history {
		result.History = append(result.History, &operatorv1.FileHistoryEntry{
			CommitHash: entry.CommitHash,
			Timestamp:  sqliteutil.FormatTimestamp(entry.Timestamp),
			Message:    entry.Message,
		})
	}

	return result, nil
}

func (hh *HistoryHandler) HandleRestoreFile(requestJSON []byte) (*operatorv1.RestoreFileResult, error) {
	var request operatorv1.RestoreFileRequested
	if err := proto.Unmarshal(requestJSON, &request); err != nil {
		return hh.restoreFileError(fmt.Errorf("invalid request format as protobuf RestoreFileRequested: %w", err).Error()), nil
	}

	if request.FilePath == "" {
		return hh.restoreFileError("file_path is required"), nil
	}
	if request.CommitHash == "" {
		return hh.restoreFileError("commit_hash is required"), nil
	}

	sessionID := request.OperatorSessionId
	if sessionID == "" {
		return hh.restoreFileError("operator_session_id is required"), nil
	}

	hh.logger.Info("Handling RESTORE_FILE request (via Protobuf)",
		"file_path", request.FilePath,
		"commit_hash", request.CommitHash)

	if err := hh.ledger.RestoreFileFromCommit(request.FilePath, request.CommitHash, sessionID); err != nil {
		return hh.restoreFileError(fmt.Errorf("failed to restore file: %w", err).Error()), nil
	}

	return &operatorv1.RestoreFileResult{
		Success:    true,
		FilePath:   request.FilePath,
		CommitHash: request.CommitHash,
	}, nil
}

func (hh *HistoryHandler) GetFileAtCommit(filePath, commitHash, operatorSessionID string) (string, error) {
	return hh.ledger.GetFileAtCommit(filePath, commitHash, operatorSessionID)
}

func (hh *HistoryHandler) IsEnabled() bool {
	return hh != nil && hh.auditVault != nil && hh.auditVault.IsEnabled()
}

func (hh *HistoryHandler) fetchHistoryError(message string) *operatorv1.FetchHistoryResult {
	return &operatorv1.FetchHistoryResult{Success: false, Error: message}
}

func (hh *HistoryHandler) fetchFileHistoryError(message string) *operatorv1.FetchFileHistoryResult {
	return &operatorv1.FetchFileHistoryResult{Success: false, Error: message}
}

func (hh *HistoryHandler) restoreFileError(message string) *operatorv1.RestoreFileResult {
	return &operatorv1.RestoreFileResult{Success: false, Error: message}
}

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
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/services/sqliteutil"
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

func (hh *HistoryHandler) HandleFetchHistory(requestJSON []byte) (*models.FetchHistoryResultPayload, error) {
	var request fetchHistoryRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		return hh.fetchHistoryError(fmt.Errorf("invalid request format: %w", err).Error()), nil
	}

	if request.OperatorSessionID == "" {
		return hh.fetchHistoryError("operator_session_id is required"), nil
	}

	if request.Limit <= 0 {
		request.Limit = 50
	}

	hh.logger.Info("Handling FETCH_HISTORY request",
		"operator_session_id", request.OperatorSessionID,
		"limit", request.Limit,
		"offset", request.Offset)

	session, err := hh.auditVault.GetSession(request.OperatorSessionID)
	if err != nil {
		return hh.fetchHistoryError(fmt.Errorf("failed to get session: %w", err).Error()), nil
	}

	events, err := hh.auditVault.GetEvents(request.OperatorSessionID, request.Limit, request.Offset)
	if err != nil {
		return hh.fetchHistoryError(fmt.Errorf("failed to get events: %w", err).Error()), nil
	}

	result := &models.FetchHistoryResultPayload{
		Success:           true,
		OperatorSessionID: request.OperatorSessionID,
		Events:            make([]models.AuditEvent, 0, len(events)),
		Limit:             request.Limit,
		Offset:            request.Offset,
	}

	if session != nil {
		result.WebSession = &models.AuditWebSession{
			ID:           session.ID,
			Title:        session.Title,
			CreatedAt:    session.CreatedAt.UTC().Format(time.RFC3339Nano),
			UserIdentity: session.UserIdentity,
		}
	}

	for _, event := range events {
		auditEvent := models.AuditEvent{
			ID:                  event.ID,
			OperatorSessionID:   event.OperatorSessionID,
			Timestamp:           sqliteutil.FormatTimestamp(event.Timestamp),
			Type:                string(event.Type),
			ContentText:         event.ContentText,
			CommandRaw:          event.CommandRaw,
			CommandExitCode:     event.CommandExitCode,
			CommandStdout:       event.CommandStdout,
			CommandStderr:       event.CommandStderr,
			ExecutionDurationMs: event.ExecutionDurationMs,
			StoredLocally:       event.StoredLocally,
			StdoutTruncated:     event.StdoutTruncated,
			StderrTruncated:     event.StderrTruncated,
			FileMutations:       []models.AuditFileMutation{},
		}

		if event.Type == EventTypeFileMutation {
			mutations, err := hh.auditVault.GetFileMutations(event.ID)
			if err != nil {
				hh.logger.Warn("Failed to get file mutations", "event_id", event.ID, "error", err)
			} else {
				for _, m := range mutations {
					auditEvent.FileMutations = append(auditEvent.FileMutations, models.AuditFileMutation{
						ID:               m.ID,
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

	result.Total = len(result.Events)
	return result, nil
}

func (hh *HistoryHandler) HandleFetchFileHistory(requestJSON []byte) (*models.FetchFileHistoryResultPayload, error) {
	var request fetchFileHistoryRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		return hh.fetchFileHistoryError(fmt.Errorf("invalid request format: %w", err).Error()), nil
	}

	if request.FilePath == "" {
		return hh.fetchFileHistoryError("file_path is required"), nil
	}

	if request.Limit <= 0 {
		request.Limit = 50
	}

	hh.logger.Info("Handling FETCH_FILE_HISTORY request",
		"file_path", request.FilePath,
		"limit", request.Limit)

	history, err := hh.ledger.GetFileHistory(request.FilePath, request.Limit)
	if err != nil {
		return hh.fetchFileHistoryError(fmt.Errorf("failed to get file history: %w", err).Error()), nil
	}

	result := &models.FetchFileHistoryResultPayload{
		Success:  true,
		FilePath: request.FilePath,
		History:  make([]models.FileHistoryEntry, 0, len(history)),
	}

	for _, entry := range history {
		result.History = append(result.History, models.FileHistoryEntry{
			CommitHash: entry.CommitHash,
			Timestamp:  sqliteutil.FormatTimestamp(entry.Timestamp),
			Message:    entry.Message,
		})
	}

	return result, nil
}

func (hh *HistoryHandler) HandleRestoreFile(requestJSON []byte) (*models.RestoreFileResultPayload, error) {
	var request restoreFileRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		return hh.restoreFileError(fmt.Errorf("invalid request format: %w", err).Error()), nil
	}

	if request.FilePath == "" {
		return hh.restoreFileError("file_path is required"), nil
	}
	if request.CommitHash == "" {
		return hh.restoreFileError("commit_hash is required"), nil
	}
	if request.OperatorSessionID == "" {
		return hh.restoreFileError("operator_session_id is required"), nil
	}

	hh.logger.Info("Handling RESTORE_FILE request",
		"file_path", request.FilePath,
		"commit_hash", request.CommitHash,
		"operator_session_id", request.OperatorSessionID)

	if err := hh.ledger.RestoreFileFromCommit(request.FilePath, request.CommitHash, request.OperatorSessionID); err != nil {
		return hh.restoreFileError(fmt.Errorf("failed to restore file: %w", err).Error()), nil
	}

	return &models.RestoreFileResultPayload{
		Success:    true,
		FilePath:   request.FilePath,
		CommitHash: request.CommitHash,
	}, nil
}

func (hh *HistoryHandler) GetFileAtCommit(filePath, commitHash string) (string, error) {
	return hh.ledger.GetFileAtCommit(filePath, commitHash)
}

func (hh *HistoryHandler) IsEnabled() bool {
	return hh != nil && hh.auditVault != nil && hh.auditVault.IsEnabled()
}

func (hh *HistoryHandler) fetchHistoryError(message string) *models.FetchHistoryResultPayload {
	return &models.FetchHistoryResultPayload{Success: false, Error: message}
}

func (hh *HistoryHandler) fetchFileHistoryError(message string) *models.FetchFileHistoryResultPayload {
	return &models.FetchFileHistoryResultPayload{Success: false, Error: message}
}

func (hh *HistoryHandler) restoreFileError(message string) *models.RestoreFileResultPayload {
	return &models.RestoreFileResultPayload{Success: false, Error: message}
}

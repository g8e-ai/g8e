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
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/config"
	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	execution "github.com/g8e-ai/g8e/components/vsa/services/execution"
	"github.com/g8e-ai/g8e/components/vsa/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/vsa/services/storage"
	system "github.com/g8e-ai/g8e/components/vsa/services/system"
)

// FileOpsService owns file edit, fs list, and fs read handling.
type FileOpsService struct {
	config      *config.Config
	logger      *slog.Logger
	fileEdit    *execution.FileEditService
	fsList      *execution.FsListService
	results     ResultsPublisher
	sentinel    *sentinel.Sentinel
	vaultWriter *VaultWriter
	auditVault  *storage.AuditVaultService
	ledger      *storage.LedgerService
	client      PubSubClient
}

// NewFileOpsService creates a new FileOpsService.
func NewFileOpsService(cfg *config.Config, logger *slog.Logger, fileEditSvc *execution.FileEditService, client PubSubClient) *FileOpsService {
	return &FileOpsService{
		config:   cfg,
		logger:   logger,
		fileEdit: fileEditSvc,
		fsList:   execution.NewFsListService(cfg.WorkDir, logger),
		client:   client,
	}
}

// HandleFileEditRequest processes an inbound file edit request.
func (fs *FileOpsService) HandleFileEditRequest(ctx context.Context, msg PubSubCommandMessage) {
	var fp models.FileEditRequestPayload
	if err := json.Unmarshal(msg.Payload, &fp); err != nil {
		fs.logger.Error("Failed to decode file edit payload", "error", err)
		return
	}
	filePath := fp.FilePath
	operation := fp.Operation

	vaultMode := fp.SentinelMode
	if vaultMode == "" {
		vaultMode = constants.Status.VaultMode.Raw
	}

	fs.logger.Info("File edit requested",
		"file_path", filePath,
		"operation", operation,
		"sentinel_mode", vaultMode)

	editReq, err := payloadToFileEditRequest(msg)
	if err != nil {
		fs.logger.Error("Failed to create file edit request", "error", err)
		return
	}

	var result *models.FileEditResult
	if fs.sentinel != nil {
		content := ""
		if editReq.Content != nil {
			content = *editReq.Content
		}
		analysis := fs.sentinel.AnalyzeFileEdit(editReq.FilePath, string(editReq.Operation), content)

		if !analysis.Safe {
			fs.logger.Error("SENTINEL BLOCKED: File operation failed pre-execution threat analysis",
				"threat_level", analysis.ThreatLevel,
				"risk_score", analysis.RiskScore,
				"block_reason", analysis.BlockReason,
				"threat_count", len(analysis.ThreatSignals),
				"file_path_scrubbed", analysis.FilePath,
				"operation", analysis.Operation,
				"is_critical_file", analysis.IsCriticalSystemFile)

			result = &models.FileEditResult{
				ExecutionID:     editReq.ExecutionID,
				CaseID:          editReq.CaseID,
				TaskID:          editReq.TaskID,
				InvestigationID: editReq.InvestigationID,
				Operation:       editReq.Operation,
				FilePath:        editReq.FilePath,
				Status:          constants.ExecutionStatusFailed,
				ErrorMessage:    system.StringPtr(fmt.Sprintf("File operation blocked by sentinel.Sentinel: %s", analysis.BlockReason)),
				ErrorType:       system.StringPtr("sentinel_blocked"),
			}

			if fs.auditVault != nil && fs.auditVault.IsEnabled() {
				exitCode := 126
				if _, err := fs.auditVault.RecordEvent(&storage.Event{
					OperatorSessionID: fs.config.OperatorSessionId,
					Timestamp:         time.Now().UTC(),
					Type:              storage.EventTypeFileMutation,
					ContentText:       fmt.Sprintf("SENTINEL BLOCKED FILE OP: %s on %s - %s (threat_level=%s, risk_score=%d)", editReq.Operation, analysis.FilePath, analysis.BlockReason, analysis.ThreatLevel, analysis.RiskScore),
					CommandRaw:        fmt.Sprintf("file_%s: %s", editReq.Operation, analysis.FilePath),
					CommandExitCode:   &exitCode,
					CommandStderr:     fmt.Sprintf("Blocked by sentinel.Sentinel: %s", analysis.BlockReason),
				}); err != nil {
					fs.logger.Warn("Failed to record sentinel blocked file op in audit vault", "error", err)
				}
			}

			if fs.results != nil {
				if err := fs.results.PublishFileEditResult(ctx, result, msg); err != nil {
					fs.logger.Error("Failed to publish blocked file edit result", "error", err)
				}
			}
			return
		}

		if len(analysis.ThreatSignals) > 0 || analysis.IsCriticalSystemFile {
			fs.logger.Warn("sentinel.Sentinel detected potential threats in file operation (allowing with review)",
				"threat_level", analysis.ThreatLevel,
				"risk_score", analysis.RiskScore,
				"threat_count", len(analysis.ThreatSignals),
				"is_critical_file", analysis.IsCriticalSystemFile,
				"requires_approval", analysis.RequiresApproval)
		}
	}

	result, err = fs.fileEdit.ExecuteFileEdit(ctx, editReq)
	if err != nil {
		result = &models.FileEditResult{
			ExecutionID:     editReq.ExecutionID,
			CaseID:          editReq.CaseID,
			TaskID:          editReq.TaskID,
			InvestigationID: editReq.InvestigationID,
			Operation:       editReq.Operation,
			FilePath:        editReq.FilePath,
			Status:          constants.ExecutionStatusFailed,
			ErrorMessage:    system.StringPtr(err.Error()),
			ErrorType:       system.StringPtr("execution_error"),
		}
	}
	if result == nil {
		result = &models.FileEditResult{
			ExecutionID:     editReq.ExecutionID,
			CaseID:          editReq.CaseID,
			TaskID:          editReq.TaskID,
			InvestigationID: editReq.InvestigationID,
			Operation:       editReq.Operation,
			FilePath:        editReq.FilePath,
			Status:          constants.ExecutionStatusFailed,
			ErrorMessage:    system.StringPtr("file edit returned no result"),
			ErrorType:       system.StringPtr("execution_error"),
		}
	}

	commandStr := fmt.Sprintf("file_%s: %s", operation, filePath)

	var exitCode *int
	if result.Status == constants.ExecutionStatusCompleted {
		zero := 0
		exitCode = &zero
	} else {
		one := 1
		exitCode = &one
	}

	var stdout string
	if result.Content != nil {
		stdout = *result.Content
	}

	var stderr string
	if result.ErrorMessage != nil {
		stderr = *result.ErrorMessage
	}

	if fs.vaultWriter != nil {
		taskID := ""
		if result.TaskID != nil {
			taskID = *result.TaskID
		}
		fs.vaultWriter.WriteExecution(executionWriteParams{
			id:              result.ExecutionID,
			command:         commandStr,
			exitCode:        exitCode,
			durationMs:      int64(result.DurationSeconds * 1000),
			stdout:          stdout,
			stderr:          stderr,
			stdoutSize:      len(stdout),
			stderrSize:      len(stderr),
			caseID:          result.CaseID,
			taskID:          taskID,
			investigationID: result.InvestigationID,
			vaultMode:       vaultMode,
		})
	}

	if fs.auditVault != nil && fs.auditVault.IsEnabled() && operation != "read" {
		event := &storage.Event{
			OperatorSessionID:   fs.config.OperatorSessionId,
			Timestamp:           time.Now().UTC(),
			Type:                storage.EventTypeFileMutation,
			ContentText:         fmt.Sprintf("File %s: %s", operation, filePath),
			CommandRaw:          fmt.Sprintf("file_%s %s", operation, filePath),
			ExecutionDurationMs: int64(result.DurationSeconds * 1000),
		}

		if result.Status == constants.ExecutionStatusCompleted {
			zero := 0
			event.CommandExitCode = &zero
		} else {
			one := 1
			event.CommandExitCode = &one
			if result.ErrorMessage != nil {
				event.CommandStderr = *result.ErrorMessage
			}
		}

		eventID, err := fs.auditVault.RecordEvent(event)
		if err != nil {
			fs.logger.Warn("Failed to record file mutation event in audit vault", "error", err)
		} else {
			fs.logger.Info("File mutation event recorded in audit vault (LFAA)",
				"event_id", eventID,
				"file_path", filePath,
				"operation", operation)

			if fs.ledger != nil && result.Status == constants.ExecutionStatusCompleted {
				var mutationOp storage.FileMutationOperation
				switch operation {
				case "write":
					mutationOp = storage.FileMutationWrite
				case "delete":
					mutationOp = storage.FileMutationDelete
				default:
					mutationOp = storage.FileMutationWrite
				}

				mutation := &storage.FileMutationLog{
					EventID:   eventID,
					Filepath:  filePath,
					Operation: mutationOp,
				}

				if err := fs.auditVault.RecordFileMutation(mutation); err != nil {
					fs.logger.Warn("Failed to record file mutation in audit log", "error", err)
				}

				if fs.vaultWriter != nil {
					fs.vaultWriter.StoreFileDiffFromLedger(filePath, operation, fmt.Sprintf("%d", eventID), fs.config.OperatorSessionId, result.CaseID, fs.ledger)
				}
			}
		}
	}

	if fs.sentinel != nil && fs.sentinel.IsEnabled() {
		if result.Content != nil {
			scrubbed := fs.sentinel.ScrubText(*result.Content)
			result.Content = &scrubbed
		}
		if result.ErrorMessage != nil {
			scrubbed := fs.sentinel.ScrubText(*result.ErrorMessage)
			result.ErrorMessage = &scrubbed
		}
	}

	if fs.results != nil {
		if err := fs.results.PublishFileEditResult(ctx, result, msg); err != nil {
			fs.logger.Error("Failed to publish file edit result", "error", err)
		}
	}
}

// HandleFsListRequest processes an inbound filesystem list request.
func (fs *FileOpsService) HandleFsListRequest(ctx context.Context, msg PubSubCommandMessage) {
	var p models.FsListRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		fs.logger.Error("Failed to decode fs list payload", "error", err)
		return
	}
	path := p.Path
	if path == "" {
		path = "."
	}

	fs.logger.Info("File system list requested", "path", path)

	fsListReq, err := payloadToFsListRequest(msg)
	if err != nil {
		fs.logger.Error("Failed to create fs list request", "error", err)
		return
	}

	result, err := fs.fsList.ExecuteFsList(ctx, fsListReq)
	if err != nil {
		result = &models.FsListResult{
			ExecutionID:     fsListReq.ExecutionID,
			CaseID:          fsListReq.CaseID,
			TaskID:          fsListReq.TaskID,
			InvestigationID: fsListReq.InvestigationID,
			Path:            fsListReq.Path,
			Status:          constants.ExecutionStatusFailed,
			ErrorMessage:    system.StringPtr(err.Error()),
			ErrorType:       system.StringPtr("execution_error"),
		}
	}

	if fs.vaultWriter != nil {
		commandStr := fmt.Sprintf("fs_list: %s", path)

		var exitCode *int
		if result.Status == constants.ExecutionStatusCompleted {
			zero := 0
			exitCode = &zero
		} else {
			one := 1
			exitCode = &one
		}

		var stdout string
		if result.Entries != nil {
			if entriesJSON, jsonErr := json.Marshal(result.Entries); jsonErr == nil {
				stdout = string(entriesJSON)
			}
		}

		var stderr string
		if result.ErrorMessage != nil {
			stderr = *result.ErrorMessage
		}

		taskID := ""
		if result.TaskID != nil {
			taskID = *result.TaskID
		}

		fs.vaultWriter.WriteExecution(executionWriteParams{
			id:              result.ExecutionID,
			command:         commandStr,
			exitCode:        exitCode,
			durationMs:      int64(result.DurationSeconds * 1000),
			stdout:          stdout,
			stderr:          stderr,
			stdoutSize:      len(stdout),
			stderrSize:      len(stderr),
			caseID:          result.CaseID,
			taskID:          taskID,
			investigationID: result.InvestigationID,
		})
		fs.logger.Info("FS list stored locally",
			"execution_id", result.ExecutionID,
			"path", path,
			"entries", result.TotalCount)
	}

	if fs.results != nil {
		if err := fs.results.PublishFsListResult(ctx, result, msg); err != nil {
			fs.logger.Error("Failed to publish fs list result", "error", err)
		}
	}
}

// HandleFsReadRequest processes an inbound filesystem read request.
func (fs *FileOpsService) HandleFsReadRequest(ctx context.Context, msg PubSubCommandMessage) {
	var p models.FsReadRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		fs.logger.Error("Failed to decode fs read payload", "error", err)
		fs.publishLFAAError(ctx, msg, constants.Event.Operator.FsRead.Failed, "invalid request payload")
		return
	}
	if p.Path == "" {
		fs.logger.Warn("Fs read request without path")
		fs.publishLFAAError(ctx, msg, constants.Event.Operator.FsRead.Failed, "missing path in request")
		return
	}

	maxSize := p.MaxSize
	if maxSize <= 0 {
		maxSize = 102400
	}

	fs.logger.Info("File system read requested", "path", p.Path)

	requestID := msg.ID
	if p.ExecutionID != "" {
		requestID = p.ExecutionID
	}

	start := time.Now()

	// SECURITY: Use io.LimitReader to prevent OOM when reading massive files.
	// Open file first to check size and then read with limit.
	file, err := os.Open(p.Path)
	if err != nil {
		duration := time.Since(start).Seconds()
		errMsg := err.Error()
		errType := "read_error"
		payload := models.FsReadResultPayload{
			ExecutionID:       requestID,
			Path:              p.Path,
			Status:            constants.ExecutionStatusFailed,
			SizeBytes:         0,
			Truncated:         false,
			DurationSeconds:   duration,
			OperatorID:        fs.config.OperatorID,
			OperatorSessionID: fs.config.OperatorSessionId,
			ErrorMessage:      &errMsg,
			ErrorType:         &errType,
		}
		fs.publishLFAATypedResponse(ctx, msg, constants.Event.Operator.FsRead.Failed, payload)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		duration := time.Since(start).Seconds()
		errMsg := err.Error()
		errType := "read_error"
		payload := models.FsReadResultPayload{
			ExecutionID:       requestID,
			Path:              p.Path,
			Status:            constants.ExecutionStatusFailed,
			SizeBytes:         0,
			Truncated:         false,
			DurationSeconds:   duration,
			OperatorID:        fs.config.OperatorID,
			OperatorSessionID: fs.config.OperatorSessionId,
			ErrorMessage:      &errMsg,
			ErrorType:         &errType,
		}
		fs.publishLFAATypedResponse(ctx, msg, constants.Event.Operator.FsRead.Failed, payload)
		return
	}

	actualSize := fileInfo.Size()

	// Read with limit to prevent OOM
	// We read up to maxSize bytes.
	readLimit := int64(maxSize)
	data, err := io.ReadAll(io.LimitReader(file, readLimit))

	duration := time.Since(start).Seconds()

	if err != nil {
		errMsg := err.Error()
		errType := "read_error"
		payload := models.FsReadResultPayload{
			ExecutionID:       requestID,
			Path:              p.Path,
			Status:            constants.ExecutionStatusFailed,
			SizeBytes:         0,
			Truncated:         false,
			DurationSeconds:   duration,
			OperatorID:        fs.config.OperatorID,
			OperatorSessionID: fs.config.OperatorSessionId,
			ErrorMessage:      &errMsg,
			ErrorType:         &errType,
		}
		fs.publishLFAATypedResponse(ctx, msg, constants.Event.Operator.FsRead.Failed, payload)
		return
	}

	truncated := actualSize > readLimit
	content := string(data)

	if fs.sentinel != nil && fs.sentinel.IsEnabled() {
		content = fs.sentinel.ScrubText(content)
	}

	payload := models.FsReadResultPayload{
		ExecutionID:       requestID,
		Path:              p.Path,
		Status:            constants.ExecutionStatusCompleted,
		Content:           content,
		SizeBytes:         len(data),
		Truncated:         truncated,
		DurationSeconds:   duration,
		OperatorID:        fs.config.OperatorID,
		OperatorSessionID: fs.config.OperatorSessionId,
	}
	fs.publishLFAATypedResponse(ctx, msg, constants.Event.Operator.FsRead.Completed, payload)
}

func (fs *FileOpsService) publishLFAATypedResponse(ctx context.Context, msg PubSubCommandMessage, eventType string, payload interface{}) {
	publishLFAATypedResponseTo(ctx, fs.client, fs.config, fs.logger, msg, eventType, payload)
}

func (fs *FileOpsService) publishLFAAError(ctx context.Context, msg PubSubCommandMessage, eventType, errorMsg string) {
	publishLFAAErrorTo(ctx, fs.client, fs.config, fs.logger, msg, eventType, errorMsg)
}

// payloadToFileEditRequest is a package-level helper shared by FileOpsService and tests.
func payloadToFileEditRequest(msg PubSubCommandMessage) (*models.FileEditRequest, error) {
	var p models.FileEditRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("failed to decode file edit payload: %w", err)
	}
	if p.FilePath == "" {
		return nil, fmt.Errorf("missing file_path in payload")
	}
	if p.Operation == "" {
		return nil, fmt.Errorf("missing operation in payload")
	}

	requestID := msg.ID
	if p.ExecutionID != "" {
		requestID = p.ExecutionID
	}

	justification := p.Justification
	if justification == "" {
		justification = "pub/sub command request"
	}

	req := &models.FileEditRequest{
		ExecutionID:     requestID,
		CaseID:          msg.CaseID,
		TaskID:          msg.TaskID,
		InvestigationID: msg.InvestigationID,
		Operation:       models.FileEditOperation(p.Operation),
		FilePath:        p.FilePath,
		RequestedBy:     "vso-system",
		Justification:   justification,
		CreateBackup:    p.CreateBackup,
		CreateIfMissing: p.CreateIfMissing,
	}

	if p.Content != "" {
		req.Content = system.StringPtr(p.Content)
	}
	if p.OldContent != "" {
		req.OldContent = system.StringPtr(p.OldContent)
	}
	if p.NewContent != "" {
		req.NewContent = system.StringPtr(p.NewContent)
	}
	if p.InsertContent != "" {
		req.InsertContent = system.StringPtr(p.InsertContent)
	}
	if p.InsertPosition != nil {
		req.InsertPosition = p.InsertPosition
	}
	if p.StartLine != nil {
		req.StartLine = p.StartLine
	}
	if p.EndLine != nil {
		req.EndLine = p.EndLine
	}
	if p.PatchContent != "" {
		req.PatchContent = system.StringPtr(p.PatchContent)
	}

	return req, nil
}

// payloadToFsListRequest is a package-level helper shared by FileOpsService and tests.
func payloadToFsListRequest(msg PubSubCommandMessage) (*models.FsListRequest, error) {
	var p models.FsListRequestPayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("failed to decode fs list payload: %w", err)
	}

	path := p.Path
	if path == "" {
		path = "."
	}

	requestID := msg.ID
	if p.ExecutionID != "" {
		requestID = p.ExecutionID
	}

	maxEntries := p.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 100
	}

	return &models.FsListRequest{
		ExecutionID:     requestID,
		CaseID:          msg.CaseID,
		TaskID:          msg.TaskID,
		InvestigationID: msg.InvestigationID,
		Path:            path,
		MaxDepth:        p.MaxDepth,
		MaxEntries:      maxEntries,
		RequestedBy:     "vso-system",
	}, nil
}

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

	"github.com/g8e-ai/g8e/services/g8eo/internal/config"
	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	"github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	execution "github.com/g8e-ai/g8e/services/g8eo/internal/services/execution"
	"github.com/g8e-ai/g8e/services/g8eo/internal/services/sentinel"
	storage "github.com/g8e-ai/g8e/services/g8eo/internal/services/storage"
	system "github.com/g8e-ai/g8e/services/g8eo/internal/services/system"
	"google.golang.org/protobuf/proto"
)

// FileOpsService owns file edit, fs list, and fs read handling.
type FileOpsService struct {
	config      *config.Config
	logger      *slog.Logger
	fileEdit    *execution.FileEditService
	fsList      *execution.FsListService
	fsGrep      *execution.FsGrepService
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
		fsGrep:   execution.NewFsGrepService(cfg.WorkDir, logger),
		client:   client,
	}
}

// HandleFileEditRequest processes an inbound file edit request.
func (fs *FileOpsService) HandleFileEditRequest(ctx context.Context, msg PubSubCommandMessage) {
	var protoEdit operatorv1.FileEditRequested
	if err := proto.Unmarshal(msg.Payload, &protoEdit); err != nil {
		fs.logger.Error("Failed to decode file edit payload as protobuf FileEditRequested", "error", err)
		return
	}

	filePath := protoEdit.FilePath
	operation := protoEdit.Operation

	vaultMode := constants.VaultMode(protoEdit.SentinelMode)
	if vaultMode == "" {
		vaultMode = constants.Status.VaultMode.Raw
	}

	fs.logger.Info("File edit requested (via Protobuf)",
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
		analysis := fs.sentinel.AnalyzeFileEdit(editReq.FilePath, editReq.Operation, content)

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
				Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
				ErrorMessage:    system.StringPtr(fmt.Sprintf("File operation blocked by sentinel.Sentinel: %s", analysis.BlockReason)),
				ErrorType:       system.StringPtr("sentinel_blocked"),
			}

			if fs.auditVault != nil && fs.auditVault.IsEnabled() {
				exitCode := 126
				if _, err := fs.auditVault.RecordEvent(&storage.Event{
					OperatorSessionID: fs.config.OperatorSessionId,
					Timestamp:         time.Now().UTC(),
					Type:              constants.Event.Operator.FileEdit.Completed,
					ContentText:       fmt.Sprintf("SENTINEL BLOCKED FILE OP: %s on %s - %s (threat_level=%s, risk_score=%d)", editReq.Operation, analysis.FilePath, analysis.BlockReason, analysis.ThreatLevel, analysis.RiskScore),
					CommandRaw:        fmt.Sprintf("file_%s: %s", editReq.Operation, analysis.FilePath),
					CommandExitCode:   &exitCode,
					CommandStderr:     fmt.Sprintf("Blocked by sentinel.Sentinel: %s", analysis.BlockReason),
				}); err != nil {
					fs.logger.Warn("Failed to record sentinel blocked file op in audit vault", "error", err)
				}
			}

			if fs.results != nil {
				protoResult := &operatorv1.FileEditResult{
					ExecutionId:  editReq.ExecutionID,
					Status:       operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
					FilePath:     editReq.FilePath,
					Operation:    string(editReq.Operation),
					ErrorMessage: fmt.Sprintf("File operation blocked by sentinel.Sentinel: %s", analysis.BlockReason),
					ErrorType:    "sentinel_blocked",
				}
				if err := fs.results.PublishFileEditResult(ctx, protoResult, msg); err != nil {
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
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
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
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ErrorMessage:    system.StringPtr("file edit returned no result"),
			ErrorType:       system.StringPtr("execution_error"),
		}
	}

	commandStr := fmt.Sprintf("file_%s: %s", operation, filePath)

	var exitCode *int
	if result.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
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
			Type:                constants.Event.Operator.FileEdit.Completed,
			ContentText:         fmt.Sprintf("File %s: %s", operation, filePath),
			CommandRaw:          fmt.Sprintf("file_%s %s", operation, filePath),
			ExecutionDurationMs: int64(result.DurationSeconds * 1000),
		}

		if result.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
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

			if fs.ledger != nil && result.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
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
		protoResult := &operatorv1.FileEditResult{
			ExecutionId:     result.ExecutionID,
			Status:          result.Status,
			FilePath:        result.FilePath,
			Operation:       string(result.Operation),
			DurationSeconds: float32(result.DurationSeconds),
		}
		if result.ErrorMessage != nil {
			protoResult.ErrorMessage = *result.ErrorMessage
		}
		if result.ErrorType != nil {
			protoResult.ErrorType = *result.ErrorType
		}
		if result.BytesWritten != nil {
			protoResult.BytesWritten = *result.BytesWritten
		}
		if result.LinesChanged != nil {
			protoResult.LinesChanged = int32(*result.LinesChanged)
		}
		if result.BackupPath != nil {
			protoResult.BackupPath = *result.BackupPath
		}
		if result.Content != nil {
			protoResult.Content = *result.Content
			protoResult.StdoutSize = int32(len(*result.Content))
		}
		if result.ErrorMessage != nil && result.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED {
			protoResult.StderrSize = int32(len(*result.ErrorMessage))
		}

		if err := fs.results.PublishFileEditResult(ctx, protoResult, msg); err != nil {
			fs.logger.Error("Failed to publish file edit result", "error", err)
		}
	}
}

// HandleFsListRequest processes an inbound filesystem list request.
func (fs *FileOpsService) HandleFsListRequest(ctx context.Context, msg PubSubCommandMessage) {
	var protoList operatorv1.FsListRequested
	if err := proto.Unmarshal(msg.Payload, &protoList); err != nil {
		fs.logger.Error("Failed to decode fs list payload as protobuf FsListRequested", "error", err)
		return
	}

	path := protoList.Path
	if path == "" {
		path = "."
	}

	vaultMode := constants.VaultMode(protoList.SentinelMode)
	if vaultMode == "" {
		vaultMode = constants.Status.VaultMode.Raw
	}

	fs.logger.Info("File system list requested (via Protobuf)", "path", path, "sentinel_mode", vaultMode)

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
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ErrorMessage:    system.StringPtr(err.Error()),
			ErrorType:       system.StringPtr("execution_error"),
		}
	}

	if fs.vaultWriter != nil {
		commandStr := fmt.Sprintf("fs_list: %s", path)

		var exitCode *int
		if result.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
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
		protoResult := &operatorv1.FsListResult{
			ExecutionId:     result.ExecutionID,
			Status:          result.Status,
			Path:            result.Path,
			Truncated:       result.Truncated,
			TotalCount:      int32(result.TotalCount),
			DurationSeconds: float32(result.DurationSeconds),
		}
		if result.ErrorMessage != nil {
			protoResult.ErrorMessage = *result.ErrorMessage
		}
		if result.ErrorType != nil {
			protoResult.ErrorType = *result.ErrorType
		}
		if result.Entries != nil {
			protoResult.Entries = make([]*operatorv1.FsEntry, len(result.Entries))
			for i, entry := range result.Entries {
				protoResult.Entries[i] = &operatorv1.FsEntry{
					Name:    entry.Name,
					IsDir:   entry.IsDir,
					Size:    entry.Size,
					ModTime: entry.ModTime,
				}
				// Skip mapping Mode for now if it's a string in models and int32 in proto,
				// or parse it if possible. For now, let's just omit or set to 0.
			}
		}

		if err := fs.results.PublishFsListResult(ctx, protoResult, msg); err != nil {
			fs.logger.Error("Failed to publish fs list result", "error", err)
		}
	}
}

// HandleFsGrepRequest processes an inbound filesystem grep request.
func (fs *FileOpsService) HandleFsGrepRequest(ctx context.Context, msg PubSubCommandMessage) {
	var protoGrep operatorv1.FsGrepRequested
	if err := proto.Unmarshal(msg.Payload, &protoGrep); err != nil {
		fs.logger.Error("Failed to decode fs grep payload as protobuf FsGrepRequested", "error", err)
		return
	}

	path := protoGrep.Path
	if path == "" {
		path = "."
	}

	vaultMode := constants.VaultMode(protoGrep.SentinelMode)
	if vaultMode == "" {
		vaultMode = constants.Status.VaultMode.Raw
	}

	fs.logger.Info("File system grep requested (via Protobuf)", "path", path, "pattern", protoGrep.Pattern, "sentinel_mode", vaultMode)

	fsGrepReq, err := payloadToFsGrepRequest(msg)
	if err != nil {
		fs.logger.Error("Failed to create fs grep request", "error", err)
		return
	}

	result, err := fs.fsGrep.ExecuteFsGrep(ctx, fsGrepReq)
	if err != nil {
		result = &models.FsGrepResult{
			ExecutionID:     fsGrepReq.ExecutionID,
			CaseID:          fsGrepReq.CaseID,
			TaskID:          fsGrepReq.TaskID,
			InvestigationID: fsGrepReq.InvestigationID,
			Path:            fsGrepReq.Path,
			Pattern:         fsGrepReq.Pattern,
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			ErrorMessage:    system.StringPtr(err.Error()),
			ErrorType:       system.StringPtr("execution_error"),
		}
	}

	if fs.vaultWriter != nil {
		commandStr := fmt.Sprintf("fs_grep: %s (pattern: %s)", path, protoGrep.Pattern)

		var exitCode *int
		if result.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED {
			zero := 0
			exitCode = &zero
		} else {
			one := 1
			exitCode = &one
		}

		var stdout string
		if result.Matches != nil {
			if matchesJSON, jsonErr := json.Marshal(result.Matches); jsonErr == nil {
				stdout = string(matchesJSON)
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
		fs.logger.Info("FS grep stored locally",
			"execution_id", result.ExecutionID,
			"path", path,
			"matches", result.TotalMatches)
	}

	if fs.results != nil {
		protoResult := &operatorv1.FsGrepResult{
			ExecutionId:     result.ExecutionID,
			Status:          result.Status,
			Path:            result.Path,
			TotalMatches:    int32(result.TotalMatches),
			Truncated:       result.Truncated,
			DurationSeconds: float32(result.DurationSeconds),
		}
		if result.ErrorMessage != nil {
			protoResult.ErrorMessage = *result.ErrorMessage
		}
		if result.ErrorType != nil {
			protoResult.ErrorType = *result.ErrorType
		}
		if result.Matches != nil {
			protoResult.Matches = make([]*operatorv1.FsGrepMatch, len(result.Matches))
			for i, match := range result.Matches {
				protoResult.Matches[i] = &operatorv1.FsGrepMatch{
					Path:       match.Path,
					LineNumber: int32(match.LineNumber),
					Content:    match.Content,
					Before:     match.Before,
					After:      match.After,
				}
			}
		}

		if err := fs.results.PublishFsGrepResult(ctx, protoResult, msg); err != nil {
			fs.logger.Error("Failed to publish fs grep result", "error", err)
		}
	}
}

// HandleFsReadRequest processes an inbound filesystem read request.
func (fs *FileOpsService) HandleFsReadRequest(ctx context.Context, msg PubSubCommandMessage) {
	var protoRead operatorv1.FsReadRequested
	if err := proto.Unmarshal(msg.Payload, &protoRead); err != nil {
		fs.logger.Error("Failed to decode fs read payload as protobuf FsReadRequested", "error", err)
		fs.publishLFAAError(ctx, msg, constants.Event.Operator.FsRead.Failed, "invalid request payload")
		return
	}

	if protoRead.Path == "" {
		fs.logger.Warn("Fs read request without path")
		fs.publishLFAAError(ctx, msg, constants.Event.Operator.FsRead.Failed, "missing path in request")
		return
	}

	maxSize := protoRead.MaxSize
	if maxSize <= 0 {
		maxSize = 102400
	}

	vaultMode := constants.VaultMode(protoRead.SentinelMode)
	if vaultMode == "" {
		vaultMode = constants.Status.VaultMode.Raw
	}

	fs.logger.Info("File system read requested (via Protobuf)", "path", protoRead.Path, "sentinel_mode", vaultMode)

	requestID := executionIDFromMessage(msg)

	start := time.Now()

	// SECURITY: Use io.LimitReader to prevent OOM when reading massive files.
	// Open file first to check size and then read with limit.
	file, err := os.Open(protoRead.Path)
	if err != nil {
		duration := time.Since(start).Seconds()
		payload := &operatorv1.FsReadResult{
			ExecutionId:     requestID,
			Path:            protoRead.Path,
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			SizeBytes:       0,
			Truncated:       false,
			DurationSeconds: float32(duration),
			ErrorMessage:    err.Error(),
			ErrorType:       "read_error",
		}
		fs.publishLFAATypedResponse(ctx, msg, constants.Event.Operator.FsRead.Failed, payload)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		duration := time.Since(start).Seconds()
		payload := &operatorv1.FsReadResult{
			ExecutionId:     requestID,
			Path:            protoRead.Path,
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			SizeBytes:       0,
			Truncated:       false,
			DurationSeconds: float32(duration),
			ErrorMessage:    err.Error(),
			ErrorType:       "read_error",
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
		payload := &operatorv1.FsReadResult{
			ExecutionId:     requestID,
			Path:            protoRead.Path,
			Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED,
			SizeBytes:       0,
			Truncated:       false,
			DurationSeconds: float32(duration),
			ErrorMessage:    err.Error(),
			ErrorType:       "read_error",
		}
		fs.publishLFAATypedResponse(ctx, msg, constants.Event.Operator.FsRead.Failed, payload)
		return
	}

	truncated := actualSize > readLimit
	content := string(data)

	if fs.sentinel != nil && fs.sentinel.IsEnabled() {
		content = fs.sentinel.ScrubText(content)
	}

	payload := &operatorv1.FsReadResult{
		ExecutionId:     requestID,
		Path:            protoRead.Path,
		Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		Content:         content,
		SizeBytes:       int64(len(data)),
		Truncated:       truncated,
		DurationSeconds: float32(duration),
	}
	fs.publishLFAATypedResponse(ctx, msg, constants.Event.Operator.FsRead.Completed, payload)
}

func (fs *FileOpsService) publishLFAATypedResponse(ctx context.Context, msg PubSubCommandMessage, eventType constants.EventType, payload proto.Message) {
	publishLFAATypedResponseTo(ctx, fs.client, fs.config, fs.logger, msg, eventType, payload)
}

func (fs *FileOpsService) publishLFAAError(ctx context.Context, msg PubSubCommandMessage, eventType constants.EventType, errorMsg string) {
	publishLFAAErrorTo(ctx, fs.client, fs.config, fs.logger, msg, eventType, errorMsg)
}

// payloadToFileEditRequest is a package-level helper shared by FileOpsService and tests.
func payloadToFileEditRequest(msg PubSubCommandMessage) (*models.FileEditRequest, error) {
	var p operatorv1.FileEditRequested
	if err := proto.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("failed to decode file edit payload as protobuf FileEditRequested: %w", err)
	}
	if p.FilePath == "" {
		return nil, fmt.Errorf("missing file_path in payload")
	}
	if p.Operation == "" {
		return nil, fmt.Errorf("missing operation in payload")
	}

	requestID := executionIDFromMessage(msg)
	if p.ExecutionId != "" {
		requestID = p.ExecutionId
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
		Operation:       constants.FileOperation(p.Operation),
		FilePath:        p.FilePath,
		RequestedBy:     "g8e-system",
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
	if p.InsertPosition != 0 {
		req.InsertPosition = system.IntPtr(int(p.InsertPosition))
	}
	if p.StartLine != 0 {
		req.StartLine = system.IntPtr(int(p.StartLine))
	}
	if p.EndLine != 0 {
		req.EndLine = system.IntPtr(int(p.EndLine))
	}
	if p.PatchContent != "" {
		req.PatchContent = system.StringPtr(p.PatchContent)
	}

	return req, nil
}

// payloadToFsListRequest is a package-level helper shared by FileOpsService and tests.
func payloadToFsListRequest(msg PubSubCommandMessage) (*models.FsListRequest, error) {
	var p operatorv1.FsListRequested
	if err := proto.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("failed to decode fs list payload as protobuf FsListRequested: %w", err)
	}

	path := p.Path
	if path == "" {
		path = "."
	}

	requestID := executionIDFromMessage(msg)
	if p.ExecutionId != "" {
		requestID = p.ExecutionId
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
		MaxDepth:        int(p.MaxDepth),
		MaxEntries:      int(maxEntries),
		RequestedBy:     "g8e-system",
	}, nil
}

// payloadToFsGrepRequest is a package-level helper shared by FileOpsService and tests.
func payloadToFsGrepRequest(msg PubSubCommandMessage) (*models.FsGrepRequest, error) {
	var p operatorv1.FsGrepRequested
	if err := proto.Unmarshal(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("failed to decode fs grep payload as protobuf FsGrepRequested: %w", err)
	}

	path := p.Path
	if path == "" {
		path = "."
	}

	requestID := executionIDFromMessage(msg)
	if p.ExecutionId != "" {
		requestID = p.ExecutionId
	}

	maxMatches := p.MaxMatches
	if maxMatches <= 0 {
		maxMatches = 100
	}

	return &models.FsGrepRequest{
		ExecutionID:     requestID,
		CaseID:          msg.CaseID,
		TaskID:          msg.TaskID,
		InvestigationID: msg.InvestigationID,
		Path:            path,
		Pattern:         p.Pattern,
		Includes:        p.Includes,
		MaxMatches:      int(maxMatches),
	}, nil
}

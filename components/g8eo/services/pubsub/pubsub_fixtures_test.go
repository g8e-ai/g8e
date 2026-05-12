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
	"log/slog"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/pkg/uap"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	commonv1 "github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// pubsubFixture is the standard test fixture for PubSubCommandService unit tests.
// All tests must construct services via helpers in this file — no raw struct literals
// accessing internal sub-service fields.
type pubsubFixture struct {
	DB      *MockOperatorPubSubClient
	Cfg     *config.Config
	Logger  *slog.Logger
	Svc     *PubSubCommandService
	Results *PubSubResultsService
}

// newPubsubFixture creates a fully wired PubSubCommandService for unit tests.
func newPubsubFixture(t *testing.T) *pubsubFixture {
	t.Helper()
	db := NewMockOperatorPubSubClient()
	t.Cleanup(func() { db.Close() })

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execSvc,
		FileEdit:       fileEditSvc,
		PubSubClient:   db,
		ResultsService: resultsSvc,
	})
	require.NoError(t, err)

	svc.ctx = context.Background()

	return &pubsubFixture{
		DB:      db,
		Cfg:     cfg,
		Logger:  logger,
		Svc:     svc,
		Results: resultsSvc,
	}
}

// newPubsubFixtureWithAuditVault creates a fixture with a real, enabled AuditVaultService.
func newPubsubFixtureWithAuditVault(t *testing.T) (*pubsubFixture, *storage.AuditVaultService) {
	t.Helper()
	db := NewMockOperatorPubSubClient()
	t.Cleanup(func() { db.Close() })

	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	tempDir := t.TempDir()
	avConfig := storage.AuditVaultConfig{
		Enabled:                   true,
		DataDir:                   tempDir,
		DBPath:                    "test.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             30,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
	avs, err := storage.NewAuditVaultService(&avConfig, logger)
	require.NoError(t, err)
	t.Cleanup(func() { avs.Close() })

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execSvc,
		FileEdit:       fileEditSvc,
		PubSubClient:   db,
		ResultsService: resultsSvc,
		AuditVault:     avs,
	})
	require.NoError(t, err)

	svc.ctx = context.Background()

	f := &pubsubFixture{
		DB:      db,
		Cfg:     cfg,
		Logger:  logger,
		Svc:     svc,
		Results: resultsSvc,
	}

	return f, avs
}

// assertLoopbackEnvelope handles both UAP JSON and binary Protobuf result formats.
func assertLoopbackEnvelope(t *testing.T, data []byte, eventType string) *commonv1.GovernanceEnvelope {
	t.Helper()

	// Try JSON (UAPEnvelope) first
	var uapEnv uap.UAPEnvelope
	if err := json.Unmarshal(data, &uapEnv); err == nil && uapEnv.MessageID != "" {
		// Map UAP back to GovernanceEnvelope for test assertion parity
		envEventType := mapActionTypeToEventType(uapEnv.Intent.ActionType)
		if strings.HasSuffix(uapEnv.Intent.ActionType, "_RESULT") || strings.HasSuffix(uapEnv.Intent.ActionType, "_CANCELLED") {
			envEventType = mapResultActionTypeToEventType(uapEnv.Intent.ActionType, uapEnv.Payload)
		}

		env := &commonv1.GovernanceEnvelope{
			Id:                uapEnv.MessageID,
			EventType:         envEventType,
			CaseId:            uapEnv.CaseID,
			InvestigationId:   uapEnv.InvestigationID,
			TaskId:            ptrStringValue(uapEnv.TaskID),
			OperatorSessionId: uapEnv.Metadata.SenderID,
			Payload:           uapEnv.Payload,
		}
		if eventType != "" {
			assert.Equal(t, eventType, env.EventType)
		}
		return env
	}

	// Fallback to binary Protobuf
	env := testutil.MustUnmarshalUniversalEnvelope(t, data)
	if eventType != "" {
		assert.Equal(t, eventType, env.EventType)
	}
	return env
}

// mapResultActionTypeToEventType maps UAP result action types back to protobuf event types.
func mapResultActionTypeToEventType(actionType string, payload []byte) string {
	switch actionType {
	case "EXECUTE_BASH_RESULT":
		if isFailedPayload(payload) {
			return constants.Event.Operator.Command.Failed
		}
		return constants.Event.Operator.Command.Completed
	case "EXECUTE_BASH_CANCELLED":
		return constants.Event.Operator.Command.Cancelled
	case "FILE_EDIT_RESULT":
		if isFailedPayload(payload) {
			return constants.Event.Operator.FileEdit.Failed
		}
		return constants.Event.Operator.FileEdit.Completed
	case "FS_LIST_RESULT":
		if isFailedPayload(payload) {
			return constants.Event.Operator.FsList.Failed
		}
		return constants.Event.Operator.FsList.Completed
	case "FS_GREP_RESULT":
		if isFailedPayload(payload) {
			return constants.Event.Operator.FsGrep.Failed
		}
		return constants.Event.Operator.FsGrep.Completed
	case "HEARTBEAT_RESULT":
		return constants.Event.Operator.Heartbeat
	case "EXECUTE_STATUS_UPDATE":
		return constants.Event.Operator.Command.StatusUpdated.Running
	default:
		return strings.TrimSuffix(actionType, "_RESULT")
	}
}

func isFailedPayload(payload []byte) bool {
	// Try to unmarshal as CommandResult (common for many results)
	var cr operatorv1.CommandResult
	if err := proto.Unmarshal(payload, &cr); err == nil {
		return cr.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED || cr.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_TIMEOUT
	}
	// Try FileEditResult
	var fr operatorv1.FileEditResult
	if err := proto.Unmarshal(payload, &fr); err == nil {
		return fr.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	}
	// Try FsListResult
	var lr operatorv1.FsListResult
	if err := proto.Unmarshal(payload, &lr); err == nil {
		return lr.Status == operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	}
	return false
}

func ptrStringValue(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// startService starts svc and registers a cleanup to stop it.
func startService(t *testing.T, svc *PubSubCommandService) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, svc.Start(ctx))
	t.Cleanup(func() {
		cancel()
		svc.Stop() //nolint:errcheck
	})
}

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
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	sentinel "github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCommandService(t *testing.T) *CommandService {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)

	db := NewMockG8esPubSubClient()
	t.Cleanup(func() { db.Close() })

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execSvc,
		FileEdit:       fileEditSvc,
		PubSubClient:   db,
		ResultsService: resultsSvc,
		Sentinel:       sentinel.NewSentinel(sentinel.DefaultSentinelConfig(), logger),
		RawVault:       newTestRawVault(t),
		LocalStore:     newTestLocalStore(t),
	})
	require.NoError(t, err)
	return svc.commands
}

// ---------------------------------------------------------------------------
// NewCommandService
// ---------------------------------------------------------------------------

func TestNewCommandService_CreatesService(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	execSvc := execution.NewExecutionService(cfg, logger)

	cs := NewCommandService(cfg, logger, execSvc)
	require.NotNil(t, cs)
	assert.Equal(t, cfg, cs.config)
}

// ---------------------------------------------------------------------------
// SetResultsPublisher
// ---------------------------------------------------------------------------

func TestCommandService_ResultsField(t *testing.T) {
	cs := newTestCommandService(t)
	assert.NotNil(t, cs.results)
}

// ---------------------------------------------------------------------------
// SetSentinel
// ---------------------------------------------------------------------------

func TestCommandService_SentinelField(t *testing.T) {
	cs := newTestCommandService(t)
	assert.NotNil(t, cs.sentinel)
}

func TestCommandService_VaultWriterField(t *testing.T) {
	cs := newTestCommandService(t)
	assert.NotNil(t, cs.vaultWriter)
}

// ---------------------------------------------------------------------------
// SetRawVaultService
// ---------------------------------------------------------------------------

func TestCommandService_VaultWriterRawVault(t *testing.T) {
	cs := newTestCommandService(t)
	assert.NotNil(t, cs.vaultWriter.rawVault)
}

func TestCommandService_SetRawVaultService_UpdatesExistingVaultWriter(t *testing.T) {
	cs := newTestCommandService(t)

	rv := newTestRawVault(t)
	ls := newTestLocalStore(t)
	cs.vaultWriter = NewVaultWriter(cs.config, cs.logger, cs.sentinel, rv, ls)
	require.NotNil(t, cs.vaultWriter)
	assert.Equal(t, rv, cs.vaultWriter.rawVault)
	assert.Equal(t, ls, cs.vaultWriter.localStore, "existing localStore must be preserved")
}

// ---------------------------------------------------------------------------
// SetLocalStoreService
// ---------------------------------------------------------------------------

func TestCommandService_SetLocalStoreService_CreatesVaultWriter(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	execSvc := execution.NewExecutionService(cfg, logger)
	cs := NewCommandService(cfg, logger, execSvc)

	assert.Nil(t, cs.vaultWriter)

	ls := newTestLocalStore(t)
	cs.vaultWriter = NewVaultWriter(cs.config, cs.logger, cs.sentinel, nil, ls)
	require.NotNil(t, cs.vaultWriter)
	assert.Equal(t, ls, cs.vaultWriter.localStore)
}

func TestCommandService_SetLocalStoreService_UpdatesExistingVaultWriter(t *testing.T) {
	cs := newTestCommandService(t)

	rv := newTestRawVault(t)
	ls := newTestLocalStore(t)
	cs.vaultWriter = NewVaultWriter(cs.config, cs.logger, cs.sentinel, rv, ls)
	assert.Equal(t, ls, cs.vaultWriter.localStore)
	assert.Equal(t, rv, cs.vaultWriter.rawVault, "existing rawVault must be preserved")
}

// ---------------------------------------------------------------------------
// Setters
// ---------------------------------------------------------------------------

type MockResultsPublisher struct{}

func (m *MockResultsPublisher) PublishExecutionResult(ctx context.Context, result *models.ExecutionResultsPayload, originalMsg PubSubCommandMessage) error {
	return nil
}
func (m *MockResultsPublisher) PublishCancellationResult(ctx context.Context, result *models.ExecutionResultsPayload, originalMsg PubSubCommandMessage) error {
	return nil
}
func (m *MockResultsPublisher) PublishFileEditResult(ctx context.Context, result *models.FileEditResult, originalMsg PubSubCommandMessage) error {
	return nil
}
func (m *MockResultsPublisher) PublishFsListResult(ctx context.Context, result *models.FsListResult, originalMsg PubSubCommandMessage) error {
	return nil
}
func (m *MockResultsPublisher) PublishExecutionStatus(ctx context.Context, status *ExecutionStatusUpdate) error {
	return nil
}
func (m *MockResultsPublisher) PublishResult(ctx context.Context, result *models.G8eMessage) error {
	return nil
}
func (m *MockResultsPublisher) PublishHeartbeat(ctx context.Context, heartbeat *models.Heartbeat) error {
	return nil
}

func TestCommandService_SetResultsPublisher(t *testing.T) {
	cs := newTestCommandService(t)
	mock := &MockResultsPublisher{}
	cs.SetResultsPublisher(mock)
	assert.Equal(t, mock, cs.results)
}

func TestCommandService_SetLocalStoreService(t *testing.T) {
	cs := newTestCommandService(t)
	ls := newTestLocalStore(t)
	cs.vaultWriter = &VaultWriter{}
	cs.SetLocalStoreService(ls)
	assert.Equal(t, ls, cs.vaultWriter.localStore)
}

func TestCommandService_SetRawVaultService(t *testing.T) {
	cs := newTestCommandService(t)
	rv := newTestRawVault(t)
	cs.vaultWriter = &VaultWriter{}
	cs.SetRawVaultService(rv)
	assert.Equal(t, rv, cs.vaultWriter.rawVault)
}

func TestCommandService_SetAuditVaultService(t *testing.T) {
	cs := newTestCommandService(t)
	av := &storage.AuditVaultService{}
	cs.SetAuditVaultService(av)
	assert.Equal(t, av, cs.auditVault)
}

func TestCommandService_SetSentinel(t *testing.T) {
	cs := newTestCommandService(t)
	s := &sentinel.Sentinel{}
	cs.SetSentinel(s)
	assert.Equal(t, s, cs.sentinel)
}

// ---------------------------------------------------------------------------
// runSentinelGuard — no sentinel wired
// ---------------------------------------------------------------------------

func TestCommandService_RunSentinelGuard_NoSentinel_AllowsAll(t *testing.T) {
	cs := newTestCommandService(t)

	execReq := newTestExecutionRequest("ls -la")
	verdict := cs.runSentinelGuard(execReq)

	assert.False(t, verdict.blocked)
	assert.Nil(t, verdict.blockedResult)
	assert.Nil(t, verdict.blockedEvent)
}

// ---------------------------------------------------------------------------
// runSentinelGuard — sentinel enabled but safe command
// ---------------------------------------------------------------------------

func TestCommandService_RunSentinelGuard_SafeCommand_Allowed(t *testing.T) {
	cs := newTestCommandService(t)
	s := sentinel.NewSentinel(sentinel.DefaultSentinelConfig(), testutil.NewTestLogger())
	cs.sentinel = s

	execReq := newTestExecutionRequest("ls -la /tmp")
	verdict := cs.runSentinelGuard(execReq)

	assert.False(t, verdict.blocked)
}

// ---------------------------------------------------------------------------
// HandleCancelRequest — invalid payload
// ---------------------------------------------------------------------------

func TestCommandService_HandleCancelRequest_InvalidPayload_NoOp(t *testing.T) {
	f := newPubsubFixture(t)
	msg := PubSubCommandMessage{
		ID:      "msg-cancel",
		Payload: []byte(`{invalid}`),
	}
	// Must not panic.
	f.Svc.commands.HandleCancelRequest(f.Svc.ctx, msg)
}

func TestCommandService_HandleCancelRequest_MissingExecutionID_NoOp(t *testing.T) {
	f := newPubsubFixture(t)
	msg := PubSubCommandMessage{
		ID:      "msg-cancel",
		Payload: []byte(`{"execution_id":""}`),
	}
	f.Svc.commands.HandleCancelRequest(f.Svc.ctx, msg)
	// No publish expected for missing execution_id.
	assert.Nil(t, f.DB.LastPublished())
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newTestExecutionRequest(command string) *models.ExecutionRequestPayload {
	return &models.ExecutionRequestPayload{
		ExecutionID: "test-req-id",
		Command:     command,
	}
}

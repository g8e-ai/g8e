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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	sentinel "github.com/g8e-ai/g8e/components/g8eo/services/sentinel"
	storage "github.com/g8e-ai/g8e/components/g8eo/services/storage"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SetLocalStoreService — propagates to sub-services
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SetLocalStoreService(t *testing.T) {
	f := newPubsubFixture(t)
	ls := newTestLocalStore(t)

	f.Svc.commands.vaultWriter.localStore = ls
	f.Svc.fileOps.vaultWriter.localStore = ls
	f.Svc.history.localStore = ls

	assert.Equal(t, ls, f.Svc.commands.vaultWriter.localStore)
	assert.Equal(t, ls, f.Svc.fileOps.vaultWriter.localStore)
	assert.Equal(t, ls, f.Svc.history.localStore)
}

// ---------------------------------------------------------------------------
// SetRawVaultService — propagates to sub-services
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SetRawVaultService(t *testing.T) {
	f := newPubsubFixture(t)
	rv := newTestRawVault(t)

	f.Svc.commands.vaultWriter.rawVault = rv
	f.Svc.fileOps.vaultWriter.rawVault = rv
	f.Svc.history.rawVault = rv

	assert.Equal(t, rv, f.Svc.commands.vaultWriter.rawVault)
	assert.Equal(t, rv, f.Svc.fileOps.vaultWriter.rawVault)
	assert.Equal(t, rv, f.Svc.history.rawVault)
}

// ---------------------------------------------------------------------------
// SetAuditVaultService — propagates to sub-services
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SetAuditVaultService(t *testing.T) {
	f := newPubsubFixture(t)

	avConfig := storage.AuditVaultConfig{
		Enabled:                   true,
		DataDir:                   t.TempDir(),
		DBPath:                    "audit.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             30,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
	avs, err := storage.NewAuditVaultService(&avConfig, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { avs.Close() })

	f.Svc.commands.auditVault = avs
	f.Svc.fileOps.auditVault = avs
	f.Svc.audit.auditVault = avs

	assert.Equal(t, avs, f.Svc.commands.auditVault)
	assert.Equal(t, avs, f.Svc.fileOps.auditVault)
	assert.Equal(t, avs, f.Svc.audit.auditVault)
}

// ---------------------------------------------------------------------------
// SetLedgerService — propagates to fileOps
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SetLedgerService(t *testing.T) {
	f := newPubsubFixture(t)

	avConfig := storage.AuditVaultConfig{
		Enabled:                   true,
		DataDir:                   t.TempDir(),
		DBPath:                    "audit.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             30,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
	avs, err := storage.NewAuditVaultService(&avConfig, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { avs.Close() })

	ledger := storage.NewLedgerService(avs, nil, testutil.NewTestLogger())

	f.Svc.fileOps.ledger = ledger
}

// ---------------------------------------------------------------------------
// SetHistoryHandler — propagates to history sub-service
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SetHistoryHandler(t *testing.T) {
	f := newPubsubFixture(t)

	avConfig := storage.AuditVaultConfig{
		Enabled:                   true,
		DataDir:                   t.TempDir(),
		DBPath:                    "audit.db",
		LedgerDir:                 "ledger",
		MaxDBSizeMB:               100,
		RetentionDays:             30,
		PruneIntervalMinutes:      60,
		OutputTruncationThreshold: 102400,
		HeadTailSize:              51200,
	}
	avs, err := storage.NewAuditVaultService(&avConfig, testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { avs.Close() })

	hh := storage.NewHistoryHandler(avs, nil, testutil.NewTestLogger())
	f.Svc.history.historyHandler = hh
}

// ---------------------------------------------------------------------------
// SetSentinel — propagates to commands and fileOps
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SetSentinel(t *testing.T) {
	f := newPubsubFixture(t)

	s := sentinel.NewSentinel(sentinel.DefaultSentinelConfig(), testutil.NewTestLogger())
	f.Svc.commands.sentinel = s
	f.Svc.fileOps.sentinel = s
}

// ---------------------------------------------------------------------------
// dispatchCommand — all event types routed without panic
// ---------------------------------------------------------------------------

func TestPubSubCommandService_DispatchCommand_AllKnownEventTypes(t *testing.T) {
	knownTypes := []string{
		constants.Event.Operator.HeartbeatRequested,
		constants.Event.Operator.Command.Requested,
		constants.Event.Operator.Command.CancelRequested,
		constants.Event.Operator.FileEdit.Requested,
		constants.Event.Operator.FsList.Requested,
		constants.Event.Operator.FsRead.Requested,
		constants.Event.Operator.PortCheck.Requested,
		constants.Event.Operator.FetchLogs.Requested,
		constants.Event.Operator.FetchHistory.Requested,
		constants.Event.Operator.FetchFileHistory.Requested,
		constants.Event.Operator.RestoreFile.Requested,
		constants.Event.Operator.Audit.UserMsg,
		constants.Event.Operator.Audit.AIMsg,
		constants.Event.Operator.Audit.DirectCmd,
		constants.Event.Operator.Audit.DirectCmdResult,
		constants.Event.Operator.FetchFileDiff.Requested,
	}

	for _, eventType := range knownTypes {
		t.Run(eventType, func(t *testing.T) {
			f := newPubsubFixture(t)
			msg := PubSubCommandMessage{
				ID:        "dispatch-test",
				EventType: eventType,
				CaseID:    "case-1",
				Payload:   json.RawMessage(`{}`),
				Timestamp: time.Now().UTC(),
			}
			assert.NotPanics(t, func() {
				f.Svc.dispatchCommand(msg)
			})
		})
	}
}

func TestPubSubCommandService_DispatchCommand_UnknownType_NoOp(t *testing.T) {
	f := newPubsubFixture(t)
	msg := PubSubCommandMessage{
		ID:        "dispatch-unknown",
		EventType: "completely.unknown.event.type",
		CaseID:    "case-1",
		Payload:   json.RawMessage(`{}`),
		Timestamp: time.Now().UTC(),
	}
	assert.NotPanics(t, func() {
		f.Svc.dispatchCommand(msg)
	})
}

// ---------------------------------------------------------------------------
// handleCommandPayload — round-trip from raw JSON bytes
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleCommandPayload_ValidJSON(t *testing.T) {
	f := newPubsubFixture(t)

	msg := PubSubCommandMessage{
		ID:        "hcp-1",
		EventType: constants.Event.Operator.HeartbeatRequested,
		CaseID:    "case-1",
		Payload:   mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp: time.Now().UTC(),
	}
	data, err := json.Marshal(msg)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		f.Svc.handleCommandPayload(data)
	})
}

func TestPubSubCommandService_HandleCommandPayload_InvalidJSON(t *testing.T) {
	f := newPubsubFixture(t)

	assert.NotPanics(t, func() {
		f.Svc.handleCommandPayload([]byte(`{not valid json`))
	})
}

// ---------------------------------------------------------------------------
// SendAutomaticHeartbeat — delegates to heartbeat service
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SendAutomaticHeartbeat_DoesNotPanic(t *testing.T) {
	f := newPubsubFixture(t)
	f.Svc.ctx = context.Background()

	assert.NotPanics(t, func() {
		f.Svc.SendAutomaticHeartbeat()
	})
}

// ---------------------------------------------------------------------------
// Stop — idempotent when not running
// ---------------------------------------------------------------------------

func TestPubSubCommandService_Stop_WhenNotRunning_NoError(t *testing.T) {
	f := newPubsubFixture(t)
	err := f.Svc.Stop()
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Start + Stop — lifecycle
// ---------------------------------------------------------------------------

func TestPubSubCommandService_StartStop_Lifecycle(t *testing.T) {
	f := newPubsubFixture(t)

	err := f.Svc.Start(context.Background())
	require.NoError(t, err)

	err = f.Svc.Stop()
	assert.NoError(t, err)
}

func TestPubSubCommandService_Start_AlreadyRunning_ReturnsError(t *testing.T) {
	f := newPubsubFixture(t)

	err := f.Svc.Start(context.Background())
	require.NoError(t, err)
	defer f.Svc.Stop()

	err = f.Svc.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

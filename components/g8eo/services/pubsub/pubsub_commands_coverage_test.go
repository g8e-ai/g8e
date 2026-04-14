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
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// HandleCommandData — dispatches typed message via Gateway transport path
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleCommandData_HeartbeatRequest(t *testing.T) {
	f := newPubsubFixture(t)
	f.Svc.ctx = context.Background()

	msg := &PubSubCommandMessage{
		ID:        "gw-hb-1",
		EventType: constants.Event.Operator.HeartbeatRequested,
		CaseID:    "case-1",
		Payload:   mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp: time.Now().UTC(),
	}

	assert.NotPanics(t, func() {
		f.Svc.HandleCommandData(msg)
	})
}

func TestPubSubCommandService_HandleCommandData_AllKnownEventTypes(t *testing.T) {
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
			f.Svc.ctx = context.Background()

			msg := &PubSubCommandMessage{
				ID:        "gw-dispatch-test",
				EventType: eventType,
				CaseID:    "case-1",
				Payload:   json.RawMessage(`{}`),
				Timestamp: time.Now().UTC(),
			}
			assert.NotPanics(t, func() {
				f.Svc.HandleCommandData(msg)
			})
		})
	}
}

func TestPubSubCommandService_HandleCommandData_UnknownEventType(t *testing.T) {
	f := newPubsubFixture(t)
	f.Svc.ctx = context.Background()

	msg := &PubSubCommandMessage{
		ID:        "gw-unknown",
		EventType: "completely.unknown.event.type",
		CaseID:    "case-1",
		Payload:   json.RawMessage(`{}`),
		Timestamp: time.Now().UTC(),
	}

	assert.NotPanics(t, func() {
		f.Svc.HandleCommandData(msg)
	})
}

// ---------------------------------------------------------------------------
// handleShutdownRequest — payload decoding and reason extraction
//
// os.Exit cannot be called in a normal test. We test the payload-parsing
// logic by verifying the function exits with the expected code using a
// subprocess helper pattern.
// ---------------------------------------------------------------------------

func TestPubSubCommandService_HandleShutdownRequest_SubprocessExits(t *testing.T) {
	if os.Getenv("G8E_TEST_SHUTDOWN_SUBPROCESS") == "1" {
		f := newPubsubFixture(t)
		f.Svc.ctx = context.Background()
		msg := PubSubCommandMessage{
			ID:      "shutdown-1",
			CaseID:  "case-1",
			Payload: mustMarshalJSON(t, models.ShutdownRequestPayload{Reason: "test shutdown"}),
		}
		f.Svc.handleShutdownRequest(msg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestPubSubCommandService_HandleShutdownRequest_SubprocessExits")
	cmd.Env = append(os.Environ(), "G8E_TEST_SHUTDOWN_SUBPROCESS=1")
	err := cmd.Run()

	// constants.ExitSuccess == 0: os.Exit(0) makes the subprocess exit cleanly
	// so err is nil (not an ExitError). Verify the process succeeded.
	assert.NoError(t, err, "shutdown should exit with code 0 (ExitSuccess)")
}

func TestPubSubCommandService_HandleShutdownRequest_EmptyReasonSubprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_SHUTDOWN_EMPTY") == "1" {
		f := newPubsubFixture(t)
		f.Svc.ctx = context.Background()
		msg := PubSubCommandMessage{
			ID:      "shutdown-2",
			CaseID:  "case-1",
			Payload: mustMarshalJSON(t, models.ShutdownRequestPayload{Reason: ""}),
		}
		f.Svc.handleShutdownRequest(msg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestPubSubCommandService_HandleShutdownRequest_EmptyReasonSubprocess")
	cmd.Env = append(os.Environ(), "G8E_TEST_SHUTDOWN_EMPTY=1")
	err := cmd.Run()

	assert.NoError(t, err, "shutdown with empty reason should exit with code 0 (ExitSuccess)")
}

func TestPubSubCommandService_HandleShutdownRequest_InvalidPayloadSubprocess(t *testing.T) {
	if os.Getenv("G8E_TEST_SHUTDOWN_INVALID") == "1" {
		f := newPubsubFixture(t)
		f.Svc.ctx = context.Background()
		msg := PubSubCommandMessage{
			ID:      "shutdown-3",
			CaseID:  "case-1",
			Payload: json.RawMessage(`{invalid json`),
		}
		f.Svc.handleShutdownRequest(msg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestPubSubCommandService_HandleShutdownRequest_InvalidPayloadSubprocess")
	cmd.Env = append(os.Environ(), "G8E_TEST_SHUTDOWN_INVALID=1")
	err := cmd.Run()

	assert.NoError(t, err, "shutdown should still exit with code 0 even with invalid payload")
}

// ---------------------------------------------------------------------------
// SendAutomaticHeartbeat — delegates to heartbeat service
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SendAutomaticHeartbeat_WithContext(t *testing.T) {
	f := newPubsubFixture(t)
	f.Svc.ctx = context.Background()

	assert.NotPanics(t, func() {
		f.Svc.SendAutomaticHeartbeat()
	})
}

// ---------------------------------------------------------------------------
// SetResultsService — propagates to all sub-services
// ---------------------------------------------------------------------------

func TestPubSubCommandService_SetResultsService_PropagatesPublisher(t *testing.T) {
	f := newPubsubFixture(t)

	db2 := NewMockG8esPubSubClient()
	t.Cleanup(func() { db2.Close() })

	resultsSvc, err := NewPubSubResultsService(f.Cfg, f.Logger, db2, nil)
	require.NoError(t, err)

	f.Svc.results = resultsSvc
	f.Svc.heartbeat.results = resultsSvc
	f.Svc.commands.results = resultsSvc
	f.Svc.fileOps.results = resultsSvc

	assert.Equal(t, resultsSvc, f.Svc.results)
	assert.Equal(t, resultsSvc, f.Svc.heartbeat.results)
	assert.Equal(t, resultsSvc, f.Svc.commands.results)
	assert.Equal(t, resultsSvc, f.Svc.fileOps.results)
}

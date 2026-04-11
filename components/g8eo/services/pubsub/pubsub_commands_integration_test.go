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

//go:build integration

package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/httpclient"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	listen "github.com/g8e-ai/g8e/components/g8eo/services/listen"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPubSubCommandService_Start_Integration tests the full Start workflow
func TestPubSubCommandService_Start_Integration(t *testing.T) {

	t.Run("starts service and listens for commands", func(t *testing.T) {
		db := NewTestPubSubClient(t)

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

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start the service
		err = svc.Start(ctx)
		require.NoError(t, err)

		// Verify service is running
		svc.mu.RLock()
		assert.True(t, svc.running)
		svc.mu.RUnlock()

		// Give listener time to start
		time.Sleep(100 * time.Millisecond)

		// Stop the service
		err = svc.Stop()
		assert.NoError(t, err)

		// Verify service stopped
		svc.mu.RLock()
		assert.False(t, svc.running)
		svc.mu.RUnlock()
	})

	t.Run("prevents double start", func(t *testing.T) {
		db := NewTestPubSubClient(t)

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

		ctx := context.Background()

		// Start the service
		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		// Try to start again
		err = svc.Start(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already running")
	})
}

// TestPubSubCommandService_ListenForCommands_Integration tests command listening
func TestPubSubCommandService_ListenForCommands_Integration(t *testing.T) {

	t.Run("receives and handles heartbeat command", func(t *testing.T) {
		db := NewTestPubSubClient(t)

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

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start service to begin listening
		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		// Subscribe to heartbeat channel (heartbeats publish to dedicated channel)
		heartbeatChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), heartbeatChannel)

		// Give listener time to start
		time.Sleep(100 * time.Millisecond)

		// Publish a heartbeat request
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msg := PubSubCommandMessage{
			ID:        "test-heartbeat-1",
			EventType: constants.Event.Operator.HeartbeatRequested,
			CaseID:    "case-123",
			Payload:   json.RawMessage(`{}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), commandChannel, string(msgJSON))

		// Wait for heartbeat response
		receivedMsg := testutil.WaitForMessage(t, msgChan, 2*time.Second)
		assert.NotNil(t, receivedMsg)
		assert.Contains(t, string(receivedMsg), constants.Event.Operator.Heartbeat)
	})

	t.Run("receives and executes command request", func(t *testing.T) {
		db := NewTestPubSubClient(t)

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

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start service
		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		// Subscribe to results channel
		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), resultsChannel)

		// Give listener time to start
		time.Sleep(100 * time.Millisecond)

		// Publish a command execution request
		commandChannel := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msg := PubSubCommandMessage{
			ID:        "test-exec-1",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-456",
			Payload:   json.RawMessage(`{"command":"echo","justification":"Testing command execution"}`),
			Timestamp: time.Now().UTC(),
		}

		msgJSON, err := json.Marshal(msg)
		require.NoError(t, err)

		testutil.PublishTestMessage(t, testutil.GetTestVSODBDirectURL(), commandChannel, string(msgJSON))

		// Wait for execution result
		receivedMsg := testutil.WaitForMessage(t, msgChan, 3*time.Second)
		assert.NotNil(t, receivedMsg)
		assert.Contains(t, string(receivedMsg), constants.Event.Operator.Command.Completed)
	})
}

// TestPubSubCommandService_SendAutomaticHeartbeat_Integration tests automatic heartbeats
func TestPubSubCommandService_SendAutomaticHeartbeat_Integration(t *testing.T) {

	t.Run("sends automatic heartbeats on schedule", func(t *testing.T) {
		db := NewTestPubSubClient(t)

		cfg := testutil.NewTestConfig(t)

		cfg.HeartbeatInterval = 200 * time.Millisecond // Fast for testing
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

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Subscribe to heartbeat channel before starting (heartbeats publish to dedicated channel)
		heartbeatChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), heartbeatChannel)

		// Start service (this starts heartbeat scheduler)
		err = svc.Start(ctx)
		require.NoError(t, err)
		defer svc.Stop()

		// Wait for at least one automatic heartbeat
		receivedMsg := testutil.WaitForMessage(t, msgChan, 1*time.Second)
		assert.NotNil(t, receivedMsg)
		assert.Contains(t, string(receivedMsg), constants.Event.Operator.Heartbeat)

		var heartbeat models.Heartbeat
		err = json.Unmarshal(receivedMsg, &heartbeat)
		require.NoError(t, err)

		assert.Equal(t, constants.Event.Operator.Heartbeat, heartbeat.EventType)
		assert.NotEmpty(t, heartbeat.SystemIdentity.OS)
	})

	t.Run("handles nil results service gracefully", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)

		logger := testutil.NewTestLogger()

		execSvc := execution.NewExecutionService(cfg, logger)
		fileEditSvc := execution.NewFileEditService(cfg, logger)

		svc, err := NewPubSubCommandService(CommandServiceConfig{Config: cfg, Logger: logger, Execution: execSvc, FileEdit: fileEditSvc, PubSubClient: nil})
		require.NoError(t, err)

		// Don't set results service
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Call SendAutomaticHeartbeat directly - should not panic
		svc.SendAutomaticHeartbeat()

		// Wait for context to complete
		<-ctx.Done()
	})
}

// TestPubSubCommandService_HandleCommandExecutionRequest_Integration tests execution handling
func TestPubSubCommandService_HandleCommandExecutionRequest_Integration(t *testing.T) {

	t.Run("executes command and publishes result", func(t *testing.T) {
		db := NewTestPubSubClient(t)

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

		// Subscribe to results channel
		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), resultsChannel)

		// Create command message
		taskID := "task-123"
		msg := PubSubCommandMessage{
			ID:              "exec-msg-1",
			EventType:       constants.Event.Operator.Command.Requested,
			CaseID:          "case-789",
			TaskID:          &taskID,
			InvestigationID: "inv-456",
			Payload:         json.RawMessage(`{"command":"echo","justification":"Integration test"}`),
			Timestamp:       time.Now().UTC(),
		}

		// Handle command execution
		ctx := context.Background()
		svc.handleCommandExecutionRequest(ctx, msg)

		// Wait for result
		receivedMsg := testutil.WaitForMessage(t, msgChan, 3*time.Second)
		assert.NotNil(t, receivedMsg)

		var result models.VSOMessage
		err = json.Unmarshal([]byte(string(receivedMsg)), &result)
		require.NoError(t, err)

		assert.Contains(t, []string{constants.Event.Operator.Command.Completed, constants.Event.Operator.Command.Failed}, result.EventType)
		assert.Equal(t, "case-789", result.CaseID)
	})

	t.Run("handles command execution failure", func(t *testing.T) {
		db := NewTestPubSubClient(t)

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

		// Subscribe to results channel
		resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
		msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), resultsChannel)

		// Create command message with invalid command
		msg := PubSubCommandMessage{
			ID:        "exec-fail-1",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-fail",
			Payload:   json.RawMessage(`{"command":"nonexistentcommandxyz","justification":"Testing failure"}`),
			Timestamp: time.Now().UTC(),
		}

		// Handle command execution
		ctx := context.Background()
		svc.handleCommandExecutionRequest(ctx, msg)

		// Wait for result
		receivedMsg := testutil.WaitForMessage(t, msgChan, 3*time.Second)
		assert.NotNil(t, receivedMsg)

		var result models.VSOMessage
		err = json.Unmarshal([]byte(string(receivedMsg)), &result)
		require.NoError(t, err)

		assert.Contains(t, []string{constants.Event.Operator.Command.Completed, constants.Event.Operator.Command.Failed}, result.EventType)
	})

	t.Run("handles default justification", func(t *testing.T) {
		db := NewTestPubSubClient(t)

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

		// Create command message without justification
		msg := PubSubCommandMessage{
			ID:        "exec-no-just-1",
			EventType: constants.Event.Operator.Command.Requested,
			CaseID:    "case-no-just",
			Payload:   json.RawMessage(`{"command":"echo"}`),
			Timestamp: time.Now().UTC(),
		}

		// Handle command - should not panic and use default justification
		ctx := context.Background()
		svc.handleCommandExecutionRequest(ctx, msg)

		// Give it time to complete
		time.Sleep(100 * time.Millisecond)
	})
}

// TestListenForCommands_MaxReconnectAttempts_Integration verifies that listenForCommands
// stops after maxReconnectAttempts when the VSODB channel closes repeatedly.
// Each attempt: the service subscribes successfully to real VSODB, we immediately
// close the underlying client (forcing the channel closed), triggering a reconnect.
// After 3 closures the loop must exit on its own.
func TestListenForCommands_MaxReconnectAttempts_Integration(t *testing.T) {
	testutil.TestPubSubAvailable(t)

	cfg := testutil.NewTestConfig(t)
	log := testutil.NewTestLogger()
	execSvc := execution.NewExecutionService(cfg, log)
	fileEditSvc := execution.NewFileEditService(cfg, log)

	svc, err := NewPubSubCommandService(cfg, log, execSvc, fileEditSvc, nil)
	require.NoError(t, err)

	// Zero out reconnect delay so the test runs fast.
	svc.reconnectBaseDelay = 0

	// Replace the client with a wrapper that dials real VSODB but closes the
	// WS conn immediately after each subscribe, forcing the channel closed.
	wsURL := testutil.GetTestVSODBDirectURL() + "/ws/pubsub"
	attempts := 0
	svc.client = &reconnectCountingClient{
		t:      t,
		wsURL:  wsURL,
		counts: &attempts,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.ctx = ctx
	svc.cancel = cancel

	done := make(chan struct{})
	go func() {
		defer close(done)
		channelName := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
		svc.listenForCommands(channelName)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("listenForCommands did not stop after max reconnect attempts")
	}

	assert.Equal(t, 3, attempts, "expected exactly 3 subscribe attempts before giving up")
}

// reconnectCountingClient dials VSODB directly for each Subscribe call so it can
// hold a reference to the underlying *websocket.Conn and close it immediately,
// forcing the returned channel closed and triggering the reconnect logic.
type reconnectCountingClient struct {
	t      *testing.T
	wsURL  string
	counts *int
}

func (r *reconnectCountingClient) Subscribe(ctx context.Context, channel string) (<-chan []byte, error) {
	r.t.Helper()

	dialer, err := httpclient.WebSocketDialer()
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS dialer: %w", err)
	}
	ws, _, err := dialer.DialContext(ctx, r.wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to VSODB pub/sub: %w", err)
	}

	subMsg := listen.PubSubMessage{Action: "subscribe", Channel: channel}
	subJSON, _ := json.Marshal(subMsg)
	if err := ws.WriteMessage(websocket.TextMessage, subJSON); err != nil {
		ws.Close()
		return nil, fmt.Errorf("failed to subscribe to channel %s: %w", channel, err)
	}

	*r.counts++

	out := make(chan []byte, 64)
	go func() {
		defer close(out)
		defer ws.Close()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			_, raw, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var event listen.PubSubEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				continue
			}
			if event.Type != "message" && event.Type != "pmessage" {
				continue
			}
			select {
			case out <- []byte(event.Data):
			case <-ctx.Done():
				return
			}
		}
	}()

	// Close the conn immediately — the read goroutine will exit and close out.
	ws.Close()
	return out, nil
}

func (r *reconnectCountingClient) Publish(_ context.Context, _ string, _ []byte) error { return nil }
func (r *reconnectCountingClient) Close()                                              {}

// ---------------------------------------------------------------------------
// Heartbeat interval — integration tests against real VSODB pub/sub
// ---------------------------------------------------------------------------

// TestHeartbeatInterval_Integration verifies that the HeartbeatInterval set via
// config (equivalent to --heartbeat-interval) drives real automatic heartbeats
// all the way through PubSubResultsService to the VSODB pub/sub broker.

func TestHeartbeatInterval_ShortIntervalFiresOnRealBroker(t *testing.T) {
	db := NewTestPubSubClient(t)

	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 150 * time.Millisecond

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

	heartbeatChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), heartbeatChannel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Stop()

	raw := testutil.WaitForMessage(t, msgChan, 2*time.Second)
	require.NotNil(t, raw)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(raw, &hb))

	assert.Equal(t, constants.Event.Operator.Heartbeat, hb.EventType)
	assert.Equal(t, models.HeartbeatTypeAutomatic, hb.HeartbeatType)
	assert.Equal(t, constants.Status.ComponentName.G8EO, hb.SourceComponent)
	assert.Equal(t, cfg.OperatorID, hb.OperatorID)
	assert.Equal(t, cfg.OperatorSessionId, hb.OperatorSessionID)
}

func TestHeartbeatInterval_ShortIntervalDeliversMulitpleTicks(t *testing.T) {
	db := NewTestPubSubClient(t)

	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 150 * time.Millisecond

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

	heartbeatChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), heartbeatChannel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Stop()

	for i := 0; i < 3; i++ {
		raw := testutil.WaitForMessage(t, msgChan, 2*time.Second)
		require.NotNil(t, raw, "expected heartbeat tick %d", i+1)

		var hb models.Heartbeat
		require.NoError(t, json.Unmarshal(raw, &hb))
		assert.Equal(t, models.HeartbeatTypeAutomatic, hb.HeartbeatType)
	}
}

func TestHeartbeatInterval_PayloadFieldsFullyPlumbed(t *testing.T) {
	db := NewTestPubSubClient(t)

	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 150 * time.Millisecond
	cfg.Version = "integration-test-version"
	cfg.LocalStoreEnabled = true

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

	heartbeatChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), heartbeatChannel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Stop()

	raw := testutil.WaitForMessage(t, msgChan, 2*time.Second)
	require.NotNil(t, raw)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(raw, &hb))

	assert.Equal(t, "integration-test-version", hb.VersionInfo.OperatorVersion)
	assert.Equal(t, constants.Status.VersionStability.Stable, hb.VersionInfo.Status)
	assert.True(t, hb.CapabilityFlags.LocalStorageEnabled)
	assert.NotEmpty(t, hb.SystemIdentity.Hostname)
	assert.NotEmpty(t, hb.SystemIdentity.OS)
	assert.NotEmpty(t, hb.SystemIdentity.Architecture)
	assert.Greater(t, hb.SystemIdentity.CPUCount, 0)
	assert.NotEmpty(t, hb.Timestamp)
}

func TestHeartbeatInterval_PublishesToCorrectChannel(t *testing.T) {
	db := NewTestPubSubClient(t)

	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 150 * time.Millisecond

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

	heartbeatChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	resultsChannel := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)

	hbChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), heartbeatChannel)
	resultsChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), resultsChannel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Stop()

	raw := testutil.WaitForMessage(t, hbChan, 2*time.Second)
	require.NotNil(t, raw, "heartbeat must arrive on the heartbeat channel")

	select {
	case unexpected := <-resultsChan:
		t.Fatalf("automatic heartbeat must NOT appear on results channel, got: %s", unexpected)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHeartbeatInterval_StopsOnServiceStop(t *testing.T) {
	db := NewTestPubSubClient(t)

	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 150 * time.Millisecond

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

	heartbeatChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	msgChan := testutil.SubscribeToChannel(t, testutil.GetTestVSODBDirectURL(), heartbeatChannel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = svc.Start(ctx)
	require.NoError(t, err)

	testutil.WaitForMessage(t, msgChan, 2*time.Second)

	require.NoError(t, svc.Stop())

	// Drain any messages in-flight before asserting silence.
	time.Sleep(50 * time.Millisecond)
	for len(msgChan) > 0 {
		<-msgChan
	}

	select {
	case unexpected := <-msgChan:
		t.Fatalf("heartbeat must not fire after Stop(), got: %s", unexpected)
	case <-time.After(400 * time.Millisecond):
	}
}

// TestPubSubCommandService_ContextCancellation_Integration tests graceful shutdown
func TestPubSubCommandService_ContextCancellation_Integration(t *testing.T) {

	t.Run("stops listening when context is cancelled", func(t *testing.T) {
		db := NewTestPubSubClient(t)

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

		ctx, cancel := context.WithCancel(context.Background())

		// Start service
		err = svc.Start(ctx)
		require.NoError(t, err)

		// Give it time to start
		time.Sleep(100 * time.Millisecond)

		// Cancel context
		cancel()

		// Stop service
		err = svc.Stop()
		assert.NoError(t, err)

		// Verify it stopped
		svc.mu.RLock()
		assert.False(t, svc.running)
		svc.mu.RUnlock()
	})
}

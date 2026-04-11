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

// Heartbeat interval loopback tests verify that the HeartbeatInterval value —
// set via --heartbeat-interval at startup and stored in config.Config — drives
// real automatic heartbeats through the full g8eo pub/sub stack using an
// in-process PubSubBroker (loopbackFixture).
//
// No external infrastructure is required.  These tests sit between the unit
// tests (MockVSODBPubSubClient) and the integration tests (live VSODB) and
// cover:
//   - Scheduler fires at the configured short interval and publishes on the
//     heartbeat channel via the real VSODBPubSubClient + PubSubBroker path.
//   - Multiple ticks arrive at the correct cadence.
//   - Every field in the published Heartbeat payload is populated correctly.
//   - Heartbeats arrive only on the heartbeat channel, not the results channel.
//   - The scheduler stops cleanly when the service is stopped.
//   - A zero interval (flag not supplied) disables the scheduler entirely.
//   - The scheduler also stops when the context is cancelled.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	execution "github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drainUntilQuiet reads from ch until no message arrives within the quiet
// window, consuming any already-buffered or in-flight messages. It is used
// after stopping a scheduler to ensure all buffered ticks are consumed before
// asserting silence with drainNone.
func drainUntilQuiet(ch <-chan []byte, quietWindow time.Duration) {
	for {
		select {
		case <-ch:
		case <-time.After(quietWindow):
			return
		}
	}
}

// newLoopbackServiceWithInterval builds a PubSubCommandService connected to
// the loopback broker with an explicit HeartbeatInterval, mirroring the effect
// of passing --heartbeat-interval on the command line.
func newLoopbackServiceWithInterval(t *testing.T, f *loopbackFixture, interval time.Duration) (*PubSubCommandService, *PubSubResultsService) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = interval
	logger := testutil.NewTestLogger()

	cmdClient, err := NewVSODBPubSubClient(f.wsURL, "", logger)
	require.NoError(t, err)
	t.Cleanup(cmdClient.Close)

	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, cmdClient, nil)
	require.NoError(t, err)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execSvc,
		FileEdit:       fileEditSvc,
		PubSubClient:   cmdClient,
		ResultsService: resultsSvc,
	})
	require.NoError(t, err)

	return svc, resultsSvc
}

// =============================================================================
// Scheduler fires at short interval
// =============================================================================

func TestLoopback_HeartbeatInterval_ShortIntervalFiresOnBroker(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackServiceWithInterval(t, f, 40*time.Millisecond)

	hbSub := watchHeartbeat(t, f, svc)
	startService(t, svc)

	raw := drainOne(t, hbSub)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(raw, &hb))

	assert.Equal(t, constants.Event.Operator.Heartbeat, hb.EventType)
	assert.Equal(t, models.HeartbeatTypeAutomatic, hb.HeartbeatType)
	assert.Equal(t, constants.Status.ComponentName.G8EO, hb.SourceComponent)
	assert.Equal(t, svc.config.OperatorID, hb.OperatorID)
	assert.Equal(t, svc.config.OperatorSessionId, hb.OperatorSessionID)
}

// =============================================================================
// Multiple ticks
// =============================================================================

func TestLoopback_HeartbeatInterval_MultipleTicks(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackServiceWithInterval(t, f, 40*time.Millisecond)

	hbSub := watchHeartbeat(t, f, svc)
	startService(t, svc)

	for i := 0; i < 3; i++ {
		raw := drainOne(t, hbSub)
		require.NotNil(t, raw, "expected heartbeat tick %d", i+1)

		var hb models.Heartbeat
		require.NoError(t, json.Unmarshal(raw, &hb))
		assert.Equal(t, models.HeartbeatTypeAutomatic, hb.HeartbeatType, "tick %d wrong type", i+1)
	}
}

// =============================================================================
// Payload fields fully populated
// =============================================================================

func TestLoopback_HeartbeatInterval_PayloadFieldsPopulated(t *testing.T) {
	f := newLoopbackFixture(t)

	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 40 * time.Millisecond
	cfg.Version = "loopback-test-version"
	cfg.LocalStoreEnabled = true
	cfg.GitAvailable = true
	cfg.NoGit = false
	logger := testutil.NewTestLogger()

	cmdClient, err := NewVSODBPubSubClient(f.wsURL, "", logger)
	require.NoError(t, err)
	t.Cleanup(cmdClient.Close)

	execSvc := execution.NewExecutionService(cfg, logger)
	fileEditSvc := execution.NewFileEditService(cfg, logger)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, cmdClient, nil)
	require.NoError(t, err)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execSvc,
		FileEdit:       fileEditSvc,
		PubSubClient:   cmdClient,
		ResultsService: resultsSvc,
	})
	require.NoError(t, err)

	hbSub := watchHeartbeat(t, f, svc)
	startService(t, svc)

	raw := drainOne(t, hbSub)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(raw, &hb))

	assert.Equal(t, cfg.OperatorID, hb.OperatorID)
	assert.Equal(t, cfg.OperatorSessionId, hb.OperatorSessionID)
	assert.Equal(t, "loopback-test-version", hb.VersionInfo.OperatorVersion)
	assert.Equal(t, constants.Status.VersionStability.Stable, hb.VersionInfo.Status)
	assert.True(t, hb.CapabilityFlags.LocalStorageEnabled)
	assert.True(t, hb.CapabilityFlags.GitAvailable)
	assert.True(t, hb.CapabilityFlags.LedgerMirrorEnabled)
	assert.NotEmpty(t, hb.SystemIdentity.Hostname)
	assert.NotEmpty(t, hb.SystemIdentity.OS)
	assert.NotEmpty(t, hb.SystemIdentity.Architecture)
	assert.Greater(t, hb.SystemIdentity.CPUCount, 0)
	assert.NotEmpty(t, hb.Timestamp)
}

// =============================================================================
// Correct channel — heartbeat must not bleed onto results channel
// =============================================================================

func TestLoopback_HeartbeatInterval_PublishesToHeartbeatChannelOnly(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackServiceWithInterval(t, f, 40*time.Millisecond)

	hbSub := watchHeartbeat(t, f, svc)
	resultsSub := watchResults(t, f, svc)

	startService(t, svc)

	// Heartbeat must arrive on the heartbeat channel.
	raw := drainOne(t, hbSub)
	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(raw, &hb))
	assert.Equal(t, models.HeartbeatTypeAutomatic, hb.HeartbeatType)

	// Nothing should appear on the results channel.
	drainNone(t, resultsSub, 120*time.Millisecond)
}

// =============================================================================
// Scheduler stops cleanly on service Stop
// =============================================================================

func TestLoopback_HeartbeatInterval_StopsOnServiceStop(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackServiceWithInterval(t, f, 40*time.Millisecond)

	hbSub := watchHeartbeat(t, f, svc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, svc.Start(ctx))

	// Wait for at least one tick to confirm the scheduler is running.
	drainOne(t, hbSub)

	require.NoError(t, svc.Stop())

	// Drain everything already queued until the channel is quiet for a full interval.
	drainUntilQuiet(hbSub, 80*time.Millisecond)

	// After Stop, no more heartbeats should arrive within several intervals.
	drainNone(t, hbSub, 200*time.Millisecond)
}

// =============================================================================
// Context cancellation stops the scheduler
// =============================================================================

func TestLoopback_HeartbeatInterval_ContextCancellationStopsScheduler(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackServiceWithInterval(t, f, 40*time.Millisecond)

	hbSub := watchHeartbeat(t, f, svc)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, svc.Start(ctx))

	// Wait for at least one tick.
	drainOne(t, hbSub)

	cancel()
	// Allow the goroutine to observe the cancellation.
	svc.wg.Wait()

	// Drain everything already buffered or in-flight.
	drainUntilQuiet(hbSub, 80*time.Millisecond)

	drainNone(t, hbSub, 200*time.Millisecond)
}

// =============================================================================
// Zero interval (flag not supplied) disables the scheduler
// =============================================================================

func TestLoopback_HeartbeatInterval_ZeroIntervalDisablesScheduler(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackServiceWithInterval(t, f, 0)

	hbSub := watchHeartbeat(t, f, svc)
	startService(t, svc)

	drainNone(t, hbSub, 150*time.Millisecond)
}

// =============================================================================
// Payload serialises as valid JSON with correct wire keys
// =============================================================================

func TestLoopback_HeartbeatInterval_PayloadSerializesCorrectly(t *testing.T) {
	f := newLoopbackFixture(t)
	svc, _ := newLoopbackServiceWithInterval(t, f, 40*time.Millisecond)

	hbSub := watchHeartbeat(t, f, svc)
	startService(t, svc)

	raw := drainOne(t, hbSub)

	var outer map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &outer))

	assert.Equal(t, constants.Event.Operator.Heartbeat, outer["event_type"])
	assert.Equal(t, string(models.HeartbeatTypeAutomatic), outer["heartbeat_type"])

	capFlags, ok := outer["capability_flags"].(map[string]interface{})
	require.True(t, ok, "capability_flags must be a nested JSON object")
	_, hasLSE := capFlags["local_storage_enabled"]
	assert.True(t, hasLSE)

	_, topLevelLSE := outer["local_storage_enabled"]
	assert.False(t, topLevelLSE, "local_storage_enabled must not appear at top level")
}

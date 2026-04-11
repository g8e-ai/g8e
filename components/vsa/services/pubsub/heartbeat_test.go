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
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
	"github.com/g8e-ai/g8e/components/vsa/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHeartbeatFixture returns a wired HeartbeatService backed by a mock pubsub client.
func newHeartbeatFixture(t *testing.T) (*HeartbeatService, *MockVSODBPubSubClient, *PubSubResultsService) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.results = resultsSvc
	hs.ctx = context.Background()
	return hs, db, resultsSvc
}

// ---------------------------------------------------------------------------
// Build — structure and field completeness
// ---------------------------------------------------------------------------

func TestBuildHeartbeat_EnvelopeFields(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })
	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.results = resultsSvc
	hs.ctx = context.Background()

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	assert.Equal(t, constants.Event.Operator.Heartbeat, hb.EventType)
	assert.Equal(t, constants.Status.ComponentName.VSA, hb.SourceComponent)
	assert.Equal(t, cfg.OperatorID, hb.OperatorID)
	assert.Equal(t, cfg.OperatorSessionId, hb.OperatorSessionID)
	assert.NotEmpty(t, hb.Timestamp)
}

func TestBuildHeartbeat_VersionFromConfig(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.Version = "2.5.0"
	logger := testutil.NewTestLogger()
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	assert.Equal(t, "2.5.0", hb.VersionInfo.OperatorVersion)
}

func TestBuildHeartbeat_VersionNotHardcoded(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.Version = "0.0.1-test"
	logger := testutil.NewTestLogger()
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	assert.Equal(t, "0.0.1-test", hb.VersionInfo.OperatorVersion)
	assert.NotEqual(t, "1.0.0", hb.VersionInfo.OperatorVersion,
		"hardcoded '1.0.0' must not appear — use config.Version")
}

func TestBuildHeartbeat_HeartbeatTypePassedThrough(t *testing.T) {
	hs, _, _ := newHeartbeatFixture(t)

	cases := []models.HeartbeatType{
		models.HeartbeatTypeAutomatic,
		models.HeartbeatTypeBootstrap,
		models.HeartbeatTypeRequested,
	}
	for _, hbType := range cases {
		hb := hs.Build(hbType)
		assert.Equal(t, hbType, hb.HeartbeatType)
	}
}

func TestBuildHeartbeat_CapabilityFlagsNestedShape(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.LocalStoreEnabled = true
	cfg.GitAvailable = true
	cfg.NoGit = false
	logger := testutil.NewTestLogger()
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	assert.True(t, hb.CapabilityFlags.LocalStorageEnabled)
	assert.True(t, hb.CapabilityFlags.GitAvailable)
	assert.True(t, hb.CapabilityFlags.LedgerMirrorEnabled)
}

func TestBuildHeartbeat_CapabilityFlagsWhenLocalStorageDisabled(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.LocalStoreEnabled = false
	cfg.GitAvailable = false
	cfg.NoGit = false
	logger := testutil.NewTestLogger()
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	assert.False(t, hb.CapabilityFlags.LocalStorageEnabled)
	assert.False(t, hb.CapabilityFlags.GitAvailable)
	assert.False(t, hb.CapabilityFlags.LedgerMirrorEnabled)
}

func TestBuildHeartbeat_LedgerMirrorDisabledWhenNoGit(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.GitAvailable = true
	cfg.NoGit = true
	logger := testutil.NewTestLogger()
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	assert.True(t, hb.CapabilityFlags.GitAvailable)
	assert.False(t, hb.CapabilityFlags.LedgerMirrorEnabled)
}

func TestBuildHeartbeat_AllSectionsPresent(t *testing.T) {
	hs, _, _ := newHeartbeatFixture(t)

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	assert.NotEmpty(t, hb.SystemIdentity.Hostname)
	assert.NotEmpty(t, hb.SystemIdentity.OS)
	assert.NotEmpty(t, hb.SystemIdentity.Architecture)
	assert.Greater(t, hb.SystemIdentity.CPUCount, 0)
	assert.NotEmpty(t, hb.VersionInfo.OperatorVersion)
	assert.NotEmpty(t, hb.VersionInfo.Status)
}

func TestBuildHeartbeat_VersionStatusIsStable(t *testing.T) {
	hs, _, _ := newHeartbeatFixture(t)

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	assert.Equal(t, constants.Status.VersionStability.Stable, hb.VersionInfo.Status)
}

func TestBuildHeartbeat_SerializesToValidJSON(t *testing.T) {
	hs, _, _ := newHeartbeatFixture(t)

	hb := hs.Build(models.HeartbeatTypeAutomatic)

	data, err := json.Marshal(hb)
	require.NoError(t, err)

	var roundTripped models.Heartbeat
	require.NoError(t, json.Unmarshal(data, &roundTripped))
	assert.Equal(t, hb.EventType, roundTripped.EventType)
	assert.Equal(t, hb.HeartbeatType, roundTripped.HeartbeatType)
	assert.Equal(t, hb.VersionInfo.OperatorVersion, roundTripped.VersionInfo.OperatorVersion)
	assert.Equal(t, hb.CapabilityFlags.LocalStorageEnabled, roundTripped.CapabilityFlags.LocalStorageEnabled)
}

func TestBuildHeartbeat_CapabilityFlagsSerializeAsNestedObject(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.LocalStoreEnabled = true
	cfg.GitAvailable = true
	logger := testutil.NewTestLogger()
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)

	hb := hs.Build(models.HeartbeatTypeAutomatic)
	data, err := json.Marshal(hb)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	capFlags, ok := raw["capability_flags"].(map[string]interface{})
	require.True(t, ok, "capability_flags must serialize as a nested JSON object")

	lse, ok := capFlags["local_storage_enabled"].(bool)
	require.True(t, ok)
	assert.True(t, lse)

	_, topLevelLSE := raw["local_storage_enabled"]
	assert.False(t, topLevelLSE, "local_storage_enabled must NOT appear as a top-level field")
}

// ---------------------------------------------------------------------------
// HandleRequest — routing and field propagation
// ---------------------------------------------------------------------------

func TestHandleHeartbeatRequest_UsesHeartbeatTypeRequested(t *testing.T) {
	hs, db, _ := newHeartbeatFixture(t)

	msg := PubSubCommandMessage{
		ID:              "msg-hbreq-1",
		EventType:       constants.Event.Operator.HeartbeatRequested,
		CaseID:          "case-hb-test",
		InvestigationID: "inv-hb-test",
		Payload:         mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp:       time.Now().UTC(),
	}

	hs.HandleRequest(context.Background(), msg)

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb))
	assert.Equal(t, models.HeartbeatTypeRequested, hb.HeartbeatType)
}

func TestHandleHeartbeatRequest_PropagatesCaseID(t *testing.T) {
	hs, db, _ := newHeartbeatFixture(t)

	msg := PubSubCommandMessage{
		ID:        "msg-hbreq-2",
		EventType: constants.Event.Operator.HeartbeatRequested,
		CaseID:    "case-propagate-123",
		Payload:   mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp: time.Now().UTC(),
	}

	hs.HandleRequest(context.Background(), msg)

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb))
	assert.Equal(t, "case-propagate-123", hb.CaseID)
}

func TestHandleHeartbeatRequest_PropagatesInvestigationID(t *testing.T) {
	hs, db, _ := newHeartbeatFixture(t)

	msg := PubSubCommandMessage{
		ID:              "msg-hbreq-3",
		EventType:       constants.Event.Operator.HeartbeatRequested,
		CaseID:          "case-x",
		InvestigationID: "inv-propagate-456",
		Payload:         mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp:       time.Now().UTC(),
	}

	hs.HandleRequest(context.Background(), msg)

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb))
	assert.Equal(t, "inv-propagate-456", hb.InvestigationID)
}

func TestHandleHeartbeatRequest_EmptyInvestigationIDIsValid(t *testing.T) {
	hs, _, _ := newHeartbeatFixture(t)

	msg := PubSubCommandMessage{
		ID:              "msg-hbreq-empty-inv",
		EventType:       constants.Event.Operator.HeartbeatRequested,
		CaseID:          "case-y",
		InvestigationID: "",
		Payload:         mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp:       time.Now().UTC(),
	}

	assert.NotPanics(t, func() {
		hs.HandleRequest(context.Background(), msg)
	})
}

func TestHandleHeartbeatRequest_NoResultsServiceDoesNotPanic(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.ctx = context.Background()

	msg := PubSubCommandMessage{
		ID:        "msg-hbreq-noResults",
		EventType: constants.Event.Operator.HeartbeatRequested,
		CaseID:    "case-z",
		Payload:   mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp: time.Now().UTC(),
	}

	assert.NotPanics(t, func() {
		hs.HandleRequest(context.Background(), msg)
	})
}

func TestHandleHeartbeatRequest_PublishesToCorrectChannel(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })
	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.results = resultsSvc
	hs.ctx = context.Background()

	msg := PubSubCommandMessage{
		ID:        "msg-hbreq-chan",
		EventType: constants.Event.Operator.HeartbeatRequested,
		CaseID:    "case-chan",
		Payload:   mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp: time.Now().UTC(),
	}

	hs.HandleRequest(context.Background(), msg)

	last := db.LastPublished()
	require.NotNil(t, last)

	expectedChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	assert.Equal(t, expectedChannel, last.Channel)
}

// ---------------------------------------------------------------------------
// SendAutomatic — type and channel
// ---------------------------------------------------------------------------

func TestSendAutomaticHeartbeat_PublishesAutomaticType(t *testing.T) {
	hs, db, _ := newHeartbeatFixture(t)

	hs.SendAutomatic()

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb))
	assert.Equal(t, models.HeartbeatTypeAutomatic, hb.HeartbeatType)
}

func TestSendAutomaticHeartbeat_UsesVersionFromConfig(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.Version = "3.1.4"
	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	defer db.Close()

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.results = resultsSvc
	hs.ctx = context.Background()

	hs.SendAutomatic()

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb))
	assert.Equal(t, "3.1.4", hb.VersionInfo.OperatorVersion)
}

func TestSendAutomaticHeartbeat_NilResultsServiceDoesNotPanic(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()
	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.ctx = context.Background()

	assert.NotPanics(t, func() {
		hs.SendAutomatic()
	})
}

// ---------------------------------------------------------------------------
// Heartbeat wire format
// ---------------------------------------------------------------------------

func TestHeartbeatWire_CapabilityFlagsJSONKeys(t *testing.T) {
	hb := &models.Heartbeat{
		CapabilityFlags: models.HeartbeatCapabilityFlags{
			LocalStorageEnabled: true,
			GitAvailable:        true,
			LedgerMirrorEnabled: false,
		},
	}

	data, err := json.Marshal(hb)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	caps, ok := raw["capability_flags"].(map[string]interface{})
	require.True(t, ok, "capability_flags must be a nested JSON object")

	assert.True(t, caps["local_storage_enabled"].(bool))
	assert.True(t, caps["git_available"].(bool))
	ledger, ledgerOK := caps["ledger_enabled"].(bool)
	require.True(t, ledgerOK)
	assert.False(t, ledger)
}

func TestHeartbeatWire_UptimeSecondsIsInteger(t *testing.T) {
	hb := &models.Heartbeat{
		UptimeInfo: models.HeartbeatUptimeInfo{
			Uptime:        "5 days",
			UptimeSeconds: 432000,
		},
	}

	data, err := json.Marshal(hb)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	uptimeInfo, ok := raw["uptime_info"].(map[string]interface{})
	require.True(t, ok)

	secsFloat, ok := uptimeInfo["uptime_seconds"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(432000), secsFloat)
	assert.Equal(t, secsFloat, float64(int64(secsFloat)))
}

func TestHeartbeatWire_NetworkLatencyFieldName(t *testing.T) {
	hb := &models.Heartbeat{
		PerformanceMetrics: models.HeartbeatPerformanceMetrics{
			NetworkLatency: 12.5,
		},
	}

	data, err := json.Marshal(hb)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	perf, ok := raw["performance_metrics"].(map[string]interface{})
	require.True(t, ok)

	_, hasNetworkLatency := perf["network_latency"]
	assert.True(t, hasNetworkLatency)

	_, hasNetworkLatencyMs := perf["network_latency_ms"]
	assert.False(t, hasNetworkLatencyMs)
}

func TestHeartbeatWire_ConnectivityStatusShape(t *testing.T) {
	hb := &models.Heartbeat{
		NetworkInfo: models.HeartbeatNetworkInfo{
			ConnectivityStatus: []models.HeartbeatNetworkInterface{
				{Name: "eth0", IP: "192.168.1.5", MTU: 1500},
			},
		},
	}

	data, err := json.Marshal(hb)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	netInfo, ok := raw["network_info"].(map[string]interface{})
	require.True(t, ok)

	connStatus, ok := netInfo["connectivity_status"].([]interface{})
	require.True(t, ok, "connectivity_status must be an array")
	require.Len(t, connStatus, 1)

	iface, ok := connStatus[0].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "eth0", iface["name"])
	assert.Equal(t, "192.168.1.5", iface["ip"])
	assert.Equal(t, float64(1500), iface["mtu"])

	_, hasHost := iface["host"]
	assert.False(t, hasHost)
	_, hasReachable := iface["reachable"]
	assert.False(t, hasReachable)
}

// ---------------------------------------------------------------------------
// Dispatcher delegates — buildHeartbeat / handleHeartbeatRequest via fixture
// ---------------------------------------------------------------------------

func TestPubSubCommandService_BuildHeartbeatDelegate(t *testing.T) {
	f := newPubsubFixture(t)

	hb := f.Svc.heartbeat.Build(models.HeartbeatTypeBootstrap)

	assert.Equal(t, constants.Event.Operator.Heartbeat, hb.EventType)
	assert.Equal(t, models.HeartbeatTypeBootstrap, hb.HeartbeatType)
}

func TestPubSubCommandService_HandleHeartbeatRequestDelegate(t *testing.T) {
	f := newPubsubFixture(t)

	msg := PubSubCommandMessage{
		ID:        "msg-delegate-1",
		EventType: constants.Event.Operator.HeartbeatRequested,
		CaseID:    "case-del",
		Payload:   mustMarshalJSON(t, models.HeartbeatRequestPayload{}),
		Timestamp: time.Now().UTC(),
	}

	f.Svc.heartbeat.HandleRequest(context.Background(), msg)

	last := f.DB.LastPublished()
	require.NotNil(t, last)
	assert.Contains(t, string(last.Data), constants.Event.Operator.Heartbeat)
}

// ---------------------------------------------------------------------------
// Scheduler — interval flag plumbing and payload verification
// ---------------------------------------------------------------------------

// newHeartbeatFixtureWithInterval creates a HeartbeatService with an explicit
// HeartbeatInterval, mirroring what --heartbeat-interval sets at startup.
func newHeartbeatFixtureWithInterval(t *testing.T, interval time.Duration) (*HeartbeatService, *MockVSODBPubSubClient) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = interval
	cfg.OperatorID = "op-sched-test"
	cfg.OperatorSessionId = "sess-sched-test"
	cfg.Version = "9.8.7"
	cfg.LocalStoreEnabled = true
	cfg.GitAvailable = true
	cfg.NoGit = false

	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.results = resultsSvc

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		hs.StopScheduler()
		wg.Wait()
	})
	hs.ctx = ctx

	return hs, db
}

func TestHeartbeatScheduler_ShortIntervalFiresAndPublishes(t *testing.T) {
	hs, db := newHeartbeatFixtureWithInterval(t, 30*time.Millisecond)

	hs.StartScheduler()

	require.Eventually(t, func() bool {
		return db.PublishedCount() >= 1
	}, 500*time.Millisecond, 5*time.Millisecond, "expected at least one heartbeat to be published")

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb))

	assert.Equal(t, constants.Event.Operator.Heartbeat, hb.EventType)
	assert.Equal(t, models.HeartbeatTypeAutomatic, hb.HeartbeatType)
	assert.Equal(t, constants.Status.ComponentName.VSA, hb.SourceComponent)
}

func TestHeartbeatScheduler_ShortIntervalPublishesMultipleTicks(t *testing.T) {
	hs, db := newHeartbeatFixtureWithInterval(t, 50*time.Millisecond)

	hs.StartScheduler()

	require.Eventually(t, func() bool {
		return db.PublishedCount() >= 3
	}, 2*time.Second, 10*time.Millisecond, "expected at least 3 heartbeats from scheduler")
}

func TestHeartbeatScheduler_PayloadCarriesConfigFields(t *testing.T) {
	hs, db := newHeartbeatFixtureWithInterval(t, 30*time.Millisecond)

	hs.StartScheduler()

	require.Eventually(t, func() bool {
		return db.PublishedCount() >= 1
	}, 500*time.Millisecond, 5*time.Millisecond)

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb))

	assert.Equal(t, "op-sched-test", hb.OperatorID)
	assert.Equal(t, "sess-sched-test", hb.OperatorSessionID)
	assert.Equal(t, "9.8.7", hb.VersionInfo.OperatorVersion)
	assert.True(t, hb.CapabilityFlags.LocalStorageEnabled)
	assert.True(t, hb.CapabilityFlags.GitAvailable)
	assert.True(t, hb.CapabilityFlags.LedgerMirrorEnabled)
}

func TestHeartbeatScheduler_PayloadSystemIdentityPopulated(t *testing.T) {
	hs, db := newHeartbeatFixtureWithInterval(t, 30*time.Millisecond)

	hs.StartScheduler()

	require.Eventually(t, func() bool {
		return db.PublishedCount() >= 1
	}, 500*time.Millisecond, 5*time.Millisecond)

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb))

	assert.NotEmpty(t, hb.SystemIdentity.Hostname)
	assert.NotEmpty(t, hb.SystemIdentity.OS)
	assert.NotEmpty(t, hb.SystemIdentity.Architecture)
	assert.Greater(t, hb.SystemIdentity.CPUCount, 0)
	assert.NotEmpty(t, hb.Timestamp)
}

func TestHeartbeatScheduler_PublishesToHeartbeatChannel(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 30 * time.Millisecond

	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.SetResultsPublisher(resultsSvc)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		hs.StopScheduler()
		wg.Wait()
	})
	hs.SetContext(ctx)

	hs.StartScheduler()

	require.Eventually(t, func() bool {
		return db.PublishedCount() >= 1
	}, 500*time.Millisecond, 5*time.Millisecond)

	last := db.LastPublished()
	require.NotNil(t, last)

	expectedChannel := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	assert.Equal(t, expectedChannel, last.Channel)
}

func TestHeartbeatScheduler_StopsCleanly(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 30 * time.Millisecond

	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.results = resultsSvc
	hs.ctx = context.Background()

	hs.StartScheduler()

	require.Eventually(t, func() bool {
		return db.PublishedCount() >= 1
	}, 500*time.Millisecond, 5*time.Millisecond)

	hs.StopScheduler()
	wg.Wait()

	hs.mu.Lock()
	assert.Nil(t, hs.ticker)
	assert.Nil(t, hs.done)
	hs.mu.Unlock()

	countAfterStop := db.PublishedCount()
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, countAfterStop, db.PublishedCount(), "scheduler must not publish after Stop")
}

func TestHeartbeatScheduler_ContextCancellationStopsScheduler(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 30 * time.Millisecond

	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	resultsSvc, err := NewPubSubResultsService(cfg, logger, db, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.results = resultsSvc

	ctx, cancel := context.WithCancel(context.Background())
	hs.ctx = ctx

	hs.StartScheduler()

	require.Eventually(t, func() bool {
		return db.PublishedCount() >= 1
	}, 500*time.Millisecond, 5*time.Millisecond)

	cancel()
	wg.Wait()

	countAfterCancel := db.PublishedCount()
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, countAfterCancel, db.PublishedCount(), "scheduler must not publish after context cancel")
}

func TestHeartbeatScheduler_ZeroIntervalSkips(t *testing.T) {
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 0

	logger := testutil.NewTestLogger()
	db := NewMockVSODBPubSubClient()
	t.Cleanup(func() { db.Close() })

	var wg sync.WaitGroup
	hs := NewHeartbeatService(cfg, logger, &wg)
	hs.ctx = context.Background()

	hs.StartScheduler()

	hs.mu.Lock()
	assert.Nil(t, hs.ticker, "ticker must not be created when HeartbeatInterval is 0")
	assert.Nil(t, hs.done)
	hs.mu.Unlock()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 0, db.PublishedCount())
}

func TestHeartbeatScheduler_ShortIntervalSerializesValidJSON(t *testing.T) {
	hs, db := newHeartbeatFixtureWithInterval(t, 30*time.Millisecond)

	hs.StartScheduler()

	require.Eventually(t, func() bool {
		return db.PublishedCount() >= 1
	}, 500*time.Millisecond, 5*time.Millisecond)

	last := db.LastPublished()
	require.NotNil(t, last)

	var hb models.Heartbeat
	require.NoError(t, json.Unmarshal(last.Data, &hb), "automatic heartbeat payload must be valid JSON")

	assert.Equal(t, constants.Event.Operator.Heartbeat, hb.EventType)
	assert.Equal(t, models.HeartbeatTypeAutomatic, hb.HeartbeatType)
	assert.NotEmpty(t, hb.Timestamp)
	assert.NotEmpty(t, hb.VersionInfo.Status)
}

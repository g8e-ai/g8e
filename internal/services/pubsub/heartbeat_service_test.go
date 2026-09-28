// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
	commonv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/common/v1"
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// mockExecutionHandler is a test-only implementation of ExecutionHandler.
type mockExecutionHandler struct {
	executed                       atomic.Bool
	err                            error
	ExecuteVerifiedTransactionFunc func(ctx context.Context, eventType constants.EventType, cmdMsg governance.CommandMessage) (string, error)
}

func (m *mockExecutionHandler) ExecuteVerifiedTransaction(ctx context.Context, eventType constants.EventType, cmdMsg governance.CommandMessage) (string, error) {
	m.executed.Store(true)
	if m.ExecuteVerifiedTransactionFunc != nil {
		return m.ExecuteVerifiedTransactionFunc(ctx, eventType, cmdMsg)
	}
	return "", m.err
}

// mockResultsPublisher is a simple manual mock for testing
type mockResultsPublisher struct {
	publishHeartbeatCalled bool
	publishHeartbeatError  error
	heartbeats             []proto.Message

	inferenceCompletions   []*operatorv1.InferenceCompletion
	inferenceCompletionErr error
}

func (m *mockResultsPublisher) PublishExecutionResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	return nil
}

func (m *mockResultsPublisher) PublishCancellationResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	return nil
}

func (m *mockResultsPublisher) PublishFileEditResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	return nil
}

func (m *mockResultsPublisher) PublishFsListResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	return nil
}

func (m *mockResultsPublisher) PublishFsGrepResult(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	return nil
}

func (m *mockResultsPublisher) PublishShutdownAcknowledgement(ctx context.Context, result proto.Message, originalMsg *PubSubCommandMessage) error {
	return nil
}

func (m *mockResultsPublisher) PublishInferenceCompletion(ctx context.Context, env *commonv1.GovernanceEnvelope, completion *operatorv1.InferenceCompletion) error {
	m.inferenceCompletions = append(m.inferenceCompletions, completion)
	return m.inferenceCompletionErr
}

func (m *mockResultsPublisher) PublishInferenceProgress(ctx context.Context, originalMsg *PubSubCommandMessage, progress *operatorv1.InferenceProgressEvent) error {
	return nil
}

func (m *mockResultsPublisher) PublishProviderBoundaryObservationCompleted(ctx context.Context, originalMsgID string, completion *evalv1.ProviderBoundaryObservationCompleted) error {
	return nil
}

func (m *mockResultsPublisher) PublishModelProvenanceObservationCompleted(ctx context.Context, originalMsgID string, completion *evalv1.ModelProvenanceObservationCompleted) error {
	return nil
}

func (m *mockResultsPublisher) PublishExecutionStatus(ctx context.Context, status proto.Message, originalMsg *PubSubCommandMessage) error {
	return nil
}

func (m *mockResultsPublisher) PublishHeartbeat(ctx context.Context, heartbeat proto.Message) error {
	m.publishHeartbeatCalled = true
	m.heartbeats = append(m.heartbeats, heartbeat)
	return m.publishHeartbeatError
}

func (m *mockResultsPublisher) PublishActionReceipt(ctx context.Context, env *commonv1.GovernanceEnvelope, receipt *operatorv1.ActionReceipt) error {
	return nil
}

// capturingConsoleAuditStore records every receipt document passed to DocSet
// so tests can assert on the ActionReceiptRecord fields derived from the
// GovernanceEnvelope without requiring a real SQLite audit store. It is a
// test-only implementation of governance.TransactionAuditStore.
type capturingConsoleAuditStore struct {
	mu      sync.Mutex
	records [][]byte
}

func (c *capturingConsoleAuditStore) DocSet(_, _ string, data json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, append([]byte(nil), data...))
	return nil
}

func (c *capturingConsoleAuditStore) DocDelete(_, _ string) error {
	return nil
}

func TestNewHeartbeatService(t *testing.T) {
	t.Run("returns non-nil service with config and logger", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)
		require.NotNil(t, svc)
		assert.Equal(t, cfg, svc.config)
		assert.Equal(t, logger, svc.logger)
	})
}

func TestHeartbeatService_SetResultsPublisher(t *testing.T) {
	t.Run("accepts nil publisher without panic", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		// Test that the setter exists and doesn't panic
		svc.SetResultsPublisher(nil)
	})
}

func TestHeartbeatService_SetContext(t *testing.T) {
	t.Run("accepts context without panic", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		// Test that the setter exists and doesn't panic
		svc.SetContext(context.TODO())
	})
}

func TestHeartbeatService_Build(t *testing.T) {
	t.Run("builds requested heartbeat", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		require.NotNil(t, heartbeat)
		assert.Equal(t, models.HeartbeatTypeRequested, heartbeat.HeartbeatType)
		assert.Equal(t, constants.Event.Operator.Heartbeat, heartbeat.EventType)
		assert.Equal(t, constants.ComponentNameG8EO, heartbeat.SourceComponent)
		assert.Equal(t, cfg.OperatorID, heartbeat.OperatorID)
		assert.Equal(t, cfg.OperatorSessionId, heartbeat.OperatorSessionID)
		assert.NotEmpty(t, heartbeat.Timestamp)
	})

	t.Run("builds automatic heartbeat", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeAutomatic)
		require.NotNil(t, heartbeat)
		assert.Equal(t, models.HeartbeatTypeAutomatic, heartbeat.HeartbeatType)
	})

	t.Run("includes system identity", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		assert.NotEmpty(t, heartbeat.SystemIdentity.Hostname)
		assert.NotEmpty(t, heartbeat.SystemIdentity.OS)
		assert.NotEmpty(t, heartbeat.SystemIdentity.Architecture)
		assert.NotEmpty(t, heartbeat.SystemIdentity.PWD)
		assert.NotEmpty(t, heartbeat.SystemIdentity.CurrentUser)
		assert.Positive(t, heartbeat.SystemIdentity.CPUCount)
		assert.Positive(t, heartbeat.SystemIdentity.MemoryMB)
	})

	t.Run("includes network info from gateway ports", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		cfg.Gateway.HTTPPort = 8080
		cfg.Gateway.HTTPSPort = 8443
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		assert.Equal(t, 8080, heartbeat.NetworkInfo.HTTPPort)
		assert.Equal(t, 8443, heartbeat.NetworkInfo.HTTPSPort)
		assert.NotEmpty(t, heartbeat.NetworkInfo.Interfaces)
		assert.NotEmpty(t, heartbeat.NetworkInfo.ConnectivityStatus)
	})

	t.Run("includes network info from operator dial ports when gateway ports unset", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		cfg.HTTPPort = 8080
		cfg.HTTPSPort = 8443
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		assert.Equal(t, 8080, heartbeat.NetworkInfo.HTTPPort)
		assert.Equal(t, 8443, heartbeat.NetworkInfo.HTTPSPort)
	})

	t.Run("includes version info", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		assert.Equal(t, cfg.Version, heartbeat.VersionInfo.OperatorVersion)
		assert.Equal(t, constants.VersionStabilityStable, heartbeat.VersionInfo.Status)
	})

	t.Run("includes uptime info", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		assert.NotEmpty(t, heartbeat.UptimeInfo.Uptime)
		assert.GreaterOrEqual(t, heartbeat.UptimeInfo.UptimeSeconds, int64(0))
	})

	t.Run("includes performance metrics", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		assert.GreaterOrEqual(t, heartbeat.PerformanceMetrics.CPUPercent, 0.0)
		assert.GreaterOrEqual(t, heartbeat.PerformanceMetrics.MemoryPercent, 0.0)
		assert.GreaterOrEqual(t, heartbeat.PerformanceMetrics.DiskPercent, 0.0)
		assert.Positive(t, heartbeat.PerformanceMetrics.MemoryUsedMB)
		assert.Positive(t, heartbeat.PerformanceMetrics.MemoryTotalMB)
		assert.GreaterOrEqual(t, heartbeat.PerformanceMetrics.DiskUsedGB, 0.0)
		assert.Greater(t, heartbeat.PerformanceMetrics.DiskTotalGB, 0.0)
	})

	t.Run("includes capability flags", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		assert.Equal(t, cfg.ExecutionVaultEnabled, heartbeat.CapabilityFlags.ExecutionVaultEnabled)
		assert.Equal(t, cfg.GitAvailable, heartbeat.CapabilityFlags.GitAvailable)
		assert.Equal(t, cfg.GitAvailable && !cfg.NoGit, heartbeat.CapabilityFlags.LedgerMirrorEnabled)
	})

	t.Run("includes fingerprint details", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		require.NotNil(t, heartbeat.FingerprintDetails)
		assert.NotEmpty(t, heartbeat.FingerprintDetails.OS)
		assert.NotEmpty(t, heartbeat.FingerprintDetails.Architecture)
		assert.Positive(t, heartbeat.FingerprintDetails.CPUCount)
		assert.Equal(t, cfg.SystemFingerprint, heartbeat.SystemFingerprint)
	})

	t.Run("includes OS details", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		assert.NotNil(t, heartbeat.OSDetails)
		assert.NotNil(t, heartbeat.UserDetails)
		assert.NotNil(t, heartbeat.DiskDetails)
		assert.NotNil(t, heartbeat.MemoryDetails)
		assert.NotNil(t, heartbeat.Environment)
	})
}

func TestHeartbeatService_buildProtoHeartbeat(t *testing.T) {
	t.Run("converts heartbeat to protobuf", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		protoHeartbeat := svc.buildProtoHeartbeat(heartbeat)

		require.NotNil(t, protoHeartbeat)
		assert.Equal(t, heartbeat.OperatorID, protoHeartbeat.OperatorId)
		assert.Equal(t, heartbeat.OperatorSessionID, protoHeartbeat.OperatorSessionId)
		assert.Equal(t, heartbeat.Timestamp, protoHeartbeat.Timestamp)
		assert.Equal(t, string(heartbeat.HeartbeatType), protoHeartbeat.Status)
		assert.Equal(t, string(heartbeat.EventType), protoHeartbeat.EventType)
		assert.Equal(t, string(heartbeat.SourceComponent), protoHeartbeat.SourceComponent)
	})

	t.Run("converts system identity", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		protoHeartbeat := svc.buildProtoHeartbeat(heartbeat)

		require.NotNil(t, protoHeartbeat.SystemIdentity)
		assert.Equal(t, heartbeat.SystemIdentity.Hostname, protoHeartbeat.SystemIdentity.Hostname)
		assert.Equal(t, string(heartbeat.SystemIdentity.OS), protoHeartbeat.SystemIdentity.Os)
		assert.Equal(t, heartbeat.SystemIdentity.Architecture, protoHeartbeat.SystemIdentity.Architecture)
		assert.Equal(t, heartbeat.SystemIdentity.PWD, protoHeartbeat.SystemIdentity.Pwd)
		assert.Equal(t, heartbeat.SystemIdentity.CurrentUser, protoHeartbeat.SystemIdentity.CurrentUser)
		assert.Equal(t, int32(heartbeat.SystemIdentity.CPUCount), protoHeartbeat.SystemIdentity.CpuCount)
		assert.Equal(t, int32(heartbeat.SystemIdentity.MemoryMB), protoHeartbeat.SystemIdentity.MemoryMb)
	})

	t.Run("converts network info", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		protoHeartbeat := svc.buildProtoHeartbeat(heartbeat)

		require.NotNil(t, protoHeartbeat.NetworkInfo)
		assert.Empty(t, protoHeartbeat.NetworkInfo.PublicIp)
		assert.Empty(t, protoHeartbeat.NetworkInfo.InternalIp)
		assert.Equal(t, heartbeat.NetworkInfo.Interfaces, protoHeartbeat.NetworkInfo.Interfaces)
		assert.Len(t, protoHeartbeat.NetworkInfo.ConnectivityStatus, len(heartbeat.NetworkInfo.ConnectivityStatus))
	})

	t.Run("converts performance metrics", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		protoHeartbeat := svc.buildProtoHeartbeat(heartbeat)

		require.NotNil(t, protoHeartbeat.PerformanceMetrics)
		assert.InDelta(t, heartbeat.PerformanceMetrics.CPUPercent, protoHeartbeat.PerformanceMetrics.CpuPercent, 0.001)
		assert.InDelta(t, heartbeat.PerformanceMetrics.MemoryPercent, protoHeartbeat.PerformanceMetrics.MemoryPercent, 0.001)
		assert.InDelta(t, heartbeat.PerformanceMetrics.DiskPercent, protoHeartbeat.PerformanceMetrics.DiskPercent, 0.001)
		assert.InDelta(t, heartbeat.PerformanceMetrics.NetworkLatency, protoHeartbeat.PerformanceMetrics.NetworkLatency, 0.001)
		assert.Equal(t, int32(heartbeat.PerformanceMetrics.MemoryUsedMB), protoHeartbeat.PerformanceMetrics.MemoryUsedMb)
		assert.Equal(t, int32(heartbeat.PerformanceMetrics.MemoryTotalMB), protoHeartbeat.PerformanceMetrics.MemoryTotalMb)
		assert.InDelta(t, heartbeat.PerformanceMetrics.DiskUsedGB, protoHeartbeat.PerformanceMetrics.DiskUsedGb, 0.001)
		assert.InDelta(t, heartbeat.PerformanceMetrics.DiskTotalGB, protoHeartbeat.PerformanceMetrics.DiskTotalGb, 0.001)
	})

	t.Run("converts capability flags", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		protoHeartbeat := svc.buildProtoHeartbeat(heartbeat)

		require.NotNil(t, protoHeartbeat.CapabilityFlags)
		assert.Equal(t, heartbeat.CapabilityFlags.ExecutionVaultEnabled, protoHeartbeat.CapabilityFlags.LocalStorageEnabled)
		assert.Equal(t, heartbeat.CapabilityFlags.GitAvailable, protoHeartbeat.CapabilityFlags.GitAvailable)
		assert.Equal(t, heartbeat.CapabilityFlags.LedgerMirrorEnabled, protoHeartbeat.CapabilityFlags.LedgerMirrorEnabled)
	})

	t.Run("converts fingerprint details when present", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		protoHeartbeat := svc.buildProtoHeartbeat(heartbeat)

		require.NotNil(t, protoHeartbeat.FingerprintDetails)
		assert.Equal(t, string(heartbeat.FingerprintDetails.OS), protoHeartbeat.FingerprintDetails.Os)
		assert.Equal(t, heartbeat.FingerprintDetails.Architecture, protoHeartbeat.FingerprintDetails.Architecture)
		assert.Equal(t, int32(heartbeat.FingerprintDetails.CPUCount), protoHeartbeat.FingerprintDetails.CpuCount)
		assert.Equal(t, heartbeat.FingerprintDetails.MachineID, protoHeartbeat.FingerprintDetails.MachineId)
	})

	t.Run("includes system fingerprint", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		protoHeartbeat := svc.buildProtoHeartbeat(heartbeat)

		assert.Equal(t, heartbeat.SystemFingerprint, protoHeartbeat.SystemFingerprint)
	})

	t.Run("builds proto heartbeat without API key", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := svc.Build(models.HeartbeatTypeRequested)
		_ = svc.buildProtoHeartbeat(heartbeat)

		// API key authentication removed - platform now uses mTLS/CSR-based enrollment
	})
}

func TestHeartbeatService_HandleRequest(t *testing.T) {
	t.Run("handles heartbeat request and publishes", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)
		svc.SetContext(context.Background())

		mockPublisher := &mockResultsPublisher{}
		svc.SetResultsPublisher(mockPublisher)

		req := &operatorv1.HeartbeatRequested{}
		payload, err := proto.Marshal(req)
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			CaseID:            "case-123",
			InvestigationID:   "invest-123",
			OperatorSessionID: "session-123",
			Payload:           payload,
		}

		svc.HandleRequest(context.Background(), msg)
		assert.True(t, mockPublisher.publishHeartbeatCalled)
	})

	t.Run("logs error when payload unmarshal fails", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		mockPublisher := &mockResultsPublisher{}
		svc.SetResultsPublisher(mockPublisher)

		msg := &PubSubCommandMessage{
			Payload: []byte("invalid protobuf"),
		}

		svc.HandleRequest(context.Background(), msg)
		// Should not panic, should log error
		assert.False(t, mockPublisher.publishHeartbeatCalled)
	})

	t.Run("skips publish when results publisher is nil in gateway mode", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		cfg.Gateway.Enabled = true
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)
		svc.SetContext(context.Background())

		req := &operatorv1.HeartbeatRequested{}
		payload, err := proto.Marshal(req)
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			Payload: payload,
		}

		svc.HandleRequest(context.Background(), msg)
		// Should not panic
	})

	t.Run("warns when results publisher is nil in non-gateway mode", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		cfg.Gateway.Enabled = false
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)
		svc.SetContext(context.Background())

		req := &operatorv1.HeartbeatRequested{}
		payload, err := proto.Marshal(req)
		require.NoError(t, err)

		msg := &PubSubCommandMessage{
			Payload: payload,
		}

		svc.HandleRequest(context.Background(), msg)
		// Should not panic, should log warning
	})
}

func TestHeartbeatService_SendAutomatic(t *testing.T) {
	t.Run("publishes an automatic heartbeat directly", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc := NewHeartbeatService(cfg, testutil.NewTestLogger(), nil)
		svc.SetContext(context.Background())
		results := &mockResultsPublisher{}
		svc.SetResultsPublisher(results)

		require.NoError(t, svc.SendAutomatic())

		require.Len(t, results.heartbeats, 1, "automatic heartbeat must be published exactly once")
		published, ok := results.heartbeats[0].(*operatorv1.HeartbeatResult)
		require.True(t, ok, "published heartbeat must be a HeartbeatResult")
		assert.Equal(t, string(models.HeartbeatTypeAutomatic), published.GetStatus(), "automatic heartbeat must not be labeled as a requested heartbeat")
		assert.Equal(t, cfg.OperatorID, published.GetOperatorId())
		assert.Equal(t, cfg.OperatorSessionId, published.GetOperatorSessionId())
	})

	t.Run("returns the publish failure", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc := NewHeartbeatService(cfg, testutil.NewTestLogger(), nil)
		svc.SetContext(context.Background())
		svc.SetResultsPublisher(&mockResultsPublisher{publishHeartbeatError: assert.AnError})

		err := svc.SendAutomatic()
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("skips publication when no results publisher is set", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		svc := NewHeartbeatService(cfg, testutil.NewTestLogger(), nil)
		svc.SetContext(context.Background())

		var sinkCalled atomic.Bool
		svc.RegisterSink(func(context.Context) { sinkCalled.Store(true) })

		require.NoError(t, svc.SendAutomatic())
		assert.True(t, sinkCalled.Load(), "sinks still run when there is nothing to publish")
	})
}

func TestHeartbeatService_Scheduler(t *testing.T) {
	t.Run("starts scheduler with valid interval", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.HeartbeatInterval = 50 * time.Millisecond
		logger := testutil.NewTestLogger()
		wg := &sync.WaitGroup{}
		svc := NewHeartbeatService(cfg, logger, wg)
		svc.SetContext(context.Background())

		svc.StartScheduler()
		defer svc.StopScheduler()

		// Verify scheduler started (ticker is set)
		assert.NotNil(t, svc.ticker)
		assert.NotNil(t, svc.done)
	})

	t.Run("does not start scheduler when interval <= 0", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.HeartbeatInterval = 0
		logger := testutil.NewTestLogger()
		wg := &sync.WaitGroup{}
		svc := NewHeartbeatService(cfg, logger, wg)
		svc.SetContext(context.Background())

		svc.StartScheduler()
		defer svc.StopScheduler()

		// Verify scheduler did not start
		assert.Nil(t, svc.ticker)
		assert.Nil(t, svc.done)
	})

	t.Run("stops scheduler cleanly", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.HeartbeatInterval = 50 * time.Millisecond
		logger := testutil.NewTestLogger()
		wg := &sync.WaitGroup{}
		svc := NewHeartbeatService(cfg, logger, wg)
		svc.SetContext(context.Background())

		svc.StartScheduler()
		time.Sleep(50 * time.Millisecond)
		svc.StopScheduler()

		// Verify scheduler stopped
		assert.Nil(t, svc.ticker)
		assert.Nil(t, svc.done)
	})

	t.Run("stops scheduler when context cancelled", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.HeartbeatInterval = 50 * time.Millisecond
		logger := testutil.NewTestLogger()
		wg := &sync.WaitGroup{}
		svc := NewHeartbeatService(cfg, logger, wg)

		ctx, cancel := context.WithCancel(context.Background())
		svc.SetContext(ctx)

		svc.StartScheduler()
		time.Sleep(50 * time.Millisecond)
		cancel()

		// Wait for goroutine to exit
		time.Sleep(100 * time.Millisecond)
		wg.Wait()
	})

	t.Run("StartSchedulerUnlocked requires lock held", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.HeartbeatInterval = 50 * time.Millisecond
		logger := testutil.NewTestLogger()
		wg := &sync.WaitGroup{}
		svc := NewHeartbeatService(cfg, logger, wg)
		svc.SetContext(context.Background())

		svc.mu.Lock()
		svc.StartSchedulerUnlocked()
		svc.mu.Unlock()

		defer svc.StopScheduler()

		// Verify scheduler started
		assert.NotNil(t, svc.ticker)
		assert.NotNil(t, svc.done)
	})

	t.Run("StopSchedulerUnlocked requires lock held", func(t *testing.T) {
		cfg := testutil.NewTestConfig(t)
		cfg.HeartbeatInterval = 50 * time.Millisecond
		logger := testutil.NewTestLogger()
		wg := &sync.WaitGroup{}
		svc := NewHeartbeatService(cfg, logger, wg)
		svc.SetContext(context.Background())

		svc.StartScheduler()
		time.Sleep(50 * time.Millisecond)

		svc.mu.Lock()
		svc.StopSchedulerUnlocked()
		svc.mu.Unlock()

		// Verify scheduler stopped
		assert.Nil(t, svc.ticker)
		assert.Nil(t, svc.done)
	})
}

func TestHeartbeatService_Publish(t *testing.T) {
	t.Run("publishes heartbeat via results publisher", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		mockPublisher := &mockResultsPublisher{}
		svc.SetResultsPublisher(mockPublisher)

		heartbeat := &operatorv1.HeartbeatResult{
			OperatorId:        "op-1",
			OperatorSessionId: "session-1",
			Status:            "automatic",
		}

		err := svc.Publish(context.Background(), heartbeat)
		require.NoError(t, err)
		assert.True(t, mockPublisher.publishHeartbeatCalled)
	})

	t.Run("returns error when results publisher is nil", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		heartbeat := &operatorv1.HeartbeatResult{
			OperatorId: "op-1",
		}

		err := svc.Publish(context.Background(), heartbeat)
		assert.Error(t, err)
	})

	t.Run("propagates publish error from results publisher", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc := NewHeartbeatService(cfg, logger, nil)

		mockPublisher := &mockResultsPublisher{
			publishHeartbeatError: assert.AnError,
		}
		svc.SetResultsPublisher(mockPublisher)

		heartbeat := &operatorv1.HeartbeatResult{
			OperatorId: "op-1",
		}

		err := svc.Publish(context.Background(), heartbeat)
		assert.Error(t, err)
	})
}

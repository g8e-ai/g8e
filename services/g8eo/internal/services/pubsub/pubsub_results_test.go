package pubsub

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
	pb "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/operatorv1"
	"github.com/g8e-ai/g8e/services/g8eo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func requireLastPublishedUniversal(t *testing.T, db *MockOperatorPubSubClient) []byte {
	t.Helper()
	published := db.LastPublished()
	require.NotNil(t, published, "expected a message to be published")
	return published.Data
}

func mustUnmarshalGovernanceEnvelope(t *testing.T, data []byte) *commonv1.GovernanceEnvelope {
	t.Helper()
	var env commonv1.GovernanceEnvelope
	err := json.Unmarshal(data, &env)
	require.NoError(t, err, "failed to unmarshal GovernanceEnvelope")
	return &env
}

func TestNewPubSubResultsService(t *testing.T) {
	t.Run("creates service", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

func TestPubSubResultsService_PublishExecutionResult(t *testing.T) {
	t.Run("successful publish", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &pb.CommandResult{
			ExecutionId:          "req-123",
			Status:               pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
			Stdout:               "test\n",
			ReturnCode:           0,
			ExecutionTimeSeconds: 2.0,
		}

		originalMsg := PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.Command.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishExecutionResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)

		assert.Equal(t, constants.Event.Operator.Command.Completed, env.EventType)
		assert.Equal(t, "EXECUTE_BASH_RESULT", env.ActionType)
		assert.Equal(t, "case-456", env.CaseId)
		assert.Equal(t, "msg-123", env.Id)

		// Verify payload_type was injected into IntentData for g8ee Pydantic
		require.NotNil(t, env.IntentData)
		assert.Equal(t, "execution_result", env.IntentData.Fields["payload_type"].GetStringValue())

		var payload pb.CommandResult
		err = proto.Unmarshal(env.Payload, &payload)
		require.NoError(t, err)
		assert.Equal(t, "req-123", payload.ExecutionId)
	})
}

func TestPubSubResultsService_PublishFileEditResult(t *testing.T) {
	t.Run("successful publish", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		result := &pb.FileEditResult{
			ExecutionId: "req-123",
			Operation:   "write",
			FilePath:    "/tmp/test.txt",
			Status:      pb.ExecutionStatus_EXECUTION_STATUS_COMPLETED,
		}

		originalMsg := PubSubCommandMessage{
			ID:                "msg-123",
			EventType:         constants.Event.Operator.FileEdit.Requested,
			CaseID:            "case-456",
			OperatorSessionID: "web-session-123",
		}

		err = svc.PublishFileEditResult(context.Background(), result, originalMsg)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, constants.Event.Operator.FileEdit.Completed, env.EventType)
	})
}

func TestPubSubResultsService_PublishHeartbeat(t *testing.T) {
	t.Run("successful heartbeat publish", func(t *testing.T) {
		db := NewMockOperatorPubSubClient()
		cfg := testutil.NewTestConfig(t)
		logger := testutil.NewTestLogger()
		svc, err := NewPubSubResultsService(cfg, logger, db, nil)
		require.NoError(t, err)

		heartbeat := &pb.HeartbeatResult{
			OperatorId:        cfg.OperatorID,
			OperatorSessionId: cfg.OperatorSessionId,
			Status:            "healthy",
			Timestamp:         models.NowTimestamp(),
		}

		err = svc.PublishHeartbeat(context.Background(), heartbeat)
		require.NoError(t, err)

		receivedMsg := requireLastPublishedUniversal(t, db)
		env := mustUnmarshalGovernanceEnvelope(t, receivedMsg)
		assert.Equal(t, "HEARTBEAT_RESULT", env.EventType)
	})
}

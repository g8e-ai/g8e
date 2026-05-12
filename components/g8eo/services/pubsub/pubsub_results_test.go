package pubsub

import (
"context"
"testing"

"github.com/g8e-ai/g8e/components/g8eo/constants"
"github.com/g8e-ai/g8e/components/g8eo/models"
pb "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
"github.com/g8e-ai/g8e/components/g8eo/testutil"
"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
"google.golang.org/protobuf/proto"
)

func requireLastPublishedUAP(t *testing.T, db *MockOperatorPubSubClient) []byte {
t.Helper()
published := db.LastPublished()
require.NotNil(t, published, "expected a message to be published")
return published.Data
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
Status:               protoExecutionStatus(constants.ExecutionStatusCompleted),
Output:               "test\n",
ExitCode:             0,
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

receivedMsg := requireLastPublishedUAP(t, db)
env := testutil.MustUnmarshalUAPEnvelope(t, receivedMsg)

assert.Equal(t, "EXECUTE_BASH_RESULT", env.Intent.ActionType)
assert.Equal(t, "case-456", env.CaseID)
assert.Equal(t, "msg-123", env.MessageID)

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
ExecutionId:     "req-123",
Operation:       "write",
FilePath:        "/tmp/test.txt",
Status:          protoExecutionStatus(constants.ExecutionStatusCompleted),
}

originalMsg := PubSubCommandMessage{
ID:                "msg-123",
EventType:         constants.Event.Operator.FileEdit.Requested,
CaseID:            "case-456",
OperatorSessionID: "web-session-123",
}

err = svc.PublishFileEditResult(context.Background(), result, originalMsg)
require.NoError(t, err)

receivedMsg := requireLastPublishedUAP(t, db)
env := testutil.MustUnmarshalUAPEnvelope(t, receivedMsg)
assert.Equal(t, "FILE_EDIT_RESULT", env.Intent.ActionType)
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

receivedMsg := requireLastPublishedUAP(t, db)
env := testutil.MustUnmarshalUAPEnvelope(t, receivedMsg)
assert.Equal(t, "HEARTBEAT_RESULT", env.Intent.ActionType)
})
}

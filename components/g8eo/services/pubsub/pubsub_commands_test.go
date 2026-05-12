package pubsub

import (
"testing"
"time"

"github.com/g8e-ai/g8e/components/g8eo/constants"
"github.com/g8e-ai/g8e/components/g8eo/testutil"
"github.com/stretchr/testify/assert"
"github.com/stretchr/testify/require"
)

func TestNewPubSubCommandService(t *testing.T) {
t.Run("creates service successfully", func(t *testing.T) {
cfg := testutil.NewTestConfig(t)
svc, err := NewPubSubCommandService(CommandServiceConfig{
Config:       cfg,
Logger:       testutil.NewTestLogger(),
PubSubClient: NewMockOperatorPubSubClient(),
})
require.NoError(t, err)
assert.NotNil(t, svc)
})
}

func TestPubSubCommandService_HandleShutdownRequest_UAP(t *testing.T) {
f := newPubsubFixture(t)
t.Run("successful UAP shutdown", func(t *testing.T) {
reason := "remote control"
msg := PubSubCommandMessage{
ID:        "shutdown-1",
EventType: constants.Event.Operator.ShutdownRequested,
Payload:   []byte(reason),
}
f.Svc.handleShutdownRequest(msg)
select {
case r := <-f.Svc.ShutdownChan:
assert.Equal(t, reason, r)
case <-time.After(1 * time.Second):
t.Fatal("shutdown not received")
}
})
}

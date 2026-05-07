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

// Loopback tests verify the full g8eo pub/sub round-trip using an in-process
// PubSubBroker served via httptest.NewServer.  No external infrastructure is
// required — the G8esPubSubClient connects over plain ws:// to the broker
// running inside the same test process.
//
// This covers the dual role g8eo plays:
//   - Normal mode: subscriber on cmd:{op}:{sess}, publisher on results/heartbeat channels
//   - Listen mode: g8eo IS the broker (PubSubBroker) — other components connect to it
//
// Both roles collapse into a single loopback: one G8esPubSubClient dials the
// in-process broker to subscribe and another (or the same) dials to publish.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/execution"
	"github.com/g8e-ai/g8e/components/g8eo/services/listen"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/commonv1"
	pb "github.com/g8e-ai/g8e/components/g8eo/shared/proto/operatorv1"
	"github.com/g8e-ai/g8e/components/g8eo/shared/proto/pubsubv1"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// =============================================================================
// Test infrastructure
// =============================================================================

// loopbackFixture wires a PubSubBroker behind an httptest.Server so that
// G8esPubSubClient instances can connect over plain ws://.
type loopbackFixture struct {
	broker *listen.PubSubBroker
	server *httptest.Server
	wsURL  string
}

func newLoopbackFixture(t *testing.T) *loopbackFixture {
	t.Helper()
	broker := listen.NewPubSubBroker(testutil.NewTestLogger())
	srv := httptest.NewServer(http.HandlerFunc(broker.HandleWebSocket))
	t.Cleanup(func() {
		srv.Close()
		broker.Close()
	})
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	return &loopbackFixture{broker: broker, server: srv, wsURL: wsURL}
}

// newLoopbackClient creates a G8esPubSubClient connected to the loopback broker.
func (f *loopbackFixture) newClient(t *testing.T) *G8esPubSubClient {
	t.Helper()
	client, err := NewG8esPubSubClient(f.wsURL, "", testutil.NewTestLogger())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

// subscribeAndWait dials a raw WebSocket to the broker, sends a subscribe
// action for channel, and blocks until the broker sends the "subscribed" ack.
// Returns the live connection so the caller can use it as an injector.
// The connection is closed automatically when the test ends.
//
// Use this whenever a test calls broker.Publish (or client.Publish) after
// subscribing — it guarantees the subscription is fully registered before
// any publish, with no time.Sleep required.
func (f *loopbackFixture) subscribeAndWait(t *testing.T, channel string) *websocket.Conn {
	t.Helper()

	wsURL := f.wsURL + "/ws/pubsub"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { ws.Close() })

	subMsg := &pubsubv1.PubSubMessage{
		Action:  constants.PubSubActionSubscribe,
		Channel: channel,
	}
	b, err := proto.Marshal(subMsg)
	require.NoError(t, err)
	require.NoError(t, ws.WriteMessage(websocket.BinaryMessage, b))

	// Read frames until we get the subscribed ack for our channel.
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, raw, err := ws.ReadMessage()
		require.NoError(t, err, "waiting for subscribed ack on %s", channel)
		var ack pubsubv1.PubSubEvent
		if err := proto.Unmarshal(raw, &ack); err != nil {
			continue
		}
		if ack.Type == constants.PubSubEventSubscribed && ack.Channel == channel {
			ws.SetReadDeadline(time.Time{}) // clear deadline
			return ws
		}
	}
}

// drainOne reads one raw payload from a subscription channel with a 3-second timeout.
func drainOne(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case msg, ok := <-ch:
		require.True(t, ok, "subscription channel closed unexpectedly")
		return msg
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for pub/sub message")
		return nil
	}
}

// drainNone asserts that no message arrives within the given window.
func drainNone(t *testing.T, ch <-chan []byte, window time.Duration) {
	t.Helper()
	select {
	case msg := <-ch:
		t.Fatalf("unexpected message received: %s", msg)
	case <-time.After(window):
	}
}

// =============================================================================
// G8esPubSubClient.Subscribe receives broker-published messages
// =============================================================================

func TestLoopback_SubscriberReceivesBrokerPublish(t *testing.T) {
	f := newLoopbackFixture(t)
	client := f.newClient(t)

	ch := constants.ResultsChannel("op1", "sess1")
	sub, err := client.Subscribe(context.Background(), ch)
	require.NoError(t, err)

	// Wait for the subscription to be registered before publishing.
	f.subscribeAndWait(t, ch)

	// In Phase 3, we publish binary Protobuf UniversalEnvelope
	payload := testutil.MustMarshalProtobufCommandRequested(t, "echo hello", "exec-1", "test", "", 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "env-1", constants.Event.Operator.Command.Completed, payload, "", "op1", "case-1", "", "sess1")

	f.broker.Publish(ch, envelopeBytes)

	msg := drainOne(t, sub)
	// The subscriber receives the raw Data field of the PubSubEvent, which is the UniversalEnvelope bytes
	var env commonv1.UniversalEnvelope
	err = proto.Unmarshal(msg, &env)
	require.NoError(t, err)
	assert.Equal(t, constants.Event.Operator.Command.Completed, env.EventType)
}

func TestLoopback_SubscriberDoesNotReceiveOtherChannel(t *testing.T) {
	f := newLoopbackFixture(t)
	client := f.newClient(t)

	ch := constants.ResultsChannel("op1", "sess1")
	sub, err := client.Subscribe(context.Background(), ch)
	require.NoError(t, err)

	f.subscribeAndWait(t, ch)

	f.broker.Publish(constants.ResultsChannel("op2", "sess2"), json.RawMessage(`{"x":1}`))

	drainNone(t, sub, 150*time.Millisecond)
}

// =============================================================================
// G8esPubSubClient.Publish fans out to broker subscribers
// =============================================================================

func TestLoopback_ClientPublishFansOutToSubscriber(t *testing.T) {
	f := newLoopbackFixture(t)

	publisher := f.newClient(t)
	subscriber := f.newClient(t)

	ch := constants.CmdChannel("opA", "sessA")

	sub, err := subscriber.Subscribe(context.Background(), ch)
	require.NoError(t, err)

	// Subscription is confirmed registered before the publish.
	f.subscribeAndWait(t, ch)

	payload := testutil.MustMarshalProtobufCommandRequested(t, "echo hello", "cmd-1", "test", "", 0)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "cmd-1", constants.Event.Operator.Command.Requested, payload, "", "opA", "caseA", "", "sessA")

	require.NoError(t, publisher.Publish(context.Background(), ch, envelopeBytes))

	msg := drainOne(t, sub)
	var env commonv1.UniversalEnvelope
	require.NoError(t, proto.Unmarshal(msg, &env))
	assert.Equal(t, "cmd-1", env.Id)
}

func TestLoopback_ClientPublishFansOutToMultipleSubscribers(t *testing.T) {
	f := newLoopbackFixture(t)

	publisher := f.newClient(t)
	ch := constants.HeartbeatChannel("opB", "sessB")

	const n = 3
	subs := make([]<-chan []byte, n)
	for i := range subs {
		c := f.newClient(t)
		var err error
		subs[i], err = c.Subscribe(context.Background(), ch)
		require.NoError(t, err)
	}

	// Wait until all n subscribers are registered.
	for i := 0; i < n; i++ {
		f.subscribeAndWait(t, ch)
	}

	payload := testutil.MustMarshalProtobufHeartbeatRequested(t)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "hb-1", constants.Event.Operator.Heartbeat, payload, "", "opB", "caseB", "", "sessB")

	require.NoError(t, publisher.Publish(context.Background(), ch, envelopeBytes))

	var wg sync.WaitGroup
	for i, sub := range subs {
		wg.Add(1)
		sub := sub
		i := i
		go func() {
			defer wg.Done()
			msg := drainOne(t, sub)
			var env commonv1.UniversalEnvelope
			require.NoError(t, proto.Unmarshal(msg, &env))
			assert.Equal(t, constants.Event.Operator.Heartbeat, env.EventType, "subscriber %d missed message", i)
		}()
	}
	wg.Wait()
}

// =============================================================================
// Self-loopback: same client is both publisher and subscriber
// This is exactly what g8eo does in listen mode — it connects to the broker
// it is hosting and both sends and receives on its own channels.
// =============================================================================

func TestLoopback_SameClientPublishesAndSubscribes(t *testing.T) {
	f := newLoopbackFixture(t)

	subClient := f.newClient(t)
	pubClient := f.newClient(t)

	ch := constants.ResultsChannel("self", "sess0")

	sub, err := subClient.Subscribe(context.Background(), ch)
	require.NoError(t, err)

	f.subscribeAndWait(t, ch)

	payload := []byte(`{"self":"true"}`) // Raw payload for self-loopback
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "self-1", "test.event", payload, "", "self", "case0", "", "sess0")

	require.NoError(t, pubClient.Publish(context.Background(), ch, envelopeBytes))

	msg := drainOne(t, sub)
	var env commonv1.UniversalEnvelope
	require.NoError(t, proto.Unmarshal(msg, &env))
	assert.Equal(t, "test.event", env.EventType)
	assert.Contains(t, string(env.Payload), `"self"`)
}

// =============================================================================
// Command dispatch: inject a command via broker, verify PubSubCommandService
// dispatches it and publishes a result on the results channel.
// =============================================================================

func TestLoopback_CommandDispatch_HeartbeatRequest(t *testing.T) {
	f := newLoopbackFixture(t)
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 0 // disable scheduler
	logger := testutil.NewTestLogger()

	cmdClient, err := NewG8esPubSubClient(f.wsURL, "", logger)
	require.NoError(t, err)
	t.Cleanup(cmdClient.Close)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, cmdClient, nil)
	require.NoError(t, err)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execution.NewExecutionService(cfg, logger),
		FileEdit:       execution.NewFileEditService(cfg, logger),
		PubSubClient:   cmdClient,
		ResultsService: resultsSvc,
	})
	require.NoError(t, err)

	// Subscribe to the heartbeat channel and confirm registration before starting
	// the service, so the automatic heartbeat is not missed.
	heartbeatClient := f.newClient(t)
	heartbeatCh := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	heartbeatSub, err := heartbeatClient.Subscribe(context.Background(), heartbeatCh)
	require.NoError(t, err)
	f.subscribeAndWait(t, heartbeatCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Stop() //nolint:errcheck

	// Automatic heartbeat is published when the command listener connects.
	msg := drainOne(t, heartbeatSub)
	assert.Contains(t, string(msg), constants.Event.Operator.Heartbeat)
}

func TestLoopback_CommandDispatch_InboundHeartbeatRequest(t *testing.T) {
	f := newLoopbackFixture(t)
	cfg := testutil.NewTestConfig(t)
	cfg.HeartbeatInterval = 0
	logger := testutil.NewTestLogger()

	cmdClient, err := NewG8esPubSubClient(f.wsURL, "", logger)
	require.NoError(t, err)
	t.Cleanup(cmdClient.Close)

	resultsSvc, err := NewPubSubResultsService(cfg, logger, cmdClient, nil)
	require.NoError(t, err)

	svc, err := NewPubSubCommandService(CommandServiceConfig{
		Config:         cfg,
		Logger:         logger,
		Execution:      execution.NewExecutionService(cfg, logger),
		FileEdit:       execution.NewFileEditService(cfg, logger),
		PubSubClient:   cmdClient,
		ResultsService: resultsSvc,
	})
	require.NoError(t, err)

	heartbeatClient := f.newClient(t)
	heartbeatCh := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	heartbeatSub, err := heartbeatClient.Subscribe(context.Background(), heartbeatCh)
	require.NoError(t, err)
	f.subscribeAndWait(t, heartbeatCh)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, svc.Start(ctx))
	defer svc.Stop() //nolint:errcheck

	// Consume the automatic heartbeat published on connect.
	_ = drainOne(t, heartbeatSub)

	// Inject a heartbeat-request on the cmd channel.
	// subscribeAndWait confirms the service's cmd subscription is live before we publish.
	cmdCh := constants.CmdChannel(cfg.OperatorID, cfg.OperatorSessionId)
	f.subscribeAndWait(t, cmdCh)

	hbPayload := testutil.MustMarshalProtobufHeartbeatRequested(t)
	envelopeBytes := testutil.MustMarshalUniversalEnvelope(t, "req-1", constants.Event.Operator.HeartbeatRequested, hbPayload, "", cfg.OperatorID, "case-loopback", "inv-loopback", cfg.OperatorSessionId)

	injector := f.newClient(t)
	require.NoError(t, injector.Publish(context.Background(), cmdCh, envelopeBytes))

	// The service should respond with a heartbeat on the heartbeat channel.
	response := drainOne(t, heartbeatSub)
	assert.Contains(t, string(response), constants.Event.Operator.Heartbeat)
	assert.Contains(t, string(response), "case-loopback")
}

// =============================================================================
// Results round-trip: PubSubResultsService publishes; loopback subscriber receives
// =============================================================================

func TestLoopback_ResultsService_PublishExecutionResult(t *testing.T) {
	f := newLoopbackFixture(t)
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	pubClient := f.newClient(t)
	resultsSvc, err := NewPubSubResultsService(cfg, logger, pubClient, nil)
	require.NoError(t, err)

	subClient := f.newClient(t)
	resultsCh := constants.ResultsChannel(cfg.OperatorID, cfg.OperatorSessionId)
	sub, err := subClient.Subscribe(context.Background(), resultsCh)
	require.NoError(t, err)
	f.subscribeAndWait(t, resultsCh)

	result := &pb.CommandResult{
		ExecutionId:          "exec-loop-1",
		Status:               string(constants.ExecutionStatusCompleted),
		Output:               "hello",
		ExecutionTimeSeconds: 0.01,
	}
	originalMsg := PubSubCommandMessage{
		ID:                "exec-loop-1",
		CaseID:            "case-r1",
		OperatorSessionID: cfg.OperatorSessionId,
	}

	require.NoError(t, resultsSvc.PublishExecutionResult(context.Background(), result, originalMsg))

	msg := drainOne(t, sub)
	assert.Contains(t, string(msg), constants.Event.Operator.Command.Completed)
	assert.Contains(t, string(msg), "hello")
}

func TestLoopback_ResultsService_PublishHeartbeat(t *testing.T) {
	f := newLoopbackFixture(t)
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	pubClient := f.newClient(t)
	resultsSvc, err := NewPubSubResultsService(cfg, logger, pubClient, nil)
	require.NoError(t, err)

	subClient := f.newClient(t)
	heartbeatCh := constants.HeartbeatChannel(cfg.OperatorID, cfg.OperatorSessionId)
	sub, err := subClient.Subscribe(context.Background(), heartbeatCh)
	require.NoError(t, err)
	f.subscribeAndWait(t, heartbeatCh)

	hb := &pb.HeartbeatResult{
		OperatorId:        cfg.OperatorID,
		OperatorSessionId: cfg.OperatorSessionId,
		Status:            "healthy",
		Timestamp:         models.NowTimestamp(),
	}

	require.NoError(t, resultsSvc.PublishHeartbeat(context.Background(), hb))

	msg := drainOne(t, sub)
	assert.Contains(t, string(msg), constants.Event.Operator.Heartbeat)
	assert.Contains(t, string(msg), cfg.OperatorID)
}

// =============================================================================
// Context cancellation stops the subscription channel
// =============================================================================

func TestLoopback_SubscribeCancelStopsChannel(t *testing.T) {
	f := newLoopbackFixture(t)
	client := f.newClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	ch := constants.ResultsChannel("opC", "sessC")

	sub, err := client.Subscribe(ctx, ch)
	require.NoError(t, err)

	cancel()

	select {
	case _, ok := <-sub:
		assert.False(t, ok, "channel should be closed after context cancellation")
	case <-time.After(2 * time.Second):
		t.Fatal("subscription channel did not close after context cancellation")
	}
}

// =============================================================================
// Closed client rejects Publish
// =============================================================================

func TestLoopback_ClosedClientRejectsPublish(t *testing.T) {
	f := newLoopbackFixture(t)
	client := f.newClient(t)

	client.Close()

	err := client.Publish(context.Background(),
		constants.ResultsChannel("opD", "sessD"), json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// =============================================================================
// MockG8esPubSubClient self-contained loopback
// When no real broker is needed (pure logic tests), the mock fans out Publish
// to its own Subscribe channels in-process — exactly the same loopback contract.
// =============================================================================

func TestMockClient_PublishFansOutToSubscriber(t *testing.T) {
	mock := NewMockG8esPubSubClient()
	defer mock.Close()

	ch := constants.CmdChannel("opM", "sessM")
	sub, err := mock.Subscribe(context.Background(), ch)
	require.NoError(t, err)

	payload := json.RawMessage(fmt.Sprintf(`{"event_type":"%s"}`, constants.Event.Operator.Command.Requested))
	require.NoError(t, mock.Publish(context.Background(), ch, payload))

	select {
	case msg := <-sub:
		assert.Contains(t, string(msg), constants.Event.Operator.Command.Requested)
	case <-time.After(time.Second):
		t.Fatal("mock did not fan out to subscriber")
	}
}

func TestMockClient_InjectMessageDelivers(t *testing.T) {
	mock := NewMockG8esPubSubClient()
	defer mock.Close()

	ch := constants.HeartbeatChannel("opI", "sessI")
	sub, err := mock.Subscribe(context.Background(), ch)
	require.NoError(t, err)

	mock.InjectMessage(ch, json.RawMessage(`{"event_type":"operator.heartbeat"}`))

	select {
	case msg := <-sub:
		assert.Contains(t, string(msg), "operator.heartbeat")
	case <-time.After(time.Second):
		t.Fatal("injected message was not delivered")
	}
}

func TestMockClient_CloseStopsAllSubscribers(t *testing.T) {
	mock := NewMockG8esPubSubClient()

	ch := constants.ResultsChannel("opK", "sessK")
	sub, err := mock.Subscribe(context.Background(), ch)
	require.NoError(t, err)

	mock.Close()

	select {
	case _, ok := <-sub:
		assert.False(t, ok, "subscriber channel should be closed after Close()")
	case <-time.After(time.Second):
		t.Fatal("subscriber channel not closed after mock.Close()")
	}
}

func TestMockClient_PublishedRecordsAllMessages(t *testing.T) {
	mock := NewMockG8esPubSubClient()
	defer mock.Close()

	ch := constants.ResultsChannel("opR", "sessR")
	for i := 0; i < 3; i++ {
		require.NoError(t, mock.Publish(context.Background(), ch, json.RawMessage(`{"n":1}`)))
	}

	assert.Equal(t, 3, mock.PublishedCount())
	msgs := mock.Published()
	require.Len(t, msgs, 3)
	for _, m := range msgs {
		assert.Equal(t, ch, m.Channel)
	}
}

func TestMockClient_ResetClearsPublished(t *testing.T) {
	mock := NewMockG8esPubSubClient()
	defer mock.Close()

	ch := constants.CmdChannel("opReset", "sessReset")
	require.NoError(t, mock.Publish(context.Background(), ch, json.RawMessage(`{}`)))
	require.Equal(t, 1, mock.PublishedCount())

	mock.Reset()
	assert.Equal(t, 0, mock.PublishedCount())
	assert.Nil(t, mock.LastPublished())
}

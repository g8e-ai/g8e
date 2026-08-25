//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time assertion that mockProgramSender implements programSender.
var _ programSender = (*mockProgramSender)(nil)

// mockProgramSender captures messages sent by monitorPasskeyRegistration
// without running a real bubbletea terminal program.
type mockProgramSender struct {
	msgCh chan tea.Msg
}

func newMockProgramSender() *mockProgramSender {
	return &mockProgramSender{msgCh: make(chan tea.Msg, 16)}
}

func (m *mockProgramSender) Send(msg tea.Msg) {
	m.msgCh <- msg
}

func (m *mockProgramSender) waitForMsg(timeout time.Duration) (tea.Msg, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-m.msgCh:
		return msg, true
	case <-timer.C:
		return nil, false
	}
}

// passkeyRegisteredEnvelope builds an SSEPushPayload JSON envelope with the
// given user ID, CLI session ID, and inner event type. Used by SSE monitor
// tests to construct event data that matches the registrar's filtering logic.
func passkeyRegisteredEnvelope(t *testing.T, userID, cliSessionID string) string {
	t.Helper()
	innerJSON, err := json.Marshal(map[string]string{"type": "passkey.registered"})
	require.NoError(t, err)
	envelope := models.SSEPushPayload{
		UserID:       userID,
		CliSessionID: cliSessionID,
		Event:        innerJSON,
	}
	b, err := json.Marshal(envelope)
	require.NoError(t, err)
	return string(b)
}

func TestMonitorPasskeyRegistration_SSEEventTriggersRegisteredMsg(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "ResponseWriter must support flushing")
		fmt.Fprintf(w, "event: passkey.registered\ndata: %s\n\n",
			passkeyRegisteredEnvelope(t, "test-user", "test-session"))
		flusher.Flush()
		<-r.Context().Done()
	}
	server := startTLSEnrollServer(t, cfg, handler)

	sseURL := fmt.Sprintf("%s/sse?since_id=0", server.URL)
	httpClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, "test-session")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	sender := newMockProgramSender()
	go r.monitorPasskeyRegistration(ctx, sseClient, sender, "test-user", "test-session", cancel)

	msg, ok := sender.waitForMsg(3 * time.Second)
	require.True(t, ok, "expected passkeyRegisteredMsg within timeout")
	_, isRegistered := msg.(passkeyRegisteredMsg)
	assert.True(t, isRegistered, "expected passkeyRegisteredMsg, got %T", msg)
}

// TestMonitorPasskeyRegistration_NoEventHeaderExtractsTypeFromPayload verifies
// that when the server omits the SSE event: field (R14), the consumer extracts
// the event type from the inner SSEPushPayload.Event.type field and still
// triggers the passkeyRegisteredMsg.
func TestMonitorPasskeyRegistration_NoEventHeaderExtractsTypeFromPayload(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "ResponseWriter must support flushing")
		// Send the envelope without the event: header so the monitor
		// must extract the type from the inner Event.type field.
		fmt.Fprintf(w, "data: %s\n\n",
			passkeyRegisteredEnvelope(t, "test-user", "test-session"))
		flusher.Flush()
		<-r.Context().Done()
	}
	server := startTLSEnrollServer(t, cfg, handler)

	sseURL := fmt.Sprintf("%s/sse?since_id=0", server.URL)
	httpClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, "test-session")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	sender := newMockProgramSender()
	go r.monitorPasskeyRegistration(ctx, sseClient, sender, "test-user", "test-session", cancel)

	msg, ok := sender.waitForMsg(3 * time.Second)
	require.True(t, ok, "expected passkeyRegisteredMsg within timeout")
	_, isRegistered := msg.(passkeyRegisteredMsg)
	assert.True(t, isRegistered, "expected passkeyRegisteredMsg from no-event-header payload, got %T", msg)
}

func TestMonitorPasskeyRegistration_TimeoutSendsEnrollErrMsg(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		<-r.Context().Done()
	}
	server := startTLSEnrollServer(t, cfg, handler)

	sseURL := fmt.Sprintf("%s/sse?since_id=0", server.URL)
	httpClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, "test-session")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	sender := newMockProgramSender()
	go r.monitorPasskeyRegistration(ctx, sseClient, sender, "test-user", "test-session", cancel)

	msg, ok := sender.waitForMsg(3 * time.Second)
	require.True(t, ok, "expected enrollErrMsg within timeout")
	errMsg, isErr := msg.(enrollErrMsg)
	assert.True(t, isErr, "expected enrollErrMsg, got %T", msg)
	assert.ErrorIs(t, errMsg.err, constants.ErrPasskeyRegistrationTimedOut)
}

func TestMonitorPasskeyRegistration_SSEStreamClosedSendsEnrollErrMsg(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "ResponseWriter must support flushing")
		flusher.Flush()
		// Immediately close the connection without sending any events
	}

	server := startTLSEnrollServer(t, cfg, handler)

	sseURL := fmt.Sprintf("%s/sse?since_id=0", server.URL)
	httpClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, "test-session")

	// Use a long context timeout so the SSE stream close is what triggers the message,
	// not the context expiry.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	sender := newMockProgramSender()
	go r.monitorPasskeyRegistration(ctx, sseClient, sender, "test-user", "test-session", cancel)

	msg, ok := sender.waitForMsg(5 * time.Second)
	require.True(t, ok, "expected enrollErrMsg within timeout after SSE stream closed")
	errMsg, isErr := msg.(enrollErrMsg)
	assert.True(t, isErr, "expected enrollErrMsg, got %T", msg)
	assert.ErrorIs(t, errMsg.err, constants.ErrPasskeySSEStreamClosed)
}

func TestGenerateEnrollmentToken_NetworkErrorReturnsErrHTTPRequestExecuteFailed(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	// Start a TLS server solely to set up the trust bundle (CA cert).
	// We then override the host to a non-listening port so the actual
	// HTTP request fails with connection refused.
	startTLSEnrollServer(t, cfg, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	cfg.Paths.Host = "https://127.0.0.1:1"

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	mtlsClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)

	_, err = r.generateEnrollmentToken(context.Background(), mtlsClient, "test-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrHTTPRequestExecuteFailed)
}

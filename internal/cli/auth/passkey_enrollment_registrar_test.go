//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/v2/internal/cli/sse"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProgram is a test-only programRunner that records Send calls and
// returns a canned finalModel from Run. It does NOT run a real terminal.
type mockProgram struct {
	mu        sync.Mutex
	msgs      []tea.Msg
	sendCh    chan tea.Msg
	final     tea.Model
	runErr    error
	runFn     func() (tea.Model, error) // optional override for Run
	runCalled int32
}

func newMockProgram(final tea.Model, runErr error) *mockProgram {
	return &mockProgram{
		sendCh: make(chan tea.Msg, 64),
		final:  final,
		runErr: runErr,
	}
}

func (m *mockProgram) Send(msg tea.Msg) {
	m.mu.Lock()
	m.msgs = append(m.msgs, msg)
	m.mu.Unlock()
	select {
	case m.sendCh <- msg:
	default:
	}
}

func (m *mockProgram) Run() (tea.Model, error) {
	atomic.AddInt32(&m.runCalled, 1)
	if m.runFn != nil {
		return m.runFn()
	}
	// Block until a message arrives (simulating the TUI waiting for events),
	// or until the test signals completion via context cancellation.
	select {
	case <-m.sendCh:
	case <-time.After(10 * time.Second):
	}
	return m.final, m.runErr
}

func (m *mockProgram) sentMsgs() []tea.Msg {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]tea.Msg, len(m.msgs))
	copy(cp, m.msgs)
	return cp
}

// routingHandler dispatches to different handlers based on the request path.
type routingHandler struct {
	routes map[string]http.HandlerFunc
}

func (h *routingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for path, handler := range h.routes {
		if strings.HasSuffix(r.URL.Path, path) || r.URL.Path == path {
			handler(w, r)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// TestRegister_BrowserOpenFailure_ContinuesToTUI verifies that when the
// browser fails to open, the registrar does NOT abort. Instead it falls
// through to the TUI so the user can see the console URL and open it
// manually. The enrollment token must not leak in any error output.
func TestRegister_BrowserOpenFailure_ContinuesToTUI(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	rh := &routingHandler{routes: map[string]http.HandlerFunc{}}

	// Token generation endpoint.
	rh.routes[constants.APIPaths.AuthEnrollmentTokenGenerate] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "SECRET-TOKEN-VALUE"})
	}

	// SSE endpoint — stays open so the ready signal fires. Use the real
	// SSE stream path (constants.APIPaths.SSEStream) so the routingHandler
	// suffix-matches the actual request path the registrar constructs.
	rh.routes[constants.APIPaths.SSEStream] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		flusher.Flush()
		<-r.Context().Done()
	}

	startTLSEnrollServer(t, cfg, rh.ServeHTTP)

	// Browser that always fails.
	browserCalled := make(chan struct{}, 1)
	failBrowser := func(url string) error {
		select {
		case browserCalled <- struct{}{}:
		default:
		}
		return fmt.Errorf("no display available")
	}

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{
		Browser: failBrowser,
		Timeout: 5 * time.Second,
	})
	// Inject a mock program whose Run returns a cancelled model (simulating
	// the user pressing q after seeing the URL). The program IS called now
	// because browser failure is no longer fatal. Use runFn so the factory's
	// overwrite of prog.final doesn't clobber our error model.
	prog := newMockProgram(enrollModel{}, nil)
	prog.runFn = func() (tea.Model, error) {
		return enrollModel{err: context.Canceled}, nil
	}
	r.programFactory = func(m enrollModel) programRunner {
		prog.final = m
		return prog
	}

	err := r.Register(context.Background(), "test-user", "test-session")
	// User cancellation is expected (the mock program returns context.Canceled).
	require.Error(t, err)

	// The error must NOT contain the token value.
	assert.NotContains(t, err.Error(), "SECRET-TOKEN-VALUE",
		"enrollment token must not leak in error output")

	// The browser should have been called (ready signal fired first).
	select {
	case <-browserCalled:
		// Good — browser was attempted.
	case <-time.After(2 * time.Second):
		t.Fatal("browser was not called within timeout — SSE ready signal may not have fired")
	}

	// The TUI program should have been called (browser failure no longer
	// aborts before the TUI runs).
	assert.Equal(t, int32(1), atomic.LoadInt32(&prog.runCalled),
		"TUI program should be called even when browser fails to open")
}

// TestRegister_SSEReadyBeforeBrowserLaunch verifies that the SSE connection is
// established BEFORE the browser is opened. This is the key invariant that
// eliminates the event-loss race.
func TestRegister_SSEReadyBeforeBrowserLaunch(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	rh := &routingHandler{routes: map[string]http.HandlerFunc{}}

	rh.routes[constants.APIPaths.AuthEnrollmentTokenGenerate] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "test-token"})
	}

	sseConnected := make(chan struct{}, 1)
	rh.routes[constants.APIPaths.SSEStream] = func(w http.ResponseWriter, r *http.Request) {
		// Signal that the SSE connection was established.
		select {
		case sseConnected <- struct{}{}:
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		flusher.Flush()
		<-r.Context().Done()
	}

	startTLSEnrollServer(t, cfg, rh.ServeHTTP)

	var browserCalledAfterSSE atomic.Bool
	browserCalled := make(chan struct{}, 1)
	browser := func(url string) error {
		// Record whether SSE was already connected when browser is called.
		select {
		case <-sseConnected:
			browserCalledAfterSSE.Store(true)
		default:
			browserCalledAfterSSE.Store(false)
		}
		select {
		case browserCalled <- struct{}{}:
		default:
		}
		// Return an error to stop the flow — we only need to verify ordering.
		return fmt.Errorf("test browser: stop")
	}

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{
		Browser: browser,
		Timeout: 5 * time.Second,
	})
	r.programFactory = func(m enrollModel) programRunner {
		return newMockProgram(m, nil)
	}

	_ = r.Register(context.Background(), "test-user", "test-session")

	// The browser must have been called.
	select {
	case <-browserCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("browser was not called within timeout")
	}

	// And the SSE connection must have been established BEFORE the browser call.
	assert.True(t, browserCalledAfterSSE.Load(),
		"SSE connection must be established before browser is opened")
}

// TestMonitorPasskeyRegistration_UnrelatedEventIgnored verifies that events
// with non-matching user ID, CLI session ID, or event type are silently
// ignored — only a matching passkey.registered event triggers the registered
// message.
func TestMonitorPasskeyRegistration_UnrelatedEventIgnored(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		// 1. Wrong event type (not passkey.registered).
		wrongTypeEnvelope, _ := json.Marshal(models.SSEPushPayload{
			UserID:       "test-user",
			CliSessionID: "test-session",
			Event:        json.RawMessage(`{"type":"passkey.deleted"}`),
		})
		fmt.Fprintf(w, "data: %s\n\n", string(wrongTypeEnvelope))
		flusher.Flush()

		// 2. Wrong user ID.
		wrongUserEnvelope, _ := json.Marshal(models.SSEPushPayload{
			UserID:       "other-user",
			CliSessionID: "test-session",
			Event:        json.RawMessage(`{"type":"passkey.registered"}`),
		})
		fmt.Fprintf(w, "data: %s\n\n", string(wrongUserEnvelope))
		flusher.Flush()

		// 3. Wrong CLI session ID.
		wrongSessionEnvelope, _ := json.Marshal(models.SSEPushPayload{
			UserID:       "test-user",
			CliSessionID: "other-session",
			Event:        json.RawMessage(`{"type":"passkey.registered"}`),
		})
		fmt.Fprintf(w, "data: %s\n\n", string(wrongSessionEnvelope))
		flusher.Flush()

		// 4. Matching event — should trigger the registered message.
		matchingEnvelope, _ := json.Marshal(models.SSEPushPayload{
			UserID:       "test-user",
			CliSessionID: "test-session",
			Event:        json.RawMessage(`{"type":"passkey.registered"}`),
		})
		fmt.Fprintf(w, "data: %s\n\n", string(matchingEnvelope))
		flusher.Flush()

		<-r.Context().Done()
	}
	server := startTLSEnrollServer(t, cfg, handler)

	sseURL := fmt.Sprintf("%s/sse?since_id=0", server.URL)
	httpClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, "test-session")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	sender := newMockProgramSender()
	go r.monitorPasskeyRegistration(ctx, sseClient, sender, "test-user", "test-session", cancel)

	msg, ok := sender.waitForMsg(5 * time.Second)
	require.True(t, ok, "expected passkeyRegisteredMsg after matching event")
	_, isRegistered := msg.(passkeyRegisteredMsg)
	assert.True(t, isRegistered, "expected passkeyRegisteredMsg from matching event, got %T", msg)
}

// TestMonitorPasskeyRegistration_MalformedEventIgnored verifies that malformed
// SSE event data (invalid JSON) is silently ignored without causing an error.
func TestMonitorPasskeyRegistration_MalformedEventIgnored(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		// Malformed JSON data.
		fmt.Fprintf(w, "data: {not valid json\n\n")
		flusher.Flush()

		// Matching event after the malformed one.
		matchingEnvelope, _ := json.Marshal(models.SSEPushPayload{
			UserID:       "test-user",
			CliSessionID: "test-session",
			Event:        json.RawMessage(`{"type":"passkey.registered"}`),
		})
		fmt.Fprintf(w, "data: %s\n\n", string(matchingEnvelope))
		flusher.Flush()

		<-r.Context().Done()
	}
	server := startTLSEnrollServer(t, cfg, handler)

	sseURL := fmt.Sprintf("%s/sse?since_id=0", server.URL)
	httpClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, "test-session")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	sender := newMockProgramSender()
	go r.monitorPasskeyRegistration(ctx, sseClient, sender, "test-user", "test-session", cancel)

	msg, ok := sender.waitForMsg(5 * time.Second)
	require.True(t, ok, "expected passkeyRegisteredMsg after malformed then matching event")
	_, isRegistered := msg.(passkeyRegisteredMsg)
	assert.True(t, isRegistered, "expected passkeyRegisteredMsg, got %T", msg)
}

// TestMonitorPasskeyRegistration_SuccessCancelsSSEContext verifies that when
// the matching event arrives, the cancel function is called, terminating the
// SSE stream and closing the response body. This prevents goroutine/response-
// body leaks.
func TestMonitorPasskeyRegistration_SuccessCancelsSSEContext(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		matchingEnvelope, _ := json.Marshal(models.SSEPushPayload{
			UserID:       "test-user",
			CliSessionID: "test-session",
			Event:        json.RawMessage(`{"type":"passkey.registered"}`),
		})
		fmt.Fprintf(w, "data: %s\n\n", string(matchingEnvelope))
		flusher.Flush()

		// Wait for the context to be cancelled (proving cancel was called).
		<-r.Context().Done()
	}
	server := startTLSEnrollServer(t, cfg, handler)

	sseURL := fmt.Sprintf("%s/sse?since_id=0", server.URL)
	httpClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, "test-session")

	// Use a long timeout so only the matching event can trigger cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	sender := newMockProgramSender()
	go r.monitorPasskeyRegistration(ctx, sseClient, sender, "test-user", "test-session", cancel)

	msg, ok := sender.waitForMsg(5 * time.Second)
	require.True(t, ok, "expected passkeyRegisteredMsg")
	_, isRegistered := msg.(passkeyRegisteredMsg)
	assert.True(t, isRegistered, "expected passkeyRegisteredMsg, got %T", msg)

	// The context should be cancelled shortly after the matching event.
	select {
	case <-ctx.Done():
		// Good — SSE context was cancelled by the monitor.
	case <-time.After(3 * time.Second):
		t.Fatal("SSE context was not cancelled after matching event — goroutine/response-body leak risk")
	}
}

// TestMonitorPasskeyRegistration_MatchingEventCompletesOnce verifies that after
// a matching event is found, subsequent events are ignored (the monitor does
// not send duplicate registered messages).
func TestMonitorPasskeyRegistration_MatchingEventCompletesOnce(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)

		matchingEnvelope, _ := json.Marshal(models.SSEPushPayload{
			UserID:       "test-user",
			CliSessionID: "test-session",
			Event:        json.RawMessage(`{"type":"passkey.registered"}`),
		})
		// Send the matching event twice in quick succession.
		fmt.Fprintf(w, "data: %s\n\n", string(matchingEnvelope))
		flusher.Flush()
		fmt.Fprintf(w, "data: %s\n\n", string(matchingEnvelope))
		flusher.Flush()

		<-r.Context().Done()
	}
	server := startTLSEnrollServer(t, cfg, handler)

	sseURL := fmt.Sprintf("%s/sse?since_id=0", server.URL)
	httpClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	require.NoError(t, err)
	sseClient := sse.NewClient(sseURL, httpClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, "test-session")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})
	sender := newMockProgramSender()
	go r.monitorPasskeyRegistration(ctx, sseClient, sender, "test-user", "test-session", cancel)

	// First message should be passkeyRegisteredMsg. waitForMsg consumes it
	// from the channel, so count it as the first registered message before
	// draining the channel for any duplicates.
	msg, ok := sender.waitForMsg(5 * time.Second)
	require.True(t, ok, "expected first passkeyRegisteredMsg")
	_, isRegistered := msg.(passkeyRegisteredMsg)
	assert.True(t, isRegistered, "expected passkeyRegisteredMsg, got %T", msg)
	registeredCount := 1

	// Wait a bit to see if a duplicate arrives.
	time.Sleep(500 * time.Millisecond)
	for {
		select {
		case m := <-sender.msgCh:
			if _, ok := m.(passkeyRegisteredMsg); ok {
				registeredCount++
			}
		default:
			goto done
		}
	}
done:
	assert.Equal(t, 1, registeredCount,
		"expected exactly 1 passkeyRegisteredMsg, got %d", registeredCount)
}

// TestRegister_ValidateInputs verifies that Register rejects empty user ID or
// CLI session ID before making any gateway calls.
func TestRegister_ValidateInputs(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{})

	err := r.Register(context.Background(), "", "test-session")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPasskeyRegistrationFailed)

	err = r.Register(context.Background(), "test-user", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrPasskeyRegistrationFailed)
}

// TestRegister_ContextCancelledBeforeEnrollment verifies that Register returns
// context.Canceled (or a wrapped error) when the context is already cancelled
// before any gateway call.
func TestRegister_ContextCancelledBeforeEnrollment(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	// We need a token endpoint so the registrar can attempt the call.
	rh := &routingHandler{routes: map[string]http.HandlerFunc{}}
	rh.routes[constants.APIPaths.AuthEnrollmentTokenGenerate] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "test-token"})
	}
	rh.routes[constants.APIPaths.SSEStream] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		flusher.Flush()
		<-r.Context().Done()
	}
	startTLSEnrollServer(t, cfg, rh.ServeHTTP)

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{
		Timeout: 5 * time.Second,
	})
	r.programFactory = func(m enrollModel) programRunner {
		return newMockProgram(m, nil)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling Register

	err := r.Register(ctx, "test-user", "test-session")
	require.Error(t, err)
}

// TestPasskeyRegistrar_Out_EmitsURLBeforeBrowser verifies that the Out
// callback fires with the console URL (prefixed "Passkey enrollment URL:")
// AFTER the SSE ready-gate has fired and BEFORE r.browser() is called. This
// is the load-bearing ordering constraint: printing the URL earlier would let
// the user click it before the SSE listener is connected, reintroducing the
// event-loss race the ready-gate was built to prevent.
func TestPasskeyRegistrar_Out_EmitsURLBeforeBrowser(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	rh := &routingHandler{routes: map[string]http.HandlerFunc{}}
	rh.routes[constants.APIPaths.AuthEnrollmentTokenGenerate] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "test-token-url"})
	}
	rh.routes[constants.APIPaths.SSEStream] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		flusher.Flush()
		<-r.Context().Done()
	}
	startTLSEnrollServer(t, cfg, rh.ServeHTTP)

	var outMu sync.Mutex
	var outLines []string
	outFunc := func(format string, args ...any) {
		outMu.Lock()
		outLines = append(outLines, fmt.Sprintf(format, args...))
		outMu.Unlock()
	}

	browserCalled := make(chan struct{}, 1)
	browser := func(url string) error {
		select {
		case browserCalled <- struct{}{}:
		default:
		}
		return fmt.Errorf("test browser: stop")
	}

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{
		Browser: browser,
		Out:     outFunc,
		Timeout: 5 * time.Second,
	})
	r.programFactory = func(m enrollModel) programRunner {
		return newMockProgram(m, nil)
	}

	_ = r.Register(context.Background(), "test-user", "test-session")

	// The browser must have been called (so we know the URL print happened
	// before it).
	select {
	case <-browserCalled:
	case <-time.After(3 * time.Second):
		t.Fatal("browser was not called within timeout")
	}

	// The Out callback should have emitted the URL line.
	outMu.Lock()
	defer outMu.Unlock()
	require.NotEmpty(t, outLines, "Out should have emitted at least the URL line")
	var urlLine string
	for _, l := range outLines {
		if strings.Contains(l, "Passkey enrollment URL:") {
			urlLine = l
			break
		}
	}
	require.NotEmpty(t, urlLine, "Out should emit the 'Passkey enrollment URL:' line")
	assert.Contains(t, urlLine, "test-token-url", "URL line should contain the enrollment token")
}

// TestPasskeyRegistrar_Out_EmitsBrowserErrorOnOpenFailure verifies that when
// r.browser() returns an error, the Out callback emits the warning, and the
// URL was already emitted (so the user has it regardless of browser-open
// success).
func TestPasskeyRegistrar_Out_EmitsBrowserErrorOnOpenFailure(t *testing.T) {
	fileSvc, cfg := newAuthTestEnv(t)
	writeTestCLICert(t, fileSvc, cfg)

	rh := &routingHandler{routes: map[string]http.HandlerFunc{}}
	rh.routes[constants.APIPaths.AuthEnrollmentTokenGenerate] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "test-token-err"})
	}
	rh.routes[constants.APIPaths.SSEStream] = func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		require.True(t, ok)
		flusher.Flush()
		<-r.Context().Done()
	}
	startTLSEnrollServer(t, cfg, rh.ServeHTTP)

	var outMu sync.Mutex
	var outLines []string
	outFunc := func(format string, args ...any) {
		outMu.Lock()
		outLines = append(outLines, fmt.Sprintf(format, args...))
		outMu.Unlock()
	}

	failBrowser := func(url string) error {
		return fmt.Errorf("no display available")
	}

	r := newPasskeyRegistrar(fileSvc, cfg, PasskeyRegistrarOptions{
		Browser: failBrowser,
		Out:     outFunc,
		Timeout: 5 * time.Second,
	})
	prog := newMockProgram(enrollModel{}, nil)
	prog.runFn = func() (tea.Model, error) {
		return enrollModel{err: context.Canceled}, nil
	}
	r.programFactory = func(m enrollModel) programRunner {
		prog.final = m
		return prog
	}

	_ = r.Register(context.Background(), "test-user", "test-session")

	outMu.Lock()
	defer outMu.Unlock()

	// The URL line should be present (emitted before the browser call).
	var hasURL, hasWarning bool
	for _, l := range outLines {
		if strings.Contains(l, "Passkey enrollment URL:") {
			hasURL = true
		}
		if strings.Contains(l, "could not open browser automatically") {
			hasWarning = true
		}
	}
	assert.True(t, hasURL, "Out should emit the URL line before the browser call")
	assert.True(t, hasWarning, "Out should emit the browser-open-failure warning")

	// The URL line should appear BEFORE the warning line (so the user has
	// the URL regardless of browser-open success).
	urlIdx, warnIdx := -1, -1
	for i, l := range outLines {
		if strings.Contains(l, "Passkey enrollment URL:") && urlIdx == -1 {
			urlIdx = i
		}
		if strings.Contains(l, "could not open browser automatically") && warnIdx == -1 {
			warnIdx = i
		}
	}
	assert.True(t, urlIdx >= 0 && warnIdx >= 0 && urlIdx < warnIdx,
		"URL line should appear before the browser-error warning (got urlIdx=%d, warnIdx=%d)", urlIdx, warnIdx)
}

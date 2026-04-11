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

package openclaw

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/components/vsa/constants"
)

// ────────────────────────────────────────────────────────────────
// Mock Gateway
// ────────────────────────────────────────────────────────────────

type mockGateway struct {
	server        *httptest.Server
	upgrader      websocket.Upgrader
	pendingEvents []ocFrame
	mu            sync.Mutex
	received      []ocFrame
}

func newMockGateway(t *testing.T) *mockGateway {
	t.Helper()
	mg := &mockGateway{
		upgrader: websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }},
	}
	mg.server = httptest.NewServer(http.HandlerFunc(mg.handle))
	t.Cleanup(mg.server.Close)
	return mg
}

func (mg *mockGateway) wsURL() string {
	return "ws" + strings.TrimPrefix(mg.server.URL, "http")
}

func (mg *mockGateway) handle(w http.ResponseWriter, r *http.Request) {
	conn, err := mg.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	challenge := ocFrame{Type: "event", Event: "connect.challenge"}
	data, _ := json.Marshal(challenge)
	_ = conn.WriteMessage(websocket.TextMessage, data)

	_, raw, err := conn.ReadMessage()
	if err != nil {
		return
	}
	var connectReq ocFrame
	if err := json.Unmarshal(raw, &connectReq); err != nil {
		return
	}
	mg.mu.Lock()
	mg.received = append(mg.received, connectReq)
	mg.mu.Unlock()

	boolTrue := true
	resp := ocFrame{Type: "res", ID: connectReq.ID, OK: &boolTrue}
	data, _ = json.Marshal(resp)
	_ = conn.WriteMessage(websocket.TextMessage, data)

	for _, evt := range mg.pendingEvents {
		data, _ := json.Marshal(evt)
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var f ocFrame
		if err := json.Unmarshal(raw, &f); err != nil {
			continue
		}
		mg.mu.Lock()
		mg.received = append(mg.received, f)
		mg.mu.Unlock()
	}
}

func (mg *mockGateway) queueInvoke(reqID, nodeID, command, paramsJSON string) {
	pj := paramsJSON
	mg.pendingEvents = append(mg.pendingEvents, ocFrame{
		Type:  "event",
		Event: "node.invoke.request",
		Params: &ocNodeInvokeRequest{
			ID:         reqID,
			NodeID:     nodeID,
			Command:    command,
			ParamsJSON: &pj,
		},
	})
}

func (mg *mockGateway) waitForReceived(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mg.mu.Lock()
		count := len(mg.received)
		mg.mu.Unlock()
		if count >= n {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func (mg *mockGateway) findInvokeResult(reqID string) *ocNodeInvokeResultParams {
	mg.mu.Lock()
	frames := make([]ocFrame, len(mg.received))
	copy(frames, mg.received)
	mg.mu.Unlock()
	for _, f := range frames {
		if f.Method != "node.invoke.result" {
			continue
		}
		raw, _ := json.Marshal(f.Params)
		var p ocNodeInvokeResultParams
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		if p.ID == reqID {
			return &p
		}
	}
	return nil
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ────────────────────────────────────────────────────────────────
// Constructor
// ────────────────────────────────────────────────────────────────

func TestNewOpenClawNodeService_RequiresURL(t *testing.T) {
	_, err := NewOpenClawNodeService("", "", "", "", "", newTestLogger())
	if err == nil {
		t.Fatal("expected error for missing gateway URL")
	}
}

func TestNewOpenClawNodeService_DefaultsNodeID(t *testing.T) {
	svc, err := NewOpenClawNodeService("ws://"+constants.DefaultEndpoint+":18789", "", "", "", "", newTestLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.nodeID == "" {
		t.Error("nodeID should default to hostname")
	}
}

func TestNewOpenClawNodeService_DisplayNameFallsBackToNodeID(t *testing.T) {
	svc, err := NewOpenClawNodeService("ws://"+constants.DefaultEndpoint+":18789", "", "my-node", "", "", newTestLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.displayName != "my-node" {
		t.Errorf("display name should default to nodeID, got %q", svc.displayName)
	}
}

func TestNewOpenClawNodeService_ExplicitDisplayName(t *testing.T) {
	svc, err := NewOpenClawNodeService("ws://"+constants.DefaultEndpoint+":18789", "", "my-node", "My Server", "", newTestLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.displayName != "My Server" {
		t.Errorf("unexpected display name: %q", svc.displayName)
	}
}

// ────────────────────────────────────────────────────────────────
// Handshake
// ────────────────────────────────────────────────────────────────

func TestHandshake_SendsCorrectConnectFrame(t *testing.T) {
	mg := newMockGateway(t)
	svc, err := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "Test Node", "", newTestLogger())
	if err != nil {
		t.Fatalf("NewOpenClawNodeService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Start(ctx) }()

	if !mg.waitForReceived(1, 2*time.Second) {
		t.Fatal("timed out waiting for connect frame")
	}
	cancel()
	<-done

	mg.mu.Lock()
	f := mg.received[0]
	mg.mu.Unlock()
	if f.Method != "connect" || f.Type != "req" {
		t.Errorf("unexpected connect frame: method=%q type=%q", f.Method, f.Type)
	}

	rawParams, _ := json.Marshal(f.Params)
	var params ocConnectParams
	json.Unmarshal(rawParams, &params)

	if params.Role != "node" {
		t.Errorf("expected role=node, got %q", params.Role)
	}
	if params.Client.InstanceID != "test-node" {
		t.Errorf("expected instanceId=test-node, got %q", params.Client.InstanceID)
	}
	if params.Client.DisplayName != "Test Node" {
		t.Errorf("expected displayName=Test Node, got %q", params.Client.DisplayName)
	}

	cmdSet := make(map[string]bool)
	for _, c := range params.Commands {
		cmdSet[c] = true
	}
	if !cmdSet["system.run"] {
		t.Error("missing system.run in commands")
	}
	if !cmdSet["system.which"] {
		t.Error("missing system.which in commands")
	}

	capSet := make(map[string]bool)
	for _, c := range params.Caps {
		capSet[c] = true
	}
	if !capSet["system"] {
		t.Error("missing system in caps")
	}
}

func TestHandshake_WithToken(t *testing.T) {
	mg := newMockGateway(t)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "secret-token", "node-1", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Start(ctx) }()

	if !mg.waitForReceived(1, 2*time.Second) {
		t.Fatal("timed out")
	}
	cancel()
	<-done

	mg.mu.Lock()
	first := mg.received[0]
	mg.mu.Unlock()
	rawAuth, _ := json.Marshal(first.Params)
	var params ocConnectParams
	json.Unmarshal(rawAuth, &params)
	if params.Auth == nil || params.Auth.Token != "secret-token" {
		t.Errorf("expected auth token 'secret-token', got %+v", params.Auth)
	}
}

// ────────────────────────────────────────────────────────────────
// system.which
// ────────────────────────────────────────────────────────────────

func TestSystemWhich_FindsExistingBinary(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("wh-1", "test-node", "system.which", `{"bins":["sh","nonexistent_xyz_abc"]}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("wh-1")
	if r == nil {
		t.Fatal("no result")
	}
	if !r.OK {
		t.Fatalf("not ok: %+v", r.Error)
	}
	var payload systemWhichResult
	json.Unmarshal([]byte(*r.PayloadJSON), &payload)
	if _, ok := payload.Bins["sh"]; !ok {
		t.Error("expected sh to be found")
	}
	if _, ok := payload.Bins["nonexistent_xyz_abc"]; ok {
		t.Error("nonexistent_xyz_abc should not be found")
	}
}

func TestSystemWhich_EmptyBins(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("wh-2", "test-node", "system.which", `{"bins":[]}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("wh-2")
	if r == nil || !r.OK {
		t.Fatal("expected ok result")
	}
	var payload systemWhichResult
	json.Unmarshal([]byte(*r.PayloadJSON), &payload)
	if len(payload.Bins) != 0 {
		t.Errorf("expected empty bins, got %v", payload.Bins)
	}
}

// ────────────────────────────────────────────────────────────────
// system.run
// ────────────────────────────────────────────────────────────────

func TestSystemRun_EchoCommand(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("sr-1", "test-node", "system.run", `{"command":["/bin/sh","-c","echo hello"]}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("sr-1")
	if r == nil {
		t.Fatal("no result")
	}
	if !r.OK {
		t.Fatalf("not ok: %+v", r.Error)
	}
	var payload systemRunResult
	json.Unmarshal([]byte(*r.PayloadJSON), &payload)
	if !payload.Success {
		t.Errorf("expected success, got %+v", payload)
	}
	if !strings.Contains(payload.Stdout, "hello") {
		t.Errorf("expected 'hello' in stdout, got %q", payload.Stdout)
	}
	if payload.ExitCode == nil || *payload.ExitCode != 0 {
		t.Errorf("expected exit 0, got %v", payload.ExitCode)
	}
}

func TestSystemRun_NonZeroExit(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("sr-2", "test-node", "system.run", `{"command":["/bin/sh","-c","exit 42"]}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("sr-2")
	if r == nil || !r.OK {
		t.Fatal("expected ok result frame")
	}
	var payload systemRunResult
	json.Unmarshal([]byte(*r.PayloadJSON), &payload)
	if payload.Success {
		t.Error("expected success=false for exit 42")
	}
	if payload.ExitCode == nil || *payload.ExitCode != 42 {
		t.Errorf("expected exit 42, got %v", payload.ExitCode)
	}
}

func TestSystemRun_StderrCaptured(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("sr-3", "test-node", "system.run", `{"command":["/bin/sh","-c","echo err_msg >&2"]}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("sr-3")
	if r == nil {
		t.Fatal("no result")
	}
	var payload systemRunResult
	json.Unmarshal([]byte(*r.PayloadJSON), &payload)
	if !strings.Contains(payload.Stderr, "err_msg") {
		t.Errorf("expected 'err_msg' in stderr, got %q", payload.Stderr)
	}
}

func TestSystemRun_WithCwd(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("sr-5", "test-node", "system.run", `{"command":["/bin/sh","-c","pwd"],"cwd":"/tmp"}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("sr-5")
	if r == nil || !r.OK {
		t.Fatal("expected ok result")
	}
	var payload systemRunResult
	json.Unmarshal([]byte(*r.PayloadJSON), &payload)
	if !strings.Contains(payload.Stdout, "/tmp") {
		t.Errorf("expected /tmp in stdout, got %q", payload.Stdout)
	}
}

func TestSystemRun_WithEnvVar(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("sr-6", "test-node", "system.run", `{"command":["/bin/sh","-c","echo $MY_OCT_VAR"],"env":{"MY_OCT_VAR":"from_test"}}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("sr-6")
	if r == nil || !r.OK {
		t.Fatal("expected ok result")
	}
	var payload systemRunResult
	json.Unmarshal([]byte(*r.PayloadJSON), &payload)
	if !strings.Contains(payload.Stdout, "from_test") {
		t.Errorf("expected 'from_test' in stdout, got %q", payload.Stdout)
	}
}

func TestSystemRun_EmptyCommandArray(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("sr-7", "test-node", "system.run", `{"command":[]}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("sr-7")
	if r == nil {
		t.Fatal("no result")
	}
	if r.OK {
		t.Error("expected ok=false for empty command")
	}
	if r.Error == nil || r.Error.Code != "INVALID_REQUEST" {
		t.Errorf("expected INVALID_REQUEST, got %+v", r.Error)
	}
}

// ────────────────────────────────────────────────────────────────
// Unknown command
// ────────────────────────────────────────────────────────────────

func TestUnknownCommand_ReturnsUnavailable(t *testing.T) {
	mg := newMockGateway(t)
	mg.queueInvoke("unk-1", "test-node", "system.nope", `{}`)
	svc, _ := NewOpenClawNodeService(mg.wsURL(), "", "test-node", "", "", newTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go svc.Start(ctx)

	if !mg.waitForReceived(2, 4*time.Second) {
		t.Fatalf("timed out: %d frames", len(mg.received))
	}
	cancel()

	r := mg.findInvokeResult("unk-1")
	if r == nil {
		t.Fatal("no result")
	}
	if r.OK {
		t.Error("expected ok=false")
	}
	if r.Error == nil || r.Error.Code != "UNAVAILABLE" {
		t.Errorf("expected UNAVAILABLE, got %+v", r.Error)
	}
}

// ────────────────────────────────────────────────────────────────
// runCommand unit tests
// ────────────────────────────────────────────────────────────────

func TestRunCommand_Success(t *testing.T) {
	ctx := context.Background()
	result, timedOut := runCommand(ctx, systemRunParams{
		Command: []string{"/bin/sh", "-c", "echo unit_test_output"},
	})
	if timedOut {
		t.Error("unexpected timeout")
	}
	if result.exitCode == nil || *result.exitCode != 0 {
		t.Errorf("expected exit 0, got %v", result.exitCode)
	}
	if !strings.Contains(result.stdout, "unit_test_output") {
		t.Errorf("expected output in stdout, got %q", result.stdout)
	}
}

func TestRunCommand_NonZeroExit(t *testing.T) {
	ctx := context.Background()
	result, _ := runCommand(ctx, systemRunParams{Command: []string{"/bin/sh", "-c", "exit 7"}})
	if result.exitCode == nil || *result.exitCode != 7 {
		t.Errorf("expected exit 7, got %v", result.exitCode)
	}
}

func TestRunCommand_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, timedOut := runCommand(ctx, systemRunParams{Command: []string{"/bin/sh", "-c", "sleep 10"}})
	if !timedOut {
		t.Error("expected timedOut=true")
	}
}

func TestRunCommand_Cwd(t *testing.T) {
	ctx := context.Background()
	result, _ := runCommand(ctx, systemRunParams{
		Command: []string{"/bin/sh", "-c", "pwd"},
		Cwd:     "/tmp",
	})
	if !strings.Contains(result.stdout, "/tmp") {
		t.Errorf("expected /tmp in stdout, got %q", result.stdout)
	}
}

func TestRunCommand_EnvOverride(t *testing.T) {
	ctx := context.Background()
	result, _ := runCommand(ctx, systemRunParams{
		Command: []string{"/bin/sh", "-c", "echo $OC_TEST_ENV"},
		Env:     map[string]string{"OC_TEST_ENV": "injected_value"},
	})
	if !strings.Contains(result.stdout, "injected_value") {
		t.Errorf("expected 'injected_value' in stdout, got %q", result.stdout)
	}
}

func TestRunCommand_BadBinary(t *testing.T) {
	ctx := context.Background()
	result, timedOut := runCommand(ctx, systemRunParams{
		Command: []string{"/nonexistent_binary_g8e_xyz"},
	})
	if timedOut {
		t.Error("bad binary should not time out")
	}
	// error should be captured in stderr or exit code should be non-zero
	if result.exitCode != nil && *result.exitCode == 0 {
		t.Error("expected non-zero exit for bad binary")
	}
}

// ────────────────────────────────────────────────────────────────
// truncateOutput
// ────────────────────────────────────────────────────────────────

func TestTruncateOutput_NoTruncation(t *testing.T) {
	s := "hello world"
	if got := truncateOutput(s, 100); got != s {
		t.Errorf("expected no truncation, got %q", got)
	}
}

func TestTruncateOutput_Truncates(t *testing.T) {
	s := strings.Repeat("x", 300)
	got := truncateOutput(s, 200)
	if !strings.Contains(got, "(truncated)") {
		t.Error("expected truncation marker")
	}
}

func TestTruncateOutput_ExactSize(t *testing.T) {
	s := strings.Repeat("a", 100)
	if got := truncateOutput(s, 100); got != s {
		t.Error("exact-size output should not be truncated")
	}
}

// ────────────────────────────────────────────────────────────────
// Stop
// ────────────────────────────────────────────────────────────────

func TestStop_BeforeStart_DoesNotPanic(t *testing.T) {
	svc, err := NewOpenClawNodeService("ws://"+constants.DefaultEndpoint+":18789", "", "node-stop", "", "", newTestLogger())
	if err != nil {
		t.Fatalf("NewOpenClawNodeService: %v", err)
	}
	// Stop before Start — cancel is nil, must not panic
	assert.NotPanics(t, func() { svc.Stop() })
}

func TestStop_CancelsRunningService(t *testing.T) {
	mg := newMockGateway(t)
	svc, err := NewOpenClawNodeService(mg.wsURL(), "", "node-stop-2", "", "", newTestLogger())
	if err != nil {
		t.Fatalf("NewOpenClawNodeService: %v", err)
	}

	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- svc.Start(ctx) }()

	// Wait until the service has connected (at least the connect frame is received)
	if !mg.waitForReceived(1, 2*time.Second) {
		t.Fatal("timed out waiting for connect frame")
	}

	svc.Stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not terminate Start within 3s")
	}
}

func TestStop_IdempotentDoubleStop(t *testing.T) {
	mg := newMockGateway(t)
	svc, err := NewOpenClawNodeService(mg.wsURL(), "", "node-stop-3", "", "", newTestLogger())
	if err != nil {
		t.Fatalf("NewOpenClawNodeService: %v", err)
	}

	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- svc.Start(ctx) }()

	if !mg.waitForReceived(1, 2*time.Second) {
		t.Fatal("timed out waiting for connect frame")
	}

	svc.Stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("first Stop did not terminate Start")
	}

	// Second Stop must not panic
	assert.NotPanics(t, func() { svc.Stop() })
}

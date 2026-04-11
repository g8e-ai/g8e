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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8e-ai/g8e/components/vsa/constants"
)

// ────────────────────────────────────────────────────────────────
// Wire types — mirror OpenClaw's Gateway Protocol JSON shapes
// ────────────────────────────────────────────────────────────────

type ocFrame struct {
	Type   string      `json:"type"`
	ID     string      `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
	OK     *bool       `json:"ok,omitempty"`
	Event  string      `json:"event,omitempty"`
	Seq    *int        `json:"seq,omitempty"`
}

type ocConnectParams struct {
	MinProtocol int          `json:"minProtocol"`
	MaxProtocol int          `json:"maxProtocol"`
	Client      ocClientInfo `json:"client"`
	Role        string       `json:"role"`
	Scopes      []string     `json:"scopes"`
	Caps        []string     `json:"caps"`
	Commands    []string     `json:"commands"`
	PathEnv     string       `json:"pathEnv,omitempty"`
	Auth        *ocAuth      `json:"auth,omitempty"`
}

type ocClientInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	Mode        string `json:"mode"`
	InstanceID  string `json:"instanceId,omitempty"`
}

type ocAuth struct {
	Token string `json:"token,omitempty"`
}

type ocNodeInvokeRequest struct {
	ID             string  `json:"id"`
	NodeID         string  `json:"nodeId"`
	Command        string  `json:"command"`
	ParamsJSON     *string `json:"paramsJSON"`
	TimeoutMs      *int    `json:"timeoutMs"`
	IdempotencyKey *string `json:"idempotencyKey"`
}

type ocNodeInvokeResultParams struct {
	ID          string  `json:"id"`
	NodeID      string  `json:"nodeId"`
	OK          bool    `json:"ok"`
	PayloadJSON *string `json:"payloadJSON,omitempty"`
	Error       *ocErr  `json:"error,omitempty"`
}

type ocErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type systemRunParams struct {
	Command    []string          `json:"command"`
	RawCommand string            `json:"rawCommand,omitempty"`
	Cwd        string            `json:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	TimeoutMs  *int              `json:"timeoutMs,omitempty"`
}

type systemRunResult struct {
	ExitCode *int   `json:"exitCode,omitempty"`
	TimedOut bool   `json:"timedOut"`
	Success  bool   `json:"success"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type systemWhichParams struct {
	Bins []string `json:"bins"`
}

type systemWhichResult struct {
	Bins map[string]string `json:"bins"`
}

// ────────────────────────────────────────────────────────────────
// OpenClawNodeService
// ────────────────────────────────────────────────────────────────

const (
	ocProtocolVersion = 1
	ocNodeVersion     = "1.0"
	ocNodeClientID    = "g8e.operator"
)

// OpenClawNodeService connects the g8e Operator binary to an OpenClaw Gateway
// as a Node Host. It advertises system.run and system.which, executes shell
// commands on request, and streams results back — with no g8e infrastructure
// dependency (no VSE, no VSOD, no pub/sub, no auth bootstrap).
type OpenClawNodeService struct {
	gatewayURL  string
	token       string
	nodeID      string
	displayName string
	pathEnv     string
	logger      *slog.Logger

	ws     *websocket.Conn
	wsMu   sync.Mutex
	seq    atomic.Int64
	ctx    context.Context
	cancel context.CancelFunc
}

// NewOpenClawNodeService creates and validates the service. Call Start() to connect.
// pathEnv is the value of the PATH environment variable to advertise to the Gateway.
func NewOpenClawNodeService(gatewayURL, token, nodeID, displayName, pathEnv string, logger *slog.Logger) (*OpenClawNodeService, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gateway URL is required")
	}

	resolvedNodeID := nodeID
	if resolvedNodeID == "" {
		h, err := os.Hostname()
		if err != nil || h == "" {
			h = "g8e.operator"
		}
		resolvedNodeID = h
	}

	resolvedDisplayName := displayName
	if resolvedDisplayName == "" {
		resolvedDisplayName = resolvedNodeID
	}

	return &OpenClawNodeService{
		gatewayURL:  gatewayURL,
		token:       token,
		nodeID:      resolvedNodeID,
		displayName: resolvedDisplayName,
		pathEnv:     pathEnv,
		logger:      logger,
	}, nil
}

// Start connects to the OpenClaw Gateway and blocks until ctx is cancelled or
// a fatal error occurs. It reconnects automatically on transient failures.
func (s *OpenClawNodeService) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	defer s.cancel()

	s.logger.Info("OpenClaw Node Host starting",
		"node_id", s.nodeID,
		"display_name", s.displayName,
		"gateway_url", s.gatewayURL)

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
		}

		if err := s.runSession(s.ctx); err != nil {
			s.logger.Warn("OperatorSession ended, reconnecting",
				"error", err,
				"backoff", backoff)
		}

		select {
		case <-s.ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		next := backoff * 2
		if next > maxBackoff {
			next = maxBackoff
		}
		backoff = next
	}
}

// Stop gracefully shuts down the service.
func (s *OpenClawNodeService) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// runSession establishes one WS connection, runs the handshake, then pumps
// incoming frames until disconnected or ctx cancelled.
func (s *OpenClawNodeService) runSession(ctx context.Context) error {
	conn, err := s.dial(ctx)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	s.wsMu.Lock()
	s.ws = conn
	s.wsMu.Unlock()

	defer func() {
		s.wsMu.Lock()
		s.ws = nil
		s.wsMu.Unlock()
	}()

	if err := s.handshake(ctx, conn); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}

	s.logger.Info("Connected to OpenClaw Gateway as node host",
		"node_id", s.nodeID)

	return s.readLoop(ctx, conn)
}

func (s *OpenClawNodeService) dial(ctx context.Context) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set(constants.HeaderUserAgent, fmt.Sprintf("%s/%s", ocNodeClientID, ocNodeVersion))

	dialer := &websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
	}

	// When connecting to wss:// outside g8e infra we use the system TLS roots.
	// If the URL is ws:// (g8e.local dev gateway) we skip TLS entirely.
	if strings.HasPrefix(s.gatewayURL, "wss://") {
		dialer.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	conn, _, err := dialer.DialContext(ctx, s.gatewayURL, header)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// handshake performs the OpenClaw Gateway Protocol handshake:
//
//  1. Wait for connect.challenge event
//  2. Send connect request with role="node" + commands
//  3. Wait for ok response
func (s *OpenClawNodeService) handshake(ctx context.Context, conn *websocket.Conn) error {
	// Step 1: read challenge
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var challenge ocFrame
	if err := s.readFrameConn(conn, &challenge); err != nil {
		return fmt.Errorf("read challenge: %w", err)
	}
	if challenge.Type != "event" || challenge.Event != "connect.challenge" {
		return fmt.Errorf("expected connect.challenge, got type=%q event=%q", challenge.Type, challenge.Event)
	}
	_ = conn.SetReadDeadline(time.Time{})

	// Step 2: send connect request
	reqID := s.nextID("connect")
	var auth *ocAuth
	if s.token != "" {
		auth = &ocAuth{Token: s.token}
	}
	connectFrame := ocFrame{
		Type:   "req",
		ID:     reqID,
		Method: "connect",
		Params: ocConnectParams{
			MinProtocol: ocProtocolVersion,
			MaxProtocol: ocProtocolVersion,
			Client: ocClientInfo{
				ID:          ocNodeClientID,
				DisplayName: s.displayName,
				Version:     ocNodeVersion,
				Platform:    runtime.GOOS,
				Mode:        "node",
				InstanceID:  s.nodeID,
			},
			Role:     "node",
			Scopes:   []string{},
			Caps:     []string{constants.Status.OperatorType.System},
			Commands: []string{"system.run", "system.which"},
			PathEnv:  s.pathEnv,
			Auth:     auth,
		},
	}
	if err := s.sendFrameConn(conn, connectFrame); err != nil {
		return fmt.Errorf("send connect: %w", err)
	}

	// Step 3: read response
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var resp ocFrame
	if err := s.readFrameConn(conn, &resp); err != nil {
		return fmt.Errorf("read connect response: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})

	if resp.Type != "res" || resp.ID != reqID {
		return fmt.Errorf("unexpected connect response: type=%q id=%q", resp.Type, resp.ID)
	}
	if resp.OK == nil || !*resp.OK {
		raw, _ := json.Marshal(resp.Params)
		return fmt.Errorf("connect rejected by gateway: %s", string(raw))
	}

	return nil
}

func (s *OpenClawNodeService) readLoop(ctx context.Context, conn *websocket.Conn) error {
	go func() {
		<-ctx.Done()
		_ = conn.SetReadDeadline(time.Now().UTC())
	}()

	for {
		var frame ocFrame
		if err := s.readFrameConn(conn, &frame); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		if frame.Type == "event" && frame.Event == "node.invoke.request" {
			go s.handleInvokeEvent(ctx, frame.Params)
		}
	}
}

// handleInvokeEvent is dispatched in a goroutine for each node.invoke.request.
func (s *OpenClawNodeService) handleInvokeEvent(ctx context.Context, rawParams interface{}) {
	data, err := json.Marshal(rawParams)
	if err != nil {
		s.logger.Warn("Failed to marshal invoke payload", "error", err)
		return
	}

	var req ocNodeInvokeRequest
	if err := json.Unmarshal(data, &req); err != nil {
		s.logger.Warn("Failed to parse node.invoke.request", "error", err)
		return
	}

	s.logger.Info("Invoke request received", "command", req.Command, "id", req.ID)

	switch req.Command {
	case "system.run":
		s.handleSystemRun(ctx, req)
	case "system.which":
		s.handleSystemWhich(ctx, req)
	default:
		s.sendInvokeError(req, "UNAVAILABLE", fmt.Sprintf("command not supported: %s", req.Command))
	}
}

// ────────────────────────────────────────────────────────────────
// Command handlers
// ────────────────────────────────────────────────────────────────

func (s *OpenClawNodeService) handleSystemRun(ctx context.Context, req ocNodeInvokeRequest) {
	if req.ParamsJSON == nil || *req.ParamsJSON == "" {
		s.sendInvokeError(req, "INVALID_REQUEST", "paramsJSON required")
		return
	}

	var p systemRunParams
	if err := json.Unmarshal([]byte(*req.ParamsJSON), &p); err != nil {
		s.sendInvokeError(req, "INVALID_REQUEST", fmt.Sprintf("invalid params: %v", err))
		return
	}

	if len(p.Command) == 0 {
		s.sendInvokeError(req, "INVALID_REQUEST", "command array is empty")
		return
	}

	// Resolve timeout: caller-specified, then request-level, then 30s default.
	timeoutMs := 30_000
	if req.TimeoutMs != nil && *req.TimeoutMs > 0 {
		timeoutMs = *req.TimeoutMs - 2000 // give ourselves a 2 s margin vs gateway timeout
		if timeoutMs < 1000 {
			timeoutMs = 1000
		}
	}
	if p.TimeoutMs != nil && *p.TimeoutMs > 0 {
		timeoutMs = *p.TimeoutMs
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	result, timedOut := runCommand(runCtx, p)

	exitCode := result.exitCode
	payload := systemRunResult{
		TimedOut: timedOut,
		Success:  result.exitCode != nil && *result.exitCode == 0 && !timedOut,
		Stdout:   result.stdout,
		Stderr:   result.stderr,
	}
	if exitCode != nil {
		payload.ExitCode = exitCode
	}

	s.logger.Info("system.run complete",
		"id", req.ID,
		"success", payload.Success,
		"exit_code", exitCode,
		"timed_out", timedOut)

	s.sendInvokeResult(req, payload)
}

func (s *OpenClawNodeService) handleSystemWhich(ctx context.Context, req ocNodeInvokeRequest) {
	if req.ParamsJSON == nil || *req.ParamsJSON == "" {
		s.sendInvokeError(req, "INVALID_REQUEST", "paramsJSON required")
		return
	}

	var p systemWhichParams
	if err := json.Unmarshal([]byte(*req.ParamsJSON), &p); err != nil {
		s.sendInvokeError(req, "INVALID_REQUEST", fmt.Sprintf("invalid params: %v", err))
		return
	}

	found := make(map[string]string, len(p.Bins))
	for _, bin := range p.Bins {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			continue
		}
		if path, err := exec.LookPath(bin); err == nil {
			found[bin] = path
		}
	}

	s.sendInvokeResult(req, systemWhichResult{Bins: found})
}

// ────────────────────────────────────────────────────────────────
// Result / error senders
// ────────────────────────────────────────────────────────────────

func (s *OpenClawNodeService) sendInvokeResult(req ocNodeInvokeRequest, payload interface{}) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		s.logger.Error("Failed to marshal invoke result payload", "error", err)
		s.sendInvokeError(req, "INTERNAL", "failed to marshal result")
		return
	}
	payloadStr := string(payloadBytes)

	resultParams := ocNodeInvokeResultParams{
		ID:          req.ID,
		NodeID:      s.nodeID,
		OK:          true,
		PayloadJSON: &payloadStr,
	}
	frame := ocFrame{
		Type:   "req",
		ID:     s.nextID("invoke_result"),
		Method: "node.invoke.result",
		Params: resultParams,
	}
	if err := s.sendFrame(frame); err != nil {
		s.logger.Warn("Failed to send invoke result", "id", req.ID, "error", err)
	}
}

func (s *OpenClawNodeService) sendInvokeError(req ocNodeInvokeRequest, code, message string) {
	resultParams := ocNodeInvokeResultParams{
		ID:     req.ID,
		NodeID: s.nodeID,
		OK:     false,
		Error:  &ocErr{Code: code, Message: message},
	}
	frame := ocFrame{
		Type:   "req",
		ID:     s.nextID("invoke_result"),
		Method: "node.invoke.result",
		Params: resultParams,
	}
	if err := s.sendFrame(frame); err != nil {
		s.logger.Warn("Failed to send invoke error", "id", req.ID, "code", code, "error", err)
	}
}

// ────────────────────────────────────────────────────────────────
// Transport helpers
// ────────────────────────────────────────────────────────────────

func (s *OpenClawNodeService) sendFrame(frame ocFrame) error {
	s.wsMu.Lock()
	conn := s.ws
	s.wsMu.Unlock()
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	return s.sendFrameConn(conn, frame)
}

func (s *OpenClawNodeService) sendFrameConn(conn *websocket.Conn, frame ocFrame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, data)
}

func (s *OpenClawNodeService) readFrameConn(conn *websocket.Conn, out *ocFrame) error {
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func (s *OpenClawNodeService) nextID(prefix string) string {
	n := s.seq.Add(1)
	return fmt.Sprintf("%s_%d", prefix, n)
}

// ────────────────────────────────────────────────────────────────
// Shell execution (standalone, no g8e dependencies)
// ────────────────────────────────────────────────────────────────

const maxOutputBytes = 200_000

type runResult struct {
	exitCode *int
	stdout   string
	stderr   string
}

// runCommand executes the argv passed by OpenClaw's exec tool.
// OpenClaw always sends a pre-built argv slice (e.g. ["/bin/sh","-c","ls -la"]),
// so we exec it directly rather than re-wrapping in a shell.
func runCommand(ctx context.Context, p systemRunParams) (runResult, bool) {
	argv := p.Command
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)

	if p.Cwd != "" {
		cmd.Dir = p.Cwd
	}

	// Inherit environment, then overlay caller-supplied vars.
	cmd.Env = os.Environ()
	for k, v := range p.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Force non-interactive to prevent hangs.
	cmd.Env = append(cmd.Env,
		"DEBIAN_FRONTEND=noninteractive",
		"CI=true",
		"NONINTERACTIVE=1",
	)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		errStr := err.Error()
		return runResult{stderr: errStr}, false
	}

	err := cmd.Wait()
	timedOut := ctx.Err() != nil

	stdout := truncateOutput(stdoutBuf.String(), maxOutputBytes)
	stderr := truncateOutput(stderrBuf.String(), maxOutputBytes)

	var exitCode *int
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		exitCode = &code
	}
	if err != nil && exitCode == nil {
		code := 1
		exitCode = &code
	}

	return runResult{exitCode: exitCode, stdout: stdout, stderr: stderr}, timedOut
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "... (truncated) " + s[len(s)-max:]
}

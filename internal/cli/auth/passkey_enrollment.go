// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/cli/sse"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/fs"
)

// DefaultPasskeyTimeout is the default maximum time to wait for passkey
// registration via SSE before giving up.
const DefaultPasskeyTimeout = 5 * time.Minute

// sseReadyTimeout is the maximum time to wait for the initial SSE connection
// to be established before aborting. This prevents indefinite blocking if the
// gateway is unreachable.
const sseReadyTimeout = 10 * time.Second

// PasskeyRegistrarOptions configures the passkey registration ceremony.
type PasskeyRegistrarOptions struct {
	// Timeout is the maximum time to wait for passkey registration via SSE.
	// Defaults to DefaultPasskeyTimeout if zero.
	Timeout time.Duration

	// Browser is an injectable browser-open function for tests. If nil,
	// platform.OpenBrowser is used.
	Browser func(string) error

	// Out is the user-facing output sink. The registrar routes its progress
	// lines (the clickable enrollment URL and the browser-open-failure
	// warning) through Out rather than writing to os.Stderr directly, so the
	// coordinator's "no direct stdout/stderr writes" invariant holds and the
	// output is testable from the coordinator layer. Defaults to a no-op
	// stub when nil, mirroring the coordinator's own OutputFunc default.
	Out OutputFunc
}

// passkeyRegistrar is the hardened implementation of the PasskeyRegistrar
// interface. It prepares the SSE listener before browser launch, uses a
// correct cursor strategy (since_id=0 for live-only events), filters events
// by type/user/session, surfaces browser-open errors, and propagates context
// cancellation.
type passkeyRegistrar struct {
	fileSvc        fs.RuntimeFileService
	cfg            *config.Config
	timeout        time.Duration
	browser        func(string) error
	out            OutputFunc
	programFactory func(enrollModel) programRunner
}

// programRunner is the subset of *tea.Program used by Register. Tests inject
// a mock implementation to avoid running a real terminal program.
type programRunner interface {
	Send(msg tea.Msg)
	Run() (tea.Model, error)
}

// Compile-time assertion that *tea.Program satisfies programRunner.
var _ programRunner = (*tea.Program)(nil)

// newPasskeyRegistrar creates a passkeyRegistrar with the given dependencies
// and options. Nil options fields get safe defaults.
func newPasskeyRegistrar(fileSvc fs.RuntimeFileService, cfg *config.Config, opts PasskeyRegistrarOptions) *passkeyRegistrar {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultPasskeyTimeout
	}
	browser := opts.Browser
	if browser == nil {
		browser = platform.OpenBrowser
	}
	out := opts.Out
	if out == nil {
		out = func(string, ...any) {} // no-op default; coordinator wires its own
	}
	r := &passkeyRegistrar{
		fileSvc: fileSvc,
		cfg:     cfg,
		timeout: timeout,
		browser: browser,
		out:     out,
	}
	r.programFactory = func(m enrollModel) programRunner { return tea.NewProgram(m) }
	return r
}

// Register performs the browser-based passkey registration ceremony.
//
// Order of operations (per §8):
//  1. Validate identity/session inputs.
//  2. Build the mTLS client (validates that local credentials are usable).
//  3. Generate the one-time enrollment token through the mTLS endpoint.
//  4. Prepare the SSE client with since_id=0 (live events only, no replay).
//  5. Start the SSE monitor goroutine and wait for it to connect.
//  6. Open the browser (error is returned, not swallowed).
//  7. Run the TUI, waiting for the matching event, timeout, or cancellation.
//
// The SSE listener is ready BEFORE the browser opens, eliminating the
// event-loss race. Browser-open errors are returned with a safe fallback
// instruction that does not leak the token. The SSE context is cancelled as
// soon as the matching event arrives, ensuring the monitor goroutine and
// response body terminate on every path.
func (r *passkeyRegistrar) Register(ctx context.Context, userID, cliSessionID string) error {
	if userID == "" || cliSessionID == "" {
		return fmt.Errorf("%w: user ID and CLI session ID are required", constants.ErrPasskeyRegistrationFailed)
	}

	// 1-2. Build mTLS client (validates that local credentials are usable
	// before any gateway token is generated or browser is opened).
	mtlsClient, err := BuildMTLSClient(r.fileSvc, r.cfg, 0)
	if err != nil {
		return fmt.Errorf("%w: build mTLS client: %w", constants.ErrPasskeyRegistrationFailed, err)
	}

	// 3. Generate the one-time enrollment token. The gateway derives the
	// user ID from the mTLS certificate context; the cliSessionID is sent
	// as a header for routing.
	token, err := r.generateEnrollmentToken(ctx, mtlsClient, cliSessionID)
	if err != nil {
		return err
	}

	// The console dispatches on #enroll=1&token=... to the dedicated
	// CLI enrollment flow (registerPasskeyEnrollment), which posts the
	// token to the enrollment register endpoints. The legacy
	// #register=1&token=... fragment targeted the browser-bootstrap
	// flow and triggered the 400 Bad Request documented in
	// passkey-enrollment-console-400.md.
	consoleURL := fmt.Sprintf("%s/console/#enroll=1&token=%s",
		r.cfg.OperatorPublicURL(),
		url.QueryEscape(token))

	// 4. Prepare SSE client with since_id=0. Per the gateway SSE handler,
	// since_id=0 without Last-Event-ID means "live events only, no replay"
	// — exactly what we want for a fresh passkey registration wait.
	sseURL := fmt.Sprintf("%s%s?since_id=0",
		r.cfg.OperatorPublicURL(),
		constants.APIPaths.SSEStream)

	sseClient := sse.NewClient(sseURL, mtlsClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, cliSessionID)

	// Create a cancellable context layered with the timeout. We cancel on
	// success to terminate the SSE monitor goroutine and close the response
	// body immediately.
	sseCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if r.timeout > 0 {
		var timeoutCancel context.CancelFunc
		sseCtx, timeoutCancel = context.WithTimeout(sseCtx, r.timeout)
		defer timeoutCancel()
	}

	// 5. Set up the ready signal so we can wait for the SSE connection to
	// be established before opening the browser.
	readyCh := make(chan struct{}, 1)
	sseClient.SetOnConnect(func() {
		select {
		case readyCh <- struct{}{}:
		default: // already signaled (reconnect)
		}
	})

	// Create the TUI model and program. Messages from the SSE monitor are
	// sent to the program; Send is safe to call before Run (messages are
	// buffered in the program's channel).
	model := newEnrollModel(consoleURL)
	p := r.programFactory(model)

	// Start the SSE monitor goroutine. It filters events by type, user ID,
	// and CLI session ID, and sends passkeyRegisteredMsg or enrollErrMsg to
	// the program. On a matching event, it cancels the SSE context to stop
	// the stream and close the response body.
	go r.monitorPasskeyRegistration(sseCtx, sseClient, p, userID, cliSessionID, cancel)

	// Wait for the SSE listener to be ready before opening the browser.
	// This eliminates the event-loss race where the browser completes
	// registration before the SSE client connects.
	select {
	case <-readyCh:
		// SSE connected — safe to open the browser.
	case <-time.After(sseReadyTimeout):
		cancel()
		return fmt.Errorf("%w: timed out waiting for SSE connection", constants.ErrPasskeyRegistrationFailed)
	case <-sseCtx.Done():
		// Context cancelled (parent cancelled or timeout) before SSE
		// connected.
		if sseCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%w: %w", constants.ErrPasskeyRegistrationFailed, constants.ErrPasskeyRegistrationTimedOut)
		}
		return sseCtx.Err()
	}

	// 6. Print the clickable enrollment URL via Out BEFORE opening the
	// browser, so the URL is in the terminal scrollback regardless of TUI
	// rendering behavior (no-TTY over SSH, TUI rendering failure, etc.).
	// The insertion point is load-bearing: this fires AFTER the SSE
	// ready-gate has fired (lines 193-206), so the user cannot click the
	// URL before the SSE listener is connected. The TUI program was
	// constructed at line 182 but construction does not render — View()
	// only paints once p.Run() is called at line 221 — so this print does
	// not conflict with the TUI. The URL is prefixed so it is grep-able in
	// scrollback and unambiguous in test assertions.
	r.out("Passkey enrollment URL: %s", consoleURL)

	// Open the browser. On failure, print a warning via Out and fall
	// through to the TUI — the SSE monitor is already running and will
	// catch the passkey registration event when the user opens the URL
	// manually. The URL was already printed above, so the user has it
	// regardless of browser-open success.
	if openErr := r.browser(consoleURL); openErr != nil {
		r.out("Warning: could not open browser automatically (%v).", openErr)
		r.out("Open the enrollment URL above in a browser to complete passkey registration.")
	}

	// 7. Run the TUI. Blocks until passkeyRegisteredMsg, enrollErrMsg, or
	// user cancellation (q/ctrl+c). The TUI displays the full console URL
	// (including the one-time token in the fragment) so the user can open
	// it manually if the browser did not launch.
	finalModel, err := p.Run()
	if err != nil {
		cancel()
		return fmt.Errorf("%w: %w", constants.ErrPasskeyRegistrationFailed, err)
	}

	m, ok := finalModel.(enrollModel)
	if !ok {
		return nil
	}
	return m.err
}

// generateEnrollmentToken calls the gateway's enrollment token generation
// endpoint to create a one-time token for secure passkey registration. The
// gateway derives the user ID from the mTLS certificate context; only the
// cliSessionID is sent as a routing header.
func (r *passkeyRegistrar) generateEnrollmentToken(ctx context.Context, mtlsClient *http.Client, cliSessionID string) (string, error) {
	tokenURL := fmt.Sprintf("%s%s", r.cfg.OperatorPublicURL(), constants.APIPaths.AuthEnrollmentTokenGenerate)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	req.Header.Set(constants.HeaderCLISessionID, cliSessionID)

	resp, err := mtlsClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("enrollment token generation failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	if result.Token == "" {
		return "", constants.ErrEnrollmentTokenGenerationFailed
	}

	return result.Token, nil
}

// VerifyPasskeyRegistration checks if a passkey is registered by querying
// the gateway's mTLS passkey status endpoint. The gateway derives the user
// ID from the authenticated CLI certificate; the cliSessionID is sent as
// the X-G8E-CLI-Session-ID header for routing.
//
// This is a read-only identity check used by `mcp agent run` to decide
// whether to prompt for the first passkey ceremony. `auth enroll user` always
// performs the explicit add-passkey ceremony via the PasskeyRegistrar.
func VerifyPasskeyRegistration(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config, cliSessionID string) (bool, error) {
	statusURL := fmt.Sprintf("%s%s", cfg.OperatorPublicURL(), constants.APIPaths.AuthPasskeysCLIStatus)

	httpClient, err := BuildMTLSClient(fileSvc, cfg, httpTimeout)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	req.Header.Set(constants.HeaderCLISessionID, cliSessionID)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrHTTPRequestExecuteFailed, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		// Fall through to parse body below.
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return false, fmt.Errorf("%w: status %d", constants.ErrPasskeyStatusUnauthorized, resp.StatusCode)
	case resp.StatusCode >= 500:
		return false, fmt.Errorf("%w: server returned status %d", constants.ErrHTTPStatusError, resp.StatusCode)
	default:
		return false, fmt.Errorf("%w: unexpected status %d", constants.ErrHTTPStatusError, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrHTTPResponseReadFailed, err)
	}

	var result struct {
		Success     bool `json:"success"`
		Credentials []struct {
			ID string `json:"id"`
		} `json:"credentials"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	return len(result.Credentials) > 0, nil
}

// monitorPasskeyRegistration runs the SSE client and sends messages to the
// provided program when the matching passkey.registered event arrives, the
// context expires, or the SSE stream closes unexpectedly.
//
// Event filtering (per §8.4): only events with:
//   - inner type "passkey.registered"
//   - UserID matching the enrolled user
//   - CliSessionID matching the enrolled CLI session
//
// are treated as a match. Unrelated events are silently ignored.
//
// On a matching event, passkeyRegisteredMsg is sent and the cancel function
// is called to terminate the SSE stream and close the response body. On
// context expiration (timeout), enrollErrMsg with
// ErrPasskeyRegistrationTimedOut is sent. On clean SSE disconnect without a
// matching event, enrollErrMsg with ErrPasskeySSEStreamClosed is sent.
func (r *passkeyRegistrar) monitorPasskeyRegistration(ctx context.Context, sseClient *sse.Client, sender programSender, userID, cliSessionID string, cancel context.CancelFunc) {
	matched := false
	sseClient.Run(ctx, func(eventType, data string) {
		if matched {
			return // already matched; ignore subsequent events
		}

		// Parse the SSEPushPayload envelope to extract user/session/type.
		var envelope models.SSEPushPayload
		if err := json.Unmarshal([]byte(data), &envelope); err != nil {
			return // ignore malformed events
		}

		// Extract the inner event type. When the server omits the event:
		// field (R14), eventType is empty and the type is inside the
		// payload's Event JSON.
		innerType := eventType
		if innerType == "" {
			var inner struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(envelope.Event, &inner)
			innerType = inner.Type
		}

		// Filter by event type.
		if innerType != "passkey.registered" {
			return
		}

		// Filter by user ID and CLI session ID.
		if envelope.UserID != userID || envelope.CliSessionID != cliSessionID {
			return
		}

		// Matching event — signal success and cancel the SSE context to
		// terminate the stream and close the response body.
		matched = true
		sender.Send(passkeyRegisteredMsg{})
		cancel()
	})

	// If we already matched, don't send an error — the success message
	// was already delivered and the TUI is exiting.
	if matched {
		return
	}

	// The stream ended without a matching event. Determine the cause.
	if ctx.Err() != nil {
		sender.Send(enrollErrMsg{err: constants.ErrPasskeyRegistrationTimedOut})
	} else {
		sender.Send(enrollErrMsg{err: constants.ErrPasskeySSEStreamClosed})
	}
}

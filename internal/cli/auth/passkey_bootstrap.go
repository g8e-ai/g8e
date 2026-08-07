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

// passkeyEnrollTimeout is the maximum time to wait for passkey registration
// via SSE before giving up.
const passkeyEnrollTimeout = 5 * time.Minute

// programSender is the interface for sending messages to a bubbletea program.
// *tea.Program implements this via its Send method. Tests use a channel-based
// mock to verify message delivery without running a real terminal program.
type programSender interface {
	Send(msg tea.Msg)
}

// RegisterPasskeyViaBrowser opens the gateway's console UI in the user's browser
// for passkey registration, then subscribes to an SSE stream scoped to the CLI
// session. When the browser completes WebAuthn registration, the gateway emits
// a passkey.registered event that the CLI receives in real-time, replacing the
// legacy polling approach.
func RegisterPasskeyViaBrowser(fileSvc fs.RuntimeFileService, cfg *config.Config, userID, cliSessionID string) error {
	// Generate a one-time enrollment token from the gateway
	token, err := generateEnrollmentToken(fileSvc, cfg, userID, cliSessionID)
	if err != nil {
		return fmt.Errorf("failed to generate enrollment token: %w", err)
	}

	consoleURL := fmt.Sprintf("%s/console/#register=1&token=%s",
		cfg.OperatorPublicURL(),
		url.QueryEscape(token))

	_ = platform.OpenBrowser(consoleURL)

	mtlsClient, err := BuildMTLSClient(fileSvc, cfg, 0)
	if err != nil {
		return err
	}

	sseURL := fmt.Sprintf("%s%s?since_id=1",
		cfg.OperatorPublicURL(),
		constants.APIPaths.SSEStream)

	sseClient := sse.NewClient(sseURL, mtlsClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, cliSessionID)

	ctx, cancel := context.WithTimeout(context.Background(), passkeyEnrollTimeout)
	defer cancel()

	model := newEnrollModel(consoleURL)
	p := tea.NewProgram(model)

	go monitorPasskeyRegistration(ctx, sseClient, p)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("passkey enrollment: %w", err)
	}

	m, ok := finalModel.(enrollModel)
	if !ok {
		return nil
	}
	return m.err
}

// generateEnrollmentToken calls the gateway's enrollment token generation endpoint
// to create a one-time token for secure passkey registration.
func generateEnrollmentToken(fileSvc fs.RuntimeFileService, cfg *config.Config, userID, cliSessionID string) (string, error) {
	mtlsClient, err := BuildMTLSClient(fileSvc, cfg, httpTimeout)
	if err != nil {
		return "", err
	}

	tokenURL := fmt.Sprintf("%s%s", cfg.OperatorPublicURL(), constants.APIPaths.AuthEnrollmentTokenGenerate)
	req, err := http.NewRequest(http.MethodPost, tokenURL, nil)
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

// VerifyPasskeyRegistration checks if a user has a passkey registered by
// querying the gateway's mTLS passkey status endpoint. The cliSessionID
// parameter is sent as the X-G8E-CLI-Session-ID header so the gateway's
// mTLS auth middleware can route the request to handleCLIAuth.
func VerifyPasskeyRegistration(fileSvc fs.RuntimeFileService, cfg *config.Config, userID, cliSessionID string) (bool, error) {
	statusURL := fmt.Sprintf("%s%s", cfg.OperatorPublicURL(), constants.APIPaths.AuthPasskeysCLIStatus)

	httpClient, err := BuildMTLSClient(fileSvc, cfg, httpTimeout)
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest(http.MethodGet, statusURL, nil)
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
		// Fall through to parse body below
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
// provided program when the passkey is registered, the context expires, or
// the SSE stream closes unexpectedly. On a passkey.registered event, it sends
// passkeyRegisteredMsg which causes the enroll model to exit with success.
// On context cancellation (timeout), it sends enrollErrMsg with
// ErrPasskeyRegistrationTimedOut. On clean SSE disconnect without a
// registration event, it sends enrollErrMsg with ErrPasskeySSEStreamClosed.
func monitorPasskeyRegistration(ctx context.Context, sseClient *sse.Client, sender programSender) {
	sseClient.Run(ctx, func(eventType, data string) {
		// When the server omits the event: field (R14), eventType is empty.
		// Extract the type from the inner payload (SSEPushPayload.Event.type).
		innerType := eventType
		if innerType == "" {
			var envelope models.SSEPushPayload
			if err := json.Unmarshal([]byte(data), &envelope); err == nil {
				var inner struct {
					Type string `json:"type"`
				}
				_ = json.Unmarshal(envelope.Event, &inner)
				innerType = inner.Type
			}
		}
		if innerType == "passkey.registered" {
			sender.Send(passkeyRegisteredMsg{})
		}
	})
	if ctx.Err() != nil {
		sender.Send(enrollErrMsg{err: constants.ErrPasskeyRegistrationTimedOut})
	} else {
		sender.Send(enrollErrMsg{err: constants.ErrPasskeySSEStreamClosed})
	}
}

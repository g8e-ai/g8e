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
)

// passkeyEnrollTimeout is the maximum time to wait for passkey registration
// via SSE before giving up.
const passkeyEnrollTimeout = 5 * time.Minute

// RegisterPasskeyViaBrowser opens the gateway's console UI in the user's browser
// for passkey registration, then subscribes to an SSE stream scoped to the CLI
// session. When the browser completes WebAuthn registration, the gateway emits
// a passkey.registered event that the CLI receives in real-time, replacing the
// legacy polling approach.
func RegisterPasskeyViaBrowser(cfg *config.Config, userID, cliSessionID string) error {
	consoleURL := fmt.Sprintf("%s/console/#register=1&user_id=%s&cli_session_id=%s",
		cfg.OperatorPublicURL(),
		url.QueryEscape(userID),
		url.QueryEscape(cliSessionID))

	_ = platform.OpenBrowser(consoleURL)

	mtlsClient, err := BuildMTLSClient(cfg, 0)
	if err != nil {
		return err
	}

	sseURL := fmt.Sprintf("%s%s?cli_session_id=%s&since_id=1",
		cfg.OperatorPublicURL(),
		constants.APIPaths.SSEStream,
		url.QueryEscape(cliSessionID))

	sseClient := sse.NewClient(sseURL, mtlsClient)
	sseClient.SetHeader(constants.HeaderCLISessionID, cliSessionID)

	ctx, cancel := context.WithTimeout(context.Background(), passkeyEnrollTimeout)
	defer cancel()

	model := newEnrollModel(consoleURL)
	p := tea.NewProgram(model)

	go func() {
		sseClient.Run(ctx, func(eventType, data string) {
			if eventType == "passkey.registered" {
				p.Send(passkeyRegisteredMsg{})
			}
		})
		if ctx.Err() != nil {
			p.Send(enrollErrMsg{err: constants.ErrPasskeyRegistrationTimedOut})
		}
	}()

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

// VerifyPasskeyRegistration checks if a user has a passkey registered by
// querying the gateway's mTLS passkey status endpoint. The cliSessionID
// parameter is sent as the X-G8E-CLI-Session-ID header so the gateway's
// mTLS auth middleware can route the request to handleCLIAuth.
func VerifyPasskeyRegistration(cfg *config.Config, userID, cliSessionID string) (bool, error) {
	statusURL := fmt.Sprintf("%s%s", cfg.OperatorPublicURL(), constants.APIPaths.AuthPasskeysCLIStatus)

	httpClient, err := BuildMTLSClient(cfg, httpTimeout)
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

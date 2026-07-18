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

package lattice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/credentials"
)

// authToken represents an OAuth2 access token with expiry.
type authToken struct {
	AccessToken string
	Expiry      time.Time
}

// isValid returns true if the token is non-empty and has not expired.
func (t *authToken) isValid() bool {
	return t != nil && t.AccessToken != "" && time.Now().Before(t.Expiry)
}

// ClientCredentialsAuth implements credentials.PerRPCCredentials for Lattice
// OAuth2 client credentials flow. It is safe for concurrent use across
// goroutines issuing parallel gRPC calls.
type ClientCredentialsAuth struct {
	ClientID       string
	ClientSecret   string
	SandboxesToken string // sandbox only; empty in production
	Endpoint       string // e.g. https://<lattice>/api/v1/oauth/token

	mu         sync.Mutex
	token      *authToken
	httpClient *http.Client
}

// NewClientCredentialsAuth creates a ClientCredentialsAuth with a 10-second
// HTTP client timeout to prevent indefinite hangs on network partition.
func NewClientCredentialsAuth(clientID, clientSecret, sandboxesToken, endpoint string) *ClientCredentialsAuth {
	return &ClientCredentialsAuth{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		SandboxesToken: sandboxesToken,
		Endpoint:       endpoint,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// acquireToken performs the OAuth2 client_credentials grant and returns the
// parsed token. If SandboxesToken is set, the Anduril-Sandbox-Authorization
// header is attached to the token request itself — the OAuth endpoint requires
// this in sandbox environments.
func (a *ClientCredentialsAuth) acquireToken(ctx context.Context) (*authToken, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {a.ClientID},
		"client_secret": {a.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLatticeTokenAcquireFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if a.SandboxesToken != "" {
		req.Header.Set("Anduril-Sandbox-Authorization", "Bearer "+a.SandboxesToken)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLatticeTokenAcquireFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrLatticeTokenAcquireFailed, resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLatticeTokenAcquireFailed, err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("%w: empty access_token in response", ErrLatticeTokenAcquireFailed)
	}

	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return &authToken{
		AccessToken: tokenResp.AccessToken,
		Expiry:      expiry,
	}, nil
}

// getToken returns a valid token, refreshing proactively at 2/3 of lifetime
// with ±10% jitter. Thread-safe via mutex. The mutex is released before the
// HTTP call to avoid blocking concurrent gRPC calls during token refresh.
func (a *ClientCredentialsAuth) getToken(ctx context.Context) (*authToken, error) {
	a.mu.Lock()
	if a.token != nil && a.token.isValid() {
		// Refresh proactively at 2/3 of lifetime, jittered ±10%.
		lifetime := a.token.Expiry.Sub(time.Now())
		refreshAt := a.token.Expiry.Add(-lifetime / 3)
		jitterRange := lifetime / 30 // ±10% of lifetime = 1/10, half of that for ±
		jitter := time.Duration(rand.Int63n(int64(jitterRange)))
		refreshAt = refreshAt.Add(jitter - jitterRange/2)

		if time.Now().Before(refreshAt) {
			token := a.token
			a.mu.Unlock()
			return token, nil
		}
	}
	a.mu.Unlock()

	token, err := a.acquireToken(ctx)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	a.token = token
	a.mu.Unlock()
	return token, nil
}

// ForceRefresh forces a token refresh on the next getToken call. Called when
// a gRPC UNAUTHENTICATED status is received.
func (a *ClientCredentialsAuth) ForceRefresh() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = nil
}

// GetRequestMetadata implements credentials.PerRPCCredentials. It ensures a
// valid token exists and returns the authorization metadata for every RPC.
// If SandboxesToken is set, the sandbox authorization header is also injected.
func (a *ClientCredentialsAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	token, err := a.getToken(ctx)
	if err != nil {
		return nil, err
	}

	md := map[string]string{
		"authorization": "Bearer " + token.AccessToken,
	}
	if a.SandboxesToken != "" {
		md["anduril-sandbox-authorization"] = "Bearer " + a.SandboxesToken
	}
	return md, nil
}

// RequireTransportSecurity implements credentials.PerRPCCredentials.
// Always true — Lattice connections are always TLS.
func (a *ClientCredentialsAuth) RequireTransportSecurity() bool {
	return true
}

// Compile-time assertion that ClientCredentialsAuth implements PerRPCCredentials.
var _ credentials.PerRPCCredentials = (*ClientCredentialsAuth)(nil)

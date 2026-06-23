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

package mcp

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// HTTPTribunalDeliberator is an HTTP client that calls the Tribunal service's
// /tribunal/v1/deliberate endpoint to collect L2 consensus votes.
// It implements the TribunalDeliberator interface.
type HTTPTribunalDeliberator struct {
	url    string
	client *http.Client
}

// NewHTTPTribunalDeliberator creates a new HTTP Tribunal deliberation client.
// The url should point to the Tribunal's deliberate endpoint
// (e.g. https://localhost:8443/tribunal/v1/deliberate).
// If tlsConfig is non-nil, it is used for mTLS authentication to the Tribunal.
func NewHTTPTribunalDeliberator(url string, tlsConfig *tls.Config) *HTTPTribunalDeliberator {
	transport := &http.Transport{}
	if tlsConfig != nil {
		transport.TLSClientConfig = tlsConfig
	}
	return &HTTPTribunalDeliberator{
		url: url,
		client: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// Deliberate sends the envelope bytes to the Tribunal service and returns
// the deliberated envelope with L2 votes populated.
func (d *HTTPTribunalDeliberator) Deliberate(ctx context.Context, envelopeBytes []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(envelopeBytes))
	if err != nil {
		return nil, fmt.Errorf("tribunal deliberator: %w", err)
	}
	req.Header.Set(constants.HeaderContentType, constants.HeaderValueApplicationJSON)
	req.Header.Set(constants.HeaderXContentTypeOptions, constants.HeaderValueNoSniff)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tribunal deliberator: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tribunal deliberator: HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tribunal deliberator: read response: %w", err)
	}

	return body, nil
}

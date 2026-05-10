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

package certs

import (
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FetchAndSetCA fetches the hub CA certificate from the given URL
// (e.g. https://host/ssl/ca.crt), validates it is a non-empty PEM block,
// and stores it via SetCA for use by all subsequent TLS connections.
//
// This is the bootstrap step that establishes trust. The CA endpoint is
// unauthenticated by design — it is equivalent to a certificate pinning
// fetch. All subsequent connections are verified against this CA.
func FetchAndSetCA(ctx context.Context, caURL string) error {
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, caURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build CA fetch request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch CA certificate from %s: %w", caURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CA fetch returned HTTP %d from %s", resp.StatusCode, caURL)
	}

	pem, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("failed to read CA certificate body: %w", err)
	}

	if len(pem) == 0 {
		return fmt.Errorf("CA certificate from %s is empty", caURL)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return fmt.Errorf("CA certificate from %s is not a valid PEM-encoded certificate", caURL)
	}

	SetCA(pem)
	return nil
}

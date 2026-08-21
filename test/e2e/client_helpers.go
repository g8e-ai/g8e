// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package e2e

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBytes limits response body reads to prevent unbounded memory
// consumption from a misbehaving or malicious endpoint. 1 MiB is generous for
// all expected E2E responses (health, pending lists, operator documents,
// command results).
const maxResponseBytes = 1 << 20

// defaultClientTimeout is the per-request timeout for standard E2E client
// operations. Long-running operations (command dispatch) override this.
const defaultClientTimeout = 30 * time.Second

// parseCAPool parses a PEM-encoded CA bundle into an x509.CertPool. Returns an
// error if the bundle is empty or contains no valid certificates.
func parseCAPool(caBundle []byte) (*x509.CertPool, error) {
	if len(caBundle) == 0 {
		return nil, fmt.Errorf("CA bundle is empty")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBundle) {
		return nil, fmt.Errorf("CA bundle contains no valid PEM certificates")
	}
	return pool, nil
}

// extractServerName parses an HTTPS URL and returns the hostname to use as
// TLS ServerName. This ensures normal Go TLS hostname verification is applied
// against the gateway's certificate SANs.
func extractServerName(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "https" {
		return "", fmt.Errorf("expected https scheme, got %q in URL %q", parsed.Scheme, rawURL)
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("URL %q has empty hostname", rawURL)
	}
	return host, nil
}

// truncateBody returns a shortened representation of body bytes for error
// messages, capped at 512 characters.
func truncateBody(body []byte) string {
	const max = 512
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...(truncated)"
}

// doRequest executes an HTTP request, reads a bounded response body, checks
// the status code, and returns the raw body bytes. Returns an error if the
// status code is not the expected value or if the response exceeds
// maxResponseBytes.
func doRequest(client *http.Client, req *http.Request, expectedStatus int) ([]byte, int, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, resp.StatusCode, fmt.Errorf("response exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode != expectedStatus {
		return body, resp.StatusCode, fmt.Errorf("status %d, expected %d: %s", resp.StatusCode, expectedStatus, truncateBody(body))
	}
	return body, resp.StatusCode, nil
}

// isEnsembleHealthy decodes the ensemble health body and returns true if the
// status field is "ok".
func isEnsembleHealthy(body []byte) bool {
	var health ensembleHealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return false
	}
	return strings.EqualFold(health.Status, "ok")
}

// decodeJSON unmarshals body into a value of type T, wrapping any decode
// error with the provided label for contextual diagnostics. It is the single
// typed decode path used by all E2EClient response handlers, so decode-failure
// behavior is unit-testable without network or platform dependencies.
func decodeJSON[T any](body []byte, label string) (T, error) {
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("decode %s: %w", label, err)
	}
	return result, nil
}

// ensembleHealthResponse is a typed model for the ensemble /health endpoint.
// The ensemble returns {"status": "ok"} when healthy.
type ensembleHealthResponse struct {
	Status string `json:"status"`
}

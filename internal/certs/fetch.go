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
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"
)

// FetchTrustBundle fetches the hub trust bundle from the given URL
// (e.g. https://host/.well-known/g8e/pki/ca-bundle), validates it is a non-empty PEM block,
// and returns the raw PEM bytes. It does NOT mutate any global state.
//
// This is the bootstrap step that establishes trust. The trust bundle endpoint is
// unauthenticated by design - it is equivalent to a certificate pinning
// fetch. All subsequent connections are verified against this trust bundle.
//
// The optional caFingerprint parameter enables OOB pinning verification. If provided,
// the fetched CA bundle's SHA-256 fingerprint must match the expected value.
func FetchTrustBundle(ctx context.Context, caURL string, caFingerprint string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, caURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build CA fetch request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch CA certificate from %s: %w", caURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CA fetch returned HTTP %d from %s", resp.StatusCode, caURL)
	}

	pem, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate body: %w", err)
	}

	if len(pem) == 0 {
		return nil, fmt.Errorf("CA certificate from %s is empty", caURL)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("CA certificate from %s is not a valid PEM-encoded certificate", caURL)
	}

	// Verify CA fingerprint if pin is provided
	if caFingerprint != "" {
		if err := verifyCAFingerprint(pem, caFingerprint); err != nil {
			return nil, fmt.Errorf("CA fingerprint verification failed: %w", err)
		}
	}

	return pem, nil
}

// FetchAndSetCA fetches the hub trust bundle and stores it via SetCA for use by
// all subsequent TLS connections.
//
// Deprecated: Use FetchTrustBundle together with TrustStore.SetCA instead.
// This function mutates global state and is retained only for backward compatibility.
func FetchAndSetCA(ctx context.Context, caURL string, caFingerprint string) error {
	pem, err := FetchTrustBundle(ctx, caURL, caFingerprint)
	if err != nil {
		return err
	}
	SetCA(pem)
	return nil
}

// verifyCAFingerprint verifies that a PEM-encoded CA bundle matches the expected fingerprint.
// This is a copy of the auth package function to avoid circular dependencies.
func verifyCAFingerprint(caPEM []byte, expectedFingerprint string) error {
	if expectedFingerprint == "" {
		return nil
	}

	// Parse the PEM to extract the DER-encoded certificate
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return fmt.Errorf("failed to decode CA PEM")
	}

	if block.Type != "CERTIFICATE" {
		return fmt.Errorf("PEM block is not a certificate (type: %s)", block.Type)
	}

	// Compute SHA-256 hash of the DER-encoded certificate
	hash := sha256.Sum256(block.Bytes)
	actualFP := hex.EncodeToString(hash[:])

	if actualFP != expectedFingerprint {
		return fmt.Errorf("CA fingerprint mismatch: expected %s, got %s", expectedFingerprint, actualFP)
	}

	return nil
}

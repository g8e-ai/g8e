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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/cli/platform"
	"github.com/g8e-ai/g8e/internal/constants"
)

// RegisterPasskeyViaBrowser opens the gateway's console UI in the user's browser
// for passkey registration, then polls the mTLS passkey status endpoint until
// registration is detected or the timeout expires. This replaces the legacy
// localhost bootstrap server approach and works identically on all platforms.
func RegisterPasskeyViaBrowser(cfg *config.Config, userID, cliSessionID string) error {
	consoleURL := fmt.Sprintf("%s/console/#register=1&user_id=%s&cli_session_id=%s",
		cfg.OperatorPublicURL(),
		url.QueryEscape(userID),
		url.QueryEscape(cliSessionID))

	fmt.Printf("\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  PASSKEY REGISTRATION REQUIRED\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("\n")
	fmt.Printf("To complete your CLI enrollment, you need to register a passkey.\n")
	fmt.Printf("This will enable secure passwordless authentication.\n")
	fmt.Printf("\n")
	fmt.Printf("Opening your browser to the g8e console...\n")
	fmt.Printf("\n")
	fmt.Printf("  %s\n", consoleURL)
	fmt.Printf("\n")
	fmt.Printf("The console will guide you through creating a WebAuthn/FIDO2 passkey.\n")
	fmt.Printf("You can use Face ID, Touch ID, Windows Hello, or a security key.\n")
	fmt.Printf("\n")
	fmt.Printf("Waiting for passkey registration...\n")
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("\n")

	_ = platform.OpenBrowser(consoleURL)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for i := 0; i < 100; i++ { // 5 minute timeout
		<-ticker.C
		hasPasskey, err := VerifyPasskeyRegistration(cfg, userID)
		if err != nil {
			continue
		}
		if hasPasskey {
			fmt.Printf("\n✓ Passkey registered successfully!\n\n")
			return nil
		}
		if i%10 == 0 && i > 0 {
			fmt.Printf("  Still waiting... (%ds elapsed)\n", i*3)
		}
	}

	return constants.ErrPasskeyRegistrationTimedOut
}

// VerifyPasskeyRegistration checks if a user has a passkey registered
func VerifyPasskeyRegistration(cfg *config.Config, userID string) (bool, error) {
	url := fmt.Sprintf("%s%s", cfg.OperatorPublicURL(), constants.APIPaths.AuthPasskeysCLIStatus)

	// Load CLI mTLS certificate for authentication
	cliCert, err := tls.LoadX509KeyPair(cfg.CLICertFile(), cfg.CLIKeyFile())
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrFailedToLoadClientCertificate, err)
	}

	// Load CA bundle for server verification
	caBundleBytes, err := os.ReadFile(cfg.TrustBundlePath())
	if err != nil {
		return false, fmt.Errorf("%w: %w", constants.ErrFailedToReadTrustBundle, err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBundleBytes) {
		return false, constants.ErrCAParseFailed
	}

	// Create HTTP client with TLS configuration
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cliCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
		Timeout: httpTimeout,
	}

	resp, err := httpClient.Get(url)
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

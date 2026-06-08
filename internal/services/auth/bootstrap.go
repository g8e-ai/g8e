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
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/httpclient"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
)

// BootstrapConfig represents the configuration received from Auth Services
type BootstrapConfig struct {
	MaxConcurrentTasks int `json:"max_concurrent_tasks"`
	MaxMemoryMB        int `json:"max_memory_mb"`

	EnableNetworkIsolation bool   `json:"enable_network_isolation"`
	DefaultNetworkSegment  string `json:"default_network_segment"`

	EnableCommandWhitelisting bool `json:"enable_command_whitelisting"`
	EnableCommandBlacklisting bool `json:"enable_command_blacklisting"`

	HeartbeatIntervalSeconds int  `json:"heartbeat_interval_seconds"`
	EnablePeriodicMonitoring bool `json:"enable_periodic_monitoring"`

	CustomerID string `json:"user_id"`
	Region     string `json:"region"`

	OperatorID string `json:"operator_id"`

	OperatorSessionId string `json:"operator_session_id"`

	OperatorCert    string `json:"operator_cert"`
	OperatorCertKey string `json:"operator_cert_key"`
}

const (
	bootstrapMaxAttempts = 5
	bootstrapBaseDelay   = 1 * time.Second
	bootstrapMaxDelay    = 30 * time.Second
	maxResponseBytes     = 1 << 20
)

// BootstrapService handles configuration bootstrap from Auth Services via HTTP
type BootstrapService struct {
	config     *config.Config
	logger     *slog.Logger
	httpClient *http.Client
	tlsConfig  *certs.TLSConfig
}

// NewBootstrapService creates a new HTTP-based bootstrap service.
// If tlsConfig is nil, it falls back to the deprecated global certs.GetTLSConfig().
func NewBootstrapService(cfg *config.Config, logger *slog.Logger, tlsConfig *certs.TLSConfig) (*BootstrapService, error) {
	var client *http.Client
	var err error

	if tlsConfig != nil {
		// DI path: use provided TLSConfig
		if cfg.TLSServerName != "" {
			client, err = httpclient.NewWithTLSConfigAndServerName(tlsConfig, cfg.TLSServerName)
		} else {
			client, err = httpclient.NewWithTLSConfig(tlsConfig)
		}
	} else {
		// Legacy path: use global state (will be removed after migration)
		// nolint:staticcheck // SA1019: deprecated - legacy fallback path
		if cfg.TLSServerName != "" {
			client, err = httpclient.NewWithServerName(cfg.TLSServerName)
		} else {
			// nolint:staticcheck // SA1019: deprecated - legacy fallback path
			client, err = httpclient.New()
		}
	}

	if err != nil {
		return nil, fmt.Errorf("bootstrap: failed to configure TLS: %w", err)
	}

	return &BootstrapService{
		config:     cfg,
		logger:     logger,
		httpClient: client,
		tlsConfig:  tlsConfig,
	}, nil
}

// AuthServicesResponse represents the response from Auth Services Operator authentication.
// Error is json.RawMessage so the decoder accepts both the bare-string and the
// standard client error envelope object {code, message, ...} forms; both are
// normalized through httpclient.ExtractErrorMessage.
type AuthServicesResponse struct {
	Success           bool             `json:"success"`
	OperatorSessionId string           `json:"operator_session_id"`
	OperatorID        string           `json:"operator_id"`
	UserID            string           `json:"user_id"`
	Config            *BootstrapConfig `json:"config"`
	Error             json.RawMessage  `json:"error,omitempty"`
	OperatorCert      string           `json:"operator_cert"`
	OperatorCertKey   string           `json:"operator_cert_key"`
}

// RequestBootstrapConfig authenticates with client and receives bootstrap configuration.
// Uses mTLS authentication (CSR-based enrollment path).
func (bs *BootstrapService) RequestBootstrapConfig(ctx context.Context) (*BootstrapConfig, error) {
	bs.logger.Info("Authenticating with endpoint...", "endpoint", bs.config.Endpoint)

	fingerprint, err := GenerateSystemFingerprint(bs.logger)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: failed to generate system fingerprint: %w", err)
	}

	bs.config.SystemFingerprint = fingerprint.Fingerprint

	bs.logger.Info("System fingerprint generated",
		"os", fingerprint.OS,
		"architecture", fingerprint.Architecture)

	bootstrapConfig, err := bs.requestHTTPAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: failed to authenticate: %w", err)
	}

	bs.logger.Info("Authentication successful")
	return bootstrapConfig, nil
}

// operatorAuthRequest is the request body for POST /api/v1/operators/reauth.
type operatorAuthRequest struct {
	RuntimeConfig *models.RuntimeConfig `json:"runtime_config"`
}

// requestHTTPAuth authenticates via POST /api/v1/operators/reauth with exponential backoff.
func (bs *BootstrapService) requestHTTPAuth(ctx context.Context) (*BootstrapConfig, error) {

	runtimeConfig := &models.RuntimeConfig{
		CloudMode:             bs.config.CloudMode,
		CloudProvider:         bs.config.CloudProvider,
		ExecutionVaultEnabled: bs.config.ExecutionVaultEnabled,
		NoGit:                 bs.config.NoGit,
		LogLevel:              bs.config.LogLevel,

		HTTPPort: bs.config.HTTPPort,
	}

	reqBody := operatorAuthRequest{
		RuntimeConfig: runtimeConfig,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: failed to marshal auth request: %w", err)
	}

	// Use g8e.local for the hostname when endpoint is an IP address to match TLS ServerName
	hostname := bs.config.Endpoint
	if bs.config.TLSServerName != "" {
		hostname = bs.config.TLSServerName
	}
	authURL := fmt.Sprintf("https://%s:%d/api/v1/operators/reauth", hostname, bs.config.HTTPPort)

	var lastErr error
	delay := bootstrapBaseDelay

	for attempt := 1; attempt <= bootstrapMaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if attempt > 1 {
			bs.logger.Info("Retrying authentication...", "attempt", attempt, "max_attempts", bootstrapMaxAttempts, "delay", delay)
			time.Sleep(delay)
			delay = min(delay*2, bootstrapMaxDelay)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", authURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("bootstrap: failed to build auth request: %w", err)
		}
		req.Header.Set(constants.HeaderContentType, "application/json")
		req.Header.Set(constants.HeaderXRequestTimestamp, sqliteutil.NowTimestamp())

		bs.logger.Info("Authentication request transmitted", "attempt", attempt)

		resp, err := bs.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("bootstrap: authentication request failed: %w", err)
			continue
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		closeErr := resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("bootstrap: failed to read auth response: %w", err)
			continue
		}
		if closeErr != nil {
			lastErr = fmt.Errorf("bootstrap: failed to close response body: %w", closeErr)
			continue
		}

		// Handle non-200 status codes. The server may reply with either a bare
		// string error or the standard client error envelope object - decode into
		// json.RawMessage and normalize via httpclient.ExtractErrorMessage so we
		// never produce a confusing "cannot unmarshal object into string" failure.
		if resp.StatusCode != http.StatusOK {
			var errResp struct {
				Error json.RawMessage `json:"error"`
			}
			msg := ""
			if json.Unmarshal(respBody, &errResp) == nil {
				msg = httpclient.ExtractErrorMessage(errResp.Error)
			}
			if msg != "" {
				// If it's a 4xx error (client error), don't retry unless it's a 429
				if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
					return nil, fmt.Errorf("bootstrap: authentication failed (status %d): %s", resp.StatusCode, msg)
				}
				lastErr = fmt.Errorf("bootstrap: authentication failed (status %d): %s", resp.StatusCode, msg)
			} else {
				lastErr = fmt.Errorf("bootstrap: authentication failed with status %d", resp.StatusCode)
			}
			continue
		}

		var authResp AuthServicesResponse
		if err := json.Unmarshal(respBody, &authResp); err != nil {
			lastErr = fmt.Errorf("bootstrap: failed to decode auth response: %w", err)
			continue
		}

		if !authResp.Success {
			// Success=false in the JSON body is a logical failure, usually shouldn't be retried
			// unless it's a transient server issue.
			return nil, fmt.Errorf("bootstrap: authentication failed: %s", httpclient.ExtractErrorMessage(authResp.Error))
		}

		if authResp.Config == nil {
			return nil, fmt.Errorf("bootstrap: no configuration returned from Auth Services")
		}

		if authResp.OperatorSessionId == "" {
			return nil, fmt.Errorf("bootstrap: no operator_session_id returned from Auth Services")
		}

		authResp.Config.OperatorSessionId = authResp.OperatorSessionId
		authResp.Config.OperatorID = authResp.OperatorID
		authResp.Config.OperatorCert = authResp.OperatorCert
		authResp.Config.OperatorCertKey = authResp.OperatorCertKey
		return authResp.Config, nil
	}

	return nil, fmt.Errorf("bootstrap: authentication failed after %d attempts: %w", bootstrapMaxAttempts, lastErr)
}

func (bs *BootstrapService) SetHTTPClient(client *http.Client) {
	bs.httpClient = client
}

// ApplyBootstrapConfig applies bootstrap configuration to the service config.
// This data is held strictly in memory within the config.Config struct and is never persisted to disk.
func (bs *BootstrapService) ApplyBootstrapConfig(bootstrapConfig *BootstrapConfig) error {
	bs.logger.Info("Applying Operator configuration in-memory")

	if bootstrapConfig.MaxConcurrentTasks > 0 {
		bs.config.MaxConcurrentTasks = bootstrapConfig.MaxConcurrentTasks
	}
	if bootstrapConfig.MaxMemoryMB > 0 {
		bs.config.MaxMemoryMB = bootstrapConfig.MaxMemoryMB
	}

	if bootstrapConfig.OperatorID != "" {
		bs.config.OperatorID = bootstrapConfig.OperatorID
	}
	if bootstrapConfig.OperatorSessionId != "" {
		bs.config.OperatorSessionId = bootstrapConfig.OperatorSessionId
	}

	if bootstrapConfig.HeartbeatIntervalSeconds > 0 {
		bs.config.HeartbeatInterval = time.Duration(bootstrapConfig.HeartbeatIntervalSeconds) * time.Second
	}

	if bootstrapConfig.OperatorCert != "" && bootstrapConfig.OperatorCertKey != "" {
		if err := bs.rebuildTransportWithOperatorCert(bootstrapConfig.OperatorCert, bootstrapConfig.OperatorCertKey); err != nil {
			// Per-operator mTLS is a hard security requirement once the platform
			// issues a cert; silently falling back to the embedded cert would
			// violate the "mTLS on every connection" contract documented in
			// docs/architecture/operator.md. Surface this as a cert trust
			// failure so ExitCodeFromError maps it to ExitCertTrustFailure (7).
			bs.logger.Error("Per-operator mTLS certificate is invalid; aborting startup",
				string(constants.ConnectionStateError), err)
			return fmt.Errorf("bootstrap: cert trust failure: per-operator mTLS cert invalid: %w", err)
		}
		bs.logger.Info("HTTP transport upgraded to per-operator mTLS certificate (in-memory)")
	}

	return nil
}

// rebuildTransportWithOperatorCert adds the per-operator mTLS client cert
// received from the bootstrap response to the HTTP client.
func (bs *BootstrapService) rebuildTransportWithOperatorCert(certPEM, keyPEM string) error {
	operatorCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("bootstrap: failed to parse per-operator cert+key: %w", err)
	}

	var baseTLSConfig *tls.Config
	if bs.tlsConfig != nil {
		baseTLSConfig, err = bs.tlsConfig.GetTLSConfig()
		if err != nil {
			return fmt.Errorf("bootstrap: failed to get base TLS config from DI: %w", err)
		}
	} else {
		baseTLSConfig, err = certs.GetTLSConfig()
		if err != nil {
			return fmt.Errorf("bootstrap: failed to get base TLS config: %w", err)
		}
	}

	operatorTLSConfig := &tls.Config{
		Certificates:     []tls.Certificate{operatorCert},
		RootCAs:          baseTLSConfig.RootCAs,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: baseTLSConfig.CurvePreferences,
	}

	bs.httpClient = httpclient.NewWithTLS(operatorTLSConfig)

	return nil
}

// SanitizeURL removes credentials from a URL for safe logging
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "invalid-url"
	}
	return parsed.Host
}

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
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/certs"
	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/httpclient"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/sqliteutil"
	"github.com/g8e-ai/g8e/components/g8eo/services/system"
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

	APIKey string `json:"api_key"`

	OperatorCert    string `json:"operator_cert"`
	OperatorCertKey string `json:"operator_cert_key"`
}

// BootstrapService handles configuration bootstrap from Auth Services via HTTP
type BootstrapService struct {
	config     *config.Config
	logger     *slog.Logger
	httpClient *http.Client
}

// NewBootstrapService creates a new HTTP-based bootstrap service
func NewBootstrapService(cfg *config.Config, logger *slog.Logger) (*BootstrapService, error) {
	var client *http.Client
	var err error
	if cfg.TLSServerName != "" {
		client, err = httpclient.NewWithServerName(cfg.TLSServerName)
	} else {
		client, err = httpclient.New()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to configure TLS: %w", err)
	}

	return &BootstrapService{
		config:     cfg,
		logger:     logger,
		httpClient: client,
	}, nil
}

// AuthServicesResponse represents the response from Auth Services Operator authentication.
// Error is json.RawMessage so the decoder tolerates both legacy bare-string
// errors and the standard g8ed error envelope object {code, message, ...}.
type AuthServicesResponse struct {
	Success           bool             `json:"success"`
	OperatorSessionId string           `json:"operator_session_id"`
	OperatorID        string           `json:"operator_id"`
	UserID            string           `json:"user_id"`
	APIKey            string           `json:"api_key"`
	Config            *BootstrapConfig `json:"config"`
	Error             json.RawMessage  `json:"error,omitempty"`
	OperatorCert      string           `json:"operator_cert"`
	OperatorCertKey   string           `json:"operator_cert_key"`
}

// RequestBootstrapConfig authenticates with g8ed and receives bootstrap configuration.
// Supports two authentication modes:
// - API key auth: POST /api/auth/operator with Bearer token
// - OperatorSession auth: Device link flow using pre-authorized operator session IDs
func (bs *BootstrapService) RequestBootstrapConfig(ctx context.Context) (*BootstrapConfig, error) {
	bs.logger.Info("Authenticating with endpoint...", "auth_mode", bs.config.AuthMode, "endpoint", bs.config.Endpoint)

	fingerprint, err := GenerateSystemFingerprint(bs.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to generate system fingerprint: %w", err)
	}

	bs.config.SystemFingerprint = fingerprint.Fingerprint

	bs.logger.Info("System fingerprint generated",
		"os", fingerprint.OS,
		"architecture", fingerprint.Architecture)

	systemInfo := &models.SystemInfo{
		Hostname:          system.GetHostname(),
		OS:                system.GetOSName(),
		Architecture:      system.GetArchitecture(),
		CPUCount:          system.GetNumCPU(),
		MemoryMB:          uint64(system.GetMemoryMB()),
		PublicIP:          system.GetPublicIP(bs.config.IPService),
		InternalIP:        system.GetLocalIP(bs.config.IPResolver),
		Interfaces:        system.GetNetworkInterfaces(),
		CurrentUser:       system.GetCurrentUser(),
		SystemFingerprint: fingerprint.Fingerprint,
		FingerprintDetails: models.FingerprintDetails{
			OS:           fingerprint.OS,
			Architecture: fingerprint.Architecture,
			CPUCount:     fingerprint.CPUCount,
			MachineID:    fingerprint.MachineID,
		},
		OSDetails:           system.GetOSDetails(),
		UserDetails:         system.GetUserDetails(bs.config.Shell),
		DiskDetails:         system.GetDiskDetails(),
		MemoryDetails:       system.GetMemoryDetails(),
		Environment:         system.GetEnvironmentDetails(bs.config.Lang, bs.config.Term, bs.config.TZ),
		IsCloudOperator:     bs.config.CloudMode,
		CloudProvider:       bs.config.CloudProvider,
		LocalStorageEnabled: bs.config.LocalStoreEnabled,
	}

	bootstrapConfig, err := bs.requestHTTPAuth(ctx, systemInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	bs.logger.Info("Authentication successful")
	return bootstrapConfig, nil
}

// operatorAuthRequest is the request body for POST /api/auth/operator.
type operatorAuthRequest struct {
	SystemInfo        *models.SystemInfo    `json:"system_info"`
	RuntimeConfig     *models.RuntimeConfig `json:"runtime_config"`
	OperatorSessionID string                `json:"operator_session_id,omitempty"`
	AuthMode          string                `json:"auth_mode"`
}

// requestHTTPAuth authenticates via POST /api/auth/operator with exponential backoff.
func (bs *BootstrapService) requestHTTPAuth(ctx context.Context, systemInfo *models.SystemInfo) (*BootstrapConfig, error) {
	const (
		maxAttempts = 5
		baseDelay   = 1 * time.Second
		maxDelay    = 30 * time.Second
	)

	authMode := bs.config.AuthMode
	if authMode == "" {
		authMode = constants.Status.AuthMode.APIKey
	}

	runtimeConfig := &models.RuntimeConfig{
		CloudMode:           bs.config.CloudMode,
		CloudProvider:       bs.config.CloudProvider,
		LocalStorageEnabled: bs.config.LocalStoreEnabled,
		NoGit:               bs.config.NoGit,
		LogLevel:            bs.config.LogLevel,
		WSSPort:             bs.config.WSSPort,
		HTTPPort:            bs.config.HTTPPort,
	}

	reqBody := operatorAuthRequest{
		SystemInfo:    systemInfo,
		RuntimeConfig: runtimeConfig,
		AuthMode:      authMode,
	}
	if authMode == constants.Status.AuthMode.OperatorSession {
		reqBody.OperatorSessionID = bs.config.OperatorSessionId
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auth request: %w", err)
	}

	authURL := fmt.Sprintf("https://%s:%d/api/auth/operator", bs.config.Endpoint, bs.config.HTTPPort)

	var lastErr error
	delay := baseDelay

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if attempt > 1 {
			bs.logger.Info("Retrying authentication...", "attempt", attempt, "max_attempts", maxAttempts, "delay", delay)
			time.Sleep(delay)
			delay = min(delay*2, maxDelay)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", authURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to build auth request: %w", err)
		}
		req.Header.Set(constants.HeaderContentType, "application/json")
		if authMode != constants.Status.AuthMode.OperatorSession {
			req.Header.Set(constants.HeaderAuthorization, "Bearer "+bs.config.APIKey)
		}
		req.Header.Set(constants.HeaderXRequestTimestamp, sqliteutil.NowTimestamp())

		bs.logger.Info("Authentication request transmitted", "attempt", attempt)

		resp, err := bs.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("authentication request failed: %w", err)
			continue
		}

		const maxResponseBytes = 1 << 20
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read auth response: %w", err)
			continue
		}

		// Handle non-200 status codes. The server may reply with either a bare
		// string error or the standard g8ed error envelope object — decode into
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
					return nil, fmt.Errorf("authentication failed (status %d): %s", resp.StatusCode, msg)
				}
				lastErr = fmt.Errorf("authentication failed (status %d): %s", resp.StatusCode, msg)
			} else {
				lastErr = fmt.Errorf("authentication failed with status %d", resp.StatusCode)
			}
			continue
		}

		var authResp AuthServicesResponse
		if err := json.Unmarshal(respBody, &authResp); err != nil {
			lastErr = fmt.Errorf("failed to decode auth response: %w", err)
			continue
		}

		if !authResp.Success {
			// Success=false in the JSON body is a logical failure, usually shouldn't be retried
			// unless it's a transient server issue.
			return nil, fmt.Errorf("authentication failed: %s", httpclient.ExtractErrorMessage(authResp.Error))
		}

		if authResp.Config == nil {
			return nil, fmt.Errorf("no configuration returned from Auth Services")
		}

		if authResp.OperatorSessionId == "" {
			return nil, fmt.Errorf("no operator_session_id returned from Auth Services")
		}

		authResp.Config.OperatorSessionId = authResp.OperatorSessionId
		authResp.Config.OperatorID = authResp.OperatorID
		authResp.Config.APIKey = authResp.APIKey
		authResp.Config.OperatorCert = authResp.OperatorCert
		authResp.Config.OperatorCertKey = authResp.OperatorCertKey
		return authResp.Config, nil
	}

	return nil, fmt.Errorf("authentication failed after %d attempts: %w", maxAttempts, lastErr)
}

func (bs *BootstrapService) SetHTTPClient(client *http.Client) {
	bs.httpClient = client
}

// ApplyBootstrapConfig applies bootstrap configuration to the service config
func (bs *BootstrapService) ApplyBootstrapConfig(bootstrapConfig *BootstrapConfig) error {
	bs.logger.Info("Applying Operator configuration...")

	if bootstrapConfig.MaxConcurrentTasks > 0 {
		bs.config.MaxConcurrentTasks = bootstrapConfig.MaxConcurrentTasks
	}
	if bootstrapConfig.MaxMemoryMB > 0 {
		bs.config.MaxMemoryMB = bootstrapConfig.MaxMemoryMB
	}

	bs.config.OperatorID = bootstrapConfig.OperatorID
	bs.config.OperatorSessionId = bootstrapConfig.OperatorSessionId
	if bootstrapConfig.APIKey != "" {
		bs.config.APIKey = bootstrapConfig.APIKey
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
				"error", err)
			return fmt.Errorf("cert trust failure: per-operator mTLS cert invalid: %w", err)
		}
		bs.logger.Info("HTTP transport upgraded to per-operator mTLS certificate")
	}

	return nil
}

// rebuildTransportWithOperatorCert adds the per-operator mTLS client cert
// received from the bootstrap response to the HTTP client.
func (bs *BootstrapService) rebuildTransportWithOperatorCert(certPEM, keyPEM string) error {
	operatorCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("failed to parse per-operator cert+key: %w", err)
	}

	baseTLSConfig, err := certs.GetTLSConfig()
	if err != nil {
		return fmt.Errorf("failed to get base TLS config: %w", err)
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

// HashAPIKey creates a SHA-256 hash of the API key for secure routing
func HashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

// SanitizeURL removes credentials from a URL for safe logging
func SanitizeURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "invalid-url"
	}
	return parsed.Host
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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
	"time"

	"github.com/g8e-ai/g8e/internal/certs"
	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/httpclient"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/timesvc"
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

	Posture string `json:"posture"`

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
// tlsConfig must be non-nil; the platform always requires mutual TLS.
func NewBootstrapService(cfg *config.Config, logger *slog.Logger, tlsConfig *certs.TLSConfig) (*BootstrapService, error) {
	if tlsConfig == nil {
		return nil, fmt.Errorf("%w: TLS config is required", constants.ErrBootstrapTLSConfig)
	}

	var client *http.Client
	var err error

	if cfg.TLSServerName != "" {
		client, err = httpclient.NewWithTLSConfigAndServerName(tlsConfig, cfg.TLSServerName)
	} else {
		client, err = httpclient.NewWithTLSConfig(tlsConfig)
	}

	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrBootstrapTLSConfig, err)
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
		return nil, fmt.Errorf("%w: %w", constants.ErrBootstrapFingerprint, err)
	}

	bs.config.SystemFingerprint = fingerprint.Fingerprint

	bs.logger.Info("System fingerprint generated",
		"os", fingerprint.OS,
		"architecture", fingerprint.Architecture)

	bootstrapConfig, err := bs.requestHTTPAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrBootstrapAuth, err)
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
		return nil, fmt.Errorf("%w: %w", constants.ErrBootstrapRequestMarshal, err)
	}

	// Use the endpoint IP for the TCP connection; TLS ServerName is already
	// set on the HTTP client's TLS config (g8e.local) so the cert validates.
	httpsPort := bs.config.HTTPSPort
	if httpsPort == 0 {
		httpsPort = constants.Ports.OperatorHttps
	}
	authURL := fmt.Sprintf("https://%s:%d/api/v1/operators/reauth", bs.config.Endpoint, httpsPort)

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
			return nil, fmt.Errorf("%w: %w", constants.ErrBootstrapRequestBuild, err)
		}
		req.Header.Set(constants.HeaderContentType, "application/json")
		req.Header.Set(constants.HeaderXRequestTimestamp, timesvc.NowTimestamp())

		bs.logger.Info("Authentication request transmitted", "attempt", attempt)

		resp, err := bs.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%w: %w", constants.ErrBootstrapRequestExecute, err)
			continue
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		closeErr := resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("%w: %w", constants.ErrBootstrapResponseRead, err)
			continue
		}
		if closeErr != nil {
			lastErr = fmt.Errorf("%w: %w", constants.ErrBootstrapResponseClose, closeErr)
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
					return nil, fmt.Errorf("%w (status %d): %s", constants.ErrBootstrapResponseStatus, resp.StatusCode, msg)
				}
				lastErr = fmt.Errorf("%w (status %d): %s", constants.ErrBootstrapResponseStatus, resp.StatusCode, msg)
			} else {
				lastErr = fmt.Errorf("%w: %d", constants.ErrBootstrapResponseStatus, resp.StatusCode)
			}
			continue
		}

		var authResp AuthServicesResponse
		if err := json.Unmarshal(respBody, &authResp); err != nil {
			lastErr = fmt.Errorf("%w: %w", constants.ErrBootstrapResponseDecode, err)
			continue
		}

		if !authResp.Success {
			// Success=false in the JSON body is a logical failure, usually shouldn't be retried
			// unless it's a transient server issue.
			return nil, fmt.Errorf("%w: %s", constants.ErrBootstrapAuthFailed, httpclient.ExtractErrorMessage(authResp.Error))
		}

		if authResp.Config == nil {
			return nil, constants.ErrBootstrapNoConfig
		}

		if authResp.OperatorSessionId == "" {
			return nil, constants.ErrBootstrapNoSessionID
		}

		authResp.Config.OperatorSessionId = authResp.OperatorSessionId
		authResp.Config.OperatorID = authResp.OperatorID
		authResp.Config.OperatorCert = authResp.OperatorCert
		authResp.Config.OperatorCertKey = authResp.OperatorCertKey
		return authResp.Config, nil
	}

	return nil, fmt.Errorf("%w after %d attempts: %w", constants.ErrBootstrapAuth, bootstrapMaxAttempts, lastErr)
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

	if bootstrapConfig.Posture != "" {
		bs.config.Posture = config.GatewayPosture(bootstrapConfig.Posture)
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
			return fmt.Errorf("%w: %w", constants.ErrBootstrapCertTrust, err)
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
		return fmt.Errorf("%w: %w", constants.ErrBootstrapCertParse, err)
	}

	baseTLSConfig, err := bs.tlsConfig.GetTLSConfig()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrBootstrapTLSConfigDI, err)
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

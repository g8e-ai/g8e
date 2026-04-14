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
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/httpclient"
)

// DeviceAuthResult contains the result of device token authentication
type DeviceAuthResult struct {
	OperatorSessionID string
	OperatorID        string
}

// DeviceInfo contains device information sent during device link registration
type DeviceInfo struct {
	SystemFingerprint string `json:"system_fingerprint"`
	Hostname          string `json:"hostname"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Username          string `json:"username"`
}

// deviceRegisterResponse is the API response from device link registration
type deviceRegisterResponse struct {
	Success           bool   `json:"success"`
	OperatorSessionID string `json:"operator_session_id"`
	OperatorID        string `json:"operator_id"`
	Error             string `json:"error,omitempty"`
}

// deviceTokenRegex validates device link token format: dlk_ + 32 base64url chars
var deviceTokenRegex = regexp.MustCompile(`^dlk_[A-Za-z0-9_-]{32}$`)

// ValidateDeviceToken checks if a device token has valid format
func ValidateDeviceToken(token string) bool {
	return deviceTokenRegex.MatchString(token)
}

// AuthenticateWithDeviceToken performs device link authentication.
// username is the OS login name (USER / USERNAME env var); pass an empty string
// to fall back to "unknown".
func AuthenticateWithDeviceToken(token string, endpoint string, logger *slog.Logger, username string) (*DeviceAuthResult, error) {
	if !ValidateDeviceToken(token) {
		return nil, fmt.Errorf("invalid device token format")
	}

	var client *http.Client
	var clientErr error
	if net.ParseIP(endpoint) != nil {
		client, clientErr = httpclient.NewWithServerName(constants.DefaultEndpoint)
	} else {
		client, clientErr = httpclient.New()
	}
	if clientErr != nil {
		return nil, fmt.Errorf("failed to configure transport security: %w", clientErr)
	}

	return authenticateWithDeviceTokenUsingClient(token, endpoint, logger, client, username)
}

// authenticateWithDeviceTokenUsingClient is the inner implementation that accepts
// an injected http.Client, enabling unit tests to substitute a test TLS server.
func authenticateWithDeviceTokenUsingClient(token string, endpoint string, logger *slog.Logger, client *http.Client, username string) (*DeviceAuthResult, error) {
	logger.Info("Authenticating with device link token")

	if !ValidateDeviceToken(token) {
		return nil, fmt.Errorf("invalid device token format")
	}

	fingerprint, err := GenerateSystemFingerprint(logger)
	if err != nil {
		return nil, fmt.Errorf("failed to generate system fingerprint: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	if username == "" {
		username = "unknown"
	}

	deviceInfo := DeviceInfo{
		SystemFingerprint: fingerprint.Fingerprint,
		Hostname:          hostname,
		OS:                fingerprint.OS,
		Arch:              fingerprint.Architecture,
		Username:          username,
	}

	logger.Info("Device info collected",
		"hostname", hostname,
		"os", fingerprint.OS,
		"arch", fingerprint.Architecture)

	registerURL := fmt.Sprintf("https://%s/auth/link/%s/register", endpoint, token)
	logger.Info("Registering with device link", "url", registerURL)

	bodyBytes, err := json.Marshal(deviceInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal device info: %w", err)
	}

	req, err := http.NewRequest("POST", registerURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set(constants.HeaderContentType, "application/json")

	req.Header.Set(constants.HeaderUserAgent, "g8e operator")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device registration request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body with size limit to prevent OOM from malicious servers
	const maxResponseBytes = 1 << 20 // 1 MB
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result deviceRegisterResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = fmt.Sprintf("registration failed with status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("device registration failed: %s", errMsg)
	}

	if result.OperatorSessionID == "" {
		return nil, fmt.Errorf("device registration succeeded but no operator session ID returned")
	}

	logger.Info("Device link authentication successful",
		"operator_id", result.OperatorID)

	return &DeviceAuthResult{
		OperatorSessionID: result.OperatorSessionID,
		OperatorID:        result.OperatorID,
	}, nil
}

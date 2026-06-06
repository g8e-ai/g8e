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

package gateway

import (
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistrationService(t *testing.T) {
	t.Parallel()

	db := &GatewayDBService{}
	pki := &PKIAuthority{}
	logger := slog.New(slog.NewTextHandler(nil, nil))
	userSvc := &UserService{}
	sessionSvc := &SessionsService{}
	cfg := &config.GatewayConfig{}

	service := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, cfg)

	assert.NotNil(t, service)
	assert.Equal(t, db, service.db)
	assert.Equal(t, pki, service.pki)
	assert.Equal(t, logger, service.logger)
	assert.Equal(t, userSvc, service.userSvc)
	assert.Equal(t, sessionSvc, service.sessionSvc)
	assert.Equal(t, cfg, service.cfg)
}

func TestPKIPhase2_CalculateSerialFromPEM(t *testing.T) {
	t.Parallel()

	// Test that calculateSerialFromPEM correctly extracts serial from a certificate
	dataDir := t.TempDir()
	pkiDir := filepath.Join(dataDir, "pki")
	logger := testutil.NewTestLogger()
	db, err := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
	require.NoError(t, err)
	sm, err := NewSecretManager(db.db, t.TempDir(), logger)
	require.NoError(t, err)

	pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	// Generate a CSR and sign it
	csr := testutil.GenerateTestCSRP256(t, "test-operator")
	certPEM, _, err := pki.SignCSR(csr, "operator", "org-123", "op-456", "", "session-789", "")
	require.NoError(t, err)

	// Extract serial using the helper function
	serial := calculateSerialFromPEM(certPEM)
	assert.NotEmpty(t, serial, "serial should be extracted from issued cert")

	// Verify serial matches the actual certificate serial
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	assert.Equal(t, cert.SerialNumber.String(), serial, "extracted serial should match certificate serial")
}

func TestSessionWebBindKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		expected  string
	}{
		{"Valid session ID", "web-session-123", "g8e:sessions:web:web-session-123:bind"},
		{"Empty session ID", "", "g8e:sessions:web::bind"},
		{"Session with special chars", "session-abc-123", "g8e:sessions:web:sessions-abc-123:bind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := sessionWebBindKey(tt.sessionID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSessionOperatorBindKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		expected  string
	}{
		{"Valid session ID", "op-session-456", "g8e:sessions:operator:op-session-456:bind"},
		{"Empty session ID", "", "g8e:sessions:operator::bind"},
		{"Session with special chars", "operator-xyz-789", "g8e:sessions:operator:operator-xyz-789:bind"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := sessionOperatorBindKey(tt.sessionID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPKIPhase3_CLI_CSR_Mandatory verifies that enrollment without CLI CSR is rejected
// This is the fix for C5 (SPIFFE drift fallback) in the PKI cleanup plan.
// See: .local.dev/docs/plans/pki_cleanup.md C5
// Updated: CLI CSR is now optional for operator-only enrollment
func TestPKIPhase3_CLI_CSR_Optional(t *testing.T) {
	t.Run("RegisterDeviceCSR accepts enrollment without CLI CSR (operator-only)", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()
		pkiDir := filepath.Join(dataDir, "pki")
		logger := testutil.NewTestLogger()
		db, err := OpenGatewayDBService(dataDir, t.TempDir(), logger, true)
		require.NoError(t, err)
		sm, err := NewSecretManager(db.db, t.TempDir(), logger)
		require.NoError(t, err)

		pki := newPKIAuthority(dataDir, pkiDir, db, sm, logger)
		err = pki.EnsurePKI(nil)
		require.NoError(t, err)

		userSvc := NewUserService(db, logger)
		sessionSvc := NewSessionService(db, logger)
		cfg := &config.GatewayConfig{}
		regSvc := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, cfg)

		// Generate only Operator CSR, no CLI CSR
		opCSR := testutil.GenerateTestCSRP256(t, "test-operator")

		req := models.OperatorRegistrationRequest{
			SystemFingerprint: "test-fingerprint",
			Hostname:          "test-host",
			CSR:               opCSR,
			CLICSR:            "", // Empty CLI CSR is now allowed for operator-only enrollment
		}

		resp, err := regSvc.RegisterDeviceCSR("user-123", "org-123", req)
		assert.NoError(t, err, "enrollment without CLI CSR should succeed for operator-only")
		assert.NotNil(t, resp)
		assert.True(t, resp.Success)
		assert.NotEmpty(t, resp.OperatorID)
		assert.NotEmpty(t, resp.OperatorSessionID)
		assert.Empty(t, resp.CLISessionID, "CLI session ID should be empty for operator-only enrollment")
		assert.Empty(t, resp.CLICert, "CLI cert should be empty for operator-only enrollment")
	})
}

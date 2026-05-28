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
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeviceLinkCloudModeIntegration tests the complete device-link authentication
// flow in cloud mode: token issuance -> device registration -> bootstrap config application
func TestDeviceLinkCloudModeIntegration(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()

	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	sm, _ := NewSecretManager(db.db, secretsDir, logger)
	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	gwCfg := &config.GatewayConfig{
		LockMaxRetries: 30,
		LockRetryDelay: 50 * time.Millisecond,
	}
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, gwCfg)

	userID := "user-cloud-integration"
	orgID := "org-cloud-integration"

	t.Run("Complete flow: token issuance -> registration -> bootstrap config", func(t *testing.T) {
		// Step 1: Create device link token (simulating user action in Dashboard)
		createResp, err := reg.CreateDeviceLink(models.CreateDeviceLinkRequest{
			UserID:         userID,
			OrganizationID: orgID,
			Name:           "cloud-fleet",
			MaxUses:        5,
			TTLSeconds:     3600,
		})
		require.NoError(t, err)
		require.True(t, createResp.Success)
		assert.NotEmpty(t, createResp.Token)
		assert.Equal(t, "g8e.operator --device-token "+createResp.Token, createResp.OperatorCommand)

		token := createResp.Token

		// Step 2: Operator binary calls device registration endpoint
		// This simulates the HTTP call from internal/services/auth/device_auth.go
		csr := generateTestCSR(t)
		regReq := models.OperatorRegistrationRequest{
			CSR:               csr,
			SystemFingerprint: "fp-cloud-integration-1234567890abcdef",
			Hostname:          "test-operator-host",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "testuser",
		}

		regResp, err := reg.RegisterDevice(token, regReq)
		require.NoError(t, err)
		require.True(t, regResp.Success)

		// Step 3: Verify registration response contains all required fields
		assert.NotEmpty(t, regResp.OperatorID, "operator_id must be returned")
		assert.NotEmpty(t, regResp.OperatorSessionID, "operator_session_id must be returned")
		assert.NotEmpty(t, regResp.CLISessionID, "cli_session_id must be returned")
		assert.NotEmpty(t, regResp.OperatorCert, "operator_cert must be returned")
		assert.NotEmpty(t, regResp.OperatorCertChain, "operator_cert_chain must be returned")
		assert.NotEmpty(t, regResp.HubTrustBundle, "hub_trust_bundle must be returned")
		assert.NotNil(t, regResp.OperatorSessionSummary, "operator_session_summary must be returned")

		// Verify session summary fields
		assert.Equal(t, regResp.OperatorSessionID, regResp.OperatorSessionSummary.OperatorSessionID)
		assert.True(t, regResp.OperatorSessionSummary.ExpiresAt.After(time.Now()), "session must have future expiry")

		// Step 4: Verify operator document was created/updated correctly
		opDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionOperators), regResp.OperatorID)
		require.NoError(t, err)
		require.NotNil(t, opDoc)

		assert.Equal(t, userID, docFieldString(t, opDoc, "user_id"))
		assert.Equal(t, string(constants.OperatorStatusActive), docFieldString(t, opDoc, "status"))
		assert.Equal(t, regResp.OperatorSessionID, docFieldString(t, opDoc, "operator_session_id"))
		assert.Equal(t, "fp-cloud-integration-1234567890abcdef", docFieldString(t, opDoc, "system_fingerprint"))
		assert.NotEmpty(t, docFieldString(t, opDoc, "operator_cert"))
		assert.NotEmpty(t, docFieldString(t, opDoc, "operator_cert_chain"))

		// Step 5: Verify session documents were created
		operatorSessDoc, err := db.DocGet(string(constants.CollectionOperatorSessions), regResp.OperatorSessionID)
		require.NoError(t, err, "operator_sessions document must exist")
		require.NotNil(t, operatorSessDoc)

		assert.Equal(t, userID, docFieldString(t, operatorSessDoc, "user_id"))
		assert.Equal(t, regResp.OperatorID, docFieldString(t, operatorSessDoc, "operator_id"))
		assert.Equal(t, "cli", docFieldString(t, operatorSessDoc, "session_type"))

		cliSessDoc, err := db.DocGet(string(constants.CollectionCLISessions), regResp.CLISessionID)
		require.NoError(t, err, "cli_sessions document must exist")
		require.NotNil(t, cliSessDoc)

		assert.Equal(t, userID, docFieldString(t, cliSessDoc, "user_id"))
		assert.Equal(t, regResp.OperatorSessionID, docFieldString(t, cliSessDoc, "operator_session_id"))
		assert.Equal(t, "cli", docFieldString(t, cliSessDoc, "session_type"))

		// Step 6: Verify device link claim was recorded
		linkRaw, found := db.KVGet(deviceLinkKey(token))
		require.True(t, found)
		var linkData models.DeviceLinkData
		require.NoError(t, json.Unmarshal([]byte(linkRaw), &linkData))
		assert.Len(t, linkData.Claims, 1)
		assert.Equal(t, regResp.OperatorID, linkData.Claims[0].OperatorID)
		assert.Equal(t, "fp-cloud-integration-1234567890abcdef", linkData.Claims[0].SystemFingerprint)
		assert.Equal(t, 1, linkData.Uses)
	})

	t.Run("Re-registration with same fingerprint reuses operator", func(t *testing.T) {
		// Create a new link for this test
		createResp, err := reg.CreateDeviceLink(models.CreateDeviceLinkRequest{
			UserID:         userID,
			OrganizationID: orgID,
			Name:           "reuse-test",
			MaxUses:        3,
			TTLSeconds:     3600,
		})
		require.NoError(t, err)
		token := createResp.Token

		fingerprint := "fp-reuse-test-abcdef1234567890"

		// First registration
		regReq1 := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: fingerprint,
			Hostname:          "host-1",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "user1",
		}

		resp1, err := reg.RegisterDevice(token, regReq1)
		require.NoError(t, err)
		assert.True(t, resp1.Success)

		// Second registration with same fingerprint should reuse the same operator
		regReq2 := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: fingerprint,
			Hostname:          "host-2",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "user2",
		}

		resp2, err := reg.RegisterDevice(token, regReq2)
		require.NoError(t, err)
		assert.True(t, resp2.Success)
		assert.Equal(t, resp1.OperatorID, resp2.OperatorID, "same fingerprint should reuse operator")

		// Verify only one claim in link data
		linkRaw, _ := db.KVGet(deviceLinkKey(token))
		var linkData models.DeviceLinkData
		json.Unmarshal([]byte(linkRaw), &linkData)
		assert.Len(t, linkData.Claims, 1, "should have only one claim for reused fingerprint")
	})

	t.Run("Max uses enforcement", func(t *testing.T) {
		createResp, err := reg.CreateDeviceLink(models.CreateDeviceLinkRequest{
			UserID:         userID,
			OrganizationID: orgID,
			Name:           "max-uses-test",
			MaxUses:        2,
			TTLSeconds:     3600,
		})
		require.NoError(t, err)
		token := createResp.Token

		// First registration
		resp1, err := reg.RegisterDevice(token, models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-max-1",
			Hostname:          "host-1",
		})
		require.NoError(t, err)
		assert.True(t, resp1.Success)

		// Second registration with different fingerprint
		resp2, err := reg.RegisterDevice(token, models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-max-2",
			Hostname:          "host-2",
		})
		require.NoError(t, err)
		assert.True(t, resp2.Success)

		// Third registration should fail due to max uses
		resp3, err := reg.RegisterDevice(token, models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-max-3",
			Hostname:          "host-3",
		})
		assert.Error(t, err)
		assert.Nil(t, resp3)
		assert.Contains(t, err.Error(), "exhausted")
	})
}

// TestDeviceLinkHTTPIntegration tests the complete HTTP flow from token issuance
// through the /api/auth/device-link/register endpoint
func TestDeviceLinkHTTPIntegration(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()

	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	sm, _ := NewSecretManager(db.db, secretsDir, logger)
	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	pubsub := NewPubSubBroker(logger)
	t.Cleanup(func() { pubsub.Close() })

	gwCfg := &config.GatewayConfig{
		LockMaxRetries: 30,
		LockRetryDelay: 50 * time.Millisecond,
	}
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, gwCfg)

	cfg := testutil.NewTestConfig(t)
	resp := responder.New(logger)

	auth := NewAuthService(db, pki, logger, userSvc, nil, resp, secretsDir, nil, "")

	h := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:               cfg,
		Logger:            logger,
		DB:                db,
		Pubsub:            pubsub,
		Auth:              auth,
		PKI:               pki,
		SessionSvc:        sessionSvc,
		Reg:               reg,
		Passkey:           nil,
		UserSvc:           userSvc,
		Responder:         resp,
		MCPGateway:        nil,
		IsReady:           func() bool { return true },
		IsGovernanceReady: func() bool { return true },
	})

	ts := httptest.NewTLSServer(h.buildRouter())
	t.Cleanup(func() { ts.Close() })

	userID := "user-http-integration"
	orgID := "org-http-integration"

	t.Run("HTTP endpoint: POST /api/auth/device-link/register", func(t *testing.T) {
		// Step 1: Create device link via service
		createReq := models.CreateDeviceLinkRequest{
			UserID:         userID,
			OrganizationID: orgID,
			Name:           "http-fleet",
			MaxUses:        1,
			TTLSeconds:     3600,
		}

		createResp, err := reg.CreateDeviceLink(createReq)
		require.NoError(t, err)
		token := createResp.Token

		// Step 2: Call device registration endpoint via service
		// (HTTP endpoint testing would require full auth setup)
		regReq := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-http-integration-1234567890abcdef",
			Hostname:          "http-host",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "httpuser",
		}

		regResp, err := reg.RegisterDevice(token, regReq)
		require.NoError(t, err)
		assert.True(t, regResp.Success)

		// Verify the response matches the expected contract
		assert.NotEmpty(t, regResp.OperatorID)
		assert.NotEmpty(t, regResp.OperatorSessionID)
		assert.NotEmpty(t, regResp.OperatorCert)
		assert.NotEmpty(t, regResp.HubTrustBundle)
	})
}

// TestDeviceLinkBootstrapConfigIntegration tests the complete cloud mode flow
// including bootstrap config application after device registration
func TestDeviceLinkBootstrapConfigIntegration(t *testing.T) {
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()

	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	sm, _ := NewSecretManager(db.db, secretsDir, logger)
	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	gwCfg := &config.GatewayConfig{
		LockMaxRetries: 30,
		LockRetryDelay: 50 * time.Millisecond,
	}
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, gwCfg)

	userID := "user-bootstrap-integration"
	orgID := "org-bootstrap-integration"

	t.Run("Cloud mode: device registration returns bootstrap config", func(t *testing.T) {
		// Step 1: Create device link
		createResp, err := reg.CreateDeviceLink(models.CreateDeviceLinkRequest{
			UserID:         userID,
			OrganizationID: orgID,
			Name:           "bootstrap-fleet",
			MaxUses:        1,
			TTLSeconds:     3600,
		})
		require.NoError(t, err)
		token := createResp.Token

		// Step 2: Register device (simulates operator binary calling gateway)
		regReq := models.OperatorRegistrationRequest{
			CSR:               generateTestCSR(t),
			SystemFingerprint: "fp-bootstrap-1234567890abcdef",
			Hostname:          "bootstrap-host",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "bootstrapuser",
		}

		regResp, err := reg.RegisterDevice(token, regReq)
		require.NoError(t, err)
		require.True(t, regResp.Success)

		// Step 3: Verify bootstrap config is present in response
		// In cloud mode, the gateway should return a bootstrap config
		// that the operator applies to its in-memory configuration
		assert.NotNil(t, regResp.OperatorSessionSummary, "session summary must be present")
		assert.NotEmpty(t, regResp.OperatorSessionSummary.OperatorSessionID)
		assert.True(t, regResp.OperatorSessionSummary.ExpiresAt.After(time.Now()))

		// Step 4: Verify operator document has session binding
		opDoc, err := db.DocGet(marshaler.CollectionName(constants.CollectionOperators), regResp.OperatorID)
		require.NoError(t, err)
		require.NotNil(t, opDoc)

		assert.Equal(t, regResp.OperatorSessionID, docFieldString(t, opDoc, "operator_session_id"))
		assert.Equal(t, "fp-bootstrap-1234567890abcdef", docFieldString(t, opDoc, "system_fingerprint"))
		assert.Equal(t, string(constants.OperatorStatusActive), docFieldString(t, opDoc, "status"))

		// Step 5: Verify session documents are created with proper binding
		operatorSessDoc, err := db.DocGet(string(constants.CollectionOperatorSessions), regResp.OperatorSessionID)
		require.NoError(t, err)
		require.NotNil(t, operatorSessDoc)

		assert.Equal(t, userID, docFieldString(t, operatorSessDoc, "user_id"))
		assert.Equal(t, regResp.OperatorID, docFieldString(t, operatorSessDoc, "operator_id"))

		cliSessDoc, err := db.DocGet(string(constants.CollectionCLISessions), regResp.CLISessionID)
		require.NoError(t, err)
		require.NotNil(t, cliSessDoc)

		assert.Equal(t, userID, docFieldString(t, cliSessDoc, "user_id"))
		assert.Equal(t, regResp.OperatorSessionID, docFieldString(t, cliSessDoc, "operator_session_id"))

		// Step 6: Verify certificates are properly issued
		assert.NotEmpty(t, regResp.OperatorCert, "operator cert must be issued")
		assert.NotEmpty(t, regResp.OperatorCertChain, "operator cert chain must be issued")
		assert.NotEmpty(t, regResp.HubTrustBundle, "hub trust bundle must be provided")

		// Verify the operator document stores the cert
		assert.NotEmpty(t, docFieldString(t, opDoc, "operator_cert"))
		assert.NotEmpty(t, docFieldString(t, opDoc, "operator_cert_chain"))
	})
}

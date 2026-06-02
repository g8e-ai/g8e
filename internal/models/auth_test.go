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

package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/assert"
)

func TestOperatorRegistrationRequest(t *testing.T) {
	t.Run("creates valid registration request", func(t *testing.T) {
		req := &OperatorRegistrationRequest{
			CSR:               testutil.GenerateTestCSR(t, "test-operator"),
			CLICSR:            testutil.GenerateTestCSR(t, "test-cli"),
			SystemFingerprint: "fp-123",
			Hostname:          "test-host",
			OS:                "linux",
			Arch:              "amd64",
			Username:          "testuser",
			IPAddress:         "192.168.1.1",
		}

		assert.Contains(t, req.CSR, "CERTIFICATE REQUEST")
		assert.Equal(t, "fp-123", req.SystemFingerprint)
	})
}

func TestOperatorRegistrationResponse(t *testing.T) {
	t.Run("creates successful registration response", func(t *testing.T) {
		now := time.Now().UTC()
		certPEM, _ := testutil.GenerateTestCertificate(t, "test-operator")
		resp := &OperatorRegistrationResponse{
			Success:           true,
			OperatorSessionID: "session-123",
			CLISessionID:      "cli-session-123",
			OperatorID:        "operator-123",
			OperatorCert:      certPEM,
			OperatorCertChain: certPEM,
			CLICert:           certPEM,
			CLICertChain:      certPEM,
			HubTrustBundle:    certPEM,
			OperatorSessionSummary: &SessionSummary{
				OperatorSessionID: "session-123",
				ExpiresAt:         now.Add(24 * time.Hour),
				CreatedAt:         now,
			},
		}

		assert.True(t, resp.Success)
		assert.Equal(t, "session-123", resp.OperatorSessionID)
		assert.Equal(t, "cli-session-123", resp.CLISessionID)
		assert.NotNil(t, resp.OperatorSessionSummary)
	})

	t.Run("creates failed registration response", func(t *testing.T) {
		resp := &OperatorRegistrationResponse{
			Success: false,
			Error:   "invalid CSR",
		}

		assert.False(t, resp.Success)
		assert.Equal(t, "invalid CSR", resp.Error)
	})
}

func TestSessionSummary(t *testing.T) {
	t.Run("creates valid session summary", func(t *testing.T) {
		now := time.Now().UTC()
		summary := &SessionSummary{
			OperatorSessionID: "session-123",
			ExpiresAt:         now.Add(24 * time.Hour),
			CreatedAt:         now,
		}

		assert.Equal(t, "session-123", summary.OperatorSessionID)
	})
}

func TestOperatorDocumentGo(t *testing.T) {
	t.Run("creates valid Operator document", func(t *testing.T) {
		now := time.Now().UTC()
		startedAt := now.Add(-1 * time.Hour)
		certPEM, _ := testutil.GenerateTestCertificate(t, "test-operator")

		doc := &OperatorDocumentGo{
			ID:                "operator-123",
			UserID:            "user-123",
			OrganizationID:    "org-123",
			Component:         constants.ComponentNameG8EO,
			Name:              "Test Operator",
			Status:            constants.OperatorStatusActive,
			OperatorSessionID: "session-123",
			OperatorCert:      certPEM,
			SlotNumber:        0,
			IsSlot:            true,
			Claimed:           true,
			OperatorType:      constants.OperatorTypeSystem,
			CloudSubtype:      "g8ep",
			CreatedAt:         now,
			UpdatedAt:         now,
			StartedAt:         &startedAt,
		}

		assert.Equal(t, "operator-123", doc.ID)
		assert.Equal(t, constants.ComponentNameG8EO, doc.Component)
		assert.True(t, doc.IsSlot)
		assert.True(t, doc.Claimed)
	})

	t.Run("marshals JSON with default Operator type", func(t *testing.T) {
		doc := &OperatorDocumentGo{
			ID:        "operator-123",
			UserID:    "user-123",
			Component: constants.ComponentNameG8EO,
			Status:    constants.OperatorStatusActive,
			IsSlot:    true,
			Claimed:   true,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}

		data, err := json.Marshal(doc)
		assert.NoError(t, err)
		assert.Contains(t, string(data), constants.OperatorTypeSystem)
	})

	t.Run("marshals JSON with explicit Operator type", func(t *testing.T) {
		doc := &OperatorDocumentGo{
			ID:           "operator-123",
			UserID:       "user-123",
			Component:    constants.ComponentNameG8EO,
			Status:       constants.OperatorStatusActive,
			OperatorType: constants.OperatorTypeSystem,
			IsSlot:       true,
			Claimed:      true,
			CreatedAt:    time.Now().UTC(),
			UpdatedAt:    time.Now().UTC(),
		}

		data, err := json.Marshal(doc)
		assert.NoError(t, err)
		assert.Contains(t, string(data), constants.OperatorTypeSystem)
	})
}

func TestOperatorSlotResponse(t *testing.T) {
	t.Run("creates valid slot response", func(t *testing.T) {
		resp := &OperatorSlotResponse{
			Success: true,
			Operators: []OperatorDocumentGo{
				{ID: "operator-1", IsSlot: true},
				{ID: "operator-2", IsSlot: true},
			},
		}

		assert.True(t, resp.Success)
		assert.Len(t, resp.Operators, 2)
	})
}

func TestTerminateOperatorRequest(t *testing.T) {
	t.Run("creates valid request", func(t *testing.T) {
		req := &TerminateOperatorRequest{
			OperatorID: "operator-123",
			UserID:     "user-123",
			Reason:     "testing",
		}

		assert.Equal(t, "operator-123", req.OperatorID)
		assert.Equal(t, "testing", req.Reason)
	})
}

func TestTerminateOperatorResponse(t *testing.T) {
	t.Run("creates valid response", func(t *testing.T) {
		resp := &TerminateOperatorResponse{
			Success: true,
			Message: "Operator terminated",
		}

		assert.True(t, resp.Success)
		assert.Equal(t, "Operator terminated", resp.Message)
	})
}

func TestBindOperatorsRequest(t *testing.T) {
	t.Run("creates valid request", func(t *testing.T) {
		req := &BindOperatorsRequest{
			OperatorIDs:  []string{"op-1", "op-2"},
			UserID:       "user-123",
			WebSessionID: "session-123",
		}

		assert.Len(t, req.OperatorIDs, 2)
		assert.Equal(t, "user-123", req.UserID)
	})
}

func TestBindOperatorsResponse(t *testing.T) {
	t.Run("creates successful response", func(t *testing.T) {
		resp := &BindOperatorsResponse{
			Success:           true,
			BoundCount:        2,
			FailedCount:       0,
			BoundOperatorIDs:  []string{"op-1", "op-2"},
			FailedOperatorIDs: []string{},
		}

		assert.True(t, resp.Success)
		assert.Equal(t, 2, resp.BoundCount)
	})
}

func TestUnbindOperatorsRequest(t *testing.T) {
	t.Run("creates valid request", func(t *testing.T) {
		req := &UnbindOperatorsRequest{
			OperatorIDs:  []string{"op-1", "op-2"},
			UserID:       "user-123",
			WebSessionID: "session-123",
		}

		assert.Len(t, req.OperatorIDs, 2)
	})
}

func TestUnbindOperatorsResponse(t *testing.T) {
	t.Run("creates successful response", func(t *testing.T) {
		resp := &UnbindOperatorsResponse{
			Success:            true,
			UnboundCount:       2,
			FailedCount:        0,
			UnboundOperatorIDs: []string{"op-1", "op-2"},
			FailedOperatorIDs:  []string{},
		}

		assert.True(t, resp.Success)
		assert.Equal(t, 2, resp.UnboundCount)
	})
}

func TestSetTargetContextRequest(t *testing.T) {
	t.Run("creates valid request", func(t *testing.T) {
		req := &SetTargetContextRequest{
			OperatorID:   "operator-123",
			UserID:       "user-123",
			WebSessionID: "session-123",
		}

		assert.Equal(t, "operator-123", req.OperatorID)
	})
}

func TestSetTargetContextResponse(t *testing.T) {
	t.Run("creates successful response", func(t *testing.T) {
		resp := &SetTargetContextResponse{
			Success:    true,
			OperatorID: "operator-123",
		}

		assert.True(t, resp.Success)
	})

	t.Run("creates failed response", func(t *testing.T) {
		resp := &SetTargetContextResponse{
			Success: false,
			Error:   "operator not found",
		}

		assert.False(t, resp.Success)
		assert.Equal(t, "operator not found", resp.Error)
	})
}

func TestBoundSessionsDocumentGo(t *testing.T) {
	t.Run("creates valid bound sessions document", func(t *testing.T) {
		now := time.Now().UTC()
		doc := &BoundSessionsDocumentGo{
			ID:                 "bound-123",
			WebSessionID:       "session-123",
			UserID:             "user-123",
			OperatorSessionIDs: []string{"op-session-1", "op-session-2"},
			OperatorIDs:        []string{"op-1", "op-2"},
			BoundAt:            now,
			LastUpdatedAt:      now,
			Status:             constants.OperatorStatusActive,
		}

		assert.Equal(t, "bound-123", doc.ID)
		assert.Len(t, doc.OperatorSessionIDs, 2)
	})
}

func TestPasskeyCredential(t *testing.T) {
	t.Run("creates valid passkey credential", func(t *testing.T) {
		cred := &PasskeyCredential{
			ID:              []byte("cred-id"),
			PublicKey:       []byte("public-key"),
			AttestationType: "none",
			CreatedAtUnixMs: time.Now().UnixMilli(),
		}

		assert.Equal(t, "cred-id", string(cred.ID))
		assert.Equal(t, "none", cred.AttestationType)
	})
}

func TestAuthenticator(t *testing.T) {
	t.Run("creates valid authenticator", func(t *testing.T) {
		auth := Authenticator{
			AAGUID:       []byte("aaguid"),
			SignCount:    1,
			CloneWarning: false,
		}

		assert.Equal(t, uint32(1), auth.SignCount)
		assert.False(t, auth.CloneWarning)
	})
}

func TestWebSession(t *testing.T) {
	t.Run("creates valid web session", func(t *testing.T) {
		now := time.Now().UnixMilli()
		expiresAt := now + (24 * 60 * 60 * 1000)

		session := &WebSession{
			ID:              "session-123",
			UserID:          "user-123",
			CreatedAtUnixMs: now,
			ExpiresAtUnixMs: expiresAt,
		}

		assert.Equal(t, "session-123", session.ID)
		assert.Equal(t, "user-123", session.UserID)
	})
}

func TestCLISession(t *testing.T) {
	t.Run("creates valid CLI session", func(t *testing.T) {
		now := time.Now().UTC()
		session := &CLISession{
			ID:                "cli-session-123",
			UserID:            "user-123",
			OperatorSessionID: "op-session-123",
			SystemFingerprint: "fp-123",
			CertFingerprint:   "cert-fp-123",
			CertSerial:        "serial-123",
			CreatedAt:         now,
			ExpiresAt:         now.Add(24 * time.Hour),
			AbsoluteExpiresAt: now.Add(7 * 24 * time.Hour),
			IdleExpiresAt:     now.Add(1 * time.Hour),
			SessionType:       "mcli",
			IsActive:          true,
			LoginMethod:       "mtls",
		}

		assert.Equal(t, "cli-session-123", session.ID)
		assert.True(t, session.IsActive)
		assert.Equal(t, "mtls", session.LoginMethod)
	})
}

func TestUser(t *testing.T) {
	t.Run("creates valid user", func(t *testing.T) {
		user := &User{
			ID:       "user-123",
			Provider: "local",
			Status:   constants.UserStatusActive,
			PasskeyCredentials: []PasskeyCredential{
				{ID: []byte("cred-1")},
			},
		}

		assert.Equal(t, "user-123", user.ID)
		assert.True(t, user.IsActive())
	})

	t.Run("active user with empty status", func(t *testing.T) {
		user := &User{
			ID:     "user-123",
			Status: "",
		}

		assert.True(t, user.IsActive())
	})

	t.Run("inactive user", func(t *testing.T) {
		user := &User{
			ID:     "user-123",
			Status: constants.UserStatusDisabled,
		}

		assert.False(t, user.IsActive())
	})

	t.Run("nil user is inactive", func(t *testing.T) {
		var user *User
		assert.False(t, user.IsActive())
	})

	t.Run("bootstrap user", func(t *testing.T) {
		user := &User{
			ID:          "user-123",
			IsBootstrap: true,
			Status:      constants.UserStatusActive,
		}

		assert.True(t, user.IsBootstrap)
	})
}

func TestAdminAuditEntry(t *testing.T) {
	t.Run("creates valid audit entry", func(t *testing.T) {
		now := time.Now().UTC()
		entry := &AdminAuditEntry{
			ID:         "audit-123",
			At:         now,
			Action:     AdminAuditActionRetireLocalOwner,
			Actor:      "user-123",
			Target:     "target-123",
			OperatorID: "operator-123",
			Details: map[string]interface{}{
				"reason": "test",
			},
		}

		assert.Equal(t, AdminAuditActionRetireLocalOwner, entry.Action)
		assert.Equal(t, "user-123", entry.Actor)
	})
}

func TestTrustedSigner(t *testing.T) {
	t.Run("creates valid trusted signer", func(t *testing.T) {
		now := time.Now().UTC()
		signer := &TrustedSigner{
			ID:        "signer-123",
			PublicKey: "public-key-hex",
			AddedAt:   now,
			Enabled:   true,
		}

		assert.Equal(t, "signer-123", signer.ID)
		assert.True(t, signer.Enabled)
	})
}

func TestAppPolicy(t *testing.T) {
	t.Run("creates valid app policy", func(t *testing.T) {
		now := time.Now().UTC()
		policy := &AppPolicy{
			AppID:              "app-123",
			AllowedCollections: []string{"collection1", "collection2"},
			AllowedEventTypes:  []string{"event1", "event2"},
			AllowedIntents:     []string{"intent1"},
			AutoApproveIntents: []string{"diagnostic"},
			RateLimitRPS:       10,
			MaxPayloadBytes:    1048576,
			RequireL3Approval:  true,
			CreatedAt:          now,
			UpdatedAt:          now,
		}

		assert.Equal(t, "app-123", policy.AppID)
		assert.Len(t, policy.AllowedCollections, 2)
		assert.True(t, policy.RequireL3Approval)
	})
}

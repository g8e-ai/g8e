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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestCLIL3Notary_VerifyL3Proof_RejectsMissingInputs(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	notary := NewCLIL3Notary(db, nil, logger, userSvc, sessionSvc)

	tests := []struct {
		name            string
		userID          string
		transactionHash string
		proof           *commonv1.L3Proof
		wantErr         string
	}{
		{
			name:            "missing user_id",
			userID:          "",
			transactionHash: "tx-hash",
			proof:           &commonv1.L3Proof{MtlsCertFingerprint: "abc123"},
			wantErr:         "user_id is required",
		},
		{
			name:            "missing transaction_hash",
			userID:          "user-1",
			transactionHash: "",
			proof:           &commonv1.L3Proof{MtlsCertFingerprint: "abc123"},
			wantErr:         "transaction_hash is required",
		},
		{
			name:            "missing proof",
			userID:          "user-1",
			transactionHash: "tx-hash",
			proof:           nil,
			wantErr:         "L3 proof is required",
		},
		{
			name:            "missing mtls_cert_fingerprint",
			userID:          "user-1",
			transactionHash: "tx-hash",
			proof:           &commonv1.L3Proof{},
			wantErr:         "mtls_cert_fingerprint is required",
		},
		{
			name:            "invalid fingerprint format",
			userID:          "user-1",
			transactionHash: "tx-hash",
			proof:           &commonv1.L3Proof{MtlsCertFingerprint: "not-hex!"},
			wantErr:         "invalid mtls_cert_fingerprint format",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ok, err := notary.VerifyL3Proof(tc.userID, tc.transactionHash, "", tc.proof)
			require.Error(t, err)
			require.False(t, ok)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestCLIL3Notary_VerifyL3Proof_RejectsInactiveUser(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	notary := NewCLIL3Notary(db, nil, logger, userSvc, sessionSvc)

	// Create a disabled user
	userID := "disabled-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusDisabled,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(userID, txHash, "", &commonv1.L3Proof{MtlsCertFingerprint: validFingerprint})
	require.Error(t, err)
	require.False(t, ok)
	require.Contains(t, err.Error(), "user is not active")
}

func TestCLIL3Notary_VerifyL3Proof_AcceptsActiveUser(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	notary := NewCLIL3Notary(db, nil, logger, userSvc, sessionSvc)

	// Create an active user
	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// Create a CLI session with a known fingerprint
	cliSessionID := "cli-session-123"
	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: "operator-session-123",
		CertFingerprint:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CertSerial:        "1234567890abcdef",
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(24 * time.Hour),
		AbsoluteExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		IdleExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "device_link",
	}
	cliSessionBytes, _ := json.Marshal(cliSession)
	require.NoError(t, db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(userID, txHash, cliSessionID, &commonv1.L3Proof{MtlsCertFingerprint: validFingerprint})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCLIL3Notary_VerifyL3Proof_RejectsUnknownFingerprint(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	notary := NewCLIL3Notary(db, nil, logger, userSvc, sessionSvc)

	// Create an active user
	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// No CLI session created - verification should fail
	unknownFingerprint := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(userID, txHash, "non-existent-session", &commonv1.L3Proof{MtlsCertFingerprint: unknownFingerprint})
	require.Error(t, err)
	require.False(t, ok)
	require.Contains(t, err.Error(), "CLI session not found")
}

func TestCertFingerprint(t *testing.T) {
	t.Parallel()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-cert",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certBytes)
	require.NoError(t, err)

	fingerprint := CertFingerprint(cert)
	require.NotEmpty(t, fingerprint)
	require.Len(t, fingerprint, 64) // SHA-256 hex encoded
}

func TestExtractCLISessionFromCert(t *testing.T) {
	t.Parallel()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-cli-cert",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
		URIs:      []*url.URL{},
	}

	// Add a valid CLI SPIFFE URI
	cliURI, err := url.Parse("spiffe://g8e.local/cli/user-123/cli-session-456")
	require.NoError(t, err)
	template.URIs = append(template.URIs, cliURI)

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certBytes)
	require.NoError(t, err)

	sessionID, err := ExtractCLISessionFromCert(cert)
	require.NoError(t, err)
	require.Equal(t, "cli-session-456", sessionID)
}

func TestCLIL3Notary_VerifyL3Proof_RejectsRevokedCertificate(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)

	sm, _ := NewSecretManager(db.db, t.TempDir(), logger)
	pki := newPKIAuthority(dbDir, filepath.Join(dbDir, "pki"), db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	notary := NewCLIL3Notary(db, pki, logger, userSvc, sessionSvc)

	userID := "user-123"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// Create a CLI session with a revoked certificate serial
	cliSessionID := "cli-session-revoked"
	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: "operator-session-123",
		CertFingerprint:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CertSerial:        "1234567890abcdef",
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(24 * time.Hour),
		AbsoluteExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		IdleExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "device_link",
	}
	cliSessionBytes, _ := json.Marshal(cliSession)
	require.NoError(t, db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	// Revoke the certificate
	err = pki.RevokeCertificate("1234567890abcdef", "test revocation")
	require.NoError(t, err)

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := notary.VerifyL3Proof(userID, txHash, cliSessionID, &commonv1.L3Proof{MtlsCertFingerprint: validFingerprint})
	require.NoError(t, err)
	require.False(t, ok)
}

func TestExtractUserIDFromCert(t *testing.T) {
	t.Parallel()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-cli-cert",
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(24 * time.Hour),
		URIs:      []*url.URL{},
	}

	// Add a valid CLI SPIFFE URI
	cliURI, err := url.Parse("spiffe://g8e.local/cli/user-123/cli-session-456")
	require.NoError(t, err)
	template.URIs = append(template.URIs, cliURI)

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certBytes)
	require.NoError(t, err)

	userID, err := ExtractUserIDFromCert(cert)
	require.NoError(t, err)
	require.Equal(t, "user-123", userID)
}

func TestCompositeL3Verifier_DelegatesToCLI(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	dbDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	userSvc := NewUserService(db, logger)
	sessionSvc := NewSessionService(db, logger)
	cliL3 := NewCLIL3Notary(db, nil, logger, userSvc, sessionSvc)
	composite := NewCompositeL3Verifier(nil, cliL3, logger)

	// Create an active user
	userID := "active-user"
	user := &models.User{
		ID:     userID,
		Status: constants.UserStatusActive,
	}
	userBytes, _ := json.Marshal(user)
	require.NoError(t, db.DocSet(marshaler.CollectionName(constants.CollectionUsers), userID, userBytes))

	// Create a CLI session with a known fingerprint
	cliSessionID := "cli-session-456"
	cliSession := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: "operator-session-456",
		CertFingerprint:   "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CertSerial:        "1234567890abcdef",
		CreatedAt:         time.Now().UTC(),
		ExpiresAt:         time.Now().UTC().Add(24 * time.Hour),
		AbsoluteExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		IdleExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		SessionType:       string(constants.SessionTypeCLI),
		IsActive:          true,
		LoginMethod:       "device_link",
	}
	cliSessionBytes, _ := json.Marshal(cliSession)
	require.NoError(t, db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, cliSessionBytes))

	validFingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	ok, err := composite.VerifyL3Proof(userID, txHash, cliSessionID, &commonv1.L3Proof{MtlsCertFingerprint: validFingerprint})
	require.NoError(t, err)
	require.True(t, ok)
}

func TestCompositeL3Verifier_DelegatesToPasskey(t *testing.T) {
	t.Parallel()
	logger := testutil.NewTestLogger()
	passkeyL3, user := newPasskeyServiceForTest(t)
	composite := NewCompositeL3Verifier(passkeyL3, nil, logger)

	// Use the user already created by newPasskeyServiceForTest
	userID := user.ID

	// Add a dummy credential
	credID := []byte("real-credential-id")
	require.NoError(t, passkeyL3.addCredential(userID, models.PasskeyCredential{
		ID:        credID,
		PublicKey: []byte("fake-pubkey"),
	}))

	// Create a WebAuthn proof (no mtls_cert_fingerprint)
	txHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	clientData := `{"type":"webauthn.get","challenge":"` + base64.RawURLEncoding.EncodeToString([]byte(txHash)) + `","origin":"localhost"}`
	ok, err := composite.VerifyL3Proof(userID, txHash, "", &commonv1.L3Proof{
		CredentialId:      base64.RawURLEncoding.EncodeToString(credID),
		ClientDataJson:    base64.RawURLEncoding.EncodeToString([]byte(clientData)),
		AuthenticatorData: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 37))),
		Signature:         base64.RawURLEncoding.EncodeToString([]byte("signature")),
	})
	// This will fail signature verification but proves delegation to passkey verifier
	require.Error(t, err)
	require.False(t, ok)
	require.Contains(t, err.Error(), "failed to parse credential assertion")
}

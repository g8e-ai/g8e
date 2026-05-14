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

package listen

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/components/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/components/g8eo/internal/models"
)

const (
	passkeyChallengeTTL = 5 * time.Minute
	webSessionTTL       = 24 * time.Hour
	challengeBytes      = 32
)

// PasskeyService handles L3 proof brokerage for passkey/WebAuthn operations.
// This moves the L3 authorization from client into g8eo as the sovereign authority.
//
// NOTE: This is a simplified ED25519-based implementation. Full WebAuthn/FIDO2
// support will be added in a future iteration using the go-webauthn library.
type PasskeyService struct {
	db       *ListenDBService
	logger   *slog.Logger
	rpID     string
	rpName   string
	webauthn *webauthn.WebAuthn
}

// PasskeyConfig holds configuration for passkey operations.
type PasskeyConfig struct {
	RpID   string
	RpName string
}

// NewPasskeyService creates a new PasskeyService with the given configuration.
func NewPasskeyService(db *ListenDBService, logger *slog.Logger, cfg *PasskeyConfig) (*PasskeyService, error) {
	rpName := cfg.RpName
	if rpName == "" {
		rpName = "g8e"
	}

	w, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RpID,
		RPDisplayName: rpName,
		RPOrigins:     []string{cfg.RpID},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize webauthn: %w", err)
	}

	return &PasskeyService{
		db:       db,
		logger:   logger,
		rpID:     cfg.RpID,
		rpName:   rpName,
		webauthn: w,
	}, nil
}

// ChallengeData stores a pending challenge for registration or authentication.
type ChallengeData struct {
	Challenge string `json:"challenge"`
	CreatedAt int64  `json:"created_at"`
	Purpose   string `json:"purpose"` // "register" or "auth"
}

// GenerateRegistrationChallenge creates a registration challenge for a user.
func (s *PasskeyService) GenerateRegistrationChallenge(userID, email, userName string) (*protocol.CredentialCreation, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	options, session, err := s.webauthn.BeginRegistration(user)
	if err != nil {
		return nil, fmt.Errorf("failed to begin registration: %w", err)
	}

	// Store session data
	if err := s.storeWebAuthnSession(userID, session); err != nil {
		return nil, err
	}

	return options, nil
}

// AttestationResponse is the client response for registration verification.
type AttestationResponse struct {
	ID                string   `json:"id"`
	RawID             string   `json:"rawId"`
	ClientDataJSON    string   `json:"clientDataJSON"`
	AttestationObject string   `json:"attestationObject"`
	Transports        []string `json:"transports,omitempty"`
}

// VerifyRegistration verifies a registration response.
func (s *PasskeyService) VerifyRegistration(userID string, r *http.Request) (*models.PasskeyCredential, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	session, err := s.getWebAuthnSession(userID)
	if err != nil {
		return nil, err
	}

	credential, err := s.webauthn.FinishRegistration(user, *session, r)
	if err != nil {
		return nil, fmt.Errorf("failed to finish registration: %w", err)
	}

	newCred := models.PasskeyCredential{
		ID:              credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		Transport:       credential.Transport,
		Authenticator: models.Authenticator{
			AAGUID:       credential.Authenticator.AAGUID,
			SignCount:    credential.Authenticator.SignCount,
			CloneWarning: credential.Authenticator.CloneWarning,
		},
		CreatedAtUnixMs: time.Now().UnixMilli(),
	}

	if err := s.addCredential(userID, newCred); err != nil {
		return nil, err
	}

	return &newCred, nil
}

// GenerateAuthenticationChallenge creates an authentication challenge.
func (s *PasskeyService) GenerateAuthenticationChallenge(userID string) (*protocol.CredentialAssertion, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	options, session, err := s.webauthn.BeginLogin(user)
	if err != nil {
		return nil, fmt.Errorf("failed to begin login: %w", err)
	}

	if err := s.storeWebAuthnSession(userID, session); err != nil {
		return nil, err
	}

	return options, nil
}

// AssertionResponse is the client response for authentication verification.
type AssertionResponse struct {
	ID                string `json:"id"`
	RawID             string `json:"rawId"`
	ClientDataJSON    string `json:"clientDataJSON"`
	AuthenticatorData string `json:"authenticatorData"`
	Signature         string `json:"signature"`
	UserHandle        string `json:"userHandle,omitempty"`
}

// VerifyAuthentication verifies an authentication assertion.
func (s *PasskeyService) VerifyAuthentication(userID string, r *http.Request) (*models.PasskeyCredential, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	session, err := s.getWebAuthnSession(userID)
	if err != nil {
		return nil, err
	}

	credential, err := s.webauthn.FinishLogin(user, *session, r)
	if err != nil {
		return nil, fmt.Errorf("failed to finish login: %w", err)
	}

	// Update credential counter and last used
	var storedCred *models.PasskeyCredential
	for i := range user.PasskeyCredentials {
		if string(user.PasskeyCredentials[i].ID) == string(credential.ID) {
			user.PasskeyCredentials[i].Authenticator.SignCount = credential.Authenticator.SignCount
			user.PasskeyCredentials[i].LastUsedAtUnixMs = time.Now().UnixMilli()
			storedCred = &user.PasskeyCredentials[i]
			break
		}
	}

	if err := s.updateUser(userID, user); err != nil {
		return nil, err
	}

	return storedCred, nil
}

// ListCredentials returns all passkey credentials for a user.
func (s *PasskeyService) ListCredentials(userID string) ([]models.PasskeyCredential, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	return user.PasskeyCredentials, nil
}

// RevokeCredential removes a passkey credential from a user.
func (s *PasskeyService) RevokeCredential(userID, credentialID string) (found bool, remaining int, err error) {
	user, err := s.getUser(userID)
	if err != nil {
		return false, 0, err
	}
	if user == nil {
		return false, 0, nil
	}

	var newCreds []models.PasskeyCredential
	found = false
	for _, c := range user.PasskeyCredentials {
		if base64.RawURLEncoding.EncodeToString(c.ID) != credentialID {
			newCreds = append(newCreds, c)
		} else {
			found = true
		}
	}

	if !found {
		return false, len(user.PasskeyCredentials), nil
	}

	if err := s.setCredentials(userID, newCreds); err != nil {
		s.logger.Error("Failed to revoke credential", "error", err, "userID", userID)
		return false, 0, err
	}

	s.logger.Info("Credential revoked", "userID", userID, "credentialID", credentialID[:12])
	return true, len(newCreds), nil
}

// CreateSession creates a web session after successful authentication.
func (s *PasskeyService) CreateSession(userID string) (*models.WebSession, error) {
	sessionID := uuid.New().String()
	now := time.Now()

	session := &models.WebSession{
		ID:              sessionID,
		UserID:          userID,
		CreatedAtUnixMs: now.UnixMilli(),
		ExpiresAtUnixMs: now.Add(webSessionTTL).UnixMilli(),
	}

	data, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}
	if err := s.db.DocSet(string(constants.CollectionWebSessions), sessionID, data); err != nil {
		s.logger.Error("Failed to create session", "error", err, "userID", userID)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	s.logger.Info("Session created", "userID", userID, "sessionID", sessionID[:8])
	return session, nil
}

// VerifyL3Proof verifies a human signature against a registered passkey.
func (s *PasskeyService) VerifyL3Proof(userID, messageID, signatureHex, pubKeyHex string) (bool, error) {
	user, err := s.getUser(userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, fmt.Errorf("user not found")
	}

	// 1. Verify the public key is registered for this user
	found := false
	for _, cred := range user.PasskeyCredentials {
		// In a real WebAuthn impl, we'd compare ID or the extracted key
		if hex.EncodeToString(cred.PublicKey) == pubKeyHex || hex.EncodeToString(cred.ID) == pubKeyHex {
			found = true
			break
		}
	}

	if !found {
		return false, fmt.Errorf("public key not registered for user")
	}

	// 2. Verify the signature
	// We'll use the same logic as VerifyL3Signature for now but with the user check
	return s.VerifyL3Signature(userID, messageID, signatureHex, pubKeyHex)
}

// VerifyL3Signature verifies an ED25519 signature for L3 authorization.
// This is used when L3 proof is provided as a direct signature rather than WebAuthn.
func (s *PasskeyService) VerifyL3Signature(userID, challenge, signatureHex, pubKeyHex string) (bool, error) {
	// Decode signature
	sigBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false, fmt.Errorf("invalid signature encoding: %w", err)
	}

	// Decode public key
	pubKeyBytes, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false, fmt.Errorf("invalid public key encoding: %w", err)
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)

	// Verify signature
	message := []byte(challenge)
	if !ed25519.Verify(pubKey, message, sigBytes) {
		return false, fmt.Errorf("signature verification failed")
	}

	return true, nil
}

func (s *PasskeyService) getUser(userID string) (*models.User, error) {
	doc, err := s.db.DocGet(string(constants.CollectionUsers), userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, nil
	}

	// Re-serialize the document data map to JSON for unmarshaling into struct
	data, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal doc data: %w", err)
	}

	var user models.User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}
	user.ID = doc.ID
	return &user, nil
}

func (s *PasskeyService) addCredential(userID string, cred models.PasskeyCredential) error {
	user, err := s.getUser(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	user.PasskeyCredentials = append(user.PasskeyCredentials, cred)

	return s.updateUser(userID, user)
}

func (s *PasskeyService) updateCredential(userID string, cred models.PasskeyCredential) error {
	user, err := s.getUser(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	for i := range user.PasskeyCredentials {
		if string(user.PasskeyCredentials[i].ID) == string(cred.ID) {
			user.PasskeyCredentials[i] = cred
			break
		}
	}

	return s.updateUser(userID, user)
}

func (s *PasskeyService) setCredentials(userID string, creds []models.PasskeyCredential) error {
	user, err := s.getUser(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	user.PasskeyCredentials = creds
	return s.updateUser(userID, user)
}

func (s *PasskeyService) updateUser(userID string, user *models.User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}
	_, err = s.db.DocUpdate(string(constants.CollectionUsers), userID, data)
	return err
}

func (s *PasskeyService) storeWebAuthnSession(userID string, session *webauthn.SessionData) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.db.DocSet(string(constants.CollectionPasskeyChallenges), userID, data)
}

func (s *PasskeyService) getWebAuthnSession(userID string) (*webauthn.SessionData, error) {
	doc, err := s.db.DocGet(string(constants.CollectionPasskeyChallenges), userID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, fmt.Errorf("webauthn session not found")
	}

	var session webauthn.SessionData
	b, err := json.Marshal(doc.Data)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

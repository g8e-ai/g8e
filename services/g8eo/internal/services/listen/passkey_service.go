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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"github.com/g8e-ai/g8e/services/g8eo/internal/constants"
	"github.com/g8e-ai/g8e/services/g8eo/internal/models"
	commonv1 "github.com/g8e-ai/g8e/services/g8eo/internal/protocol/proto/commonv1"
)

const (
	passkeyChallengeTTL = 5 * time.Minute
	webSessionTTL       = 24 * time.Hour
	challengeBytes      = 32
)

// PasskeyService handles L3 proof brokerage for passkey/WebAuthn operations.
// This moves the L3 authorization from client into g8eo as the sovereign authority.
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
	webSessionID := uuid.New().String()
	now := time.Now()

	session := &models.WebSession{
		ID:              webSessionID,
		UserID:          userID,
		CreatedAtUnixMs: now.UnixMilli(),
		ExpiresAtUnixMs: now.Add(webSessionTTL).UnixMilli(),
	}

	data, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}
	if err := s.db.DocSet(string(constants.CollectionWebSessions), webSessionID, data); err != nil {
		s.logger.Error("Failed to create session", "error", err, "userID", userID)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	s.logger.Info("Session created", "userID", userID, "webSessionID", webSessionID[:8])
	return session, nil
}

// VerifyL3Proof verifies a WebAuthn assertion against a registered passkey.
// The challenge is the transaction_hash.
func (s *PasskeyService) VerifyL3Proof(userID, transactionHash string, proof *commonv1.L3Proof) (bool, error) {
	if userID == "" {
		return false, fmt.Errorf("user_id is required for L3 WebAuthn verification")
	}
	if transactionHash == "" {
		return false, fmt.Errorf("transaction_hash is required for L3 WebAuthn verification")
	}
	if proof == nil {
		return false, fmt.Errorf("L3 WebAuthn proof is required")
	}
	if proof.CredentialId == "" {
		return false, fmt.Errorf("L3 WebAuthn credential_id is required")
	}
	if proof.ClientDataJson == "" {
		return false, fmt.Errorf("L3 WebAuthn client_data_json is required")
	}
	if proof.AuthenticatorData == "" {
		return false, fmt.Errorf("L3 WebAuthn authenticator_data is required")
	}
	if proof.Signature == "" {
		return false, fmt.Errorf("L3 WebAuthn signature is required")
	}

	user, err := s.getUser(userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, fmt.Errorf("user not found")
	}
	if len(user.PasskeyCredentials) == 0 {
		return false, fmt.Errorf("user has no registered passkey credentials")
	}

	allowedCredentialIDs := make([][]byte, 0, len(user.PasskeyCredentials))
	for _, credential := range user.PasskeyCredentials {
		allowedCredentialIDs = append(allowedCredentialIDs, credential.ID)
	}

	session := webauthn.SessionData{
		Challenge:            base64.RawURLEncoding.EncodeToString([]byte(transactionHash)),
		RelyingPartyID:       s.rpID,
		UserID:               []byte(userID),
		AllowedCredentialIDs: allowedCredentialIDs,
		Expires:              time.Now().Add(passkeyChallengeTTL),
	}

	assertionResponse := map[string]interface{}{
		"id":    proof.CredentialId,
		"rawId": proof.CredentialId,
		"type":  "public-key",
		"response": map[string]string{
			"clientDataJSON":    proof.ClientDataJson,
			"authenticatorData": proof.AuthenticatorData,
			"signature":         proof.Signature,
		},
	}

	body, err := json.Marshal(assertionResponse)
	if err != nil {
		return false, fmt.Errorf("failed to marshal assertion response: %w", err)
	}

	req, _ := http.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	parsedResponse, err := protocol.ParseCredentialRequestResponse(req)
	if err != nil {
		return false, fmt.Errorf("failed to parse credential assertion: %w", err)
	}

	_, err = s.webauthn.ValidateLogin(user, session, parsedResponse)
	if err != nil {
		return false, fmt.Errorf("L3 WebAuthn verification failed: %w", err)
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

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
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

// SetupService manages first-run setup and passkey RP configuration.
// This replaces g8ed's SetupService as the substrate authority.
type SetupService struct {
	db        *ListenDBService
	userSvc   *UserService
	logger    *slog.Logger
	setupLock sync.Mutex
}

// NewSetupService creates a new SetupService.
func NewSetupService(db *ListenDBService, userSvc *UserService, logger *slog.Logger) *SetupService {
	return &SetupService{
		db:      db,
		userSvc: userSvc,
		logger:  logger,
	}
}

// IsFirstRun checks if setup has been completed.
func (s *SetupService) IsFirstRun() (bool, error) {
	doc, err := s.db.DocGet(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings))
	if err != nil {
		return false, err
	}
	if doc == nil {
		return true, nil
	}

	setupComplete, ok := doc.Data["setup_complete"]
	if !ok {
		return true, nil
	}

	var complete bool
	if err := json.Unmarshal(setupComplete, &complete); err != nil {
		return true, nil
	}

	return !complete, nil
}

// CompleteSetup marks setup as complete.
func (s *SetupService) CompleteSetup() error {
	update := map[string]interface{}{
		"setup_complete": true,
	}
	updateBytes, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("failed to marshal setup complete: %w", err)
	}

	_, err = s.db.DocUpdate(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings), updateBytes)
	if err != nil {
		return fmt.Errorf("failed to complete setup: %w", err)
	}

	s.logger.Info("[SETUP-SERVICE] Setup marked complete")
	return nil
}

// PerformFirstRunSetup performs the first-run setup sequence with locking.
// This includes deriving passkey fields, creating the admin user, and updating settings.
func (s *SetupService) PerformFirstRunSetup(email, name string, r *http.Request) (*models.User, error) {
	s.setupLock.Lock()
	defer s.setupLock.Unlock()

	s.logger.Info("[SETUP-SERVICE] Performing first-run setup", "email", email)

	// Derive passkey fields from request
	rpID, origin := s.DerivePasskeyFields(r)

	// Save passkey configuration to settings
	settingsUpdate := map[string]interface{}{
		"passkey_rp_id":  rpID,
		"passkey_origin": origin,
		"setup_complete": false,
		"updated_at":     time.Now().UTC().UnixMilli(),
	}
	updateBytes, err := json.Marshal(settingsUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	_, err = s.db.DocUpdate(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings), updateBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to save passkey settings: %w", err)
	}

	// Create admin user (SUPERADMIN role)
	user, err := s.userSvc.CreateUser(email, name, []string{"SUPERADMIN"})
	if err != nil {
		return nil, fmt.Errorf("failed to create admin user: %w", err)
	}

	s.logger.Info("[SETUP-SERVICE] First-run setup completed", "user_id", user.ID, "email", email)
	return user, nil
}

// DerivePasskeyFields derives the RP ID and origin from the HTTP request.
// Uses X-Forwarded-Host and X-Forwarded-Proto if available (reverse proxy).
func (s *SetupService) DerivePasskeyFields(r *http.Request) (rpID, origin string) {
	if r == nil {
		return "localhost", "https://localhost"
	}

	// Check for reverse proxy headers
	xForwardedHost := r.Header.Get("X-Forwarded-Host")
	xForwardedProto := r.Header.Get("X-Forwarded-Proto")

	host := xForwardedHost
	if host == "" {
		host = r.Host
	}
	if host == "" {
		host = "localhost"
	}

	proto := xForwardedProto
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	// Remove port from host for RP ID
	rpID = host
	if h, _, err := net.SplitHostPort(host); err == nil {
		rpID = h
	}

	origin = fmt.Sprintf("%s://%s", proto, host)

	return rpID, origin
}

// GetPasskeyConfig retrieves the current passkey configuration from settings.
func (s *SetupService) GetPasskeyConfig() (rpID, origin string, err error) {
	doc, err := s.db.DocGet(string(constants.CollectionSettings), string(constants.DocIDPlatformSettings))
	if err != nil {
		return "", "", err
	}
	if doc == nil {
		return "localhost", "https://localhost", nil
	}

	rpIDVal, ok := doc.Data["passkey_rp_id"]
	if !ok {
		return "localhost", "https://localhost", nil
	}

	originVal, ok := doc.Data["passkey_origin"]
	if !ok {
		return "localhost", "https://localhost", nil
	}

	if err := json.Unmarshal(rpIDVal, &rpID); err != nil {
		return "localhost", "https://localhost", nil
	}

	if err := json.Unmarshal(originVal, &origin); err != nil {
		return "localhost", "https://localhost", nil
	}

	return rpID, origin, nil
}

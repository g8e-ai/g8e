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
	"errors"
	"fmt"
	"time"
)

// Invitation represents an owner-created invitation for a user to join an organization.
// This is used for JIT provisioning and onboarding.
type Invitation struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Email          string    `json:"email"` // Or sub if we use IdP sub
	Sub            string    `json:"sub"`   // The JWT sub claim this invitation is for
	Roles          []string  `json:"roles"`
	CreatedBy      string    `json:"created_by"` // User ID of the owner who created this
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	ConsumedAt     time.Time `json:"consumed_at,omitempty"`
	IsConsumed     bool      `json:"is_consumed"`
}

// IsValid checks if the invitation is active and not expired.
func (i *Invitation) IsValid() error {
	if i == nil {
		return errors.New("models: invitation is nil")
	}
	if i.IsConsumed {
		return fmt.Errorf("models: invitation %s is already consumed", i.ID)
	}
	if time.Now().UTC().After(i.ExpiresAt) {
		return fmt.Errorf("models: invitation %s expired at %s", i.ID, i.ExpiresAt.UTC().Format(time.RFC3339))
	}
	return nil
}

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
	"time"
)

// DeviceLinkData represents a device link for operator enrollment.
// Authority: components/g8ed/models/auth_models.js::DeviceLinkData
type DeviceLinkData struct {
	Token          string            `json:"token"`
	UserID         string            `json:"user_id"`
	OrganizationID string            `json:"organization_id,omitempty"`
	OperatorID     string            `json:"operator_id,omitempty"`
	WebSessionID   string            `json:"web_session_id,omitempty"`
	Name           string            `json:"name,omitempty"`
	MaxUses        int               `json:"max_uses"`
	Uses           int               `json:"uses"`
	Status         string            `json:"status"`
	CreatedAt      time.Time         `json:"created_at"`
	ExpiresAt      time.Time         `json:"expires_at"`
	UsedAt         *time.Time        `json:"used_at,omitempty"`
	RevokedAt      *time.Time        `json:"revoked_at,omitempty"`
	DeviceInfo     *DeviceLinkInfo   `json:"device_info,omitempty"`
	Claims         []DeviceLinkClaim `json:"claims,omitempty"`
}

// DeviceLinkInfo captures the device details of the first user of a link.
type DeviceLinkInfo struct {
	SystemFingerprint string `json:"system_fingerprint"`
	Hostname          string `json:"hostname"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Username          string `json:"username"`
}

// DeviceLinkClaim records which operator ID claimed a slot via a multi-use link.
type DeviceLinkClaim struct {
	SystemFingerprint string    `json:"system_fingerprint"`
	Hostname          string    `json:"hostname"`
	OperatorID        string    `json:"operator_id"`
	ClaimedAt         time.Time `json:"claimed_at"`
}

// OperatorRegistrationRequest is the inbound body for /auth/link/:token/register
type OperatorRegistrationRequest struct {
	SystemFingerprint string `json:"system_fingerprint"`
	Hostname          string `json:"hostname"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	Username          string `json:"username"`
	IPAddress         string `json:"ip_address,omitempty"`
}

// OperatorRegistrationResponse is the response for /auth/link/:token/register
type OperatorRegistrationResponse struct {
	Success           bool            `json:"success"`
	OperatorSessionID string          `json:"operator_session_id,omitempty"`
	OperatorID        string          `json:"operator_id,omitempty"`
	APIKey            string          `json:"api_key,omitempty"`
	OperatorCert      string          `json:"operator_cert,omitempty"`
	OperatorCertKey   string          `json:"operator_cert_key,omitempty"`
	Session           *SessionSummary `json:"session,omitempty"`
	Config            json.RawMessage `json:"config,omitempty"`
	Error             string          `json:"error,omitempty"`
}

// SessionSummary provides a brief overview of the created operator session.
type SessionSummary struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// OperatorDocumentGo is a Go representation of the canonical OperatorDocument.
// Authority: shared/models/operator_document.json
type OperatorDocumentGo struct {
	ID                     string                 `json:"id"`
	UserID                 string                 `json:"user_id"`
	OrganizationID         string                 `json:"organization_id,omitempty"`
	Component              string                 `json:"component"`
	Name                   string                 `json:"name,omitempty"`
	Status                 string                 `json:"status"`
	OperatorSessionID      string                 `json:"operator_session_id,omitempty"`
	BoundWebSessionID      string                 `json:"bound_web_session_id,omitempty"`
	APIKey                 string                 `json:"api_key,omitempty"`
	OperatorAPIKey         string                 `json:"operator_api_key,omitempty"`
	OperatorCert           string                 `json:"operator_cert,omitempty"`
	OperatorCertSerial     string                 `json:"operator_cert_serial,omitempty"`
	SlotNumber             int                    `json:"slot_number,omitempty"`
	IsSlot                 bool                   `json:"is_slot"`
	Claimed                bool                   `json:"claimed"`
	OperatorType           string                 `json:"operator_type"`
	CloudSubtype           string                 `json:"cloud_subtype,omitempty"`
	SystemFingerprint      string                 `json:"system_fingerprint,omitempty"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	StartedAt              *time.Time             `json:"started_at,omitempty"`
	ClaimedAt              *time.Time             `json:"claimed_at,omitempty"`
	LatestHeartbeat        json.RawMessage        `json:"latest_heartbeat_snapshot,omitempty"`
	RuntimeConfig          *RuntimeConfig         `json:"runtime_config,omitempty"`
	ConsumedByOperatorID   string                 `json:"consumed_by_operator_id,omitempty"`
}

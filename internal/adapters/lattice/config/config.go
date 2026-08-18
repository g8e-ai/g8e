// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package latticeconfig

import (
	"fmt"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
)

// LatticeConfig holds configuration for the Lattice gRPC adapter.
type LatticeConfig struct {
	Enabled        bool
	Endpoint       string // LATTICE_ENDPOINT
	ClientID       string // LATTICE_CLIENT_ID
	ClientSecret   string // LATTICE_CLIENT_SECRET
	SandboxesToken string // SANDBOXES_TOKEN (sandbox only)

	Entity       EntityConfig
	PostureFloor string   // refuse to start below this posture
	TaskCatalog  []string // taskSpecificationUrl values
}

// EntityConfig holds entity model parameters for presence publishing.
type EntityConfig struct {
	Name         string
	PlatformType string
	Latitude     float64
	Longitude    float64
}

// Validate checks that all required fields are present and valid.
func (c *LatticeConfig) Validate() error {
	if c == nil {
		return constants.ErrLatticeConfigMissing
	}
	if c.Endpoint == "" {
		return constants.ErrLatticeEndpointRequired
	}
	if c.ClientID == "" {
		return constants.ErrLatticeClientIDRequired
	}
	if c.ClientSecret == "" {
		return constants.ErrLatticeClientSecretRequired
	}
	if c.Entity.Name == "" {
		return fmt.Errorf("%w: entity.name required", constants.ErrLatticeConfigMissing)
	}
	if c.PostureFloor == "" {
		c.PostureFloor = "consensus"
	}
	return nil
}

// ValidateHeartbeatInterval checks that the heartbeat interval is compatible
// with Lattice's 5-minute soft-state entity expiry contract. The interval must
// be > 0 (scheduler enabled) and < 4 minutes to guarantee republish within
// the 5-minute window with margin for retry backoff.
func ValidateHeartbeatInterval(interval time.Duration) error {
	if interval <= 0 || interval >= 4*time.Minute {
		return constants.ErrLatticeHeartbeatIntervalInvalid
	}
	return nil
}

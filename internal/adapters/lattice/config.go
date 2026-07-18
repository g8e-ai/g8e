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

package lattice

import (
	"fmt"
	"time"
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
		return ErrLatticeConfigMissing
	}
	if c.Endpoint == "" {
		return ErrLatticeEndpointRequired
	}
	if c.ClientID == "" {
		return ErrLatticeClientIDRequired
	}
	if c.ClientSecret == "" {
		return ErrLatticeClientSecretRequired
	}
	if c.Entity.Name == "" {
		return fmt.Errorf("%w: entity.name required", ErrLatticeConfigMissing)
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
		return ErrLatticeHeartbeatIntervalInvalid
	}
	return nil
}

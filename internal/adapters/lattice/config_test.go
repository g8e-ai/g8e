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
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatticeConfigValidate_ReturnsErrLatticeConfigMissingWhenNil(t *testing.T) {
	t.Parallel()

	var cfg *LatticeConfig
	err := cfg.Validate()
	assert.ErrorIs(t, err, constants.ErrLatticeConfigMissing)
}

func TestLatticeConfigValidate_ReturnsErrLatticeEndpointRequiredWhenEndpointEmpty(t *testing.T) {
	t.Parallel()

	cfg := &LatticeConfig{
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		Entity:       EntityConfig{Name: "g8e-operator"},
	}
	err := cfg.Validate()
	assert.ErrorIs(t, err, constants.ErrLatticeEndpointRequired)
}

func TestLatticeConfigValidate_ReturnsErrLatticeClientIDRequiredWhenClientIDEmpty(t *testing.T) {
	t.Parallel()

	cfg := &LatticeConfig{
		Endpoint:     "https://lattice.example.com/api/v1/oauth/token",
		ClientSecret: "test-secret",
		Entity:       EntityConfig{Name: "g8e-operator"},
	}
	err := cfg.Validate()
	assert.ErrorIs(t, err, constants.ErrLatticeClientIDRequired)
}

func TestLatticeConfigValidate_ReturnsErrLatticeClientSecretRequiredWhenClientSecretEmpty(t *testing.T) {
	t.Parallel()

	cfg := &LatticeConfig{
		Endpoint: "https://lattice.example.com/api/v1/oauth/token",
		ClientID: "test-id",
		Entity:   EntityConfig{Name: "g8e-operator"},
	}
	err := cfg.Validate()
	assert.ErrorIs(t, err, constants.ErrLatticeClientSecretRequired)
}

func TestLatticeConfigValidate_ReturnsErrorWhenEntityNameEmpty(t *testing.T) {
	t.Parallel()

	cfg := &LatticeConfig{
		Endpoint:     "https://lattice.example.com/api/v1/oauth/token",
		ClientID:     "test-id",
		ClientSecret: "test-secret",
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrLatticeConfigMissing)
}

func TestLatticeConfigValidate_DefaultsPostureFloorToConsensusWhenEmpty(t *testing.T) {
	t.Parallel()

	cfg := &LatticeConfig{
		Endpoint:     "https://lattice.example.com/api/v1/oauth/token",
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		Entity:       EntityConfig{Name: "g8e-operator"},
	}
	err := cfg.Validate()
	require.NoError(t, err)
	assert.Equal(t, "consensus", cfg.PostureFloor)
}

func TestLatticeConfigValidate_PassesWithAllRequiredFields(t *testing.T) {
	t.Parallel()

	cfg := &LatticeConfig{
		Endpoint:     "https://lattice.example.com/api/v1/oauth/token",
		ClientID:     "test-id",
		ClientSecret: "test-secret",
		Entity:       EntityConfig{Name: "g8e-operator"},
		PostureFloor: "notary",
	}
	err := cfg.Validate()
	require.NoError(t, err)
	assert.Equal(t, "notary", cfg.PostureFloor)
}

func TestValidateHeartbeatInterval_RejectsZero(t *testing.T) {
	t.Parallel()

	err := ValidateHeartbeatInterval(0)
	assert.ErrorIs(t, err, constants.ErrLatticeHeartbeatIntervalInvalid)
}

func TestValidateHeartbeatInterval_RejectsNegative(t *testing.T) {
	t.Parallel()

	err := ValidateHeartbeatInterval(-1 * time.Second)
	assert.ErrorIs(t, err, constants.ErrLatticeHeartbeatIntervalInvalid)
}

func TestValidateHeartbeatInterval_RejectsFourMinutes(t *testing.T) {
	t.Parallel()

	err := ValidateHeartbeatInterval(4 * time.Minute)
	assert.ErrorIs(t, err, constants.ErrLatticeHeartbeatIntervalInvalid)
}

func TestValidateHeartbeatInterval_RejectsAboveFourMinutes(t *testing.T) {
	t.Parallel()

	err := ValidateHeartbeatInterval(5 * time.Minute)
	assert.ErrorIs(t, err, constants.ErrLatticeHeartbeatIntervalInvalid)
}

func TestValidateHeartbeatInterval_AcceptsThirtySeconds(t *testing.T) {
	t.Parallel()

	err := ValidateHeartbeatInterval(30 * time.Second)
	assert.NoError(t, err)
}

func TestValidateHeartbeatInterval_AcceptsOneMinute(t *testing.T) {
	t.Parallel()

	err := ValidateHeartbeatInterval(1 * time.Minute)
	assert.NoError(t, err)
}

func TestValidateHeartbeatInterval_AcceptsThreeMinutesFiftyNineSeconds(t *testing.T) {
	t.Parallel()

	err := ValidateHeartbeatInterval(3*time.Minute + 59*time.Second)
	assert.NoError(t, err)
}

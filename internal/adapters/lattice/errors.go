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

import "errors"

// Lattice adapter errors. These are adapter-local — no Lattice-specific
// error constants exist in internal/constants/.
var (
	ErrLatticeConfigMissing            = errors.New("lattice config is required")
	ErrLatticeEndpointRequired         = errors.New("lattice endpoint required")
	ErrLatticeClientIDRequired         = errors.New("lattice client_id required")
	ErrLatticeClientSecretRequired     = errors.New("lattice client_secret required")
	ErrLatticeTokenAcquireFailed       = errors.New("lattice: failed to acquire token")
	ErrLatticeTokenRefreshFailed       = errors.New("lattice: failed to refresh token")
	ErrLatticeTokenExpired             = errors.New("lattice: token expired and refresh failed")
	ErrLatticeEntityIDRequired         = errors.New("lattice: entity_id required")
	ErrLatticeEntityIDPersistFailed    = errors.New("lattice: failed to persist entity_id")
	ErrLatticeEntityIDReadFailed       = errors.New("lattice: failed to read entity_id")
	ErrLatticeDialFailed               = errors.New("lattice: gRPC dial failed")
	ErrLatticePresencePublishFailed    = errors.New("lattice: failed to publish entity presence")
	ErrLatticeStreamClosed             = errors.New("lattice: task stream closed")
	ErrLatticeStreamReconnectFailed    = errors.New("lattice: task stream reconnect failed")
	ErrLatticeStatusReportFailed       = errors.New("lattice: failed to report task status")
	ErrLatticeHeartbeatIntervalInvalid = errors.New("lattice: heartbeat interval must be > 0 and < 4 minutes")
	ErrLatticePostureFloorViolated     = errors.New("lattice: active posture below configured floor")
	ErrLatticeNotInitialized           = errors.New("lattice: adapter not initialized")
	ErrLatticeAlreadyRunning           = errors.New("lattice: adapter already running")
)

// LatticeEntityIDFilename is the persisted entity ID filename for the
// Lattice adapter. Defined locally — no Lattice-specific path constants
// exist in internal/constants/paths.go.
const LatticeEntityIDFilename = "lattice_entity_id"

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

package governance

import "fmt"

// GovernancePosture defines which layers of the verification pipeline are
// enforced as fail-closed gates versus audited.
//
// Three postures are defined, each adding a stricter layer of enforcement:
//
//	doctrine  — L1 enforced, L2/L3 audited (minimum)
//	consensus — L1+L2 enforced, L3 audited
//	notary    — L1+L2+L3 strictly enforced (maximum)
//
// Adding a new posture only requires implementing this interface and extending
// the factory functions below; no changes to L4Warden or L5Actuator are needed.
//
//go:generate mockery --name GovernancePosture --output ./mocks --dir .
type GovernancePosture interface {
	// Name returns the canonical posture name ("doctrine", "consensus", "notary").
	Name() string

	// Description returns a human-readable summary of what is enforced.
	Description() string

	// RequiresL2Signature returns true when Tribunal L2 signatures must be valid.
	RequiresL2Signature() bool

	// RequiresL3Proof returns true when L3Notary proofs are required for mutations.
	RequiresL3Proof() bool
}

// DoctrinePosture enforces only L1 (static analysis / forbidden patterns).
// L2 and L3 results are recorded for audit but do not gate execution.
type DoctrinePosture struct{}

func (p *DoctrinePosture) Name() string              { return "doctrine" }
func (p *DoctrinePosture) Description() string       { return "doctrine (L1 enforced, L2/L3 audited)" }
func (p *DoctrinePosture) RequiresL2Signature() bool { return false }
func (p *DoctrinePosture) RequiresL3Proof() bool     { return false }

// ConsensusPosture enforces L1 and L2 (multi-agent quorum via Ed25519 signatures).
// L3 results are recorded for audit but do not gate execution.
type ConsensusPosture struct{}

func (p *ConsensusPosture) Name() string              { return "consensus" }
func (p *ConsensusPosture) Description() string       { return "consensus (L1/L2 enforced, L3 audited)" }
func (p *ConsensusPosture) RequiresL2Signature() bool { return true }
func (p *ConsensusPosture) RequiresL3Proof() bool     { return false }

// NotaryPosture enforces L1, L2, and L3 (human-in-the-loop via WebAuthn/mTLS).
// All three layers are fail-closed gates; any failure blocks execution.
type NotaryPosture struct{}

func (p *NotaryPosture) Name() string              { return "notary" }
func (p *NotaryPosture) Description() string       { return "notary (L1/L2/L3 strictly enforced)" }
func (p *NotaryPosture) RequiresL2Signature() bool { return true }
func (p *NotaryPosture) RequiresL3Proof() bool     { return true }

// NewGovernancePosture returns the GovernancePosture for the given name.
// Panics on an unrecognized name so that misconfigured deployments fail at
// startup rather than silently running under a weaker posture.
func NewGovernancePosture(posture string) GovernancePosture {
	p, err := ParseGovernancePosture(posture)
	if err != nil {
		panic(err.Error())
	}
	return p
}

// ParseGovernancePosture returns the GovernancePosture for the given name,
// or an error if the name is not recognized. Use this for CLI flag validation
// where a user-friendly error is preferable to a panic.
func ParseGovernancePosture(posture string) (GovernancePosture, error) {
	switch posture {
	case "doctrine":
		return &DoctrinePosture{}, nil
	case "consensus":
		return &ConsensusPosture{}, nil
	case "notary":
		return &NotaryPosture{}, nil
	default:
		return nil, fmt.Errorf("invalid governance posture %q (must be one of: doctrine, consensus, notary)", posture)
	}
}

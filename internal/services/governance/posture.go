// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package governance

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// GovernancePosture defines which layers of the verification pipeline are
// enforced as fail-closed gates versus audited.
//
// Four postures are defined from the independent L2 and L3 enforcement capabilities:
//
//	doctrine  — L1 enforced, L2/L3 audited (minimum)
//	consensus — L1+L2 enforced, L3 audited
//	ratify    — L1+L3 enforced, L2 audited
//	notary    — L1+L2+L3 strictly enforced (maximum)
//
// Adding a new posture only requires implementing this interface and extending
// the factory functions below; no changes to L4Warden or L5Actuator are needed.
//
//go:generate mockery --name GovernancePosture --output ./mocks --dir .
type GovernancePosture interface {
	// Name returns the canonical posture name ("doctrine", "consensus", "ratify", "notary").
	Name() string

	// Description returns a human-readable summary of what is enforced.
	Description() string

	// RequiresL2Signature returns true when Consensus L2 signatures must be valid.
	RequiresL2Signature() bool

	// RequiresL3Proof returns true when L3Notary proofs are required for mutations.
	RequiresL3Proof() bool
}

// DoctrinePosture enforces only L1 (static analysis / forbidden patterns).
// L2 and L3 results are recorded for audit but do not gate execution.
type DoctrinePosture struct{}

func (p *DoctrinePosture) Name() string              { return constants.PostureDoctrine }
func (p *DoctrinePosture) Description() string       { return "doctrine (L1 enforced, L2/L3 audited)" }
func (p *DoctrinePosture) RequiresL2Signature() bool { return false }
func (p *DoctrinePosture) RequiresL3Proof() bool     { return false }

// ConsensusPosture enforces L1 and L2 (multi-agent quorum via Ed25519 signatures).
// L3 results are recorded for audit but do not gate execution.
type ConsensusPosture struct{}

func (p *ConsensusPosture) Name() string              { return constants.PostureConsensus }
func (p *ConsensusPosture) Description() string       { return "consensus (L1/L2 enforced, L3 audited)" }
func (p *ConsensusPosture) RequiresL2Signature() bool { return true }
func (p *ConsensusPosture) RequiresL3Proof() bool     { return false }

// RatifyPosture enforces L1 and L3 (human-in-the-loop via WebAuthn/mTLS).
// L2 results are recorded for audit but do not gate execution.
type RatifyPosture struct{}

func (p *RatifyPosture) Name() string              { return constants.PostureRatify }
func (p *RatifyPosture) Description() string       { return "ratify (L1/L3 enforced, L2 audited)" }
func (p *RatifyPosture) RequiresL2Signature() bool { return false }
func (p *RatifyPosture) RequiresL3Proof() bool     { return true }

// NotaryPosture enforces L1, L2, and L3 (human-in-the-loop via WebAuthn/mTLS).
// All three layers are fail-closed gates; any failure blocks execution.
type NotaryPosture struct{}

func (p *NotaryPosture) Name() string              { return constants.PostureNotary }
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
	case constants.PostureDoctrine:
		return &DoctrinePosture{}, nil
	case constants.PostureConsensus:
		return &ConsensusPosture{}, nil
	case constants.PostureRatify:
		return &RatifyPosture{}, nil
	case constants.PostureNotary:
		return &NotaryPosture{}, nil
	default:
		return nil, fmt.Errorf("invalid governance posture %q (must be one of: doctrine, consensus, ratify, notary)", posture)
	}
}

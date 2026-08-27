// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package pubsub

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/consensus"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
)

// GovernanceCoreDeps holds the governance dependencies required by both
// outbound and gateway modes. Embedded by GatewayModeDeps and OutboundModeDeps
// so the shared fields are declared once.
type GovernanceCoreDeps struct {
	ReplayStore       governance.ReplayStore
	StateRootProvider governance.StateRootProvider
	TransactionAudit  governance.TransactionAuditStore
	L3Notary          governance.L3Notary
	SignerStore       governance.SignerStore
	Doctrine          *governance.L1Doctrine
}

// GatewayModeDeps embeds GovernanceCoreDeps and adds gateway-only fields.
// All fields are non-nil at construction (except Consensus, nil only when
// Posture == PostureDoctrine); the constructor rejects nils with typed errors.
// PlatformEnrollmentDeps and GovernedDocStore are gateway-only; the compiler
// proves they do not exist in outbound mode.
type GatewayModeDeps struct {
	GovernanceCoreDeps
	GovernedDocStore       governance.GovernedDocumentStore
	ConsensusPolicyStore   governance.L2ConsensusPolicyStore
	FieldReader            mcp.FieldReader
	Consensus              *consensus.ConsensusService // nil only when Posture == PostureDoctrine
	PlatformEnrollmentDeps *PlatformEnrollmentDeps
	Posture                config.GatewayPosture
}

// OutboundModeDeps embeds GovernanceCoreDeps only. There is no
// GovernedDocStore, no ConsensusPolicyStore, no FieldReader, no Consensus, no
// MCPGateway, no PlatformEnrollmentDeps — the type statically proves they do not
// exist in outbound mode.
type OutboundModeDeps struct {
	GovernanceCoreDeps
}

// validateGovernanceCoreDeps checks that all fields in GovernanceCoreDeps are non-nil.
func validateGovernanceCoreDeps(core GovernanceCoreDeps) error {
	if core.ReplayStore == nil {
		return fmt.Errorf("mode_deps: replay store: %w", constants.ErrReplayStoreRequired)
	}
	if core.StateRootProvider == nil {
		return fmt.Errorf("mode_deps: state root provider: %w", constants.ErrStateRootProviderRequired)
	}
	if core.TransactionAudit == nil {
		return fmt.Errorf("mode_deps: transaction audit: %w", constants.ErrTransactionAuditRequired)
	}
	if core.L3Notary == nil {
		return fmt.Errorf("mode_deps: l3 notary: %w", constants.ErrL3NotaryRequired)
	}
	if core.SignerStore == nil {
		return fmt.Errorf("mode_deps: signer store: %w", constants.ErrSignerStoreRequired)
	}
	if core.Doctrine == nil {
		return fmt.Errorf("mode_deps: doctrine: %w", constants.ErrDoctrineRequired)
	}
	return nil
}

// NewOutboundModeDeps validates and constructs OutboundModeDeps.
func NewOutboundModeDeps(deps OutboundModeDeps) (*OutboundModeDeps, error) {
	if err := validateGovernanceCoreDeps(deps.GovernanceCoreDeps); err != nil {
		return nil, err
	}
	return &deps, nil
}

// NewGatewayModeDeps validates and constructs GatewayModeDeps.
func NewGatewayModeDeps(deps GatewayModeDeps) (*GatewayModeDeps, error) {
	if err := validateGovernanceCoreDeps(deps.GovernanceCoreDeps); err != nil {
		return nil, err
	}
	if deps.GovernedDocStore == nil {
		return nil, fmt.Errorf("mode_deps: governed doc store: %w", constants.ErrGovernedDocStoreRequired)
	}
	if deps.ConsensusPolicyStore == nil {
		return nil, fmt.Errorf("mode_deps: consensus policy store: %w", constants.ErrConsensusPolicyStoreRequired)
	}
	if deps.FieldReader == nil {
		return nil, fmt.Errorf("mode_deps: field reader: %w", constants.ErrFieldReaderRequired)
	}
	if deps.PlatformEnrollmentDeps == nil {
		return nil, fmt.Errorf("mode_deps: platform enrollment deps: %w", constants.ErrPlatformEnrollmentDepsRequired)
	}
	if deps.Posture != config.PostureDoctrine && deps.Consensus == nil {
		return nil, fmt.Errorf("mode_deps: consensus: %w", constants.ErrConsensusServiceRequired)
	}
	return &deps, nil
}

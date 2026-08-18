// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package constants provides Go registry files generated from protocol/constants/ JSON.
//
// This file contains agent mode and prompt section identifier constants used throughout
// the platform for agent configuration and prompt engineering. These constants are
// generated from protocol/constants/prompts.json (SSOT).
//
// Adding new constants:
// 1. Add to protocol/constants/prompts.json
// 2. Run `make constants` to regenerate this file
// 3. Run `go run ./internal/constants/check_registry.go` to verify
package constants

const (
	AgentModeG8eBound           = "g8e.bound"
	AgentModeG8eNotBound        = "g8e.not.bound"
	AgentModeCloudOperatorBound = "g8e.cloud.bound"
)

const (
	PromptSectionIdentity             = "identity"
	PromptSectionSafety               = "safety"
	PromptSectionLoyalty              = "loyalty"
	PromptSectionDissent              = "dissent"
	PromptSectionCapabilities         = "capabilities"
	PromptSectionExecution            = "execution"
	PromptSectionTools                = "tools"
	PromptSectionDocs                 = "docs"
	PromptSectionSystemContext        = "system_context"
	PromptSectionVaultMode            = "sentinel_mode"
	PromptSectionTriageContext        = "triage_context"
	PromptSectionInvestigationContext = "investigation_context"
	PromptSectionResponseConstraints  = "response_constraints"
	PromptSectionLearnedContext       = "learned_context"
	PromptSectionAgentPersona         = "agent_persona"
)

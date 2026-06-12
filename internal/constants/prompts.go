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

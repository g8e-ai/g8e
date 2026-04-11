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

/**
 * Agent Constants
 * 
 * Centralized agent-related enums and metadata sourced from shared/constants/agents.json.
 * These constants provide type-safe access to agent configuration values used across VSOD.
 */

import { _AGENTS } from './shared.js';

/**
 * Triage Complexity Classification
 * Determines whether a task is simple (Assistant) or complex (Primary).
 */
export const TriageComplexity = Object.freeze({
    SIMPLE:  _AGENTS['triage.complexity'].simple,
    COMPLEX: _AGENTS['triage.complexity'].complex,
});

/**
 * Triage Confidence Level
 * Confidence score for complexity and intent classifications.
 */
export const TriageConfidence = Object.freeze({
    HIGH: _AGENTS['triage.confidence'].high,
    LOW:  _AGENTS['triage.confidence'].low,
});

/**
 * Triage Intent Classification
 * Categorizes the user's intent (information-seeking vs action-oriented).
 */
export const TriageIntent = Object.freeze({
    INFORMATION: _AGENTS['triage.intent'].information,
    ACTION:      _AGENTS['triage.intent'].action,
    UNKNOWN:     _AGENTS['triage.intent'].unknown,
});

/**
 * Tribunal Members
 * The three permanent members of the Tribunal.
 */
export const TribunalMember = Object.freeze({
    AXIOM:   _AGENTS['tribunal.members'].axiom,
    CONCORD: _AGENTS['tribunal.members'].concord,
    VARIANCE: _AGENTS['tribunal.members'].variance,
});

/**
 * Tribunal Temperatures
 * Canonical temperature values for each Tribunal member.
 * Axiom (0.0) - Fully deterministic
 * Concord (0.4) - Moderate determinism with ethical flexibility
 * Variance (0.8) - High creativity and intentional unpredictability
 */
export const TribunalTemperatures = Object.freeze({
    AXIOM:   _AGENTS['tribunal.temperatures'].axiom,
    CONCORD: _AGENTS['tribunal.temperatures'].concord,
    VARIANCE: _AGENTS['tribunal.temperatures'].variance,
});

/**
 * Agent Metadata
 * Display metadata (id, display_name, icon, description) for all agents.
 * Use this for UI rendering (icons, labels) instead of hardcoded strings.
 */
export const AgentMetadata = Object.freeze({
    TRIAGE:           _AGENTS['agent.metadata'].triage,
    PRIMARY:          _AGENTS['agent.metadata'].primary,
    ASSISTANT:        _AGENTS['agent.metadata'].assistant,
    TRIBUNAL:         _AGENTS['agent.metadata'].tribunal,
    VERIFIER:         _AGENTS['agent.metadata'].verifier,
    TITLE_GENERATOR:  _AGENTS['agent.metadata'].title_generator,
    AXIOM:            _AGENTS['agent.metadata'].axiom,
    CONCORD:          _AGENTS['agent.metadata'].concord,
    VARIANCE:         _AGENTS['agent.metadata'].variance,
});

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
 * These constants provide type-safe access to agent configuration values used across g8ed.
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
 * Triage Request Posture
 * Triage's read of the user's state for this turn. Downstream agents
 * (Primary, Assistant) calibrate dissent and denial-memory behavior on
 * this value. See components/g8ee/app/prompts_data/core/dissent.txt.
 */
export const TriageRequestPosture = Object.freeze({
    NORMAL:      _AGENTS['triage.posture'].normal,
    ESCALATED:   _AGENTS['triage.posture'].escalated,
    ADVERSARIAL: _AGENTS['triage.posture'].adversarial,
    CONFUSED:    _AGENTS['triage.posture'].confused,
});

/**
 * Tribunal Members
 * The three permanent members of the Tribunal.
 */
export const TribunalMember = Object.freeze({
    AXIOM:   _AGENTS['tribunal.members'].axiom,
    CONCORD: _AGENTS['tribunal.members'].concord,
    VARIANCE: _AGENTS['tribunal.members'].variance,
    PRAGMA:  _AGENTS['tribunal.members'].pragma,
    NEMESIS: _AGENTS['tribunal.members'].nemesis,
});

/**
 * Verifier Reason
 * The Verifier's stated reason for its verdict in Tribunal SSE payloads.
 */
export const VerifierReason = Object.freeze({
    OK:                _AGENTS['tribunal.verifier_reason'].ok,
    REVISED:           _AGENTS['tribunal.verifier_reason'].revised,
    EMPTY_RESPONSE:    _AGENTS['tribunal.verifier_reason'].empty_response,
    NO_VALID_REVISION: _AGENTS['tribunal.verifier_reason'].no_valid_revision,
    VERIFIER_ERROR:    _AGENTS['tribunal.verifier_reason'].verifier_error,
});

/**
 * Tribunal Outcome
 * Terminal outcomes the Tribunal pipeline can produce.
 */
export const TribunalOutcome = Object.freeze({
    CONSENSUS:           _AGENTS['tribunal.outcome'].consensus,
    VERIFIED:            _AGENTS['tribunal.outcome'].verified,
    VERIFICATION_FAILED: _AGENTS['tribunal.outcome'].verification_failed,
});

/**
 * Tribunal Member Icons
 * Icon names for each tribunal member, indexed by pass number.
 */
export const TribunalMemberIcons = Object.freeze({
    0: 'minimize-2',
    1: 'shield',
    2: 'layers',
    3: 'code',
    4: 'alert-triangle',
});


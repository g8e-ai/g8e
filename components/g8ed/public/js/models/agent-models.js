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

import { FrontendBaseModel, F } from './base.js';

/**
 * TriageResult - The outcome of a triage operation.
 * Matches @shared/models/agents/triage.json
 */
export class TriageResult extends FrontendBaseModel {
    static fields = {
        complexity:            { type: F.string,  required: true },
        complexity_confidence: { type: F.string,  required: true },
        intent:                { type: F.string,  required: true },
        intent_confidence:     { type: F.string,  required: true },
        intent_summary:        { type: F.string,  required: true },
        follow_up_question:    { type: F.string,  default: null },
        clarifying_questions:  { type: F.array,  items: F.string, default: null },
        request_posture:       { type: F.string,  default: 'normal' },
        posture_confidence:    { type: F.string,  default: 'low' },
    };
}

/**
 * PrimaryResult - The text output and tool calls from the Primary AI.
 * Matches @shared/models/agents/primary.json
 */
export class PrimaryResult extends FrontendBaseModel {
    static fields = {
        content:    { type: F.string, required: true },
        tool_calls: { type: F.array,  default: () => [] },
    };
}

/**
 * CandidateCommand - A single command candidate from a Tribunal member.
 * Matches @shared/models/agents/tribunal.json
 */
export class CandidateCommand extends FrontendBaseModel {
    static fields = {
        command:    { type: F.string, required: true },
        member:     { type: F.string, required: true },
        pass_index: { type: F.number, required: true },
        reasoning:  { type: F.string, required: true },
    };
}

/**
 * CommandGenerationResult - The final outcome of a Tribunal session.
 * Matches @shared/models/agents/tribunal.json voting_result
 */
export class CommandGenerationResult extends FrontendBaseModel {
    static fields = {
        vote_winner: { type: F.string, required: true },
        vote_score:  { type: F.number, required: true },
        outcome:     { type: F.string, required: true },
        candidates:  { type: F.array,  items: CandidateCommand, default: () => [] },
    };
}

/**
 * AuditorResult - Syntactic validation outcome.
 * Matches @shared/models/agents/verifier.json
 */
export class AuditorResult extends FrontendBaseModel {
    static fields = {
        passed:      { type: F.boolean, required: true },
        revision:    { type: F.string,  default: null },
        reason:      { type: F.string,  required: true },
        reason_enum: { type: F.string,  required: true },
    };
}

/**
 * CaseTitleResult - Outcome of title generation.
 * Matches @shared/models/agents/title_generator.json
 */
export class CaseTitleResult extends FrontendBaseModel {
    static fields = {
        generated_title: { type: F.string,  required: true },
        fallback:        { type: F.boolean, default: false },
    };
}

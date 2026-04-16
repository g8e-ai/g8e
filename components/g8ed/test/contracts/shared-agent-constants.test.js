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
 * Agent Shared Constants Contract Tests
 *
 * Verifies that g8ed's agent-related constants match the canonical values 
 * in shared/constants/agents.json.
 */

import { describe, it, expect } from 'vitest';
import { _AGENTS } from '@g8ed/constants/shared.js';
import { TriageComplexity, TriageConfidence, TriageIntent, TriageRequestPosture, AgentMetadata, TribunalMember } from '@g8ed/constants/agents.js';

describe('g8ed Agent Constants match shared/constants/agents.json', () => {
    describe('TriageComplexity constants', () => {
        it('SIMPLE constant matches JSON', () => {
            expect(TriageComplexity.SIMPLE).toBe(_AGENTS['triage.complexity'].simple);
        });
        it('COMPLEX constant matches JSON', () => {
            expect(TriageComplexity.COMPLEX).toBe(_AGENTS['triage.complexity'].complex);
        });
    });

    describe('TriageConfidence constants', () => {
        it('HIGH constant matches JSON', () => {
            expect(TriageConfidence.HIGH).toBe(_AGENTS['triage.confidence'].high);
        });
        it('LOW constant matches JSON', () => {
            expect(TriageConfidence.LOW).toBe(_AGENTS['triage.confidence'].low);
        });
    });

    describe('TriageIntent constants', () => {
        it('INFORMATION constant matches JSON', () => {
            expect(TriageIntent.INFORMATION).toBe(_AGENTS['triage.intent'].information);
        });
        it('ACTION constant matches JSON', () => {
            expect(TriageIntent.ACTION).toBe(_AGENTS['triage.intent'].action);
        });
        it('UNKNOWN constant matches JSON', () => {
            expect(TriageIntent.UNKNOWN).toBe(_AGENTS['triage.intent'].unknown);
        });
    });

    describe('TriageRequestPosture constants', () => {
        it('NORMAL constant matches JSON', () => {
            expect(TriageRequestPosture.NORMAL).toBe(_AGENTS['triage.posture'].normal);
        });
        it('ESCALATED constant matches JSON', () => {
            expect(TriageRequestPosture.ESCALATED).toBe(_AGENTS['triage.posture'].escalated);
        });
        it('ADVERSARIAL constant matches JSON', () => {
            expect(TriageRequestPosture.ADVERSARIAL).toBe(_AGENTS['triage.posture'].adversarial);
        });
        it('CONFUSED constant matches JSON', () => {
            expect(TriageRequestPosture.CONFUSED).toBe(_AGENTS['triage.posture'].confused);
        });
    });

    describe('TribunalMember constants', () => {
        it('AXIOM member matches JSON', () => {
            expect(TribunalMember.AXIOM).toBe(_AGENTS['tribunal.members'].axiom);
        });
        it('CONCORD member matches JSON', () => {
            expect(TribunalMember.CONCORD).toBe(_AGENTS['tribunal.members'].concord);
        });
        it('VARIANCE member matches JSON', () => {
            expect(TribunalMember.VARIANCE).toBe(_AGENTS['tribunal.members'].variance);
        });
    });

    describe('AgentMetadata constants', () => {
        it('TRIAGE metadata matches JSON', () => {
            expect(AgentMetadata.TRIAGE).toEqual(_AGENTS['agent.metadata'].triage);
        });
        it('PRIMARY metadata matches JSON', () => {
            expect(AgentMetadata.PRIMARY).toEqual(_AGENTS['agent.metadata'].primary);
        });
        it('ASSISTANT metadata matches JSON', () => {
            expect(AgentMetadata.ASSISTANT).toEqual(_AGENTS['agent.metadata'].assistant);
        });
        it('TRIBUNAL metadata matches JSON', () => {
            expect(AgentMetadata.TRIBUNAL).toEqual(_AGENTS['agent.metadata'].tribunal);
        });
        it('VERIFIER metadata matches JSON', () => {
            expect(AgentMetadata.VERIFIER).toEqual(_AGENTS['agent.metadata'].verifier);
        });
        it('TITLE_GENERATOR metadata matches JSON', () => {
            expect(AgentMetadata.TITLE_GENERATOR).toEqual(_AGENTS['agent.metadata'].title_generator);
        });
        it('AXIOM metadata matches JSON', () => {
            expect(AgentMetadata.AXIOM).toEqual(_AGENTS['agent.metadata'].axiom);
        });
        it('CONCORD metadata matches JSON', () => {
            expect(AgentMetadata.CONCORD).toEqual(_AGENTS['agent.metadata'].concord);
        });
        it('VARIANCE metadata matches JSON', () => {
            expect(AgentMetadata.VARIANCE).toEqual(_AGENTS['agent.metadata'].variance);
        });
        it('MEMORY_GENERATOR metadata matches JSON', () => {
            expect(AgentMetadata.MEMORY_GENERATOR).toEqual(_AGENTS['agent.metadata'].memory_generator);
        });
        it('EVAL_JUDGE metadata matches JSON', () => {
            expect(AgentMetadata.EVAL_JUDGE).toEqual(_AGENTS['agent.metadata'].eval_judge);
        });
        it('RESPONSE_ANALYZER metadata matches JSON', () => {
            expect(AgentMetadata.RESPONSE_ANALYZER).toEqual(_AGENTS['agent.metadata'].response_analyzer);
        });
        it('RESPONSE_ANALYZER_COMMAND_RISK metadata matches JSON', () => {
            expect(AgentMetadata.RESPONSE_ANALYZER_COMMAND_RISK).toEqual(_AGENTS['agent.metadata'].response_analyzer_command_risk);
        });
        it('RESPONSE_ANALYZER_ERROR metadata matches JSON', () => {
            expect(AgentMetadata.RESPONSE_ANALYZER_ERROR).toEqual(_AGENTS['agent.metadata'].response_analyzer_error);
        });
        it('RESPONSE_ANALYZER_FILE_RISK metadata matches JSON', () => {
            expect(AgentMetadata.RESPONSE_ANALYZER_FILE_RISK).toEqual(_AGENTS['agent.metadata'].response_analyzer_file_risk);
        });
    });

    describe('AgentMetadata persona fields', () => {
        const REQUIRED_PERSONA_FIELDS = ['role', 'model_tier', 'temperature', 'tools', 'identity', 'purpose', 'autonomy'];
        const VALID_AUTONOMY_VALUES = ['fully_autonomous', 'human_approved'];
        const ALL_AGENT_KEYS = ['triage', 'primary', 'assistant', 'tribunal', 'verifier', 'title_generator', 'axiom', 'concord', 'variance', 'memory_generator', 'eval_judge', 'response_analyzer', 'response_analyzer_command_risk', 'response_analyzer_error', 'response_analyzer_file_risk'];

        ALL_AGENT_KEYS.forEach(agentKey => {
            describe(`${agentKey} agent`, () => {
                const agent = _AGENTS['agent.metadata'][agentKey];

                REQUIRED_PERSONA_FIELDS.forEach(field => {
                    it(`has '${field}' field`, () => {
                        expect(agent).toHaveProperty(field);
                    });
                });

                it('role is a non-empty string', () => {
                    expect(typeof agent.role).toBe('string');
                    expect(agent.role.length).toBeGreaterThan(0);
                });

                it('model_tier is a non-empty string', () => {
                    expect(typeof agent.model_tier).toBe('string');
                    expect(agent.model_tier.length).toBeGreaterThan(0);
                });

                it('temperature is null or a number', () => {
                    expect(agent.temperature === null || typeof agent.temperature === 'number').toBe(true);
                });

                it('tools is an array', () => {
                    expect(Array.isArray(agent.tools)).toBe(true);
                });

                it('identity is a non-empty string', () => {
                    expect(typeof agent.identity).toBe('string');
                    expect(agent.identity.length).toBeGreaterThan(0);
                });

                it('purpose is a non-empty string', () => {
                    expect(typeof agent.purpose).toBe('string');
                    expect(agent.purpose.length).toBeGreaterThan(0);
                });

                it('autonomy is a valid value', () => {
                    expect(VALID_AUTONOMY_VALUES).toContain(agent.autonomy);
                });
            });
        });
    });

    describe('Raw JSON values (legacy)', () => {
        it('triage.complexity values are correct', () => {
            expect(_AGENTS['triage.complexity'].simple).toBe('simple');
            expect(_AGENTS['triage.complexity'].complex).toBe('complex');
        });

        it('triage.confidence values are correct', () => {
            expect(_AGENTS['triage.confidence'].high).toBe('high');
            expect(_AGENTS['triage.confidence'].low).toBe('low');
        });

        it('triage.intent values are correct', () => {
            expect(_AGENTS['triage.intent'].information).toBe('information');
            expect(_AGENTS['triage.intent'].action).toBe('action');
            expect(_AGENTS['triage.intent'].unknown).toBe('unknown');
        });

        it('agent.metadata contains all agents', () => {
            const expectedAgents = ['triage', 'primary', 'assistant', 'tribunal', 'verifier', 'title_generator', 'axiom', 'concord', 'variance', 'memory_generator', 'eval_judge', 'response_analyzer', 'response_analyzer_command_risk', 'response_analyzer_error', 'response_analyzer_file_risk'];
            const actualAgents = Object.keys(_AGENTS['agent.metadata']);
            expect(actualAgents.sort()).toEqual(expectedAgents.sort());
        });
    });
});

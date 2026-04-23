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
import { TriageComplexity, TriageConfidence, TriageIntent, TriageRequestPosture, TribunalMember, AuditorReason, TribunalMemberIcons } from '@g8ed/constants/agents.js';

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
        it('PRAGMA member matches JSON', () => {
            expect(TribunalMember.PRAGMA).toBe(_AGENTS['tribunal.members'].pragma);
        });
        it('NEMESIS member matches JSON', () => {
            expect(TribunalMember.NEMESIS).toBe(_AGENTS['tribunal.members'].nemesis);
        });
    });

    describe('AuditorReason constants', () => {
        it('OK matches JSON', () => {
            expect(AuditorReason.OK).toBe(_AGENTS['tribunal.auditor_reason'].ok);
        });
        it('REVISED matches JSON', () => {
            expect(AuditorReason.REVISED).toBe(_AGENTS['tribunal.auditor_reason'].revised);
        });
        it('EMPTY_RESPONSE matches JSON', () => {
            expect(AuditorReason.EMPTY_RESPONSE).toBe(_AGENTS['tribunal.auditor_reason'].empty_response);
        });
        it('NO_VALID_REVISION matches JSON', () => {
            expect(AuditorReason.NO_VALID_REVISION).toBe(_AGENTS['tribunal.auditor_reason'].no_valid_revision);
        });
        it('AUDITOR_ERROR matches JSON', () => {
            expect(AuditorReason.AUDITOR_ERROR).toBe(_AGENTS['tribunal.auditor_reason'].auditor_error);
        });
    });

    describe('TribunalMemberIcons constants', () => {
        it('Pass 0 (Axiom) icon matches JSON metadata', () => {
            expect(TribunalMemberIcons[0]).toBe(_AGENTS['agent.metadata'].axiom.icon);
        });
        it('Pass 1 (Concord) icon matches JSON metadata', () => {
            expect(TribunalMemberIcons[1]).toBe(_AGENTS['agent.metadata'].concord.icon);
        });
        it('Pass 2 (Variance) icon matches JSON metadata', () => {
            expect(TribunalMemberIcons[2]).toBe(_AGENTS['agent.metadata'].variance.icon);
        });
        it('Pass 3 (Pragma) icon matches JSON metadata', () => {
            expect(TribunalMemberIcons[3]).toBe(_AGENTS['agent.metadata'].pragma.icon);
        });
        it('Pass 4 (Nemesis) icon matches JSON metadata', () => {
            expect(TribunalMemberIcons[4]).toBe(_AGENTS['agent.metadata'].nemesis.icon);
        });
    });

    describe('agent.metadata persona fields', () => {
        const REQUIRED_PERSONA_FIELDS = ['role', 'model_tier', 'tools', 'identity', 'purpose', 'autonomy'];
        const ALL_AGENT_KEYS = ['triage', 'sage', 'dash', 'tribunal', 'auditor', 'scribe', 'axiom', 'concord', 'variance', 'pragma', 'nemesis', 'codex', 'judge', 'warden', 'warden_command_risk', 'warden_error', 'warden_file_risk'];

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

                it('autonomy is a non-empty string', () => {
                    expect(typeof agent.autonomy).toBe('string');
                    expect(agent.autonomy.length).toBeGreaterThan(0);
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
            const expectedAgents = ['triage', 'sage', 'dash', 'tribunal', 'auditor', 'scribe', 'axiom', 'concord', 'variance', 'pragma', 'nemesis', 'codex', 'judge', 'warden', 'warden_command_risk', 'warden_error', 'warden_file_risk'];
            const actualAgents = Object.keys(_AGENTS['agent.metadata']);
            expect(actualAgents.sort()).toEqual(expectedAgents.sort());
        });
    });
});

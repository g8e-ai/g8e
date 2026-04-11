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
 * Verifies that VSOD's agent-related constants match the canonical values 
 * in shared/constants/agents.json.
 */

import { describe, it, expect } from 'vitest';
import { _AGENTS } from '@vsod/constants/shared.js';
import { TriageComplexity, TriageConfidence, TriageIntent, AgentMetadata, TribunalTemperatures } from '@vsod/constants/agents.js';

describe('VSOD Agent Constants match shared/constants/agents.json', () => {
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

    describe('TribunalTemperatures constants', () => {
        it('AXIOM temperature matches JSON', () => {
            expect(TribunalTemperatures.AXIOM).toBe(_AGENTS['tribunal.temperatures'].axiom);
        });
        it('CONCORD temperature matches JSON', () => {
            expect(TribunalTemperatures.CONCORD).toBe(_AGENTS['tribunal.temperatures'].concord);
        });
        it('VARIANCE temperature matches JSON', () => {
            expect(TribunalTemperatures.VARIANCE).toBe(_AGENTS['tribunal.temperatures'].variance);
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
            const expectedAgents = ['triage', 'primary', 'assistant', 'tribunal', 'verifier', 'title_generator', 'axiom', 'concord', 'variance'];
            const actualAgents = Object.keys(_AGENTS['agent.metadata']);
            expect(actualAgents.sort()).toEqual(expectedAgents.sort());
        });
    });
});

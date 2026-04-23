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

import { describe, it, expect } from 'vitest';
import { 
    TriageResult, 
    PrimaryResult, 
    CommandGenerationResult, 
    AuditorResult, 
    CaseTitleResult 
} from '../../../../public/js/models/agent-models.js';

describe('Agent Models', () => {
    describe('TriageResult', () => {
        it('parses valid triage result from wire', () => {
            const raw = {
                complexity: 'complex',
                complexity_confidence: 'high',
                intent: 'action',
                intent_confidence: 'high',
                intent_summary: 'The user wants to fix a bug.',
                follow_up_question: null
            };
            const model = TriageResult.parse(raw);
            expect(model.complexity).toBe('complex');
            expect(model.intent_summary).toBe('The user wants to fix a bug.');
        });

        it('throws on missing required fields', () => {
            const raw = { complexity: 'complex' };
            expect(() => TriageResult.parse(raw)).toThrow();
        });
    });

    describe('PrimaryResult', () => {
        it('parses valid primary result from wire', () => {
            const raw = {
                content: 'I will help you with that.',
                tool_calls: [{ name: 'ls', arguments: { path: '/' } }]
            };
            const model = PrimaryResult.parse(raw);
            expect(model.content).toBe('I will help you with that.');
            expect(model.tool_calls).toHaveLength(1);
        });
    });

    describe('CommandGenerationResult', () => {
        it('parses valid tribunal outcome', () => {
            const raw = {
                vote_winner: 'ls -la',
                vote_score: 0.9,
                outcome: 'consensus',
                candidates: [
                    { command: 'ls -la', member: 'axiom', pass_index: 0, reasoning: 'Standard command' }
                ]
            };
            const model = CommandGenerationResult.parse(raw);
            expect(model.vote_winner).toBe('ls -la');
            expect(model.candidates).toHaveLength(1);
            expect(model.candidates[0].member).toBe('axiom');
        });
    });

    describe('AuditorResult', () => {
        it('parses valid verifier result', () => {
            const raw = {
                passed: true,
                reason: 'Command is safe',
                reason_enum: 'ok'
            };
            const model = AuditorResult.parse(raw);
            expect(model.passed).toBe(true);
            expect(model.reason_enum).toBe('ok');
        });
    });

    describe('CaseTitleResult', () => {
        it('parses valid title result', () => {
            const raw = {
                generated_title: 'Debugging Auth Issue',
                fallback: false
            };
            const model = CaseTitleResult.parse(raw);
            expect(model.generated_title).toBe('Debugging Auth Issue');
            expect(model.fallback).toBe(false);
        });
    });
});

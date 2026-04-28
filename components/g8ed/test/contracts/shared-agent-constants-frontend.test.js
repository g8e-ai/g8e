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
 * Browser-side Agent Constants Contract Tests
 *
 * The browser cannot `require()` JSON, so g8ed maintains a hand-mirrored
 * copy of select agent constants at public/js/constants/agents.js. This
 * test enforces that the mirrored values stay in lock-step with the
 * canonical shared/constants/agents.json (and, transitively, with the
 * Node-side g8ed/constants/agents.js which loads from the same JSON).
 *
 * Without this contract, hardcoded literals like `AuditorIcon = 'search-check'`
 * silently diverge whenever agents.json changes.
 */

import { describe, it, expect } from 'vitest';
import { _AGENTS } from '@g8ed/constants/shared.js';
import {
    TribunalMemberIcons as ServerTribunalMemberIcons,
    AuditorIcon as ServerAuditorIcon,
    TribunalOutcome as ServerTribunalOutcome,
} from '@g8ed/constants/agents.js';
import {
    TribunalMemberIcons,
    AuditorIcon,
    TribunalOutcome,
} from '@g8ed/public/js/constants/agents.js';

describe('Browser-side agent constants mirror shared/constants/agents.json', () => {
    describe('TribunalMemberIcons (browser mirror)', () => {
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

    describe('AuditorIcon (browser mirror)', () => {
        it('matches JSON metadata', () => {
            expect(AuditorIcon).toBe(_AGENTS['agent.metadata'].auditor.icon);
        });
    });

    describe('TribunalOutcome (browser mirror)', () => {
        it('CONSENSUS matches JSON', () => {
            expect(TribunalOutcome.CONSENSUS).toBe(_AGENTS['tribunal.outcome'].consensus);
        });
        it('VERIFIED matches JSON', () => {
            expect(TribunalOutcome.VERIFIED).toBe(_AGENTS['tribunal.outcome'].verified);
        });
        it('VERIFICATION_FAILED matches JSON', () => {
            expect(TribunalOutcome.VERIFICATION_FAILED).toBe(_AGENTS['tribunal.outcome'].verification_failed);
        });
        it('CONSENSUS_FAILED matches JSON', () => {
            expect(TribunalOutcome.CONSENSUS_FAILED).toBe(_AGENTS['tribunal.outcome'].consensus_failed);
        });
    });

    describe('Browser mirror matches Node-side mirror', () => {
        it('AuditorIcon is identical across mirrors', () => {
            expect(AuditorIcon).toBe(ServerAuditorIcon);
        });
        it('TribunalMemberIcons are identical across mirrors', () => {
            for (const pass of [0, 1, 2, 3, 4]) {
                expect(TribunalMemberIcons[pass]).toBe(ServerTribunalMemberIcons[pass]);
            }
        });
        it('TribunalOutcome shared keys are identical across mirrors', () => {
            expect(TribunalOutcome.CONSENSUS).toBe(ServerTribunalOutcome.CONSENSUS);
            expect(TribunalOutcome.VERIFIED).toBe(ServerTribunalOutcome.VERIFIED);
            expect(TribunalOutcome.VERIFICATION_FAILED).toBe(ServerTribunalOutcome.VERIFICATION_FAILED);
        });
    });
});

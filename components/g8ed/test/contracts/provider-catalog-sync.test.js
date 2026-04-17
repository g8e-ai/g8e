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
 * Provider Catalog Sync Contract
 *
 * g8ed carries two copies of `PROVIDER_MODELS`:
 *   - Server-side:  components/g8ed/constants/ai.js            (loaded by SSE)
 *   - Browser-side: components/g8ed/public/js/constants/ai-constants.js
 *                   (loaded by the setup wizard before SSE is available)
 *
 * They MUST stay in sync. Two browser-side models appearing with no
 * server-side counterpart (or vice versa) would silently break the model
 * dropdowns or the SSE-driven settings UI.
 *
 * Until the browser catalog is replaced by a server-injected or
 * server-fetched payload, this test guards against accidental drift.
 */

import { describe, it, expect } from 'vitest';
import { PROVIDER_MODELS as SERVER_CATALOG, LLMProvider as SERVER_LLM } from '@g8ed/constants/ai.js';
import { PROVIDER_MODELS as BROWSER_CATALOG, LLMProvider as BROWSER_LLM } from '@g8ed/public/js/constants/ai-constants.js';

describe('PROVIDER_MODELS sync (server vs browser catalog)', () => {
    it('LLMProvider enum values match exactly', () => {
        expect(BROWSER_LLM).toEqual(SERVER_LLM);
    });

    it('provider keys match', () => {
        expect(Object.keys(BROWSER_CATALOG).sort()).toEqual(Object.keys(SERVER_CATALOG).sort());
    });

    for (const provider of Object.values(SERVER_LLM)) {
        describe(`provider "${provider}"`, () => {
            const serverTiers = SERVER_CATALOG[provider];
            const browserTiers = BROWSER_CATALOG[provider];

            it('exists in both catalogs', () => {
                expect(serverTiers).toBeDefined();
                expect(browserTiers).toBeDefined();
            });

            for (const tier of ['primary', 'assistant', 'lite', 'all']) {
                it(`tier "${tier}" ids match`, () => {
                    const serverIds = (serverTiers?.[tier] ?? []).map(m => m.id);
                    const browserIds = (browserTiers?.[tier] ?? []).map(m => m.id);
                    expect(browserIds).toEqual(serverIds);
                });

                it(`tier "${tier}" labels match`, () => {
                    const serverLabels = (serverTiers?.[tier] ?? []).map(m => [m.id, m.label]);
                    const browserLabels = (browserTiers?.[tier] ?? []).map(m => [m.id, m.label]);
                    expect(browserLabels).toEqual(serverLabels);
                });
            }
        });
    }
});

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

// @vitest-environment jsdom

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { MessageRenderer } from '@g8ed/public/js/components/message-renderer.js';
import { EventType } from '@g8ed/public/js/constants/events.js';

function makeMarkdownRenderer() {
    return {
        parseMarkdown: (text, _streaming) => text,
        renderMermaidDiagrams: vi.fn(),
    };
}

function makeSource(overrides = {}) {
    return {
        citation_num: 1,
        uri: 'https://example.com/page',
        display_name: 'Example',
        domain: 'example.com',
        full_title: 'Example Page Title',
        favicon_url: null,
        segments: ['Relevant excerpt from source.'],
        ...overrides,
    };
}

function makeGrounding({ sources = [], grounding_supports = [], grounding_used = true } = {}) {
    return { grounding_used, sources, grounding_supports };
}

describe('MessageRenderer [FRONTEND - jsdom]', () => {
    let renderer;

    beforeEach(() => {
        renderer = new MessageRenderer(makeMarkdownRenderer());
    });

    describe('renderMessage — basic structure', () => {
        it('returns a DIV element', () => {
            const el = renderer.renderMessage({
                content: 'Hello world.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
            });
            expect(el.tagName).toBe('DIV');
        });

        it('sender value applied as class', () => {
            const el = renderer.renderMessage({
                content: 'Hello.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
            });
            expect(el.className).toContain(EventType.EVENT_SOURCE_AI_PRIMARY);
        });

        it('.message-header and .message-content present', () => {
            const el = renderer.renderMessage({
                content: 'Hello.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
            });
            expect(el.querySelector('.message-header')).not.toBeNull();
            expect(el.querySelector('.message-content')).not.toBeNull();
        });

        it('content text in .content-text', () => {
            const el = renderer.renderMessage({
                content: 'My response text.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
            });
            expect(el.querySelector('.content-text').textContent).toContain('My response text.');
        });

        it('.copy-response-btn present for AI_PRIMARY', () => {
            const el = renderer.renderMessage({
                content: 'AI response.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
            });
            expect(el.querySelector('.copy-response-btn')).not.toBeNull();
        });

        it('.copy-response-btn absent for USER_CHAT', () => {
            const el = renderer.renderMessage({
                content: 'User message.',
                sender: EventType.EVENT_SOURCE_USER_CHAT,
                timestamp: null,
            });
            expect(el.querySelector('.copy-response-btn')).toBeNull();
        });

        it('.sender-label is g8e for AI_PRIMARY', () => {
            const el = renderer.renderMessage({
                content: 'AI.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
            });
            expect(el.querySelector('.sender-label').textContent).toBe('g8e');
        });

        it('.sender-label is You for USER_CHAT', () => {
            const el = renderer.renderMessage({
                content: 'User.',
                sender: EventType.EVENT_SOURCE_USER_CHAT,
                timestamp: null,
            });
            expect(el.querySelector('.sender-label').textContent).toBe('You');
        });

        it('.sender-label is SYSTEM for SYSTEM sender', () => {
            const el = renderer.renderMessage({
                content: 'System notification.',
                sender: EventType.EVENT_SOURCE_SYSTEM,
                timestamp: null,
            });
            expect(el.querySelector('.sender-label').textContent).toBe('SYSTEM');
        });
    });

    describe('renderMessage — citation integration', () => {
        it('.sources-panel appended inside .content-text when grounding_used: true and sources present', () => {
            const el = renderer.renderMessage({
                content: 'Nginx can handle high traffic.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                groundingMetadata: makeGrounding({
                    sources: [makeSource({ citation_num: 1 })],
                    grounding_supports: [{ start_index: 0, end_index: 29, grounding_chunk_indices: [0] }],
                }),
            });

            const contentText = el.querySelector('.content-text');
            expect(contentText.querySelector('.sources-panel')).not.toBeNull();
        });

        it('no .sources-panel when groundingMetadata is null', () => {
            const el = renderer.renderMessage({
                content: 'No grounding here.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                groundingMetadata: null,
            });

            expect(el.querySelector('.sources-panel')).toBeNull();
        });

        it('.sources-panel still appended when grounding_used: false but sources are present — renderMessage gates on sources.length > 0 only, not grounding_used', () => {
            const el = renderer.renderMessage({
                content: 'Grounding not used.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                groundingMetadata: makeGrounding({
                    grounding_used: false,
                    sources: [makeSource()],
                }),
            });

            expect(el.querySelector('.sources-panel')).not.toBeNull();
        });

        it('no .sources-panel when sources is empty', () => {
            const el = renderer.renderMessage({
                content: 'Sources empty.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                groundingMetadata: makeGrounding({ sources: [] }),
            });

            expect(el.querySelector('.sources-panel')).toBeNull();
        });

        it('.source-item count matches source array length', () => {
            const sources = [
                makeSource({ citation_num: 1, uri: 'https://a.com', domain: 'a.com', display_name: 'A' }),
                makeSource({ citation_num: 2, uri: 'https://b.com', domain: 'b.com', display_name: 'B' }),
            ];
            const el = renderer.renderMessage({
                content: 'Two sources cited.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                groundingMetadata: makeGrounding({ sources, grounding_supports: [] }),
            });

            const panel = el.querySelector('.sources-panel');
            expect(panel.querySelectorAll('.source-item').length).toBe(2);
        });

        it('.sources-panel rendered for any sender value when grounding metadata and sources are present', () => {
            const el = renderer.renderMessage({
                content: 'User sent this.',
                sender: EventType.EVENT_SOURCE_USER_CHAT,
                timestamp: null,
                groundingMetadata: makeGrounding({ sources: [makeSource()] }),
            });

            expect(el.querySelector('.sources-panel')).not.toBeNull();
        });
    });

    describe('renderMessage — context info', () => {
        it('case_id present — Case: {id} in output', () => {
            const el = renderer.renderMessage({
                content: 'Response.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                contextInfo: { case_id: 'case-abc-123', investigation_id: null },
            });

            expect(el.innerHTML).toContain('Case: case-abc-123');
        });

        it('investigation_id present — Investigation: {id} in output', () => {
            const el = renderer.renderMessage({
                content: 'Response.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                contextInfo: { case_id: null, investigation_id: 'inv-xyz-456' },
            });

            expect(el.innerHTML).toContain('Investigation: inv-xyz-456');
        });

        it('contextInfo null — no .message-context element', () => {
            const el = renderer.renderMessage({
                content: 'Response.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                contextInfo: null,
            });

            expect(el.querySelector('.message-context')).toBeNull();
        });

        it('both IDs null — no .message-context element', () => {
            const el = renderer.renderMessage({
                content: 'Response.',
                sender: EventType.EVENT_SOURCE_AI_PRIMARY,
                timestamp: null,
                contextInfo: { case_id: null, investigation_id: null },
            });

            expect(el.querySelector('.message-context')).toBeNull();
        });
    });

    describe('renderContent', () => {
        it('returns a string', () => {
            const result = renderer.renderContent('Hello **world**.');
            expect(typeof result).toBe('string');
        });

        it('HTML entities decoded before the markdown pass', () => {
            const result = renderer.renderContent('&lt;nginx&gt; config &amp; tuning.');
            expect(result).toContain('<nginx> config & tuning.');
        });

        it("returns '' for null input", () => {
            const result = renderer.renderContent(null);
            expect(result).toBe('');
        });
    });

    describe('addInlineCitations delegation', () => {
        it('grounding_used: false — HTML returned unchanged (delegates correctly to CitationsHandler)', () => {
            const html = '<p>Some text.</p>';
            const result = renderer.addInlineCitations(html, makeGrounding({ grounding_used: false, sources: [makeSource()] }));
            expect(result).toBe(html);
        });

        it('empty grounding_supports — HTML returned unchanged', () => {
            const html = '<p>Some text.</p>';
            const result = renderer.addInlineCitations(html, makeGrounding({ sources: [makeSource()], grounding_supports: [] }));
            expect(result).toBe(html);
        });
    });

    describe('renderSourcesPanel delegation', () => {
        it('returns element with class sources-panel', () => {
            const panel = renderer.renderSourcesPanel([makeSource()]);
            expect(panel.className).toContain('sources-panel');
        });

        it('.source-item count matches source array length', () => {
            const sources = [
                makeSource({ citation_num: 1 }),
                makeSource({ citation_num: 2, uri: 'https://b.com', domain: 'b.com', display_name: 'B' }),
            ];
            const panel = renderer.renderSourcesPanel(sources);
            expect(panel.querySelectorAll('.source-item').length).toBe(2);
        });
    });

    describe('_decodeHtmlEntities (via renderContent)', () => {
        const cases = [
            ['&amp;', '&'],
            ['&lt;', '<'],
            ['&gt;', '>'],
            ['&quot;', '"'],
            ['&#39;', "'"],
            ['&#x27;', "'"],
            ['&#x2F;', '/'],
            ['&#47;', '/'],
            ['&nbsp;', ' '],
        ];

        for (const [entity, expected] of cases) {
            it(`${entity} -> ${expected}`, () => {
                const result = renderer.renderContent(entity);
                expect(result).toBe(expected);
            });
        }
    });
});

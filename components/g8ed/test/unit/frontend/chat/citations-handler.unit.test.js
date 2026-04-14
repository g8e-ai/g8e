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

import { describe, it, expect, beforeEach } from 'vitest';
import { CitationsHandler } from '@g8ed/public/js/components/citations.js';
import { CitationSource, CitationItem } from '@g8ed/public/js/models/ai-event-models.js';

function makeSource(overrides = {}) {
    return {
        citation_num: 1,
        uri: 'https://example.com/page',
        display_name: 'Example',
        domain: 'example.com',
        full_title: 'Example Page Title',
        favicon_url: null,
        segments: [],
        ...overrides,
    };
}

function makeGrounding({ sources = [], grounding_supports = [], grounding_used = true } = {}) {
    return { grounding_used, sources, grounding_supports };
}

describe('CitationsHandler [FRONTEND - jsdom]', () => {
    let handler;

    beforeEach(() => {
        handler = new CitationsHandler();
    });

    describe('_isSafeUrl', () => {
        it('allows https URLs', () => {
            expect(handler._isSafeUrl('https://example.com/page')).toBe(true);
        });

        it('allows http URLs', () => {
            expect(handler._isSafeUrl('https://example.com/page')).toBe(true);
        });

        it('rejects javascript: protocol', () => {
            expect(handler._isSafeUrl('javascript:alert(1)')).toBe(false);
        });

        it('rejects data: protocol', () => {
            expect(handler._isSafeUrl('data:text/html,<h1>XSS</h1>')).toBe(false);
        });

        it('rejects null', () => {
            expect(handler._isSafeUrl(null)).toBe(false);
        });

        it('rejects empty string', () => {
            expect(handler._isSafeUrl('')).toBe(false);
        });

        it('rejects non-string', () => {
            expect(handler._isSafeUrl(42)).toBe(false);
        });

        it('rejects ftp: protocol', () => {
            expect(handler._isSafeUrl('ftp://files.example.com/file.txt')).toBe(false);
        });
    });

    describe('_isPositionInCodeBlock', () => {
        it('returns false for empty ranges', () => {
            expect(handler._isPositionInCodeBlock(5, [])).toBe(false);
        });

        it('returns true when position is inside a range', () => {
            expect(handler._isPositionInCodeBlock(10, [{ start: 5, end: 15 }])).toBe(true);
        });

        it('returns true at range boundary (start)', () => {
            expect(handler._isPositionInCodeBlock(5, [{ start: 5, end: 15 }])).toBe(true);
        });

        it('returns true at range boundary (end)', () => {
            expect(handler._isPositionInCodeBlock(15, [{ start: 5, end: 15 }])).toBe(true);
        });

        it('returns false when position is before range', () => {
            expect(handler._isPositionInCodeBlock(3, [{ start: 5, end: 15 }])).toBe(false);
        });

        it('returns false when position is after range', () => {
            expect(handler._isPositionInCodeBlock(20, [{ start: 5, end: 15 }])).toBe(false);
        });

        it('returns true when position matches any of multiple ranges', () => {
            const ranges = [{ start: 0, end: 5 }, { start: 20, end: 30 }];
            expect(handler._isPositionInCodeBlock(25, ranges)).toBe(true);
        });
    });

    describe('_getCodeBlockRanges', () => {
        it('returns empty array when container has no code blocks', () => {
            const container = document.createElement('div');
            container.innerHTML = '<p>Plain text only.</p>';
            const ranges = handler._getCodeBlockRanges(container, 'Plain text only.');
            expect(ranges).toEqual([]);
        });

        it('returns range covering pre>code content', () => {
            const container = document.createElement('div');
            container.innerHTML = '<p>Text before. </p><pre><code>nginx -t</code></pre>';
            const plainText = 'Text before. nginx -t';
            const ranges = handler._getCodeBlockRanges(container, plainText);
            expect(ranges.length).toBeGreaterThan(0);
            const codeStart = 'Text before. '.length;
            expect(ranges[0].start).toBe(codeStart);
            expect(ranges[0].end).toBe(codeStart + 'nginx -t'.length);
        });

        it('merges adjacent code block ranges into fewer entries than raw elements', () => {
            const container = document.createElement('div');
            container.innerHTML = '<pre><code>first block</code></pre><pre><code>second block</code></pre>';
            const plainText = 'first blocksecond block';
            const ranges = handler._getCodeBlockRanges(container, plainText);
            expect(ranges.length).toBeLessThan(4);
            expect(ranges.every(r => typeof r.start === 'number' && typeof r.end === 'number')).toBe(true);
        });
    });

    describe('addInlineCitations — guard conditions', () => {
        it('returns original html when groundingMetadata is null', () => {
            const html = '<p>Hello world.</p>';
            expect(handler.addInlineCitations(html, null)).toBe(html);
        });

        it('returns original html when grounding_used is false', () => {
            const html = '<p>Hello world.</p>';
            const grounding = makeGrounding({ grounding_used: false, sources: [makeSource()] });
            expect(handler.addInlineCitations(html, grounding)).toBe(html);
        });

        it('returns original html when sources is empty', () => {
            const html = '<p>Hello world.</p>';
            const grounding = makeGrounding({
                sources: [],
                grounding_supports: [{ start_index: 0, end_index: 5, grounding_chunk_indices: [0] }],
            });
            expect(handler.addInlineCitations(html, grounding)).toBe(html);
        });

        it('returns original html when grounding_supports is empty', () => {
            const html = '<p>Hello world.</p>';
            const grounding = makeGrounding({
                sources: [makeSource()],
                grounding_supports: [],
            });
            expect(handler.addInlineCitations(html, grounding)).toBe(html);
        });
    });

    describe('addInlineCitations — citation insertion', () => {
        it('inserts citation superscript for a supported text segment', () => {
            const text = 'Nginx handles high traffic well.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 1 })],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            expect(result).toContain('citation-superscript');
            expect(result).toContain('[1]');
        });

        it('includes citation-wrapper span around each citation group', () => {
            const text = 'The system is stable.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 1 })],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            expect(result).toContain('citation-wrapper');
        });

        it('handles multiple sources referenced in one support', () => {
            const text = 'Both sources agree on this point.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [
                    makeSource({ citation_num: 1, uri: 'https://a.com', domain: 'a.com', display_name: 'A' }),
                    makeSource({ citation_num: 2, uri: 'https://b.com', domain: 'b.com', display_name: 'B' }),
                ],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0, 1] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            const container = document.createElement('div');
            container.innerHTML = result;
            const superscript = container.querySelector('.citation-superscript');
            expect(superscript).not.toBeNull();
            expect(superscript.textContent).toContain('1');
            expect(superscript.textContent).toContain('2');
        });

        it('skips supports where end_index exceeds text length', () => {
            const text = 'Short text.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 1 })],
                grounding_supports: [{ start_index: 0, end_index: 99999, grounding_chunk_indices: [0] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            expect(result).not.toContain('citation-superscript');
        });

        it('skips supports with empty grounding_chunk_indices', () => {
            const text = 'No chunk indices here.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 1 })],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            expect(result).not.toContain('citation-superscript');
        });

        it('does not insert citation into code blocks', () => {
            const codeContent = 'nginx -t';
            const html = `<p>Run the command.</p><pre><code>${codeContent}</code></pre>`;
            const plainText = 'Run the command.' + codeContent;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 1 })],
                grounding_supports: [{
                    start_index: 16,
                    end_index: plainText.length,
                    grounding_chunk_indices: [0],
                }],
            });

            const result = handler.addInlineCitations(html, grounding);

            const container = document.createElement('div');
            container.innerHTML = result;
            const codeBlock = container.querySelector('pre code');
            expect(codeBlock.textContent).not.toContain('[1]');
        });

        it('returns html unchanged on internal error (graceful fallback)', () => {
            const html = '<p>Some text.</p>';
            const badGrounding = {
                grounding_used: true,
                sources: [makeSource()],
                grounding_supports: null,
            };

            const result = handler.addInlineCitations(html, badGrounding);

            expect(result).toBe(html);
        });
    });

    describe('renderSourcesPanel', () => {
        it('returns a div with class sources-panel', () => {
            const panel = handler.renderSourcesPanel([makeSource()]);
            expect(panel.tagName).toBe('DIV');
            expect(panel.className).toContain('sources-panel');
        });

        it('renders one source item per source', () => {
            const sources = [
                makeSource({ citation_num: 1, uri: 'https://a.com', domain: 'a.com', display_name: 'A' }),
                makeSource({ citation_num: 2, uri: 'https://b.com', domain: 'b.com', display_name: 'B' }),
            ];
            const panel = handler.renderSourcesPanel(sources);
            const items = panel.querySelectorAll('.source-item');
            expect(items.length).toBe(2);
        });

        it('renders sources-panel-header', () => {
            const panel = handler.renderSourcesPanel([makeSource()]);
            expect(panel.querySelector('.sources-panel-header')).not.toBeNull();
        });

        it('renders sources-panel-toggle', () => {
            const panel = handler.renderSourcesPanel([makeSource()]);
            expect(panel.querySelector('.sources-panel-toggle')).not.toBeNull();
        });

        it('renders sources count in header', () => {
            const sources = [
                makeSource(),
                makeSource({ citation_num: 2, uri: 'https://b.com', domain: 'b.com', display_name: 'B' }),
            ];
            const panel = handler.renderSourcesPanel(sources);
            expect(panel.querySelector('.sources-panel-count').textContent).toBe('2');
        });

        it('renders source domain text', () => {
            const source = makeSource({ domain: 'nginx.org' });
            const panel = handler.renderSourcesPanel([source]);
            expect(panel.innerHTML).toContain('nginx.org');
        });

        it('renders source citation number', () => {
            const source = makeSource({ citation_num: 3 });
            const panel = handler.renderSourcesPanel([source]);
            expect(panel.querySelector('.source-citation-number').textContent).toBe('3');
        });

        it('renders full_title when it differs from domain', () => {
            const source = makeSource({ full_title: 'Nginx Configuration Guide', domain: 'nginx.org', display_name: 'nginx.org' });
            const panel = handler.renderSourcesPanel([source]);
            expect(panel.innerHTML).toContain('Nginx Configuration Guide');
        });

        it('falls back to display_name when full_title equals domain', () => {
            const source = makeSource({ full_title: 'nginx.org', domain: 'nginx.org', display_name: 'nginx.org' });
            const panel = handler.renderSourcesPanel([source]);
            const title = panel.querySelector('.source-title');
            expect(title.textContent).toBe('nginx.org');
        });

        it('renders segments as source snippets when present', () => {
            const source = makeSource({ segments: ['This is a relevant excerpt from the page.'] });
            const panel = handler.renderSourcesPanel([source]);
            expect(panel.querySelector('.source-snippet')).not.toBeNull();
            expect(panel.innerHTML).toContain('This is a relevant excerpt from the page.');
        });

        it('renders no snippets when segments is empty', () => {
            const source = makeSource({ segments: [] });
            const panel = handler.renderSourcesPanel([source]);
            expect(panel.querySelector('.source-snippet')).toBeNull();
        });

        it('each source item links to the source uri', () => {
            const source = makeSource({ uri: 'https://docs.nginx.com/guide' });
            const panel = handler.renderSourcesPanel([source]);
            const link = panel.querySelector('.source-link');
            expect(link.href).toBe('https://docs.nginx.com/guide');
        });

        it('source link has target _blank', () => {
            const source = makeSource();
            const panel = handler.renderSourcesPanel([source]);
            const link = panel.querySelector('.source-link');
            expect(link.target).toBe('_blank');
        });

        it('source link rel contains noopener', () => {
            const source = makeSource();
            const panel = handler.renderSourcesPanel([source]);
            const link = panel.querySelector('.source-link');
            expect(link.rel).toContain('noopener');
        });

        it('sources list is initially hidden', () => {
            const panel = handler.renderSourcesPanel([makeSource()]);
            const list = panel.querySelector('.sources-list');
            expect(list.classList.contains('initially-hidden')).toBe(true);
        });

        it('clicking header toggles sources list visibility', () => {
            const panel = handler.renderSourcesPanel([makeSource()]);
            const header = panel.querySelector('.sources-panel-header');
            const list = panel.querySelector('.sources-list');

            header.click();
            expect(list.classList.contains('initially-hidden')).toBe(false);

            header.click();
            expect(list.classList.contains('initially-hidden')).toBe(true);
        });

        it('uses favicon_url when provided', () => {
            const source = makeSource({ favicon_url: 'https://cdn.example.com/favicon.ico' });
            const panel = handler.renderSourcesPanel([source]);
            const favicon = panel.querySelector('.source-favicon');
            expect(favicon.src).toBe('https://cdn.example.com/favicon.ico');
        });

        it('falls back to google favicon service when favicon_url is null', () => {
            const source = makeSource({ favicon_url: null, domain: 'nginx.org' });
            const panel = handler.renderSourcesPanel([source]);
            const favicon = panel.querySelector('.source-favicon');
            expect(favicon.src).toContain('google.com/s2/favicons');
            expect(favicon.src).toContain('nginx.org');
        });

        it('escapes XSS in source domain — no live script element injected', () => {
            const source = makeSource({ domain: '<script>alert(1)</script>' });
            const panel = handler.renderSourcesPanel([source]);
            expect(panel.querySelectorAll('script').length).toBe(0);
            const domainEl = panel.querySelector('.source-domain');
            expect(domainEl.textContent).toContain('<script>alert(1)</script>');
        });

        it('escapes XSS in source full_title — no event handler attribute injected', () => {
            const source = makeSource({ full_title: '<img src=x onerror=alert(1)>', domain: 'safe.com', display_name: 'safe.com' });
            const panel = handler.renderSourcesPanel([source]);
            const titleEl = panel.querySelector('.source-title');
            expect(titleEl.textContent).toBe('<img src=x onerror=alert(1)>');
            const imgs = panel.querySelectorAll('img');
            imgs.forEach(img => {
                expect(img.getAttribute('onerror')).toBeNull();
            });
        });
    });

    describe('CitationItem typed DOM round-trip', () => {
        it('citation-hover-trigger contains citation-data-item child elements after addInlineCitations', () => {
            const text = 'The system is stable.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 1 })],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            const container = document.createElement('div');
            container.innerHTML = result;
            const trigger = container.querySelector('.citation-hover-trigger');
            expect(trigger).not.toBeNull();

            const items = trigger.querySelectorAll('.citation-data-item');
            expect(items.length).toBe(1);
        });

        it('citation-data-item elements carry typed data-* attributes for all CitationItem fields', () => {
            const text = 'Nginx is a web server.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 5, uri: 'https://nginx.org/', domain: 'nginx.org', display_name: 'Nginx', full_title: 'Nginx Documentation' })],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            const container = document.createElement('div');
            container.innerHTML = result;
            const item = container.querySelector('.citation-data-item');
            expect(item).not.toBeNull();

            expect(item.dataset.citationNum).toBe('5');
            expect(item.dataset.uri).toBe('https://nginx.org/');
            expect(item.dataset.displayName).toBe('Nginx');
            expect(item.dataset.domain).toBe('nginx.org');
            expect(item.dataset.fullTitle).toBe('Nginx Documentation');
        });

        it('citation-hover-trigger has no data-citations attribute — no serialized blob on the DOM', () => {
            const text = 'The service runs well.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 2, display_name: 'Example' })],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            const container = document.createElement('div');
            container.innerHTML = result;
            const trigger = container.querySelector('.citation-hover-trigger');
            expect(trigger.dataset.citations).toBeUndefined();
        });

        it('typed DOM round-trip: data-* attributes restore all CitationItem fields correctly', () => {
            const text = 'Round-trip test sentence.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 3, uri: 'https://docs.example.com/', domain: 'docs.example.com', display_name: 'Docs', full_title: 'Documentation Site' })],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            const container = document.createElement('div');
            container.innerHTML = result;
            const el = container.querySelector('.citation-data-item');

            expect(el.dataset.citationNum).toBe('3');
            expect(el.dataset.uri).toBe('https://docs.example.com/');
            expect(el.dataset.domain).toBe('docs.example.com');
            expect(el.dataset.displayName).toBe('Docs');
            expect(el.dataset.fullTitle).toBe('Documentation Site');
        });

        it('multiple sources produce one citation-data-item element per source', () => {
            const text = 'Multiple sources confirm this.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [
                    makeSource({ citation_num: 1, uri: 'https://a.com', domain: 'a.com', display_name: 'A' }),
                    makeSource({ citation_num: 2, uri: 'https://b.com', domain: 'b.com', display_name: 'B' }),
                ],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0, 1] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            const container = document.createElement('div');
            container.innerHTML = result;
            const trigger = container.querySelector('.citation-hover-trigger');
            const items = trigger.querySelectorAll('.citation-data-item');

            expect(items.length).toBe(2);
            expect(items[0].dataset.domain).toBe('a.com');
            expect(items[1].dataset.domain).toBe('b.com');
        });

        it('citation-data-item with empty fullTitle sets data-full-title to empty string', () => {
            const text = 'No title source.';
            const html = `<p>${text}</p>`;
            const grounding = makeGrounding({
                sources: [makeSource({ citation_num: 1, full_title: null })],
                grounding_supports: [{ start_index: 0, end_index: text.length, grounding_chunk_indices: [0] }],
            });

            const result = handler.addInlineCitations(html, grounding);

            const container = document.createElement('div');
            container.innerHTML = result;
            const item = container.querySelector('.citation-data-item');
            expect(item.dataset.fullTitle).toBe('');
        });
    });

    describe('CitationSource typed model in renderSourcesPanel', () => {
        it('parses raw source objects via CitationSource.parse() and renders all fields correctly', () => {
            const raw = makeSource({
                citation_num: 7,
                uri: 'https://docs.nginx.com/',
                domain: 'docs.nginx.com',
                display_name: 'Nginx Docs',
                full_title: 'Nginx Official Documentation',
                favicon_url: 'https://cdn.example.com/fav.ico',
                segments: ['relevant excerpt'],
            });

            const panel = handler.renderSourcesPanel([raw]);

            expect(panel.querySelector('.source-citation-number').textContent).toBe('7');
            expect(panel.querySelector('.source-title').textContent).toBe('Nginx Official Documentation');
            expect(panel.querySelector('.source-domain').textContent).toContain('docs.nginx.com');
            expect(panel.querySelector('.source-favicon').src).toBe('https://cdn.example.com/fav.ico');
            expect(panel.querySelector('.source-snippet')).not.toBeNull();
            expect(panel.querySelector('.source-link').href).toBe('https://docs.nginx.com/');
        });

        it('strips unknown fields from raw source input — only declared CitationSource fields are used', () => {
            const raw = makeSource({
                citation_num: 2,
                uri: 'https://example.com/',
                domain: 'example.com',
                display_name: 'Example',
                injected_field: '<script>evil()</script>',
                __proto__: { polluted: true },
            });

            const panel = handler.renderSourcesPanel([raw]);

            expect(panel.innerHTML).not.toContain('injected_field');
            expect(panel.querySelectorAll('script').length).toBe(0);
        });

        it('renders source without full_title — falls back to display_name', () => {
            const raw = makeSource({ full_title: null, display_name: 'Fallback Name', domain: 'fallback.com' });
            const panel = handler.renderSourcesPanel([raw]);
            expect(panel.querySelector('.source-title').textContent).toBe('Fallback Name');
        });

        it('renders multiple segments as individual source-snippet elements', () => {
            const raw = makeSource({ segments: ['first excerpt', 'second excerpt'] });
            const panel = handler.renderSourcesPanel([raw]);
            const snippets = panel.querySelectorAll('.source-snippet');
            expect(snippets.length).toBe(2);
            expect(snippets[0].textContent).toContain('first excerpt');
            expect(snippets[1].textContent).toContain('second excerpt');
        });
    });
});

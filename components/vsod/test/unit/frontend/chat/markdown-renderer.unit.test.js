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
import markdownitFactory from 'markdown-it';
import domPurify from 'dompurify';
import highlightJs from 'highlight.js';
import { MarkdownRenderer } from '@vsod/public/js/utils/markdown.js';

function makeRenderer() {
    return new MarkdownRenderer({ markdownitFactory, domPurify, highlightJs });
}

describe('MarkdownRenderer [FRONTEND - jsdom]', () => {

    describe('constructor', () => {
        it('constructs successfully with real vendor deps injected', () => {
            expect(() => makeRenderer()).not.toThrow();
        });

        it('throws when markdownit is not provided and global is absent', () => {
            expect(() => new MarkdownRenderer({ markdownitFactory: null })).toThrow('markdown-it is required');
        });

        it('constructs without domPurify and still renders', () => {
            const renderer = new MarkdownRenderer({ markdownitFactory, domPurify: null, highlightJs: null });
            expect(renderer.parseMarkdown('**hello**')).toContain('hello');
        });
    });

    describe('parseMarkdown — inline formatting', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('returns empty string for empty input', () => {
            expect(renderer.parseMarkdown('')).toBe('');
        });

        it('returns empty string for null input', () => {
            expect(renderer.parseMarkdown(null)).toBe('');
        });

        it('renders bold to <strong class="markdown-bold">', () => {
            const html = renderer.parseMarkdown('**bold**');
            expect(html).toContain('<strong class="markdown-bold">');
            expect(html).toContain('bold');
        });

        it('renders italic to <em class="markdown-italic">', () => {
            const html = renderer.parseMarkdown('_italic_');
            expect(html).toContain('<em class="markdown-italic">');
        });

        it('renders inline code to <code class="inline-code">', () => {
            const html = renderer.parseMarkdown('`code`');
            expect(html).toContain('<code class="inline-code">');
            expect(html).toContain('code');
        });

        it('appends streaming cursor span when streaming=true', () => {
            const html = renderer.parseMarkdown('hello', true);
            expect(html).toContain('<span class="streaming-cursor">');
        });

        it('does not append streaming cursor when streaming=false', () => {
            const html = renderer.parseMarkdown('hello', false);
            expect(html).not.toContain('streaming-cursor');
        });

        it('does not append streaming cursor when streaming omitted', () => {
            const html = renderer.parseMarkdown('hello');
            expect(html).not.toContain('streaming-cursor');
        });
    });

    describe('parseMarkdown — headings', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('renders h1 with markdown-heading and markdown-h1 classes', () => {
            const html = renderer.parseMarkdown('# Heading One');
            expect(html).toContain('<h1');
            expect(html).toContain('markdown-heading');
            expect(html).toContain('markdown-h1');
        });

        it('renders h2 with markdown-h2 class', () => {
            const html = renderer.parseMarkdown('## Heading Two');
            expect(html).toContain('<h2');
            expect(html).toContain('markdown-h2');
        });

        it('renders h3 with markdown-h3 class', () => {
            const html = renderer.parseMarkdown('### Heading Three');
            expect(html).toContain('<h3');
            expect(html).toContain('markdown-h3');
        });
    });

    describe('parseMarkdown — links', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('renders link with markdown-link class, target="_blank", and rel="noopener noreferrer"', () => {
            const html = renderer.parseMarkdown('[text](https://example.com)');
            expect(html).toContain('markdown-link');
            expect(html).toContain('target="_blank"');
            expect(html).toContain('noopener noreferrer');
            expect(html).toContain('href="https://example.com"');
        });

        it('renders link text content', () => {
            const html = renderer.parseMarkdown('[click here](https://example.com)');
            expect(html).toContain('click here');
        });
    });

    describe('parseMarkdown — lists', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('renders unordered list with markdown-list class', () => {
            const html = renderer.parseMarkdown('- item one\n- item two');
            expect(html).toContain('markdown-list');
            expect(html).toContain('<ul');
        });

        it('renders ordered list with markdown-list class', () => {
            const html = renderer.parseMarkdown('1. first\n2. second');
            expect(html).toContain('markdown-list');
            expect(html).toContain('<ol');
        });

        it('renders list items with markdown-list-item class', () => {
            const html = renderer.parseMarkdown('- item');
            expect(html).toContain('markdown-list-item');
        });
    });

    describe('parseMarkdown — paragraphs', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('renders paragraph with markdown-paragraph class', () => {
            const html = renderer.parseMarkdown('plain text paragraph');
            expect(html).toContain('<p class="markdown-paragraph">');
        });
    });

    describe('parseMarkdown — tables', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('renders table wrapped in markdown-table-wrapper with markdown-table class', () => {
            const md = '| A | B |\n|---|---|\n| 1 | 2 |';
            const html = renderer.parseMarkdown(md);
            expect(html).toContain('markdown-table-wrapper');
            expect(html).toContain('markdown-table');
        });

        it('renders thead with markdown-table-head class', () => {
            const md = '| A | B |\n|---|---|\n| 1 | 2 |';
            const html = renderer.parseMarkdown(md);
            expect(html).toContain('markdown-table-head');
        });

        it('renders tbody with markdown-table-body class', () => {
            const md = '| A | B |\n|---|---|\n| 1 | 2 |';
            const html = renderer.parseMarkdown(md);
            expect(html).toContain('markdown-table-body');
        });

        it('renders th cells with markdown-table-header-cell class', () => {
            const md = '| A | B |\n|---|---|\n| 1 | 2 |';
            const html = renderer.parseMarkdown(md);
            expect(html).toContain('markdown-table-header-cell');
        });

        it('renders td cells with markdown-table-cell class', () => {
            const md = '| A | B |\n|---|---|\n| 1 | 2 |';
            const html = renderer.parseMarkdown(md);
            expect(html).toContain('markdown-table-cell');
        });

        it('renders table rows with markdown-table-row class', () => {
            const md = '| A | B |\n|---|---|\n| 1 | 2 |';
            const html = renderer.parseMarkdown(md);
            expect(html).toContain('markdown-table-row');
        });
    });

    describe('parseMarkdown — fenced code blocks', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('renders fenced code block with code-block-wrapper class', () => {
            const html = renderer.parseMarkdown('```javascript\nconst x = 1;\n```');
            expect(html).toContain('code-block-wrapper');
        });

        it('includes language label in code-block-header for named language', () => {
            const html = renderer.parseMarkdown('```javascript\nconst x = 1;\n```');
            expect(html).toContain('javascript');
            expect(html).toContain('code-lang-label');
        });

        it('renders fenced code block without language without lang label span', () => {
            const html = renderer.parseMarkdown('```\nplain text\n```');
            expect(html).toContain('code-block-wrapper');
            expect(html).not.toContain('code-lang-label');
        });

        it('includes copy button with code-copy-btn class', () => {
            const html = renderer.parseMarkdown('```python\nprint("hi")\n```');
            expect(html).toContain('code-copy-btn');
            expect(html).toContain('data-code=');
        });

        it('renders mermaid fence as mermaid-wrapper with DIAGRAM label', () => {
            const html = renderer.parseMarkdown('```mermaid\ngraph TD\nA --> B\n```');
            expect(html).toContain('mermaid-wrapper');
            expect(html).toContain('DIAGRAM');
        });

        it('renders python code block with hljs language class', () => {
            const html = renderer.parseMarkdown('```python\nprint("hello")\n```');
            expect(html).toContain('language-python');
        });
    });

    describe('parseMarkdown — DOMPurify sanitization', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('strips <script> tags', () => {
            const html = renderer.parseMarkdown('<script>alert(1)</script>');
            expect(html).not.toContain('<script');
        });

        it('entity-escapes raw inline HTML when html option is false (no live event handlers)', () => {
            const html = renderer.parseMarkdown('<div onclick="alert(1)">x</div>');
            expect(html).not.toContain('<div');
            expect(html).toContain('&lt;div');
        });

        it('entity-escapes raw inline HTML tags containing onerror', () => {
            const html = renderer.parseMarkdown('<img src="x" onerror="alert(1)">');
            expect(html).not.toContain('<img');
            expect(html).toContain('&lt;img');
        });
    });

    describe('escapeHtml', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('escapes ampersand', () => {
            expect(renderer.escapeHtml('a & b')).toBe('a &amp; b');
        });

        it('escapes less-than', () => {
            expect(renderer.escapeHtml('a < b')).toBe('a &lt; b');
        });

        it('escapes greater-than', () => {
            expect(renderer.escapeHtml('a > b')).toBe('a &gt; b');
        });

        it('escapes all HTML special characters in one string', () => {
            expect(renderer.escapeHtml('a & b < c > d')).toBe('a &amp; b &lt; c &gt; d');
        });

        it('returns empty string for empty input', () => {
            expect(renderer.escapeHtml('')).toBe('');
        });
    });

    describe('escapeAttr', () => {
        let renderer;

        beforeEach(() => {
            renderer = makeRenderer();
        });

        it('escapes ampersand to &amp;', () => {
            expect(renderer.escapeAttr('a & b')).toBe('a &amp; b');
        });

        it('escapes double quote to &quot;', () => {
            expect(renderer.escapeAttr('"quoted"')).toBe('&quot;quoted&quot;');
        });

        it('escapes single quote to &#39;', () => {
            expect(renderer.escapeAttr("it's")).toBe('it&#39;s');
        });

        it('escapes less-than to &lt;', () => {
            expect(renderer.escapeAttr('<tag>')).toContain('&lt;');
        });

        it('escapes greater-than to &gt;', () => {
            expect(renderer.escapeAttr('<tag>')).toContain('&gt;');
        });

        it('escapes all attribute special characters in one string', () => {
            expect(renderer.escapeAttr('& " \' < >')).toBe('&amp; &quot; &#39; &lt; &gt;');
        });

        it('returns empty string for empty input', () => {
            expect(renderer.escapeAttr('')).toBe('');
        });
    });
});

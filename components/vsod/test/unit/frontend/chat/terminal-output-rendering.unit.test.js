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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import markdownitFactory from 'markdown-it';
import domPurifyImpl from 'dompurify';
import { MockEventBus, MockTemplateLoader } from '@test/mocks/mock-browser-env.js';
import { TEMPLATE_FIXTURES } from '@test/fixtures/templates.fixture.js';
import { EventType } from '@vsod/public/js/constants/events.js';

const seededLoader = new MockTemplateLoader();
Object.keys(TEMPLATE_FIXTURES).forEach(name => {
    seededLoader.seed(name, TEMPLATE_FIXTURES[name]);
});

const WEB_SESSION_ID = 'session-output-abc';

function buildDOM() {
    document.body.innerHTML = `
        <div id="anchored-terminal-container"></div>
        <div id="anchored-terminal">
            <div class="anchored-terminal__input-area"></div>
        </div>
        <div id="anchored-terminal-output"></div>
        <textarea id="anchored-terminal-input"></textarea>
        <button id="anchored-terminal-send"></button>
        <span id="anchored-terminal-hostname"></span>
        <span id="anchored-terminal-prompt"></span>
        <button id="anchored-terminal-attach"></button>
        <div id="anchored-terminal-attachments"></div>
        <span id="anchored-terminal-mode"></span>
        <div id="panel-resize-handle"></div>
        <button id="anchored-terminal-maximize"></button>
        <div id="anchored-terminal-body" style="height:400px;overflow:auto;"></div>
    `;
}

let mockTemplateLoader;

function installGlobals() {
    global.markdownit = markdownitFactory;
    global.DOMPurify = domPurifyImpl;
    mockTemplateLoader = seededLoader;
}

function cleanupGlobals() {
    delete global.markdownit;
    delete global.DOMPurify;
}

vi.mock('@vsod/public/js/utils/web-session-service.js', () => ({
    webSessionService: {
        getWebSessionId: vi.fn().mockReturnValue(null),
        getSession: vi.fn().mockReturnValue(null),
        isAuthenticated: vi.fn().mockReturnValue(false),
    }
}));

vi.mock('@vsod/public/js/utils/template-loader.js', () => ({
    templateLoader: seededLoader,
}));

async function makeTerminal(eventBus) {
    const { AnchoredOperatorTerminal } = await import('@vsod/public/js/components/anchored-terminal.js');
    const t = new AnchoredOperatorTerminal(eventBus);
    t.cacheDOMReferences();
    t.thinkingContentRaw = new Map();
    t.escapeHtml = (text) => {
        if (!text) return '';
        const d = document.createElement('div');
        d.textContent = text;
        return d.innerHTML;
    };
    t.formatTimestamp = () => '12:00:00 AM';
    t.scrollToBottom = vi.fn();
    t.initScrollState = vi.fn();
    t.initOperatorState = vi.fn();
    t.initExecutionState = vi.fn();
    return t;
}

describe('TerminalOutputMixin — DOM rendering [FRONTEND - jsdom]', () => {
    let eventBus;
    let terminal;

    beforeEach(async () => {
        buildDOM();
        installGlobals();

        eventBus = new MockEventBus();
        terminal = await makeTerminal(eventBus);
    });

    afterEach(() => {
        vi.clearAllMocks();
        eventBus.removeAllListeners();
        cleanupGlobals();
        document.body.innerHTML = '';
    });

    describe('appendUserMessage', () => {
        it('appends a user message element to the output container', () => {
            terminal.appendUserMessage('hello world', '05:00:00 AM');

            const entry = terminal.outputContainer.querySelector('.anchored-terminal__user-message');
            expect(entry).not.toBeNull();
        });

        it('sets the text content to the message', () => {
            terminal.appendUserMessage('good morning', '05:00:00 AM');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__user-message-content');
            expect(content.textContent).toBe('good morning');
        });

        it('displays the provided timestamp', () => {
            terminal.appendUserMessage('hello', '09:30:00 AM');

            const time = terminal.outputContainer.querySelector('.anchored-terminal__user-message-time');
            expect(time.textContent).toBe('09:30:00 AM');
        });

        it('uses formatTimestamp() when no timestamp is provided', () => {
            terminal.formatTimestamp = () => 'mock-time';
            terminal.appendUserMessage('hello');

            const time = terminal.outputContainer.querySelector('.anchored-terminal__user-message-time');
            expect(time.textContent).toBe('mock-time');
        });

        it('displays sender label "You"', () => {
            terminal.appendUserMessage('hi');

            const sender = terminal.outputContainer.querySelector('.anchored-terminal__user-message-sender');
            expect(sender.textContent).toBe('You');
        });

        it('returns the created DOM element', () => {
            const el = terminal.appendUserMessage('test');

            expect(el).not.toBeNull();
            expect(el.classList.contains('anchored-terminal__user-message')).toBe(true);
        });

        it('calls scrollToBottom', () => {
            terminal.appendUserMessage('test');

            expect(terminal.scrollToBottom).toHaveBeenCalled();
        });

        it('removes the welcome element if present', () => {
            const welcome = document.createElement('div');
            welcome.className = 'anchored-terminal__welcome';
            terminal.outputContainer.appendChild(welcome);

            terminal.appendUserMessage('test');

            expect(terminal.outputContainer.querySelector('.anchored-terminal__welcome')).toBeNull();
        });
    });

    describe('getOrCreateAIResponse', () => {
        it('creates a new streaming response element on first call', () => {
            terminal.getOrCreateAIResponse(WEB_SESSION_ID);

            const el = document.getElementById(`ai-response-${WEB_SESSION_ID}`);
            expect(el).not.toBeNull();
        });

        it('applies the "streaming" class to the new element', () => {
            terminal.getOrCreateAIResponse(WEB_SESSION_ID);

            const el = document.getElementById(`ai-response-${WEB_SESSION_ID}`);
            expect(el.classList.contains('streaming')).toBe(true);
        });

        it('returns the existing element on subsequent calls — no duplicates', () => {
            const first = terminal.getOrCreateAIResponse(WEB_SESSION_ID);
            const second = terminal.getOrCreateAIResponse(WEB_SESSION_ID);

            expect(first).toBe(second);
            const all = terminal.outputContainer.querySelectorAll(`#ai-response-${WEB_SESSION_ID}`);
            expect(all.length).toBe(1);
        });

        it('creates independent elements for different session IDs', () => {
            terminal.getOrCreateAIResponse('session-A');
            terminal.getOrCreateAIResponse('session-B');

            expect(document.getElementById('ai-response-session-A')).not.toBeNull();
            expect(document.getElementById('ai-response-session-B')).not.toBeNull();
        });
    });

    describe('appendStreamingTextChunk', () => {
        it('appends text nodes to the content element', () => {
            terminal.appendStreamingTextChunk(WEB_SESSION_ID, 'Hello');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-content');
            expect(content.textContent).toBe('Hello');
        });

        it('accumulates text on subsequent calls (not replacement)', () => {
            terminal.appendStreamingTextChunk(WEB_SESSION_ID, 'Hello');
            terminal.appendStreamingTextChunk(WEB_SESSION_ID, ' world');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-content');
            expect(content.textContent).toBe('Hello world');
        });

        it('does not render HTML markup (text nodes, not innerHTML)', () => {
            terminal.appendStreamingTextChunk(WEB_SESSION_ID, '<strong>bold</strong>');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-content');
            expect(content.innerHTML).toBe('&lt;strong&gt;bold&lt;/strong&gt;');
            expect(content.querySelector('strong')).toBeNull();
        });

        it('calls scrollToBottom after each chunk', () => {
            terminal.scrollToBottom.mockClear();
            terminal.appendStreamingTextChunk(WEB_SESSION_ID, 'chunk');

            expect(terminal.scrollToBottom).toHaveBeenCalled();
        });

        it('removes the waiting indicator when creating the AI response entry', () => {
            const indicator = document.createElement('div');
            indicator.id = 'waiting-indicator';
            terminal.outputContainer.appendChild(indicator);

            terminal.appendStreamingTextChunk(WEB_SESSION_ID, 'text');

            expect(document.getElementById('waiting-indicator')).toBeNull();
        });
    });

    describe('replaceStreamingHtml', () => {
        it('sets innerHTML of the content element to the provided HTML', () => {
            terminal.replaceStreamingHtml(WEB_SESSION_ID, '<p>Good morning</p>');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-content');
            expect(content.innerHTML).toBe('<p>Good morning</p>');
        });

        it('replaces content on subsequent calls (full re-render, not accumulation)', () => {
            terminal.replaceStreamingHtml(WEB_SESSION_ID, '<p>first</p>');
            terminal.replaceStreamingHtml(WEB_SESSION_ID, '<p>first second</p>');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-content');
            expect(content.innerHTML).toBe('<p>first second</p>');
        });

        it('renders HTML markup (not escaped)', () => {
            terminal.replaceStreamingHtml(WEB_SESSION_ID, '<strong>bold</strong>');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-content');
            expect(content.querySelector('strong')).not.toBeNull();
        });

        it('calls scrollToBottom after each chunk', () => {
            terminal.scrollToBottom.mockClear();
            terminal.replaceStreamingHtml(WEB_SESSION_ID, '<p>chunk</p>');

            expect(terminal.scrollToBottom).toHaveBeenCalled();
        });

        it('removes the waiting indicator when creating the AI response entry', () => {
            const indicator = document.createElement('div');
            indicator.id = 'waiting-indicator';
            terminal.outputContainer.appendChild(indicator);

            terminal.replaceStreamingHtml(WEB_SESSION_ID, '<p>text</p>');

            expect(document.getElementById('waiting-indicator')).toBeNull();
        });
    });

    describe('appendDirectHtmlResponse', () => {
        it('appends an AI response element to the output container', () => {
            terminal.appendDirectHtmlResponse('<p>Full response</p>');

            const entry = terminal.outputContainer.querySelector('.anchored-terminal__ai-response');
            expect(entry).not.toBeNull();
        });

        it('sets innerHTML of the content to the provided HTML', () => {
            terminal.appendDirectHtmlResponse('<p>Hello there</p>');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-content');
            expect(content.innerHTML).toContain('Hello there');
        });

        it('does NOT have the "streaming" class', () => {
            terminal.appendDirectHtmlResponse('<p>done</p>');

            const entry = terminal.outputContainer.querySelector('.anchored-terminal__ai-response');
            expect(entry.classList.contains('streaming')).toBe(false);
        });

        it('shows sender label "g8e.local"', () => {
            terminal.appendDirectHtmlResponse('<p>ok</p>');

            const sender = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-sender');
            expect(sender.textContent).toBe('g8e');
        });

        it('uses the provided timestamp', () => {
            terminal.appendDirectHtmlResponse('<p>ok</p>', '03:45:00 PM');

            const time = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-time');
            expect(time.textContent).toBe('03:45:00 PM');
        });

        it('uses formatTimestamp when timestamp is null', () => {
            terminal.formatTimestamp = () => 'fallback-time';
            terminal.appendDirectHtmlResponse('<p>ok</p>', null);

            const time = terminal.outputContainer.querySelector('.anchored-terminal__ai-response-time');
            expect(time.textContent).toBe('fallback-time');
        });

        it('returns the created DOM element', () => {
            const el = terminal.appendDirectHtmlResponse('<p>ok</p>');

            expect(el).not.toBeNull();
            expect(el.classList.contains('anchored-terminal__ai-response')).toBe(true);
        });

        it('appends multiple responses in order', () => {
            terminal.appendDirectHtmlResponse('<p>first</p>');
            terminal.appendDirectHtmlResponse('<p>second</p>');

            const entries = terminal.outputContainer.querySelectorAll('.anchored-terminal__ai-response');
            expect(entries.length).toBe(2);
        });

        it('applies inline citations when grounding_used is true', () => {
            const citationsHandler = {
                addInlineCitations: vi.fn((html) => html + '<!-- cited -->'),
                renderSourcesPanel: vi.fn(() => {
                    const el = document.createElement('div');
                    el.className = 'sources-panel';
                    return el;
                }),
            };
            terminal.citationsHandler = citationsHandler;

            const metadata = {
                grounding_used: true,
                sources: [{ url: 'https://example.com', title: 'Example' }],
            };
            terminal.appendDirectHtmlResponse('<p>text</p>', null, metadata);

            expect(citationsHandler.addInlineCitations).toHaveBeenCalledOnce();
            expect(citationsHandler.renderSourcesPanel).toHaveBeenCalledOnce();
        });

        it('does not call citation handler when grounding_used is false', () => {
            const citationsHandler = {
                addInlineCitations: vi.fn(),
                renderSourcesPanel: vi.fn(),
            };
            terminal.citationsHandler = citationsHandler;

            terminal.appendDirectHtmlResponse('<p>text</p>', null, { grounding_used: false, sources: [] });

            expect(citationsHandler.addInlineCitations).not.toHaveBeenCalled();
        });

        it('does not call citation handler when groundingMetadata is null', () => {
            const citationsHandler = {
                addInlineCitations: vi.fn(),
                renderSourcesPanel: vi.fn(),
            };
            terminal.citationsHandler = citationsHandler;

            terminal.appendDirectHtmlResponse('<p>text</p>', null, null);

            expect(citationsHandler.addInlineCitations).not.toHaveBeenCalled();
        });
    });

    describe('appendSystemMessage', () => {
        it('appends a system message entry', () => {
            terminal.appendSystemMessage('Operator connected');

            const entry = terminal.outputContainer.querySelector('.anchored-terminal__entry');
            expect(entry).not.toBeNull();
        });

        it('includes the message text', () => {
            terminal.appendSystemMessage('Hello system');

            const msg = terminal.outputContainer.querySelector('.system-message');
            expect(msg.textContent).toContain('Hello system');
        });

        it('returns the created entry element', () => {
            const el = terminal.appendSystemMessage('test');

            expect(el).not.toBeNull();
        });
    });

    describe('appendErrorMessage', () => {
        it('appends an error message element', () => {
            terminal.appendErrorMessage('Something went wrong');

            const entry = terminal.outputContainer.querySelector('.anchored-terminal__error-message');
            expect(entry).not.toBeNull();
        });

        it('includes the error text in the content element', () => {
            terminal.appendErrorMessage('Auth failed');

            const content = terminal.outputContainer.querySelector('.anchored-terminal__error-content');
            expect(content.textContent).toBe('Auth failed');
        });

        it('includes an error header element', () => {
            terminal.appendErrorMessage('err');

            const header = terminal.outputContainer.querySelector('.anchored-terminal__error-header');
            expect(header).not.toBeNull();
        });

        it('does not throw when outputContainer is null', () => {
            terminal.outputContainer = null;

            expect(() => terminal.appendErrorMessage('err')).not.toThrow();
        });
    });

    describe('appendThinkingContent', () => {
        it('creates a thinking entry on first call', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, 'thinking...');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            expect(entry).not.toBeNull();
        });

        it('applies the "active" class to the thinking entry', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, 'thinking...');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            expect(entry.classList.contains('active')).toBe(true);
        });

        it('accumulates text across calls with a newline separator', () => {
            terminal.markdownRenderer = { parseMarkdown: (text) => `<p>${text}</p>` };
            terminal.appendThinkingContent(WEB_SESSION_ID, 'line one');
            terminal.appendThinkingContent(WEB_SESSION_ID, 'line two');

            expect(terminal.thinkingContentRaw.get(WEB_SESSION_ID)).toBe('line one\nline two');
        });

        it('reuses the same entry element on subsequent calls', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, 'a');
            terminal.appendThinkingContent(WEB_SESSION_ID, 'b');

            const entries = terminal.outputContainer.querySelectorAll(`#thinking-${WEB_SESSION_ID}`);
            expect(entries.length).toBe(1);
        });

        it('uses textContent fallback when markdownRenderer is absent', () => {
            terminal.markdownRenderer = null;
            terminal.appendThinkingContent(WEB_SESSION_ID, 'raw text');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            const content = entry.querySelector('.anchored-terminal__thinking-content');
            expect(content.textContent).toBe('raw text');
        });

        it('updates the title from a bold markdown heading', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, '**Revising Temperature Defaults**\nSome content here');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            const title = entry.querySelector('.anchored-terminal__thinking-title');
            expect(title.textContent).toBe('Revising Temperature Defaults');
        });

        it('updates the title from a markdown heading', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, '## Clarifying Configuration Strategy\nDetails');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            const title = entry.querySelector('.anchored-terminal__thinking-title');
            expect(title.textContent).toBe('Clarifying Configuration Strategy');
        });

        it('replaces the title with the latest heading as new chunks arrive', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, '**Revising Temperature Defaults**\nFirst analysis');
            terminal.appendThinkingContent(WEB_SESSION_ID, '**Clarifying Configuration Strategy**\nSecond analysis');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            const title = entry.querySelector('.anchored-terminal__thinking-title');
            expect(title.textContent).toBe('Clarifying Configuration Strategy');
        });

        it('keeps "Thinking..." when no markdown title is present', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, 'plain text without any heading');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            const title = entry.querySelector('.anchored-terminal__thinking-title');
            expect(title.textContent).toBe('Thinking...');
        });

        it('creates header with toggle and title elements', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, 'text');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            expect(entry.querySelector('.anchored-terminal__thinking-header')).not.toBeNull();
            expect(entry.querySelector('.anchored-terminal__thinking-toggle')).not.toBeNull();
            expect(entry.querySelector('.anchored-terminal__thinking-title')).not.toBeNull();
        });

        it('toggle starts as dash when entry is active', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, 'text');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            const toggle = entry.querySelector('.anchored-terminal__thinking-toggle');
            expect(toggle.textContent).toBe('\u2013');
        });

        it('clicking header toggles collapsed state and toggle text', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, 'text');

            const entry = document.getElementById(`thinking-${WEB_SESSION_ID}`);
            const header = entry.querySelector('.anchored-terminal__thinking-header');
            const toggle = entry.querySelector('.anchored-terminal__thinking-toggle');

            header.click();
            expect(entry.classList.contains('collapsed')).toBe(true);
            expect(toggle.textContent).toBe('+');

            header.click();
            expect(entry.classList.contains('collapsed')).toBe(false);
            expect(toggle.textContent).toBe('\u2013');
        });
    });

    describe('completeThinkingEntry', () => {
        beforeEach(() => {
            terminal.appendThinkingContent(WEB_SESSION_ID, 'thought');
        });

        it('removes the "active" class', () => {
            terminal.completeThinkingEntry(WEB_SESSION_ID);

            const entries = terminal.outputContainer.querySelectorAll(`[id^="thinking-${WEB_SESSION_ID}"]`);
            const stillActive = [...entries].filter(e => e.classList.contains('active'));
            expect(stillActive).toHaveLength(0);
        });

        it('adds the "collapsed" class', () => {
            terminal.completeThinkingEntry(WEB_SESSION_ID);

            const entries = terminal.outputContainer.querySelectorAll(`[id^="thinking-${WEB_SESSION_ID}"]`);
            expect(entries.length).toBeGreaterThan(0);
            expect(entries[0].classList.contains('collapsed')).toBe(true);
        });

        it('renames the element ID so the next turn can create a fresh entry', () => {
            terminal.completeThinkingEntry(WEB_SESSION_ID);

            expect(document.getElementById(`thinking-${WEB_SESSION_ID}`)).toBeNull();
        });

        it('clears raw thinking content from the map', () => {
            terminal.completeThinkingEntry(WEB_SESSION_ID);

            expect(terminal.thinkingContentRaw.has(WEB_SESSION_ID)).toBe(false);
        });

        it('does not throw when entry does not exist', () => {
            expect(() => terminal.completeThinkingEntry('ghost-session')).not.toThrow();
        });

        it('sets the toggle indicator to +', () => {
            terminal.completeThinkingEntry(WEB_SESSION_ID);

            const entries = terminal.outputContainer.querySelectorAll(`[id^="thinking-${WEB_SESSION_ID}"]`);
            const toggle = entries[0].querySelector('.anchored-terminal__thinking-toggle');
            expect(toggle.textContent).toBe('+');
        });

        it('preserves the last title in the header', () => {
            terminal.appendThinkingContent(WEB_SESSION_ID, '**Examining Setting Resolution**\nContent');
            terminal.completeThinkingEntry(WEB_SESSION_ID);

            const entries = terminal.outputContainer.querySelectorAll(`[id^="thinking-${WEB_SESSION_ID}"]`);
            const title = entries[0].querySelector('.anchored-terminal__thinking-title');
            expect(title.textContent).toBe('Examining Setting Resolution');
        });
    });

    describe('_extractThinkingTitle', () => {
        it('extracts title from bold markdown', () => {
            expect(terminal._extractThinkingTitle('**Revising Temperature Defaults**')).toBe('Revising Temperature Defaults');
        });

        it('extracts title from h2 heading', () => {
            expect(terminal._extractThinkingTitle('## Analysis Phase')).toBe('Analysis Phase');
        });

        it('extracts title from h1 heading', () => {
            expect(terminal._extractThinkingTitle('# Top Level')).toBe('Top Level');
        });

        it('extracts title from h3 heading', () => {
            expect(terminal._extractThinkingTitle('### Sub Section')).toBe('Sub Section');
        });

        it('returns the last title when multiple are present', () => {
            const text = '**First Title**\nSome text\n**Second Title**\nMore text';
            expect(terminal._extractThinkingTitle(text)).toBe('Second Title');
        });

        it('returns null when no title pattern is found', () => {
            expect(terminal._extractThinkingTitle('just plain text without any heading')).toBeNull();
        });

        it('returns null for empty string', () => {
            expect(terminal._extractThinkingTitle('')).toBeNull();
        });

        it('does not match partial bold (bold text with trailing content)', () => {
            expect(terminal._extractThinkingTitle('**bold** and more')).toBeNull();
        });

        it('handles mixed headings and bold — returns last match', () => {
            const text = '## Heading One\nContent\n**Bold Title**\nMore';
            expect(terminal._extractThinkingTitle(text)).toBe('Bold Title');
        });
    });

    describe('applyCitations', () => {
        beforeEach(() => {
            terminal.getOrCreateAIResponse(WEB_SESSION_ID);
        });

        it('calls citationsHandler.addInlineCitations with entry HTML', () => {
            const citationsHandler = {
                addInlineCitations: vi.fn((html) => html + '<!-- cited -->'),
                renderSourcesPanel: vi.fn(() => document.createElement('div')),
            };
            terminal.citationsHandler = citationsHandler;

            const metadata = { grounding_used: true, sources: [{ url: 'x', title: 'X' }] };
            terminal.applyCitations(WEB_SESSION_ID, metadata);

            expect(citationsHandler.addInlineCitations).toHaveBeenCalledOnce();
        });

        it('calls citationsHandler.renderSourcesPanel with sources', () => {
            const sources = [{ url: 'https://a.com', title: 'A' }];
            const citationsHandler = {
                addInlineCitations: vi.fn((html) => html),
                renderSourcesPanel: vi.fn(() => document.createElement('div')),
            };
            terminal.citationsHandler = citationsHandler;

            terminal.applyCitations(WEB_SESSION_ID, { grounding_used: true, sources });

            expect(citationsHandler.renderSourcesPanel).toHaveBeenCalledWith(sources);
        });

        it('does nothing when grounding_used is false', () => {
            const citationsHandler = {
                addInlineCitations: vi.fn(),
                renderSourcesPanel: vi.fn(),
            };
            terminal.citationsHandler = citationsHandler;

            terminal.applyCitations(WEB_SESSION_ID, { grounding_used: false, sources: [] });

            expect(citationsHandler.addInlineCitations).not.toHaveBeenCalled();
        });

        it('does nothing when groundingMetadata is null', () => {
            const citationsHandler = {
                addInlineCitations: vi.fn(),
                renderSourcesPanel: vi.fn(),
            };
            terminal.citationsHandler = citationsHandler;

            terminal.applyCitations(WEB_SESSION_ID, null);

            expect(citationsHandler.addInlineCitations).not.toHaveBeenCalled();
        });

        it('does nothing when sources array is empty', () => {
            const citationsHandler = {
                addInlineCitations: vi.fn(),
                renderSourcesPanel: vi.fn(),
            };
            terminal.citationsHandler = citationsHandler;

            terminal.applyCitations(WEB_SESSION_ID, { grounding_used: true, sources: [] });

            expect(citationsHandler.addInlineCitations).not.toHaveBeenCalled();
        });

        it('falls back to searching by prefixed ID after finalizeAIResponseChunk renames the element', () => {
            terminal.finalizeAIResponseChunk(WEB_SESSION_ID, '<p>sealed</p>', null);

            const citationsHandler = {
                addInlineCitations: vi.fn((html) => html),
                renderSourcesPanel: vi.fn(() => document.createElement('div')),
            };
            terminal.citationsHandler = citationsHandler;

            const metadata = { grounding_used: true, sources: [{ url: 'x', title: 'X' }] };

            expect(() => terminal.applyCitations(WEB_SESSION_ID, metadata)).not.toThrow();
            expect(citationsHandler.addInlineCitations).toHaveBeenCalledOnce();
        });
    });

    describe('showWaitingIndicator', () => {
        it('creates a waiting indicator element', () => {
            terminal.showWaitingIndicator(WEB_SESSION_ID);

            const el = document.getElementById('waiting-indicator');
            expect(el).not.toBeNull();
        });

        it('sets the web-session-id data attribute', () => {
            terminal.showWaitingIndicator(WEB_SESSION_ID);

            const el = document.getElementById('waiting-indicator');
            expect(el.dataset.webSessionId).toBe(WEB_SESSION_ID);
        });

        it('replaces an existing indicator rather than duplicating', () => {
            terminal.showWaitingIndicator(WEB_SESSION_ID);
            terminal.showWaitingIndicator('other-session');

            const all = terminal.outputContainer.querySelectorAll('#waiting-indicator');
            expect(all.length).toBe(1);
        });
    });

    describe('showTribunal', () => {
        const WIDGET_ID = 'tribunal-show-test';
        const TEST_COMMAND = 'apt-get update && apt-get install -y tcpdump';

        it('renders the initial command literally via textContent', async () => {
            await terminal.showTribunal({ id: WIDGET_ID, model: 'test-model', numPasses: 3, command: TEST_COMMAND });

            const widget = document.getElementById(WIDGET_ID);
            const commandEl = widget.querySelector('.tribunal__command');

            expect(commandEl.textContent).toBe(TEST_COMMAND);
            // Check innerHTML to ensure no double escaping of &
            expect(commandEl.innerHTML).not.toContain('&amp;amp;');
        });
    });

    describe('hideWaitingIndicator', () => {
        it('removes the waiting indicator from the DOM', () => {
            terminal.showWaitingIndicator(WEB_SESSION_ID);
            terminal.hideWaitingIndicator();

            expect(document.getElementById('waiting-indicator')).toBeNull();
        });

        it('does not throw when no indicator exists', () => {
            expect(() => terminal.hideWaitingIndicator()).not.toThrow();
        });
    });

    describe('completeTribunal', () => {
        const WIDGET_ID = 'tribunal-test-widget-1';

        beforeEach(async () => {
            await terminal.showTribunal({ id: WIDGET_ID, model: 'test-model', numPasses: 3, command: 'ls -la' });
        });

        it('widget remains in the DOM after completion', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la', outcome: 'consensus' });

            expect(document.getElementById(WIDGET_ID)).not.toBeNull();
        });

        it('adds tribunal--done class', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la', outcome: 'consensus' });

            expect(document.getElementById(WIDGET_ID).classList.contains('tribunal--done')).toBe(true);
        });

        it('removes the spinner element', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la', outcome: 'consensus' });

            expect(document.getElementById(WIDGET_ID).querySelector('.tribunal__spinner')).toBeNull();
        });

        it('sets icon to check_circle', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la', outcome: 'consensus' });

            const icon = document.getElementById(WIDGET_ID).querySelector('.tribunal__icon');
            expect(icon.textContent).toBe('check_circle');
        });

        it('shows "Consensus" label for consensus outcome', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la', outcome: 'consensus' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toContain('Consensus');
        });

        it('shows "Revised" label for verification_failed outcome', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la', outcome: 'verification_failed' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toContain('Revised');
        });

        it('shows "Verified" label for verified outcome', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la', outcome: 'verified' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toContain('Verified');
        });

        it('includes the final command in the status text', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la /home', outcome: 'consensus' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toContain('ls -la /home');
        });

        it('renders the final command literally without double-escaping', () => {
            const complexCommand = 'apt-get update && apt-get install -y tcpdump';
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: complexCommand, outcome: 'verified' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toBe(`Verified · ${complexCommand}`);
            // Check innerHTML to ensure no double escaping of &
            // Literal & in textContent will be &amp; in innerHTML (normal browser behavior)
            // Double escaping would produce &amp;amp;
            expect(status.innerHTML).not.toContain('&amp;amp;');
        });

        it('adds tribunal__status--done class to status element', () => {
            terminal.completeTribunal({ id: WIDGET_ID, finalCommand: 'ls -la', outcome: 'consensus' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.classList.contains('tribunal__status--done')).toBe(true);
        });

        it('does not throw when widget does not exist', () => {
            expect(() => terminal.completeTribunal({ id: 'ghost-id', finalCommand: 'x', outcome: 'consensus' })).not.toThrow();
        });
    });

    describe('failTribunal', () => {
        const WIDGET_ID = 'tribunal-test-widget-2';

        beforeEach(async () => {
            await terminal.showTribunal({ id: WIDGET_ID, model: 'test-model', numPasses: 3, command: 'rm -rf /' });
        });

        it('widget remains in the DOM after fallback', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'disabled' });

            expect(document.getElementById(WIDGET_ID)).not.toBeNull();
        });

        it('adds tribunal--fallback class', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'disabled' });

            expect(document.getElementById(WIDGET_ID).classList.contains('tribunal--fallback')).toBe(true);
        });

        it('removes the spinner element', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'disabled' });

            expect(document.getElementById(WIDGET_ID).querySelector('.tribunal__spinner')).toBeNull();
        });

        it('sets icon to info', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'disabled' });

            const icon = document.getElementById(WIDGET_ID).querySelector('.tribunal__icon');
            expect(icon.textContent).toBe('info');
        });

        it('adds tribunal__icon--fallback class to icon', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'disabled' });

            const icon = document.getElementById(WIDGET_ID).querySelector('.tribunal__icon');
            expect(icon.classList.contains('tribunal__icon--fallback')).toBe(true);
        });

        it('shows "TribunalDisabled" for disabled reason', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'disabled' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toBe('TribunalDisabled');
        });

        it('shows "Tribunal unavailable" for provider_unavailable reason', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'provider_unavailable' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toBe('Tribunal unavailable');
        });

        it('shows correct label for all_passes_failed reason', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'all_passes_failed' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toBe('All passes failed \u2014 using original');
        });

        it('shows correct label for no_vote_winner reason', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'no_vote_winner' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toBe('No consensus \u2014 using original');
        });

        it('shows fallback label for unknown reason', () => {
            terminal.failTribunal({ id: WIDGET_ID, reason: 'something_unexpected' });

            const status = document.getElementById(WIDGET_ID).querySelector('.tribunal__status');
            expect(status.textContent).toBe('Using original command');
        });

        it('does not throw when widget does not exist', () => {
            expect(() => terminal.failTribunal({ id: 'ghost-id', reason: 'disabled' })).not.toThrow();
        });
    });

    describe('Approval Flow: Preparing to Requested', () => {
        it('hides "Preparing" indicator when approval.requested arrives after preparing', async () => {
            const executionId = 'exec-123';
            const command = 'ls -la';

            // First, simulate the PREPARING event
            await terminal.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
                execution_id: executionId,
                command: command
            });

            // Verify the preparing indicator was created
            const preparingIndicator = terminal.outputContainer.querySelector('.anchored-terminal__executing');
            expect(preparingIndicator).not.toBeNull();
            expect(preparingIndicator.textContent).toContain(command);
            expect(terminal.activeExecutions.has(executionId)).toBe(true);

            // Now simulate the REQUESTED event
            await terminal.handleApprovalRequest({
                execution_id: executionId,
                approval_id: 'approval-456',
                command: command,
                justification: 'Test justification'
            });

            // Verify the preparing indicator was removed
            const preparingIndicatorAfter = terminal.outputContainer.querySelector('.anchored-terminal__executing');
            expect(preparingIndicatorAfter).toBeNull();
            expect(terminal.activeExecutions.has(executionId)).toBe(false);

            // Verify the approval card was created
            const approvalCard = terminal.outputContainer.querySelector('.anchored-terminal__approval');
            expect(approvalCard).not.toBeNull();
            expect(approvalCard.dataset.approvalId).toBe('approval-456');
        });

        it('does not hide preparing indicator for different execution_id', async () => {
            const executionId1 = 'exec-123';
            const executionId2 = 'exec-456';
            const command = 'ls -la';

            // Create preparing indicator for execution 1
            await terminal.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
                execution_id: executionId1,
                command: command
            });

            // Verify preparing indicator exists
            const preparingIndicator = terminal.outputContainer.querySelector('.anchored-terminal__executing');
            expect(preparingIndicator).not.toBeNull();

            // Send approval request for different execution
            await terminal.handleApprovalRequest({
                execution_id: executionId2,
                approval_id: 'approval-789',
                command: 'different-command',
                justification: 'Test justification'
            });

            // Verify original preparing indicator is still there
            const preparingIndicatorAfter = terminal.outputContainer.querySelector('.anchored-terminal__executing');
            expect(preparingIndicatorAfter).not.toBeNull();
            expect(terminal.activeExecutions.has(executionId1)).toBe(true);
        });

        it('handles approval.requested without preceding preparing event', async () => {
            const executionId = 'exec-123';

            // Send approval request without preparing event
            await terminal.handleApprovalRequest({
                execution_id: executionId,
                approval_id: 'approval-456',
                command: 'ls -la',
                justification: 'Test justification'
            });

            // Should not throw and should create approval card
            const approvalCard = terminal.outputContainer.querySelector('.anchored-terminal__approval');
            expect(approvalCard).not.toBeNull();
            expect(approvalCard.dataset.approvalId).toBe('approval-456');
        });
    });
});

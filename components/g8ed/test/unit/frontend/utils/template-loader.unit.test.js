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
import { ComponentName } from '@g8ed/public/js/models/investigation-models.js';

let TemplateLoader;

beforeEach(async () => {
    vi.resetModules();
    ({ TemplateLoader } = await import('@g8ed/public/js/utils/template-loader.js'));
});

afterEach(() => {
    const loader = new TemplateLoader();
    loader.cache.clear();
    loader.loading.clear();
});

describe('TemplateLoader [UNIT - jsdom]', () => {
    describe('seed()', () => {
        it('stores html in the cache under the given name', () => {
            const loader = new TemplateLoader();
            loader.seed('my-template', '<div>hello</div>');
            expect(loader.cache.get('my-template')).toBe('<div>hello</div>');
        });

        it('seeded template is returned synchronously by load()', async () => {
            const loader = new TemplateLoader();
            loader.seed('approval-card', '<div class="approval">{{command}}</div>');
            const result = await loader.load('approval-card');
            expect(result).toBe('<div class="approval">{{command}}</div>');
        });

        it('seeding overwrites a previously seeded value', () => {
            const loader = new TemplateLoader();
            loader.seed('foo', 'v1');
            loader.seed('foo', 'v2');
            expect(loader.cache.get('foo')).toBe('v2');
        });

        it('seeded template is served without hitting the network', async () => {
            const transportGet = vi.fn();
            const loader = new TemplateLoader('/templates/', { get: transportGet });
            loader.seed('command-result', '<span>result</span>');
            await loader.load('command-result');
            expect(transportGet).not.toHaveBeenCalled();
        });

        it('cache.has() returns true after seeding', () => {
            const loader = new TemplateLoader();
            loader.seed('results-toggle', '<button>toggle</button>');
            expect(loader.cache.has('results-toggle')).toBe(true);
        });
    });

    describe('injectable transport', () => {
        it('uses the injected transport instead of window.serviceClient', async () => {
            const html = '<div class="approval-card">{{header}}</div>';
            const transportGet = vi.fn().mockResolvedValue({ text: async () => html });
            const loader = new TemplateLoader('/templates/', { get: transportGet });

            const result = await loader.load('approval-card');

            expect(transportGet).toHaveBeenCalledOnce();
            expect(result).toBe(html);
        });

        it('passes ComponentName.G8ED and the constructed path to the transport', async () => {
            const transportGet = vi.fn().mockResolvedValue({ text: async () => '<div/>' });
            const loader = new TemplateLoader('/js/components/templates/', { get: transportGet });

            await loader.load('approval-status');

            const [componentName, path] = transportGet.mock.calls[0];
            expect(componentName).toBe('g8ed');
            expect(path).toBe('/js/components/templates/approval-status.html');
        });

        it('caches the result from the transport so a second load() does not call transport again', async () => {
            const transportGet = vi.fn().mockResolvedValue({ text: async () => '<p>cached</p>' });
            const loader = new TemplateLoader('/templates/', { get: transportGet });

            await loader.load('my-tpl');
            await loader.load('my-tpl');

            expect(transportGet).toHaveBeenCalledOnce();
        });

        it('constructor with no transport falls back to window.serviceClient', async () => {
            const clientGet = vi.fn().mockResolvedValue({ text: async () => '<div/>' });
            window.serviceClient = { get: clientGet };

            const loader = new TemplateLoader();
            await loader.load('some-template');

            expect(clientGet).toHaveBeenCalledOnce();
            delete window.serviceClient;
        });
    });

    describe('in-flight deduplication', () => {
        it('concurrent load() calls for the same template only call the transport once', async () => {
            let resolveHtml;
            const htmlPromise = new Promise(r => { resolveHtml = r; });
            const transportGet = vi.fn().mockResolvedValue({ text: () => htmlPromise });
            const loader = new TemplateLoader('/templates/', { get: transportGet });

            const p1 = loader.load('dedup-tpl');
            const p2 = loader.load('dedup-tpl');
            resolveHtml('<div/>', undefined);

            await Promise.all([p1, p2]);

            expect(transportGet).toHaveBeenCalledOnce();
        });
    });

    describe('replace()', () => {
        it('replaces {{var}} with HTML-escaped value', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('<div>{{name}}</div>', { name: '<script>' });
            expect(result).toBe('<div>&lt;script&gt;</div>');
        });

        it('replaces {{{var}}} with raw unescaped value', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('<div>{{{html}}}</div>', { html: '<b>bold</b>' });
            expect(result).toBe('<div><b>bold</b></div>');
        });

        it('replaces {{!var}} with attribute-escaped value', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('<div data-id="{{!id}}">', { id: 'abc"def' });
            expect(result).toBe('<div data-id="abc&quot;def">');
        });

        it('replaces all occurrences of a placeholder', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('{{x}} and {{x}}', { x: 'hi' });
            expect(result).toBe('hi and hi');
        });

        it('leaves unreferenced placeholders untouched', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('{{a}} {{b}}', { a: 'A' });
            expect(result).toBe('A {{b}}');
        });

        it('coerces null/undefined values to empty string', () => {
            const loader = new TemplateLoader();
            expect(loader.replace('{{v}}', { v: null })).toBe('');
            expect(loader.replace('{{v}}', { v: undefined })).toBe('');
        });

        it('{{!var}} escapes single quotes to prevent attribute breakout', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('<div data-id="{{!id}}">', { id: "abc'def" });
            expect(result).toBe('<div data-id="abc&#39;def">');
        });

        it('{{!var}} escapes HTML entities in attributes', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('<div data-id="{{!id}}">', { id: '<script>' });
            expect(result).toBe('<div data-id="&lt;script&gt;">');
        });

        it('{{!var}} prevents XSS via onmouseover injection by escaping quotes', () => {
            const loader = new TemplateLoader();
            const malicious = 'abc" onmouseover="alert(1)';
            const result = loader.replace('<div data-id="{{!id}}">', { id: malicious });
            // Quotes are escaped, preventing attribute breakout
            expect(result).toContain('&quot;');
            expect(result).not.toContain('data-id="abc"');
            // The malicious payload is rendered as text, not executable
            expect(result).toContain('data-id="abc&quot; onmouseover=&quot;alert(1)"');
        });

        it('{{!var}} prevents XSS via onerror injection by escaping quotes', () => {
            const loader = new TemplateLoader();
            const malicious = 'abc" onerror="alert(1)';
            const result = loader.replace('<img src="{{!src}}">', { src: malicious });
            // Quotes are escaped, preventing attribute breakout
            expect(result).toContain('&quot;');
            expect(result).not.toContain('src="abc"');
            // The malicious payload is rendered as text, not executable
            expect(result).toContain('src="abc&quot; onerror=&quot;alert(1)"');
        });

        it('{{!var}} handles empty string safely', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('<div data-id="{{!id}}">', { id: '' });
            expect(result).toBe('<div data-id="">');
        });

        it('{{!var}} handles null safely', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('<div data-id="{{!id}}">', { id: null });
            expect(result).toBe('<div data-id="">');
        });

        it('{{!var}} handles undefined safely', () => {
            const loader = new TemplateLoader();
            const result = loader.replace('<div data-id="{{!id}}">', { id: undefined });
            expect(result).toBe('<div data-id="">');
        });

        it('{{!var}} in class attribute prevents class injection by escaping quotes', () => {
            const loader = new TemplateLoader();
            const malicious = 'class1" onclick="alert(1)';
            const result = loader.replace('<div class="{{!cls}}">', { cls: malicious });
            // Quotes are escaped, preventing attribute breakout
            expect(result).toContain('&quot;');
            expect(result).not.toContain('class="class1"');
            // The malicious payload is rendered as text, not executable
            expect(result).toContain('class="class1&quot; onclick=&quot;alert(1)"');
        });

        it('{{{var}}} in attributes is unsafe but allowed for trusted content', () => {
            const loader = new TemplateLoader();
            const trustedHtml = '<span class="icon">check</span>';
            const result = loader.replace('<div>{{{content}}}</div>', { content: trustedHtml });
            expect(result).toBe('<div><span class="icon">check</span></div>');
        });
    });

    describe('load() from cache after seed', () => {
        it('does not add entry to loading Map when cache hit', async () => {
            const loader = new TemplateLoader();
            loader.seed('cached-tpl', '<div/>');
            await loader.load('cached-tpl');
            expect(loader.loading.has('cached-tpl')).toBe(false);
        });
    });

    describe('clearCache()', () => {
        it('removes a specific template from the cache', () => {
            const loader = new TemplateLoader();
            loader.seed('a', '<a/>');
            loader.seed('b', '<b/>');
            loader.clearCache('a');
            expect(loader.cache.has('a')).toBe(false);
            expect(loader.cache.has('b')).toBe(true);
        });

        it('clears all templates when called with no argument', () => {
            const loader = new TemplateLoader();
            loader.seed('a', '<a/>');
            loader.seed('b', '<b/>');
            loader.clearCache();
            expect(loader.cache.size).toBe(0);
        });
    });

    describe('preload()', () => {
        it('resolves all templates and populates the cache', async () => {
            const transportGet = vi.fn().mockResolvedValue({ text: async () => '<div/>' });
            const loader = new TemplateLoader('/templates/', { get: transportGet });

            await loader.preload(['tpl-a', 'tpl-b', 'tpl-c']);

            expect(loader.cache.has('tpl-a')).toBe(true);
            expect(loader.cache.has('tpl-b')).toBe(true);
            expect(loader.cache.has('tpl-c')).toBe(true);
        });
    });
});

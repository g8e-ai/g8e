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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { MockElement, MockDocument } from '@test/mocks/mock-browser-env.js';
import { HamburgerMenu } from '@g8ed/public/js/components/hamburger-menu.js';

describe('HamburgerMenu [UNIT - Frontend Component]', () => {
    let mockDocument;
    let btn;
    let dropdown;
    let originalDocument;
    let originalWindow;

    beforeEach(() => {
        originalDocument = globalThis.document;
        originalWindow = globalThis.window;

        mockDocument = new MockDocument();

        btn = new MockElement('button', 'hamburger-btn');
        dropdown = new MockElement('div', 'hamburger-dropdown');
        mockDocument.registerElement('hamburger-btn', btn);
        mockDocument.registerElement('hamburger-dropdown', dropdown);

        mockDocument._documentListeners = new Map();
        mockDocument.addEventListener = (event, handler) => {
            if (!mockDocument._documentListeners.has(event)) {
                mockDocument._documentListeners.set(event, []);
            }
            mockDocument._documentListeners.get(event).push(handler);
        };
        mockDocument.removeEventListener = (event, handler) => {
            if (mockDocument._documentListeners.has(event)) {
                const listeners = mockDocument._documentListeners.get(event);
                const idx = listeners.indexOf(handler);
                if (idx !== -1) listeners.splice(idx, 1);
            }
        };
        mockDocument._dispatchDocumentEvent = (event, target) => {
            const listeners = mockDocument._documentListeners.get(event.type) || [];
            listeners.forEach(h => h({ ...event, target }));
        };

        globalThis.document = mockDocument;
        globalThis.window = {
            location: { pathname: '/chat' },
            ThemeManager: null
        };
    });

    afterEach(() => {
        globalThis.document = originalDocument;
        globalThis.window = originalWindow;
        vi.restoreAllMocks();
    });

    describe('init() — missing DOM elements', () => {
        it('returns without error when hamburger-btn is absent', () => {
            mockDocument._elements.delete('hamburger-btn');
            const menu = new HamburgerMenu();
            expect(() => menu.init()).not.toThrow();
        });

        it('returns without error when hamburger-dropdown is absent', () => {
            mockDocument._elements.delete('hamburger-dropdown');
            const menu = new HamburgerMenu();
            expect(() => menu.init()).not.toThrow();
        });
    });

    describe('Button click — dropdown toggle', () => {
        let menu;

        beforeEach(() => {
            menu = new HamburgerMenu();
            menu.init();
        });

        afterEach(() => {
            menu.destroy();
        });

        it('adds active class when dropdown is closed', () => {
            btn._eventListeners.get('click')[0].handler({ stopPropagation: () => {} });
            expect(dropdown.classList.contains('active')).toBe(true);
        });

        it('removes active class when dropdown is already open', () => {
            dropdown.classList.add('active');
            btn._eventListeners.get('click')[0].handler({ stopPropagation: () => {} });
            expect(dropdown.classList.contains('active')).toBe(false);
        });
    });

    describe('Document click — outside closes dropdown', () => {
        let menu;

        beforeEach(() => {
            menu = new HamburgerMenu();
            menu.init();
            dropdown.classList.add('active');
        });

        afterEach(() => {
            menu.destroy();
        });

        it('closes dropdown on click outside btn and dropdown', () => {
            const outside = new MockElement('div');
            mockDocument._dispatchDocumentEvent({ type: 'click' }, outside);
            expect(dropdown.classList.contains('active')).toBe(false);
        });

        it('does not close dropdown on click inside btn', () => {
            mockDocument._dispatchDocumentEvent({ type: 'click' }, btn);
            expect(dropdown.classList.contains('active')).toBe(true);
        });

        it('does not close dropdown on click inside dropdown', () => {
            mockDocument._dispatchDocumentEvent({ type: 'click' }, dropdown);
            expect(dropdown.classList.contains('active')).toBe(true);
        });
    });

    describe('Keydown — Escape closes dropdown', () => {
        let menu;

        beforeEach(() => {
            menu = new HamburgerMenu();
            menu.init();
            dropdown.classList.add('active');
        });

        afterEach(() => {
            menu.destroy();
        });

        it('closes dropdown on Escape key', () => {
            const listeners = mockDocument._documentListeners.get('keydown');
            listeners.forEach(h => h({ key: 'Escape' }));
            expect(dropdown.classList.contains('active')).toBe(false);
        });

        it('does not close dropdown on other keys', () => {
            const listeners = mockDocument._documentListeners.get('keydown');
            listeners.forEach(h => h({ key: 'Enter' }));
            expect(dropdown.classList.contains('active')).toBe(true);
        });
    });

    describe('_highlightActivePage()', () => {
        it('marks the menu item whose href matches the current pathname', () => {
            const chatItem = new MockElement('a');
            chatItem._attributes.set('href', '/chat');
            chatItem.classList = { _classes: new Set(), add(...c) { c.forEach(x => this._classes.add(x)); }, contains(c) { return this._classes.has(c); } };

            const homeItem = new MockElement('a');
            homeItem._attributes.set('href', '/');
            homeItem.classList = { _classes: new Set(), add(...c) { c.forEach(x => this._classes.add(x)); }, contains(c) { return this._classes.has(c); } };

            mockDocument.querySelectorAll = (selector) => {
                if (selector === '.hamburger-menu-item') return [chatItem, homeItem];
                return [];
            };

            globalThis.window.location.pathname = '/chat';
            const menu = new HamburgerMenu();
            menu.init();

            expect(chatItem.classList.contains('active')).toBe(true);
            expect(homeItem.classList.contains('active')).toBe(false);
            menu.destroy();
        });
    });

    describe('Theme toggle', () => {
        let themeToggle;
        let themeText;

        beforeEach(() => {
            themeToggle = new MockElement('button', 'hamburger-theme-toggle');
            themeText = new MockElement('span');
            themeText.textContent = '';
            themeToggle.children.push(themeText);
            themeToggle.querySelector = (selector) => selector === 'span' ? themeText : null;
            mockDocument.registerElement('hamburger-theme-toggle', themeToggle);
        });

        it('updates label via ThemeManager.onChange callback', () => {
            let changeCallback = null;
            globalThis.window.ThemeManager = {
                getTheme: () => 'dark',
                getDefaultTheme: () => 'dark',
                toggle: () => 'light',
                onChange: (cb) => { changeCallback = cb; }
            };

            const menu = new HamburgerMenu();
            menu.init();

            changeCallback('light');
            expect(themeText.textContent).toBe('Go Dark');

            changeCallback('dark');
            expect(themeText.textContent).toBe('Go Light');

            menu.destroy();
        });

        it('calls ThemeManager.toggle on theme toggle click', () => {
            const toggleSpy = vi.fn(() => 'light');
            globalThis.window.ThemeManager = {
                getTheme: () => 'dark',
                getDefaultTheme: () => 'dark',
                toggle: toggleSpy,
                onChange: () => {}
            };

            const menu = new HamburgerMenu();
            menu.init();

            const clickListeners = themeToggle._eventListeners.get('click');
            expect(clickListeners).toBeDefined();
            clickListeners[0].handler({ stopPropagation: () => {} });

            expect(toggleSpy).toHaveBeenCalledOnce();
            menu.destroy();
        });
    });

    describe('destroy()', () => {
        it('removes document event listeners on destroy', () => {
            const menu = new HamburgerMenu();
            menu.init();

            const clickListenersBefore = (mockDocument._documentListeners.get('click') || []).length;
            const keydownListenersBefore = (mockDocument._documentListeners.get('keydown') || []).length;

            menu.destroy();

            expect((mockDocument._documentListeners.get('click') || []).length).toBe(clickListenersBefore - 1);
            expect((mockDocument._documentListeners.get('keydown') || []).length).toBe(keydownListenersBefore - 1);
        });

        it('nulls btn and dropdown references on destroy', () => {
            const menu = new HamburgerMenu();
            menu.init();
            menu.destroy();

            expect(menu.btn).toBeNull();
            expect(menu.dropdown).toBeNull();
        });
    });
});

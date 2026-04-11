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
 * Mock Browser Environment
 * 
 * Provides comprehensive mocking for browser globals (window, document, DOM elements)
 * for unit testing frontend components like CasesManager, AuthManager, etc.
 * 
 * Usage:
 *   const env = createMockBrowserEnv();
 *   env.setAuthenticated(mockWebSession);
 *   const casesManager = new CasesManager(env.eventBus);
 *   casesManager.init();
 *   env.cleanup();
 */

import { now } from '@test/fixtures/base.fixture.js';

/**
 * Mock EventBus - mirrors frontend EventBus behavior
 */
export class MockEventBus {
    constructor() {
        this.listeners = new Map();
        this.emittedEvents = [];
    }

    on(event, handler) {
        if (!this.listeners.has(event)) this.listeners.set(event, []);
        this.listeners.get(event).push(handler);
        return () => this.off(event, handler);
    }

    once(event, handler) {
        const wrapper = (...args) => {
            this.off(event, wrapper);
            handler(...args);
        };
        return this.on(event, wrapper);
    }

    off(event, handler) {
        if (!this.listeners.has(event)) return;
        const handlers = this.listeners.get(event);
        const idx = handlers.indexOf(handler);
        if (idx > -1) handlers.splice(idx, 1);
    }

    emit(event, data) {
        this.emittedEvents.push({ event, payload: data, data, timestamp: Date.now() });
        if (this.listeners.has(event)) {
            [...this.listeners.get(event)].forEach(h => h(data));
        }
    }

    getListenerCount(event) {
        return (this.listeners.get(event) ?? []).length;
    }

    hasListeners(event) {
        return this.getListenerCount(event) > 0;
    }

    getEmittedEvents(eventType = null) {
        if (eventType) return this.emittedEvents.filter(e => e.event === eventType);
        return this.emittedEvents;
    }

    getEmitted(event) {
        return this.emittedEvents.filter(e => e.event === event);
    }

    emitted(event) {
        return this.emittedEvents.filter(e => e.event === event);
    }

    clear() {
        this.emittedEvents = [];
    }

    getEventLog() {
        return this.emittedEvents;
    }

    getEventsOfType(eventType) {
        return this.emittedEvents.filter(e => e.event === eventType);
    }

    clearLog() {
        this.emittedEvents = [];
    }

    removeAllListeners() {
        this.listeners.clear();
    }
}

/**
 * Mock DOM Element - simulates HTML elements
 */
export class MockElement {
    constructor(tagName, id = null) {
        this.tagName = tagName.toUpperCase();
        this.id = id;
        this.className = '';
        this.classList = new MockClassList();
        this.style = {};
        this.disabled = false;
        this.value = '';
        this.textContent = '';
        this.innerHTML = '';
        this.title = '';
        this.selected = false;
        this.options = [];
        this.children = [];
        this.parentNode = null;
        this.nextSibling = null;
        this._eventListeners = new Map();
        this._attributes = new Map();
        this.dataset = {};
        this.scrollTop = 0;
        this.scrollHeight = 0;
        this.clientHeight = 0;
    }

    addEventListener(event, handler, options) {
        if (!this._eventListeners.has(event)) {
            this._eventListeners.set(event, []);
        }
        this._eventListeners.get(event).push({ handler, options });
    }

    removeEventListener(event, handler) {
        if (this._eventListeners.has(event)) {
            const listeners = this._eventListeners.get(event);
            const idx = listeners.findIndex(l => l.handler === handler);
            if (idx !== -1) {
                listeners.splice(idx, 1);
            }
        }
    }

    dispatchEvent(event) {
        const listeners = this._eventListeners.get(event.type);
        for (const { handler } of listeners) {
            handler(event);
        }
        return true;
    }

    click() {
        this.dispatchEvent({ type: 'click', target: this });
    }

    focus() {}
    blur() {}

    remove() {
        if (this.parentNode) {
            this.parentNode.removeChild(this);
        }
    }

    closest(selector) {
        let current = this.parentNode;
        while (current) {
            if (current.classList && current.classList.contains(selector.replace('.', ''))) {
                return current;
            }
            if (selector.startsWith('#') && current.id === selector.slice(1)) {
                return current;
            }
            if (current.tagName === selector.toUpperCase()) {
                return current;
            }
            current = current.parentNode;
        }
        return null;
    }

    scrollTo(options) {
        if (typeof options === 'object') {
            if (options.top !== undefined) this.scrollTop = options.top;
        }
    }

    setAttribute(name, value) {
        this._attributes.set(name, value);
    }

    getAttribute(name) {
        return this._attributes.get(name) || null;
    }

    removeAttribute(name) {
        this._attributes.delete(name);
    }

    appendChild(child) {
        // Handle DocumentFragment - move its children to this element
        if (child && child.nodeType === 11) { // Node.DOCUMENT_FRAGMENT_NODE
            const fragmentChildren = Array.from(child.children || []);
            fragmentChildren.forEach(fc => this.appendChild(fc));
            return child;
        }

        child.parentNode = this;
        if (this.children.length > 0) {
            this.children[this.children.length - 1].nextSibling = child;
        }
        this.children.push(child);
        if (this.tagName === 'SELECT') {
            this.options.push(child);
        }
        return child;
    }

    insertBefore(newChild, refChild) {
        newChild.parentNode = this;
        const refIndex = this.children.indexOf(refChild);
        if (refIndex === -1) {
            this.children.push(newChild);
        } else {
            this.children.splice(refIndex, 0, newChild);
            if (refIndex > 0) {
                this.children[refIndex - 1].nextSibling = newChild;
            }
            newChild.nextSibling = refChild;
        }
        return newChild;
    }

    contains(el) {
        if (el === this) return true;
        for (const child of this.children) {
            if (child === el || (child.contains && child.contains(el))) {
                return true;
            }
        }
        return false;
    }

    removeChild(child) {
        const idx = this.children.indexOf(child);
        if (idx !== -1) {
            this.children.splice(idx, 1);
            child.parentNode = null;
        }
        if (this.tagName === 'SELECT') {
            const optIdx = this.options.indexOf(child);
            if (optIdx !== -1) {
                this.options.splice(optIdx, 1);
            }
        }
        return child;
    }

    querySelector(selector) {
        return this._findElement(selector);
    }

    querySelectorAll(selector) {
        return this._findAllElements(selector);
    }

    _findElement(selector) {
        for (const child of this.children) {
            if (this._matchesSelector(child, selector)) {
                return child;
            }
            const found = child._findElement?.(selector);
            if (found) return found;
        }
        return null;
    }

    _findAllElements(selector) {
        const results = [];
        for (const child of this.children) {
            if (this._matchesSelector(child, selector)) {
                results.push(child);
            }
            if (child._findAllElements) {
                results.push(...child._findAllElements(selector));
            }
        }
        return results;
    }

    _matchesSelector(el, selector) {
        if (selector.startsWith('#')) {
            return el.id === selector.slice(1);
        }
        if (selector.startsWith('.')) {
            const classes = selector.split('.').filter(Boolean);
            return classes.every(c => {
                const [className, ...pseudos] = c.split(':');
                return el.classList.contains(className);
            });
        }
        if (selector.startsWith('[data-')) {
            const match = selector.match(/\[data-([^=]+)="([^"]*)"\]/);
            if (match) {
                return el.dataset && el.dataset[match[1]] === match[2];
            }
        }
        return el.tagName === selector.toUpperCase();
    }

    hasListenersFor(eventType) {
        return this._eventListeners.has(eventType) && 
               this._eventListeners.get(eventType).length > 0;
    }

    getListenerCount(eventType) {
        return this._eventListeners.get(eventType)?.length || 0;
    }
}

/**
 * Mock ClassList - simulates element.classList
 */
export class MockClassList {
    constructor() {
        this._classes = new Set();
    }

    add(...classes) {
        classes.forEach(c => this._classes.add(c));
    }

    remove(...classes) {
        classes.forEach(c => this._classes.delete(c));
    }

    toggle(className, force) {
        if (force === undefined) {
            if (this._classes.has(className)) {
                this._classes.delete(className);
                return false;
            } else {
                this._classes.add(className);
                return true;
            }
        }
        if (force) {
            this._classes.add(className);
        } else {
            this._classes.delete(className);
        }
        return force;
    }

    contains(className) {
        return this._classes.has(className);
    }

    get length() {
        return this._classes.size;
    }
}

/**
 * Mock AuthState - simulates window.authState
 */
export class MockAuthState {
    constructor() {
        this._subscribers = new Set();
        this._state = {
            isAuthenticated: false,
            webSessionModel: null,
            loading: false
        };
        this._notificationLog = [];
    }

    subscribe(callback) {
        this._subscribers.add(callback);
        return () => this._subscribers.delete(callback);
    }

    getState() {
        return { ...this._state };
    }

    isAuthenticated() {
        return this._state.isAuthenticated;
    }

    notifySubscribers(event, data) {
        this._notificationLog.push({ event, data, timestamp: Date.now() });
        for (const subscriber of this._subscribers) {
            subscriber(event, data);
        }
    }

    getWebSessionId() {
        return this._state.webSessionModel?.id ?? null;
    }

    getWebSessionModel() {
        return this._state.webSessionModel ?? null;
    }

    setAuthenticated(webSessionModel) {
        this._state = {
            isAuthenticated: true,
            webSessionModel,
            loading: false
        };
    }

    setUnauthenticated() {
        this._state = {
            isAuthenticated: false,
            webSessionModel: null,
            loading: false
        };
    }

    getNotificationLog() {
        return this._notificationLog;
    }

    clearLog() {
        this._notificationLog = [];
    }

    createMockWebSession(overrides = {}) {
        return {
            id: 'session_test_123',
            user_id: 'user_test_456',
            email: 'test@example.com',
            ...overrides
        };
    }
}

/**
 * Mock ServiceClient - simulates window.serviceClient
 */
export class MockServiceClient {
    constructor() {
        this._responses = new Map();
        this._requestLog = [];
    }

    setResponse(service, path, response) {
        this._responses.set(`${service}:${path}`, response);
    }

    setInvestigationsResponse(investigations) {
        this.setResponse('vsod', '/api/chat/investigations', {
            ok: true,
            status: 200,
            json: async () => ({
                success: true,
                investigations
            })
        });
    }

    setInvestigationsError(status, message) {
        this.setResponse('vsod', '/api/chat/investigations', {
            ok: false,
            status,
            statusText: message,
            json: async () => ({ success: false, error: message })
        });
    }

    async get(service, path) {
        this._requestLog.push({ method: 'GET', service, path, timestamp: Date.now() });
        const key = `${service}:${path}`;
        if (this._responses.has(key)) {
            return this._responses.get(key);
        }
        return {
            ok: false,
            status: 404,
            statusText: 'Not Found',
            json: async () => ({ success: false, error: 'Not found' })
        };
    }

    async post(service, path, body) {
        this._requestLog.push({ method: 'POST', service, path, body, timestamp: Date.now() });
        const key = `${service}:${path}`;
        if (this._responses.has(key)) {
            return this._responses.get(key);
        }
        return {
            ok: true,
            status: 200,
            json: async () => ({ success: true })
        };
    }

    async delete(service, path) {
        this._requestLog.push({ method: 'DELETE', service, path, timestamp: Date.now() });
        const key = `${service}:${path}`;
        if (this._responses.has(key)) {
            return this._responses.get(key);
        }
        return {
            ok: true,
            status: 200,
            json: async () => ({ success: true })
        };
    }

    getRequestLog() {
        return this._requestLog;
    }

    clearLog() {
        this._requestLog = [];
    }
}

/**
 * Mock Document - simulates document object
 */
export class MockDocument {
    constructor() {
        this.body = new MockElement('body');
        this._elements = new Map();
    }

    getElementById(id) {
        return this._elements.get(id) || null;
    }

    createElement(tagName) {
        return new MockElement(tagName);
    }

    querySelector(selector) {
        if (selector.startsWith('#')) {
            return this._elements.get(selector.slice(1)) || null;
        }
        return this.body.querySelector(selector);
    }

    querySelectorAll(selector) {
        return this.body.querySelectorAll(selector);
    }

    registerElement(id, element) {
        element.id = id;
        this._elements.set(id, element);
    }
}

/**
 * Mock Window - simulates window object
 */
export class MockWindow {
    constructor() {
        this._eventListeners = new Map();
        this.location = {
            pathname: '/chat',
            search: '',
            href: 'https://localhost/chat',
            origin: 'https://localhost'
        };
        this.history = {
            _states: [],
            pushState: (state, title, url) => {
                this.history._states.push({ state, title, url });
                if (url) {
                    const urlObj = new URL(url, this.location.origin);
                    this.location.search = urlObj.search;
                    this.location.pathname = urlObj.pathname;
                }
            },
            replaceState: (state, title, url) => {
                if (this.history._states.length > 0) {
                    this.history._states[this.history._states.length - 1] = { state, title, url };
                } else {
                    this.history._states.push({ state, title, url });
                }
                if (url) {
                    const urlObj = new URL(url, this.location.origin);
                    this.location.search = urlObj.search;
                    this.location.pathname = urlObj.pathname;
                }
            }
        };
        this.innerWidth = 1024;
    }

    addEventListener(event, handler, options) {
        if (!this._eventListeners.has(event)) {
            this._eventListeners.set(event, []);
        }
        this._eventListeners.get(event).push({ handler, options });
    }

    removeEventListener(event, handler) {
        if (this._eventListeners.has(event)) {
            const listeners = this._eventListeners.get(event);
            const idx = listeners.findIndex(l => l.handler === handler);
            if (idx !== -1) {
                listeners.splice(idx, 1);
            }
        }
    }

    dispatchEvent(event) {
        const listeners = this._eventListeners.get(event.type);
        for (const { handler } of listeners) {
            handler(event);
        }
    }

    hasListenersFor(eventType) {
        return this._eventListeners.has(eventType) && 
               this._eventListeners.get(eventType).length > 0;
    }
}

/**
 * Create a complete mock browser environment for testing
 */
export function createMockBrowserEnv(options = {}) {
    const eventBus = new MockEventBus();
    const authState = new MockAuthState();
    const serviceClient = new MockServiceClient();
    const mockDocument = new MockDocument();
    const mockWindow = new MockWindow();

    // Create custom dropdown elements (not native select)
    const caseDropdown = new MockElement('div', 'case-dropdown');
    caseDropdown.setAttribute('tabindex', '0');
    mockDocument.registerElement('case-dropdown', caseDropdown);

    const dropdownSelected = new MockElement('div', 'case-dropdown-selected');
    caseDropdown.appendChild(dropdownSelected);
    mockDocument.registerElement('case-dropdown-selected', dropdownSelected);

    const dropdownText = new MockElement('span');
    dropdownText.classList.add('case-dropdown__text');
    dropdownText.classList.add('placeholder');
    dropdownText.textContent = 'Select a past conversation here, or start a new one below';
    dropdownSelected.appendChild(dropdownText);

    const dropdownArrow = new MockElement('span');
    dropdownArrow.classList.add('case-dropdown__arrow');
    dropdownSelected.appendChild(dropdownArrow);

    const dropdownMenu = new MockElement('div', 'case-dropdown-menu');
    caseDropdown.appendChild(dropdownMenu);
    mockDocument.registerElement('case-dropdown-menu', dropdownMenu);

    const dropdownValueInput = new MockElement('input', 'case-dropdown-value');
    dropdownValueInput.value = '';
    mockDocument.registerElement('case-dropdown-value', dropdownValueInput);

    const newCaseBtn = new MockElement('button', 'new-case-btn');
    newCaseBtn.disabled = true;
    mockDocument.registerElement('new-case-btn', newCaseBtn);

    // Store original globals
    const originalWindow = globalThis.window;
    const originalDocument = globalThis.document;

    // Install mocks
    globalThis.window = mockWindow;
    globalThis.window.authState = authState;
    globalThis.window.serviceClient = serviceClient;
    globalThis.document = mockDocument;

    // Helper to create mock web session
    const createMockWebSession = (overrides = {}) => ({
        id: 'session_test_123',
        user_id: 'user_test_456',
        email: 'test@example.com',
        ...overrides
    });

    // Helper to create mock investigations
    const createMockInvestigations = (count = 3) => {
        return Array.from({ length: count }, (_, i) => ({
            case_id: `case_${i + 1}_abc123`,
            case_title: `Test Case ${i + 1}`,
            case_description: `Description for case ${i + 1}`,
            id: `inv_${i + 1}`,
            created_at: new Date(Date.now() - i * 86400000).toISOString()
        }));
    };

    return {
        eventBus,
        authState,
        serviceClient,
        document: mockDocument,
        window: mockWindow,
        elements: {
            caseDropdown,
            dropdownSelected,
            dropdownText,
            dropdownMenu,
            dropdownValueInput,
            newCaseBtn
        },
        
        // Helpers
        createMockWebSession,
        createMockInvestigations,

        // Set authenticated state with optional investigations
        async setAuthenticated(webSession = null, investigations = null) {
            const session = webSession || createMockWebSession();
            authState.setAuthenticated(session);
            
            if (investigations !== null) {
                serviceClient.setInvestigationsResponse(investigations);
            }
        },

        // Trigger auth event (simulates what AuthManager does)
        triggerAuthEvent(webSession = null) {
            const session = webSession || createMockWebSession();
            authState.setAuthenticated(session);
            authState.notifySubscribers('auth.user.authenticated', {
                isAuthenticated: true,
                webSessionModel: session,
                webSessionId: session.id
            });
        },

        // Trigger unauth event
        triggerUnauthEvent() {
            authState.setUnauthenticated();
            authState.notifySubscribers('auth.user.unauthenticated', {
                isAuthenticated: false,
                webSessionModel: null,
                webSessionId: null
            });
        },

        // Trigger SSE investigation list event (simulates what SSE connection does on connect)
        triggerSSEInvestigationList(investigations) {
            eventBus.emit('investigation.list.completed', {
                investigations: investigations,
                count: investigations?.length || 0,
                timestamp: now()
            });
        },

        // Wait for async operations
        async tick(ms = 0) {
            await new Promise(resolve => setTimeout(resolve, ms));
        },

        // Cleanup
        cleanup() {
            globalThis.window = originalWindow;
            globalThis.document = originalDocument;
            eventBus.removeAllListeners();
        }
    };
}

/**
 * Create a mock for testing with assertions
 */
export function createTestHarness(options = {}) {
    const env = createMockBrowserEnv(options);
    
    return {
        ...env,

        // Assertion helpers
        assertNewCaseBtnEnabled() {
            if (env.elements.newCaseBtn.disabled) {
                throw new Error('Expected New Case button to be enabled, but it is disabled');
            }
        },

        assertNewCaseBtnDisabled() {
            if (!env.elements.newCaseBtn.disabled) {
                throw new Error('Expected New Case button to be disabled, but it is enabled');
            }
        },

        assertDropdownEnabled() {
            if (env.elements.caseDropdown.disabled) {
                throw new Error('Expected case dropdown to be enabled, but it is disabled');
            }
        },

        assertDropdownDisabled() {
            if (!env.elements.caseDropdown.disabled) {
                throw new Error('Expected case dropdown to be disabled, but it is enabled');
            }
        },

        assertDropdownHasOptions(expectedCount) {
            const actualCount = env.elements.caseDropdown.options.length;
            if (actualCount !== expectedCount) {
                throw new Error(`Expected dropdown to have ${expectedCount} options, but has ${actualCount}`);
            }
        },

        assertEventEmitted(eventType, minCount = 1) {
            const events = env.eventBus.getEventsOfType(eventType);
            if (events.length < minCount) {
                throw new Error(`Expected at least ${minCount} '${eventType}' events, but found ${events.length}`);
            }
        },

        assertApiCalled(path, minCount = 1) {
            const calls = env.serviceClient.getRequestLog().filter(r => r.path.includes(path));
            if (calls.length < minCount) {
                throw new Error(`Expected API '${path}' to be called at least ${minCount} times, but was called ${calls.length} times`);
            }
        },

        assertButtonHasClickListener(button) {
            if (!button.hasListenersFor('click')) {
                throw new Error(`Expected button '${button.id}' to have click listener, but none found`);
            }
        }
    };
}

/**
 * MockTemplateLoader - test double for TemplateLoader
 *
 * Exposes seed() to pre-populate the cache so tests that exercise code paths
 * depending on pre-cached templates (handleApprovalRequest, showWelcomeMessage,
 * etc.) have a clean, network-free setup path.
 *
 * replace() uses the same {{var}} / {{{var}}} logic as the production class.
 * load() resolves from the cache synchronously (wrapped in a resolved Promise)
 * and rejects with a descriptive error when the template was not seeded.
 */
export class MockTemplateLoader {
    constructor() {
        this.cache = new Map();
        this._fetchLog = [];
    }

    seed(templateName, html) {
        this.cache.set(templateName, html);
    }

    async load(templateName) {
        if (this.cache.has(templateName)) {
            return this.cache.get(templateName);
        }
        throw new Error(`MockTemplateLoader: template '${templateName}' not seeded`);
    }

    async render(templateName, variables = {}) {
        const template = await this.load(templateName);
        return this.replace(template, variables);
    }

    replace(template, variables = {}) {
        const keys = Object.keys(variables);
        if (keys.length === 0) {
            return template;
        }

        const escapedKeys = keys.map(key => key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
        const keyPattern = escapedKeys.join('|');
        
        const combinedPattern = new RegExp(`\\{\\{(?:(\\{)|(!))?(${keyPattern})\\}\\}\\}?`, 'g');
        
        return template.replace(combinedPattern, (match, isTriple, isAttr, key) => {
            const value = variables[key] ?? '';
            
            if (isTriple && match.endsWith('}}}')) {
                return value;
            }
            
            if (isAttr) {
                return String(value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
            }
            
            return String(value).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        });
    }

    async preload(templateNames) {
        for (const name of templateNames) {
            this._fetchLog.push(name);
        }
    }

    async createFragment(templateName, variables = {}) {
        const html = await this.render(templateName, variables);
        const templateEl = document.createElement('template');
        templateEl.innerHTML = html;
        return templateEl.content;
    }

    async renderTo(container, templateName, variables = {}) {
        const html = await this.render(templateName, variables);
        container.innerHTML = html;
    }

    clearCache(templateName) {
        if (templateName) {
            this.cache.delete(templateName);
        } else {
            this.cache.clear();
        }
    }

    getFetchLog() {
        return this._fetchLog.slice();
    }
}

export function createMockCitationsHandler() {
    const calls = { addInlineCitations: [], renderSourcesPanel: [] };
    return {
        calls,
        addInlineCitations: (html, metadata) => {
            calls.addInlineCitations.push({ html, metadata });
            return html + '<!-- citations-applied -->';
        },
        renderSourcesPanel: (sources) => {
            calls.renderSourcesPanel.push({ sources });
            const panel = new MockElement('div');
            panel.className = 'sources-panel';
            panel._sources = sources;
            return panel;
        }
    };
}

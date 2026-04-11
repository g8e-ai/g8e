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
import { JSDOM } from 'jsdom';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { now } from '@test/fixtures/base.fixture.js';
import { CasesManager } from '@g8ed/public/js/components/cases-manager.js';
import { MockEventBus, MockAuthState, MockServiceClient } from '@test/mocks/mock-browser-env.js';
import { ServiceName } from '@g8ed/public/js/constants/service-client-constants.js';

const CASES_HTML = `
<!DOCTYPE html>
<html><body>
  <div id="case-dropdown" tabindex="0">
    <div id="case-dropdown-selected">
      <span class="case-dropdown__text placeholder">Select a past conversation here, or start a new one below</span>
      <span class="case-dropdown__arrow"></span>
    </div>
    <div id="case-dropdown-menu"></div>
  </div>
  <input id="case-dropdown-value" type="hidden" value="" />
  <button id="new-case-btn" disabled></button>
</body></html>`;

function makeInvestigations(count = 3) {
    return Array.from({ length: count }, (_, i) => ({
        case_id: `case_${i + 1}_abc123`,
        case_title: `Test Case ${i + 1}`,
        case_description: `Description ${i + 1}`,
        id: `inv_${i + 1}`,
        created_at: new Date(Date.now() - i * 86400000).toISOString()
    }));
}

function makeSwitchToCaseResponse(caseId, overrides = {}) {
    return {
        ok: true,
        status: 200,
        json: async () => ({
            success: true,
            investigations: [{
                id: `inv_for_${caseId}`,
                case_id: caseId,
                case_title: 'Test Case',
                conversation_history: [],
                ...overrides
            }]
        })
    };
}

describe('CasesManager [UNIT]', () => {
    let dom;
    let eventBus;
    let serviceClient;
    let authState;

    beforeEach(() => {
        dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat' });

        global.window = dom.window;
        global.document = dom.window.document;
        global.URLSearchParams = dom.window.URLSearchParams;
        global.URL = dom.window.URL;

        authState = new MockAuthState();
        serviceClient = new MockServiceClient();

        global.window.authState = authState;
        global.window.serviceClient = serviceClient;

        vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
        vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

        eventBus = new MockEventBus();
    });

    afterEach(() => {
        vi.restoreAllMocks();
        eventBus.removeAllListeners();
        delete global.window;
        delete global.document;
        delete global.URLSearchParams;
        delete global.URL;
    });

    function makeManager() {
        const manager = new CasesManager(eventBus);
        manager.init();
        return manager;
    }

    function triggerAuthEvent(session) {
        authState.setAuthenticated(session);
        authState.notifySubscribers(EventType.AUTH_USER_AUTHENTICATED, {
            isAuthenticated: true,
            webSessionModel: session,
            webSessionId: session.id
        });
    }

    function triggerUnauthEvent() {
        authState.setUnauthenticated();
        authState.notifySubscribers(EventType.AUTH_USER_UNAUTHENTICATED, {
            isAuthenticated: false,
            webSessionModel: null
        });
    }

    function triggerSSEInvestigationList(investigations) {
        eventBus.emit(EventType.INVESTIGATION_LIST_COMPLETED, {
            investigations,
            count: investigations.length,
            timestamp: now()
        });
    }

    describe('initialization', () => {
        it('resolves DOM elements on init', () => {
            const manager = makeManager();

            expect(manager.caseDropdown).not.toBeNull();
            expect(manager.dropdownSelected).not.toBeNull();
            expect(manager.dropdownMenu).not.toBeNull();
            expect(manager.dropdownValueInput).not.toBeNull();
            expect(manager.newCaseBtn).not.toBeNull();
        });

        it('registers bound handlers on init', () => {
            const manager = makeManager();

            expect(manager.boundHandlers.dropdownToggle).toBeTypeOf('function');
            expect(manager.boundHandlers.dropdownKeydown).toBeTypeOf('function');
            expect(manager.boundHandlers.documentClick).toBeTypeOf('function');
            expect(manager.boundHandlers.newCaseClick).toBeTypeOf('function');
            expect(manager.boundHandlers.windowResize).toBeTypeOf('function');
            expect(manager.boundHandlers.popstate).toBeTypeOf('function');
        });

        it('does not throw when DOM elements are absent', () => {
            dom = new JSDOM('<!DOCTYPE html><html><body></body></html>', { url: 'https://localhost/chat' });
            global.document = dom.window.document;

            expect(() => makeManager()).not.toThrow();
        });

        it('subscribes to authState on init', () => {
            const subscribeSpy = vi.spyOn(authState, 'subscribe');
            makeManager();
            expect(subscribeSpy).toHaveBeenCalledOnce();
        });

        it('calls handleUserAuthenticated immediately if already authenticated', () => {
            const session = authState.createMockWebSession();
            authState.setAuthenticated(session);

            const manager = makeManager();

            expect(manager._isHandlingAuth).toBe(false);
        });
    });

    describe('dropdown toggle', () => {
        it('opens dropdown on first toggle', () => {
            const manager = makeManager();
            manager.toggleDropdown();
            expect(manager.caseDropdown.classList.contains('open')).toBe(true);
        });

        it('closes dropdown on second toggle', () => {
            const manager = makeManager();
            manager.toggleDropdown();
            manager.toggleDropdown();
            expect(manager.caseDropdown.classList.contains('open')).toBe(false);
        });

        it('openDropdown adds open class', () => {
            const manager = makeManager();
            manager.openDropdown();
            expect(manager.caseDropdown.classList.contains('open')).toBe(true);
        });

        it('closeDropdown removes open class', () => {
            const manager = makeManager();
            manager.openDropdown();
            manager.closeDropdown();
            expect(manager.caseDropdown.classList.contains('open')).toBe(false);
        });
    });

    describe('keyboard navigation', () => {
        it('Enter opens dropdown when closed', () => {
            const manager = makeManager();
            const e = { key: 'Enter', preventDefault: vi.fn() };
            manager.handleDropdownKeydown(e);
            expect(manager.caseDropdown.classList.contains('open')).toBe(true);
            expect(e.preventDefault).toHaveBeenCalled();
        });

        it('Escape closes dropdown', () => {
            const manager = makeManager();
            manager.openDropdown();
            const e = { key: 'Escape', preventDefault: vi.fn() };
            manager.handleDropdownKeydown(e);
            expect(manager.caseDropdown.classList.contains('open')).toBe(false);
        });

        it('ArrowDown opens dropdown when closed', () => {
            const manager = makeManager();
            const e = { key: 'ArrowDown', preventDefault: vi.fn() };
            manager.handleDropdownKeydown(e);
            expect(manager.caseDropdown.classList.contains('open')).toBe(true);
        });

        it('Space opens dropdown when closed', () => {
            const manager = makeManager();
            const e = { key: ' ', preventDefault: vi.fn() };
            manager.handleDropdownKeydown(e);
            expect(manager.caseDropdown.classList.contains('open')).toBe(true);
        });
    });

    describe('SSE investigation list', () => {
        it('sets casesLoaded and userCases on INVESTIGATION.LIST_COMPLETED', () => {
            const manager = makeManager();
            const investigations = makeInvestigations(3);
            triggerSSEInvestigationList(investigations);

            expect(manager.casesLoaded).toBe(true);
            expect(manager.userCases).toHaveLength(3);
        });

        it('populates dropdown: 1 placeholder + N cases', () => {
            const manager = makeManager();
            const investigations = makeInvestigations(3);
            triggerSSEInvestigationList(investigations);

            const options = manager.dropdownMenu.querySelectorAll('.case-dropdown__option');
            expect(options.length).toBe(4);
        });

        it('skips investigations without case_id', () => {
            const manager = makeManager();
            triggerSSEInvestigationList([
                { case_id: 'valid_1', case_title: 'Valid 1' },
                { case_title: 'No ID' },
                { case_id: 'valid_2', case_title: 'Valid 2' }
            ]);

            const options = manager.dropdownMenu.querySelectorAll('.case-dropdown__option:not(.placeholder)');
            expect(options.length).toBe(2);
        });

        it('sorts cases newest first', () => {
            const manager = makeManager();
            triggerSSEInvestigationList([
                { case_id: 'old', case_title: 'Old', created_at: '2025-01-01T00:00:00Z' },
                { case_id: 'new', case_title: 'New', created_at: '2026-01-01T00:00:00Z' },
                { case_id: 'mid', case_title: 'Mid', created_at: '2025-06-01T00:00:00Z' }
            ]);

            expect(manager.userCases[0].case_id).toBe('new');
            expect(manager.userCases[1].case_id).toBe('mid');
            expect(manager.userCases[2].case_id).toBe('old');
        });

        it('handles empty list without throwing', () => {
            const manager = makeManager();
            triggerSSEInvestigationList([]);

            expect(manager.casesLoaded).toBe(true);
            expect(manager.userCases).toEqual([]);
            const options = manager.dropdownMenu.querySelectorAll('.case-dropdown__option');
            expect(options.length).toBe(1);
        });

        it('multiple SSE events are idempotent — last wins', () => {
            const manager = makeManager();
            triggerSSEInvestigationList(makeInvestigations(2));
            triggerSSEInvestigationList(makeInvestigations(5));

            expect(manager.userCases).toHaveLength(5);
        });
    });

    describe('authentication flow', () => {
        it('handles construct-before-auth: SSE list after auth sets casesLoaded', () => {
            const manager = makeManager();
            const session = authState.createMockWebSession();
            triggerAuthEvent(session);
            triggerSSEInvestigationList(makeInvestigations(3));

            expect(manager.casesLoaded).toBe(true);
            expect(manager.userCases).toHaveLength(3);
        });

        it('handles auth-before-init: already authenticated when manager is created', () => {
            const session = authState.createMockWebSession();
            authState.setAuthenticated(session);

            const manager = makeManager();
            triggerSSEInvestigationList(makeInvestigations(3));

            expect(manager.casesLoaded).toBe(true);
            expect(manager.userCases).toHaveLength(3);
        });

        it('resets state on UNAUTHENTICATED', () => {
            const manager = makeManager();
            const session = authState.createMockWebSession();
            triggerAuthEvent(session);
            triggerSSEInvestigationList(makeInvestigations(3));

            expect(manager.casesLoaded).toBe(true);

            triggerUnauthEvent();

            expect(manager.casesLoaded).toBe(false);
            expect(manager.userCases).toHaveLength(0);
            expect(manager.currentCaseId).toBeNull();
            expect(manager.currentInvestigationId).toBeNull();
        });

        it('reloads cases after logout and re-login via SSE', () => {
            const manager = makeManager();
            const session = authState.createMockWebSession();

            triggerAuthEvent(session);
            triggerSSEInvestigationList(makeInvestigations(2));
            expect(manager.userCases).toHaveLength(2);

            triggerUnauthEvent();
            expect(manager.casesLoaded).toBe(false);

            triggerAuthEvent(session);
            triggerSSEInvestigationList(makeInvestigations(5));
            expect(manager.userCases).toHaveLength(5);
        });

        it('rapid auth state changes resolve correctly', () => {
            const manager = makeManager();
            const session = authState.createMockWebSession();

            triggerAuthEvent(session);
            triggerUnauthEvent();
            triggerAuthEvent(session);
            triggerSSEInvestigationList([]);

            expect(manager.casesLoaded).toBe(true);
        });

        it('_isHandlingAuth guard prevents duplicate processing', () => {
            const manager = makeManager();
            manager._isHandlingAuth = true;

            const session = authState.createMockWebSession();
            manager.handleUserAuthenticated({ webSessionModel: session });

            expect(manager._isHandlingAuth).toBe(true);
        });
    });

    describe('loadUserCases', () => {
        it('does not make HTTP call if casesLoaded is true', async () => {
            const manager = makeManager();
            manager.casesLoaded = true;

            await manager.loadUserCases();

            expect(serviceClient.getRequestLog()).toHaveLength(0);
        });

        it('does not make HTTP call if unauthenticated', async () => {
            const manager = makeManager();

            await manager.loadUserCases();

            expect(serviceClient.getRequestLog()).toHaveLength(0);
        });

        it('loads investigations via HTTP when authenticated and not yet loaded', async () => {
            const investigations = makeInvestigations(2);
            serviceClient.setInvestigationsResponse(investigations);

            const session = authState.createMockWebSession();
            authState.setAuthenticated(session);

            const manager = makeManager();
            await manager.loadUserCases();

            const log = serviceClient.getRequestLog();
            expect(log).toHaveLength(1);
            expect(log[0].service).toBe(ComponentName.G8ED);
            expect(log[0].path).toContain('investigations');
            expect(manager.casesLoaded).toBe(true);
            expect(manager.userCases).toHaveLength(2);
        });

        it('resets casesLoaded on HTTP error so retry is possible', async () => {
            serviceClient.setResponse(ComponentName.G8ED, '/api/chat/investigations', {
                ok: false,
                status: 500,
                statusText: 'network error',
                json: async () => ({ success: false, error: 'network error' })
            });

            const session = authState.createMockWebSession();
            authState.setAuthenticated(session);

            const manager = makeManager();
            await manager.loadUserCases();

            expect(manager.casesLoaded).toBe(false);
        });

        it('skips if serviceClient is unavailable', async () => {
            const session = authState.createMockWebSession();
            authState.setAuthenticated(session);
            global.window.serviceClient = undefined;

            const manager = makeManager();

            await expect(manager.loadUserCases()).resolves.not.toThrow();
            expect(manager.casesLoaded).toBe(false);
        });
    });

    describe('refreshUserCases', () => {
        it('skips refresh when unauthenticated', async () => {
            const manager = makeManager();
            await manager.refreshUserCases();
            expect(serviceClient.getRequestLog()).toHaveLength(0);
        });

        it('resets casesLoaded before reloading', async () => {
            const investigations = makeInvestigations(2);
            serviceClient.setInvestigationsResponse(investigations);

            const session = authState.createMockWebSession();
            authState.setAuthenticated(session);

            const manager = makeManager();
            manager.casesLoaded = true;

            await manager.refreshUserCases();

            expect(serviceClient.getRequestLog()).toHaveLength(1);
            expect(manager.casesLoaded).toBe(true);
        });

        it('deduplicates concurrent refresh calls', async () => {
            const investigations = makeInvestigations(2);
            serviceClient.setInvestigationsResponse(investigations);

            const session = authState.createMockWebSession();
            authState.setAuthenticated(session);

            const manager = makeManager();

            await Promise.all([
                manager.refreshUserCases(),
                manager.refreshUserCases()
            ]);

            expect(serviceClient.getRequestLog()).toHaveLength(1);
        });
    });

    describe('switchToCase', () => {
        it('fetches investigation data for the given caseId', async () => {
            const caseId = 'case_abc';
            serviceClient.setResponse(ComponentName.G8ED, `/api/chat/investigations?case_id=${caseId}`, makeSwitchToCaseResponse(caseId));

            const manager = makeManager();
            await manager.switchToCase(caseId);

            const log = serviceClient.getRequestLog();
            expect(log).toHaveLength(1);
            expect(log[0].service).toBe(ComponentName.G8ED);
            expect(log[0].path).toContain(caseId);
        });

        it('sets currentCaseId and currentInvestigationId on success', async () => {
            const caseId = 'case_abc';
            serviceClient.setResponse(ComponentName.G8ED, `/api/chat/investigations?case_id=${caseId}`, makeSwitchToCaseResponse(caseId));

            const manager = makeManager();
            await manager.switchToCase(caseId);

            expect(manager.currentCaseId).toBe(caseId);
            expect(manager.currentInvestigationId).toBe(`inv_for_${caseId}`);
        });

        it('emits CASES.SELECTED with full investigation data', async () => {
            const caseId = 'case_abc';
            serviceClient.setResponse(ComponentName.G8ED, `/api/chat/investigations?case_id=${caseId}`, makeSwitchToCaseResponse(caseId));

            const manager = makeManager();
            eventBus.clear();
            await manager.switchToCase(caseId);

            const selected = eventBus.emitted(EventType.CASE_SELECTED);
            expect(selected).toHaveLength(1);
            expect(selected[0].data.caseId).toBe(caseId);
        });

        it('emits CASES.SWITCHED after selecting', async () => {
            const caseId = 'case_abc';
            serviceClient.setResponse(ComponentName.G8ED, `/api/chat/investigations?case_id=${caseId}`, makeSwitchToCaseResponse(caseId));

            const manager = makeManager();
            eventBus.clear();
            await manager.switchToCase(caseId);

            const switched = eventBus.emitted(EventType.CASE_SWITCHED);
            expect(switched).toHaveLength(1);
            expect(switched[0].data.caseId).toBe(caseId);
        });

        it('clears currentCaseId and emits CASES.CLEARED when called with empty string', async () => {
            const manager = makeManager();
            manager.currentCaseId = 'existing_case';
            eventBus.clear();

            await manager.switchToCase('');

            expect(manager.currentCaseId).toBeNull();
            expect(eventBus.emitted(EventType.CASE_CLEARED)).toHaveLength(1);
            expect(serviceClient.getRequestLog()).toHaveLength(0);
        });

        it('pushes URL state after switching to a case', async () => {
            const caseId = 'case_abc';
            serviceClient.setResponse(ComponentName.G8ED, `/api/chat/investigations?case_id=${caseId}`, makeSwitchToCaseResponse(caseId));

            const manager = makeManager();
            await manager.switchToCase(caseId);

            expect(dom.window.history.pushState).toHaveBeenCalled();
        });
    });

    describe('_applyCaseCreationResult', () => {
        it('sets currentCaseId from case_id field', () => {
            const manager = makeManager();
            manager._applyCaseCreationResult({
                case_id: 'new_case_id',
                title: 'New Case',
                investigation_id: 'inv_new'
            });

            expect(manager.currentCaseId).toBe('new_case_id');
        });

        it('sets currentCaseId from id field as fallback', () => {
            const manager = makeManager();
            manager._applyCaseCreationResult({
                id: 'new_case_id_2',
                title: 'New Case 2',
                investigation_id: 'inv_new_2'
            });

            expect(manager.currentCaseId).toBe('new_case_id_2');
        });

        it('sets wasJustCreated flag', () => {
            const manager = makeManager();
            manager._applyCaseCreationResult({
                case_id: 'some_case',
                title: 'Some Case',
                investigation_id: 'inv_some'
            });

            expect(manager.wasJustCreated).toBe(true);
        });

        it('adds new case to front of userCases', () => {
            const manager = makeManager();
            triggerSSEInvestigationList(makeInvestigations(2));

            manager._applyCaseCreationResult({
                case_id: 'brand_new',
                title: 'Brand New Case',
                investigation_id: 'inv_brand_new'
            });

            expect(manager.userCases[0].case_id).toBe('brand_new');
        });

        it('updates existing case in userCases instead of adding duplicate', () => {
            const manager = makeManager();
            triggerSSEInvestigationList([
                { case_id: 'existing', case_title: 'Old Title', id: 'inv_1', created_at: now() }
            ]);

            manager._applyCaseCreationResult({
                case_id: 'existing',
                title: 'Updated Title',
                investigation_id: 'inv_1'
            });

            expect(manager.userCases).toHaveLength(1);
        });

        it('does nothing when caseData has no id', () => {
            const manager = makeManager();
            expect(() => manager._applyCaseCreationResult({})).not.toThrow();
            expect(manager.currentCaseId).toBeNull();
        });

        it('CASES.CREATED SSE event triggers _applyCaseCreationResult via eventBus', () => {
            const manager = makeManager();
            const spy = vi.spyOn(manager, '_applyCaseCreationResult');

            eventBus.emit(EventType.CASE_CREATED, {
                case_id: 'sse_case',
                title: 'SSE Case',
                investigation_id: 'inv_sse'
            });

            expect(spy).toHaveBeenCalledOnce();
        });
    });

    describe('resetForNewCase', () => {
        it('clears currentCaseId and currentInvestigationId', () => {
            const manager = makeManager();
            manager.currentCaseId = 'some_case';
            manager.currentInvestigationId = 'some_inv';

            manager.resetForNewCase();

            expect(manager.currentCaseId).toBeNull();
            expect(manager.currentInvestigationId).toBeNull();
        });

        it('emits CASES.CLEARED', () => {
            const manager = makeManager();
            eventBus.clear();
            manager.resetForNewCase();

            expect(eventBus.emitted(EventType.CASE_CLEARED)).toHaveLength(1);
        });

        it('resets dropdown value to empty string', () => {
            const manager = makeManager();
            triggerSSEInvestigationList(makeInvestigations(1));
            manager._dropdownValue = 'some_value';
            manager.resetForNewCase();

            expect(manager.getDropdownValue()).toBe('');
        });

        it('new-case-btn click triggers resetForNewCase', () => {
            const manager = makeManager();
            manager.currentCaseId = 'active_case';
            eventBus.clear();

            manager.newCaseBtn.dispatchEvent(new dom.window.MouseEvent('click'));

            expect(manager.currentCaseId).toBeNull();
            expect(eventBus.emitted(EventType.CASE_CLEARED)).toHaveLength(1);
        });
    });

    describe('handleCaseUpdated', () => {
        it('updates case_title when CASES.UPDATED event fires for known case', () => {
            const manager = makeManager();
            triggerSSEInvestigationList([
                { case_id: 'case_x', case_title: 'Original', id: 'inv_x', created_at: now() }
            ]);

            eventBus.emit(EventType.CASE_UPDATED, { case_id: 'case_x', title: 'Renamed' });

            expect(manager.userCases[0].case_title).toBe('Renamed');
        });

        it('ignores update for unknown case_id', () => {
            const manager = makeManager();
            triggerSSEInvestigationList([
                { case_id: 'case_x', case_title: 'Original', id: 'inv_x', created_at: now() }
            ]);

            eventBus.emit(EventType.CASE_UPDATED, { case_id: null, title: 'Should Not Apply' });

            expect(manager.userCases[0].case_title).toBe('Original');
        });

        it('does nothing when case_id is missing from event', () => {
            const manager = makeManager();
            triggerSSEInvestigationList(makeInvestigations(1));

            expect(() => {
                eventBus.emit(EventType.CASE_UPDATED, { title: 'No case_id' });
            }).not.toThrow();
        });

        it('updates dropdown option text for the updated case', () => {
            const manager = makeManager();
            triggerSSEInvestigationList([
                { case_id: 'case_y', case_title: 'Original', id: 'inv_y', created_at: now() }
            ]);

            eventBus.emit(EventType.CASE_UPDATED, { case_id: 'case_y', title: 'Renamed' });

            const option = manager.dropdownMenu.querySelector('[data-value="case_y"]');
            expect(option.textContent).toContain('Renamed');
        });
    });

    describe('getResponsiveTitle', () => {
        it('returns full title when under 120 chars on desktop', () => {
            const manager = makeManager();
            global.window.innerWidth = 1024;
            const title = 'Short title';
            expect(manager.getResponsiveTitle(title)).toBe(title);
        });

        it('truncates at 120 chars with ellipsis on desktop', () => {
            const manager = makeManager();
            global.window.innerWidth = 1024;
            const title = 'A'.repeat(130);
            const result = manager.getResponsiveTitle(title);
            expect(result).toBe('A'.repeat(120) + '...');
        });

        it('truncates at 100 chars on narrow screen (<=720px)', () => {
            const manager = makeManager();
            global.window.innerWidth = 700;
            const title = 'B'.repeat(110);
            const result = manager.getResponsiveTitle(title);
            expect(result).toBe('B'.repeat(100) + '...');
        });

        it('truncates at 80 chars on mobile (<= 480px)', () => {
            const manager = makeManager();
            global.window.innerWidth = 400;
            const title = 'C'.repeat(90);
            const result = manager.getResponsiveTitle(title);
            expect(result).toBe('C'.repeat(80) + '...');
        });
    });

    describe('updateUrlState', () => {
        it('pushes URL state with investigation param when caseId provided', () => {
            const manager = makeManager();
            manager.updateUrlState('case_123');

            expect(dom.window.history.pushState).toHaveBeenCalled();
            const call = dom.window.history.pushState.mock.calls[0];
            expect(call[2]).toContain('case_123');
        });

        it('removes investigation param from URL when caseId is null', () => {
            const manager = makeManager();
            manager.updateUrlState(null);

            const call = dom.window.history.pushState.mock.calls[0];
            expect(call[2]).not.toContain('investigation');
        });

        it('uses replaceState when _restoringFromUrl is true', () => {
            const manager = makeManager();
            manager._restoringFromUrl = true;
            manager.updateUrlState('case_123');

            expect(dom.window.history.replaceState).toHaveBeenCalled();
        });
    });

    describe('handlePopState', () => {
        it('calls switchToCase with caseId from event state', async () => {
            const caseId = 'pop_case';

            const manager = makeManager();
            const spy = vi.spyOn(manager, 'switchToCase').mockResolvedValue();

            manager.handlePopState({ state: { caseId } });

            expect(spy).toHaveBeenCalledWith(caseId);
        });

        it('calls resetForNewCase when state has no caseId and currentCaseId is set', () => {
            const manager = makeManager();
            manager.currentCaseId = 'existing_case';
            const spy = vi.spyOn(manager, 'resetForNewCase');

            manager.handlePopState({ state: {} });

            expect(spy).toHaveBeenCalledOnce();
        });

        it('does nothing when state caseId matches currentCaseId', () => {
            const manager = makeManager();
            manager.currentCaseId = 'same_case';
            const spy = vi.spyOn(manager, 'switchToCase').mockResolvedValue();

            manager.handlePopState({ state: { caseId: 'same_case' } });

            expect(spy).not.toHaveBeenCalled();
        });
    });

    describe('cleanup', () => {
        it('unsubscribes from authState', () => {
            const manager = makeManager();
            const unsubSpy = vi.fn();
            manager.authStateUnsubscribe = unsubSpy;

            manager.cleanup();

            expect(unsubSpy).toHaveBeenCalledOnce();
            expect(manager.authStateUnsubscribe).toBeNull();
        });

        it('does not throw when called without DOM elements', () => {
            dom = new JSDOM('<!DOCTYPE html><html><body></body></html>', { url: 'https://localhost/chat' });
            global.document = dom.window.document;

            const manager = makeManager();

            expect(() => manager.cleanup()).not.toThrow();
        });
    });

    describe('URL state restoration', () => {
        it('restores case from URL query param on investigation list load', async () => {
            dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat?investigation=case_url_1' });
            global.window = dom.window;
            global.document = dom.window.document;
            global.URLSearchParams = dom.window.URLSearchParams;
            global.URL = dom.window.URL;
            global.window.authState = authState;
            global.window.serviceClient = serviceClient;
            vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
            vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

            serviceClient.setResponse(ComponentName.G8ED, '/api/chat/investigations?case_id=case_url_1', makeSwitchToCaseResponse('case_url_1'));

            const manager = new CasesManager(eventBus);
            manager.init();

            triggerSSEInvestigationList([
                { case_id: 'case_url_1', case_title: 'URL Case', id: 'inv_u1', created_at: now() }
            ]);

            await new Promise(r => setTimeout(r, 10));

            const log = serviceClient.getRequestLog();
            expect(log.some(r => r.path.includes('case_url_1'))).toBe(true);
        });
    });

    describe('missing DOM graceful degradation', () => {
        it('stores investigations in userCases even without dropdownMenu', () => {
            dom = new JSDOM('<!DOCTYPE html><html><body></body></html>', { url: 'https://localhost/chat' });
            global.document = dom.window.document;

            const manager = makeManager();
            expect(manager.dropdownMenu).toBeNull();

            triggerSSEInvestigationList(makeInvestigations(3));

            expect(manager.userCases).toHaveLength(3);
            expect(manager.casesLoaded).toBe(true);
        });
    });
});

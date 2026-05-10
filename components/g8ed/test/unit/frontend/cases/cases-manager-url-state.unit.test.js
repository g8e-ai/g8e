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

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { JSDOM } from 'jsdom';
import { CasesManager } from '@g8ed/public/js/components/cases-manager.js';
import { MockEventBus, MockAuthState, MockServiceClient } from '@test/mocks/mock-browser-env.js';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { ComponentName } from '@g8ed/public/js/constants/service-client-constants.js';
import { now } from '@test/fixtures/base.fixture.js';

function flushPromises() {
    return new Promise(resolve => setImmediate(resolve));
}

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

describe('CasesManager URL State [UNIT]', () => {
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
        vi.useRealTimers();
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

    function triggerSSEInvestigationList(investigations) {
        eventBus.emit(EventType.INVESTIGATION_LIST_COMPLETED, {
            investigations,
            count: investigations.length,
            timestamp: now()
        });
    }

    describe('getInvestigationFromUrl', () => {
        it('reads investigation ID from URL query parameter', () => {
            dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat?investigation=case_abc' });
            global.window = dom.window;
            global.document = dom.window.document;
            global.URLSearchParams = dom.window.URLSearchParams;
            global.URL = dom.window.URL;
            global.window.authState = authState;
            global.window.serviceClient = serviceClient;
            vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
            vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

            const manager = makeManager();

            expect(manager.getInvestigationFromUrl()).toBe('case_abc');
        });

        it('returns null when no investigation param in URL', () => {
            const manager = makeManager();

            expect(manager.getInvestigationFromUrl()).toBeNull();
        });

        it('returns correct value when investigation param is among multiple params', () => {
            dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat?foo=bar&investigation=case_xyz&baz=qux' });
            global.window = dom.window;
            global.document = dom.window.document;
            global.URLSearchParams = dom.window.URLSearchParams;
            global.URL = dom.window.URL;
            global.window.authState = authState;
            global.window.serviceClient = serviceClient;
            vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
            vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

            const manager = makeManager();

            expect(manager.getInvestigationFromUrl()).toBe('case_xyz');
        });
    });

    describe('updateUrlState', () => {
        it('pushes URL with investigation param when caseId provided', () => {
            const manager = makeManager();
            manager.updateUrlState('case_abc');

            expect(dom.window.history.pushState).toHaveBeenCalled();
            const call = dom.window.history.pushState.mock.calls[0];
            expect(call[0]).toEqual({ caseId: 'case_abc' });
            expect(call[2]).toContain('investigation=case_abc');
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
            manager.updateUrlState('case_abc');

            expect(dom.window.history.replaceState).toHaveBeenCalled();
            expect(dom.window.history.pushState).not.toHaveBeenCalled();
        });

        it('preserves other query parameters when setting investigation', () => {
            dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat?foo=bar' });
            global.window = dom.window;
            global.document = dom.window.document;
            global.URLSearchParams = dom.window.URLSearchParams;
            global.URL = dom.window.URL;
            global.window.authState = authState;
            global.window.serviceClient = serviceClient;
            vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
            vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

            const manager = makeManager();
            manager.updateUrlState('case_abc');

            const call = dom.window.history.pushState.mock.calls[0];
            expect(call[2]).toContain('foo=bar');
            expect(call[2]).toContain('investigation=case_abc');
        });
    });

    describe('URL state restoration', () => {
        it('restores case from URL param when matching case exists in list', async () => {
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

            const manager = makeManager();

            triggerSSEInvestigationList([
                { case_id: 'case_url_1', case_title: 'URL Case', id: 'inv_u1', created_at: now() }
            ]);

            await flushPromises();

            const log = serviceClient.getRequestLog();
            expect(log.some(r => r.path.includes('case_url_1'))).toBe(true);
        });

        it('does not restore when URL case ID is not in user cases list', () => {
            dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat?investigation=case_nonexistent' });
            global.window = dom.window;
            global.document = dom.window.document;
            global.URLSearchParams = dom.window.URLSearchParams;
            global.URL = dom.window.URL;
            global.window.authState = authState;
            global.window.serviceClient = serviceClient;
            vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
            vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

            const manager = makeManager();

            triggerSSEInvestigationList([
                { case_id: 'case_abc', case_title: 'Test Case', id: 'inv_1', created_at: now() }
            ]);

            expect(manager.currentCaseId).toBeNull();
            expect(serviceClient.getRequestLog()).toHaveLength(0);
        });

        it('resets for new case when no investigation in URL and cases exist', () => {
            const manager = makeManager();
            eventBus.clear();

            triggerSSEInvestigationList([
                { case_id: 'case_abc', case_title: 'Test Case', id: 'inv_1', created_at: now() }
            ]);

            expect(manager.currentCaseId).toBeNull();
            expect(eventBus.emitted(EventType.CASE_CLEARED)).toHaveLength(1);
        });

        it('does not call resetForNewCase when cases list is empty', () => {
            const manager = makeManager();
            const spy = vi.spyOn(manager, 'resetForNewCase');
            eventBus.clear();

            triggerSSEInvestigationList([]);

            expect(spy).not.toHaveBeenCalled();
        });

        it('sets _restoringFromUrl flag during URL-based restoration', async () => {
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

            const manager = makeManager();

            triggerSSEInvestigationList([
                { case_id: 'case_url_1', case_title: 'URL Case', id: 'inv_u1', created_at: now() }
            ]);

            await flushPromises();

            expect(dom.window.history.replaceState).toHaveBeenCalled();
            expect(dom.window.history.pushState).not.toHaveBeenCalled();
        });
    });

    describe('handlePopState', () => {
        it('calls switchToCase with caseId from event state', () => {
            const manager = makeManager();
            const spy = vi.spyOn(manager, 'switchToCase').mockResolvedValue();

            manager.handlePopState({ state: { caseId: 'case_pop' } });

            expect(spy).toHaveBeenCalledWith('case_pop');
        });

        it('falls back to URL param when popstate state is null', () => {
            dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat?investigation=case_abc' });
            global.window = dom.window;
            global.document = dom.window.document;
            global.URLSearchParams = dom.window.URLSearchParams;
            global.URL = dom.window.URL;
            global.window.authState = authState;
            global.window.serviceClient = serviceClient;
            vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
            vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

            const manager = makeManager();
            const spy = vi.spyOn(manager, 'switchToCase').mockResolvedValue();

            manager.handlePopState({ state: null });

            expect(spy).toHaveBeenCalledWith('case_abc');
        });

        it('calls resetForNewCase when state has no caseId and currentCaseId is set', () => {
            const manager = makeManager();
            manager.currentCaseId = 'case_existing';
            const spy = vi.spyOn(manager, 'resetForNewCase');

            manager.handlePopState({ state: {} });

            expect(spy).toHaveBeenCalledOnce();
        });

        it('does nothing when state caseId matches currentCaseId', () => {
            const manager = makeManager();
            manager.currentCaseId = 'case_same';
            const switchSpy = vi.spyOn(manager, 'switchToCase').mockResolvedValue();
            const resetSpy = vi.spyOn(manager, 'resetForNewCase');

            manager.handlePopState({ state: { caseId: 'case_same' } });

            expect(switchSpy).not.toHaveBeenCalled();
            expect(resetSpy).not.toHaveBeenCalled();
        });

        it('popstate listener is registered on init', () => {
            const addEventListenerSpy = vi.spyOn(dom.window, 'addEventListener');
            makeManager();

            expect(addEventListenerSpy).toHaveBeenCalledWith('popstate', expect.any(Function));
        });

        it('popstate listener is removed on cleanup', () => {
            const removeEventListenerSpy = vi.spyOn(dom.window, 'removeEventListener');
            const manager = makeManager();
            manager.cleanup();

            expect(removeEventListenerSpy).toHaveBeenCalledWith('popstate', expect.any(Function));
        });
    });

    describe('resetForNewCase — URL clearing', () => {
        it('clears investigation param from URL when resetting for new case', () => {
            dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat?investigation=case_abc' });
            global.window = dom.window;
            global.document = dom.window.document;
            global.URLSearchParams = dom.window.URLSearchParams;
            global.URL = dom.window.URL;
            global.window.authState = authState;
            global.window.serviceClient = serviceClient;
            vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
            vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

            const manager = makeManager();
            manager.currentCaseId = 'case_abc';
            manager.resetForNewCase();

            const call = dom.window.history.pushState.mock.calls[0];
            expect(call[0]).toEqual({ caseId: null });
            expect(call[2]).not.toContain('investigation');
        });
    });

    describe('switchToCase — URL updates', () => {
        it('pushes URL with investigation param after switching to a case', async () => {
            serviceClient.setResponse(ComponentName.G8ED, '/api/chat/investigations?case_id=case_abc', makeSwitchToCaseResponse('case_abc'));

            const manager = makeManager();
            await manager.switchToCase('case_abc');

            expect(dom.window.history.pushState).toHaveBeenCalled();
            const call = dom.window.history.pushState.mock.calls[0];
            expect(call[2]).toContain('investigation=case_abc');
        });

        it('clears investigation param from URL when switching to null', async () => {
            dom = new JSDOM(CASES_HTML, { url: 'https://localhost/chat?investigation=case_abc' });
            global.window = dom.window;
            global.document = dom.window.document;
            global.URLSearchParams = dom.window.URLSearchParams;
            global.URL = dom.window.URL;
            global.window.authState = authState;
            global.window.serviceClient = serviceClient;
            vi.spyOn(dom.window.history, 'pushState').mockImplementation(() => {});
            vi.spyOn(dom.window.history, 'replaceState').mockImplementation(() => {});

            const manager = makeManager();
            await manager.switchToCase(null);

            const call = dom.window.history.pushState.mock.calls[0];
            expect(call[0]).toEqual({ caseId: null });
            expect(call[2]).not.toContain('investigation');
        });
    });
});

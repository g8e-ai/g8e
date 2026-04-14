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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

let notificationService;
let NotificationService;

beforeEach(async () => {
    vi.resetModules();
    vi.useFakeTimers();
    const existing = document.getElementById('system-notification-tray');
    if (existing) existing.remove();
    ({ notificationService, NotificationService } = await import('@g8ed/public/js/utils/notification-service.js'));
    notificationService.init();
});

afterEach(() => {
    vi.runAllTimers();
    vi.useRealTimers();
    vi.restoreAllMocks();
});

describe('NotificationService — init() [UNIT]', () => {
    it('creates the system-notification-tray element in document.body', () => {
        expect(document.getElementById('system-notification-tray')).not.toBeNull();
    });

    it('sets the correct CSS class on the container', () => {
        const container = document.getElementById('system-notification-tray');
        expect(container.className).toBe('system-notification-tray');
    });

    it('is idempotent — calling init() twice reuses the existing element', () => {
        notificationService.init();
        const containers = document.querySelectorAll('#system-notification-tray');
        expect(containers.length).toBe(1);
    });

    it('adopts a pre-existing tray element if already in the DOM', () => {
        const existing = document.getElementById('system-notification-tray');
        notificationService.init();
        expect(notificationService.container).toBe(existing);
    });
});

describe('NotificationService — show() before init() [UNIT]', () => {
    it('returns -1 when container is not initialized', async () => {
        vi.resetModules();
        const { notificationService: uninit } = await import('@g8ed/public/js/utils/notification-service.js');
        expect(uninit.show('test')).toBe(-1);
    });
});

describe('NotificationService — show() [UNIT]', () => {
    it('returns a positive integer notification ID', () => {
        const id = notificationService.show('Hello');
        expect(typeof id).toBe('number');
        expect(id).toBeGreaterThan(0);
    });

    it('increments the ID for each new notification', () => {
        const id1 = notificationService.show('First');
        const id2 = notificationService.show('Second');
        expect(id2).toBe(id1 + 1);
    });

    it('appends a notification element to the container', () => {
        notificationService.show('Hello');
        const container = document.getElementById('system-notification-tray');
        expect(container.children.length).toBe(1);
    });

    it('prepends — most recent notification is first child', () => {
        notificationService.show('First');
        notificationService.show('Second');
        const container = document.getElementById('system-notification-tray');
        expect(container.children[0].querySelector('.system-notification__message').textContent).toBe('Second');
    });

    it('sets the message text on the notification element', () => {
        notificationService.show('My message');
        const el = document.querySelector('.system-notification__message');
        expect(el.textContent).toBe('My message');
    });

    it('adds the type as a CSS class', () => {
        notificationService.show('Oops', 'error');
        const el = document.querySelector('.system-notification');
        expect(el.classList.contains('error')).toBe(true);
    });

    it('defaults to info type when type is omitted', () => {
        notificationService.show('Info');
        const el = document.querySelector('.system-notification');
        expect(el.classList.contains('info')).toBe(true);
    });

    it('sets role=alert for error type', () => {
        notificationService.show('Err', 'error');
        expect(document.querySelector('.system-notification').getAttribute('role')).toBe('alert');
    });

    it('sets role=alert for warning type', () => {
        notificationService.show('Warn', 'warning');
        expect(document.querySelector('.system-notification').getAttribute('role')).toBe('alert');
    });

    it('sets role=status for info type', () => {
        notificationService.show('Info', 'info');
        expect(document.querySelector('.system-notification').getAttribute('role')).toBe('status');
    });

    it('sets role=status for success type', () => {
        notificationService.show('Ok', 'success');
        expect(document.querySelector('.system-notification').getAttribute('role')).toBe('status');
    });

    it('sets aria-live=assertive for error type', () => {
        notificationService.show('Err', 'error');
        expect(document.querySelector('.system-notification').getAttribute('aria-live')).toBe('assertive');
    });

    it('sets aria-live=polite for info type', () => {
        notificationService.show('Info', 'info');
        expect(document.querySelector('.system-notification').getAttribute('aria-live')).toBe('polite');
    });

    it('renders optional title when provided', () => {
        notificationService.show('Msg', 'info', { title: 'My Title' });
        const titleEl = document.querySelector('.system-notification__title');
        expect(titleEl).not.toBeNull();
        expect(titleEl.textContent).toBe('My Title');
    });

    it('does not render title element when title is absent', () => {
        notificationService.show('Msg', 'info');
        expect(document.querySelector('.system-notification__title')).toBeNull();
    });

    it('renders optional detail when provided', () => {
        notificationService.show('Msg', 'info', { detail: 'extra info' });
        const detailEl = document.querySelector('.system-notification__detail');
        expect(detailEl).not.toBeNull();
        expect(detailEl.textContent).toBe('extra info');
    });

    it('does not render detail element when detail is absent', () => {
        notificationService.show('Msg', 'info');
        expect(document.querySelector('.system-notification__detail')).toBeNull();
    });

    it('renders action link when action.text and action.url are provided', () => {
        notificationService.show('Msg', 'info', { action: { text: 'Click me', url: '/go' } });
        const actionEl = document.querySelector('.system-notification__action');
        expect(actionEl).not.toBeNull();
        expect(actionEl.textContent).toBe('Click me');
        expect(actionEl.href).toContain('/go');
    });

    it('does not render action link when action is absent', () => {
        notificationService.show('Msg', 'info');
        expect(document.querySelector('.system-notification__action')).toBeNull();
    });

    it('does not render action link when action.url is missing', () => {
        notificationService.show('Msg', 'info', { action: { text: 'Click me' } });
        expect(document.querySelector('.system-notification__action')).toBeNull();
    });

    it('tracks the notification in the notifications map', () => {
        const id = notificationService.show('Hello');
        expect(notificationService.notifications.has(id)).toBe(true);
    });

    it('uses custom icon when options.icon is provided', () => {
        notificationService.show('Msg', 'info', { icon: 'star' });
        const iconEl = document.querySelector('.system-notification__icon');
        expect(iconEl.textContent).toBe('star');
    });

    it('uses type-default icon when options.icon is absent', () => {
        notificationService.show('Msg', 'error');
        const iconEl = document.querySelector('.system-notification__icon');
        expect(iconEl.textContent).toBe('cancel');
    });

    it('auto-removes the notification after duration elapses', () => {
        notificationService.show('Auto', 'info', { duration: 3000 });
        expect(notificationService.notifications.size).toBe(1);
        vi.advanceTimersByTime(3000);
        expect(notificationService.notifications.size).toBe(0);
    });

    it('does not auto-remove when duration is 0', () => {
        notificationService.show('Sticky', 'info', { duration: 0 });
        vi.advanceTimersByTime(60000);
        expect(notificationService.notifications.size).toBe(1);
    });

    it('does not apply is-exiting before is-visible when duration < NOTIFICATION_EXIT_ANIMATION_MS', () => {
        notificationService.show('Fast', 'info', { duration: 100 });
        const el = document.querySelector('.system-notification');
        expect(el.classList.contains('is-exiting')).toBe(false);
        vi.advanceTimersByTime(100);
        expect(notificationService.notifications.size).toBe(0);
    });
});

describe('NotificationService — dismiss() [UNIT]', () => {
    it('removes the notification from the tracking map', () => {
        const id = notificationService.show('Hello', 'info', { duration: 0 });
        notificationService.dismiss(id);
        vi.advanceTimersByTime(1000);
        expect(notificationService.notifications.has(id)).toBe(false);
    });

    it('removes the element from the DOM after animation', () => {
        const id = notificationService.show('Hello', 'info', { duration: 0 });
        notificationService.dismiss(id);
        vi.advanceTimersByTime(1000);
        expect(document.querySelector('.system-notification')).toBeNull();
    });

    it('adds is-exiting class immediately on dismiss', () => {
        const id = notificationService.show('Hello', 'info', { duration: 0 });
        const el = document.querySelector('.system-notification');
        notificationService.dismiss(id);
        expect(el.classList.contains('is-exiting')).toBe(true);
    });

    it('is a no-op for an unknown ID', () => {
        expect(() => notificationService.dismiss(9999)).not.toThrow();
    });

    it('cancels pending auto-dismiss timers when dismissed early', () => {
        const id = notificationService.show('Hello', 'info', { duration: 5000 });
        notificationService.dismiss(id);
        vi.advanceTimersByTime(5000);
        expect(notificationService.notifications.has(id)).toBe(false);
        expect(document.querySelectorAll('.system-notification').length).toBe(0);
    });
});

describe('NotificationService — clearAll() [UNIT]', () => {
    it('dismisses all active notifications', () => {
        notificationService.show('A', 'info', { duration: 0 });
        notificationService.show('B', 'info', { duration: 0 });
        notificationService.show('C', 'info', { duration: 0 });
        notificationService.clearAll();
        vi.advanceTimersByTime(1000);
        expect(notificationService.notifications.size).toBe(0);
    });

    it('is safe to call with no active notifications', () => {
        expect(() => notificationService.clearAll()).not.toThrow();
    });
});

describe('NotificationService — convenience methods [UNIT]', () => {
    it('info() calls show() with type "info" and returns an ID', () => {
        const spy = vi.spyOn(notificationService, 'show');
        const id = notificationService.info('Info message');
        expect(spy).toHaveBeenCalledWith('Info message', 'info', {});
        expect(typeof id).toBe('number');
    });

    it('success() calls show() with type "success"', () => {
        const spy = vi.spyOn(notificationService, 'show');
        notificationService.success('Done');
        expect(spy).toHaveBeenCalledWith('Done', 'success', {});
    });

    it('warning() calls show() with type "warning"', () => {
        const spy = vi.spyOn(notificationService, 'show');
        notificationService.warning('Careful');
        expect(spy).toHaveBeenCalledWith('Careful', 'warning', {});
    });

    it('error() calls show() with type "error"', () => {
        const spy = vi.spyOn(notificationService, 'show');
        notificationService.error('Bad');
        expect(spy).toHaveBeenCalledWith('Bad', 'error', {});
    });

    it('convenience methods forward options', () => {
        const spy = vi.spyOn(notificationService, 'show');
        notificationService.error('Oops', { duration: 443 });
        expect(spy).toHaveBeenCalledWith('Oops', 'error', { duration: 443 });
    });
});

describe('NotificationService — window global [UNIT]', () => {
    it('assigns the singleton to window.notificationService', () => {
        expect(window.notificationService).toBe(notificationService);
    });
});

describe('NotificationService — singleton [UNIT]', () => {
    it('exports a NotificationService instance as notificationService', () => {
        expect(notificationService).toBeInstanceOf(NotificationService);
    });
});

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

import { NOTIFICATION_DEFAULT_DURATION_MS, NOTIFICATION_EXIT_ANIMATION_MS } from '../constants/app-constants.js';

class NotificationService {
    constructor() {
        this.container = null;
        this.notifications = new Map();
        this.notificationId = 0;
    }

    init() {
        let container = document.getElementById('system-notification-tray');
        if (!container) {
            container = document.createElement('div');
            container.id = 'system-notification-tray';
            container.className = 'system-notification-tray';
            document.body.appendChild(container);
        }
        this.container = container;
    }

    show(message, type = 'info', options = {}) {
        if (!this.container) return -1;

        const id = ++this.notificationId;
        const duration = options.duration !== undefined ? options.duration : NOTIFICATION_DEFAULT_DURATION_MS;

        const iconMap = {
            error: 'cancel',
            warning: 'warning',
            success: 'check_circle',
            info: 'info'
        };
        const iconName = options.icon || iconMap[type] || iconMap.info;

        const notification = document.createElement('div');
        notification.className = `system-notification ${type}`;
        const isUrgent = type === 'error' || type === 'warning';
        notification.setAttribute('role', isUrgent ? 'alert' : 'status');
        notification.setAttribute('aria-live', isUrgent ? 'assertive' : 'polite');
        notification.setAttribute('data-notification-id', id);

        const iconWrapper = document.createElement('span');
        iconWrapper.className = 'material-symbols-outlined system-notification__icon';
        iconWrapper.textContent = iconName;

        const body = document.createElement('div');
        body.className = 'system-notification__body';

        if (options.title) {
            const titleEl = document.createElement('p');
            titleEl.className = 'system-notification__title';
            titleEl.textContent = options.title;
            body.appendChild(titleEl);
        }

        const content = document.createElement('p');
        content.className = 'system-notification__message';
        content.textContent = message;
        body.appendChild(content);

        if (options.detail) {
            const detailEl = document.createElement('p');
            detailEl.className = 'system-notification__detail';
            detailEl.textContent = options.detail;
            body.appendChild(detailEl);
        }

        if (options.action && options.action.text && options.action.url) {
            const actionEl = document.createElement('a');
            actionEl.className = 'system-notification__action';
            actionEl.href = options.action.url;
            actionEl.textContent = options.action.text;
            actionEl.addEventListener('click', (e) => e.stopPropagation());
            body.appendChild(actionEl);
        }

        notification.appendChild(iconWrapper);
        notification.appendChild(body);
        this.container.prepend(notification);

        requestAnimationFrame(() => {
            notification.classList.add('is-visible');
        });

        const timeouts = { exit: null, remove: null };

        if (duration > 0) {
            timeouts.exit = setTimeout(() => {
                notification.classList.add('is-exiting');
            }, Math.max(0, duration - NOTIFICATION_EXIT_ANIMATION_MS));

            timeouts.remove = setTimeout(() => {
                this._removeNotification(id);
            }, duration);
        }

        notification.addEventListener('pointerdown', (e) => {
            if (e.target.tagName !== 'A') {
                this.dismiss(id);
            }
        }, { once: true });

        this.notifications.set(id, { element: notification, timeouts });
        return id;
    }

    dismiss(id) {
        const entry = this.notifications.get(id);
        if (!entry) return;

        const { element, timeouts } = entry;

        if (timeouts.exit) clearTimeout(timeouts.exit);
        if (timeouts.remove) clearTimeout(timeouts.remove);

        element.classList.add('is-exiting');
        setTimeout(() => {
            this._removeNotification(id);
        }, NOTIFICATION_EXIT_ANIMATION_MS);
    }

    _removeNotification(id) {
        const entry = this.notifications.get(id);
        if (!entry) return;

        entry.element.remove();
        this.notifications.delete(id);
    }

    info(message, options = {}) {
        return this.show(message, 'info', options);
    }

    success(message, options = {}) {
        return this.show(message, 'success', options);
    }

    warning(message, options = {}) {
        return this.show(message, 'warning', options);
    }

    error(message, options = {}) {
        return this.show(message, 'error', options);
    }

    clearAll() {
        for (const id of this.notifications.keys()) {
            this.dismiss(id);
        }
    }
}

const notificationService = new NotificationService();

if (typeof window !== 'undefined') {
    window.notificationService = notificationService;
}

export { notificationService, NotificationService };

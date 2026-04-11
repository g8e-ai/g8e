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
 * ThemeManager - Single source of truth for application theme state.
 *
 * Loaded as a plain (non-module) script early in <head> so it is available
 * synchronously to all other scripts on the page.
 *
 * Responsibilities:
 *   - Read the server-rendered data-theme on <body> (set from cookie via server.js)
 *   - Apply the theme immediately to prevent flash
 *   - Persist theme changes to the cookie
 *   - Notify listeners of theme changes via a custom DOM event
 *   - Broadcast theme changes into embedded iframes via postMessage
 *
 * All other components (auth.js, hamburger-menu.js, app.js, operator-panel.js,
 * markdown.js) must delegate to window.ThemeManager instead of managing theme
 * state themselves.
 */
(function () {
    'use strict';

    const DEFAULT_THEME = 'dark';
    const VALID_THEMES = ['dark', 'light'];
    const COOKIE_NAME = 'theme';
    const COOKIE_MAX_AGE = 31536000;
    const EVENT_NAME = 'g8e:themechange';

    function getBodyTheme() {
        return document.body ? document.body.getAttribute('data-theme') : null;
    }

    function isValid(theme) {
        return VALID_THEMES.includes(theme);
    }

    function readCookie() {
        const match = document.cookie.match(/(?:^|;\s*)theme=([^;]+)/);
        return match ? match[1] : null;
    }

    function writeCookie(theme) {
        document.cookie = COOKIE_NAME + '=' + theme + '; path=/; max-age=' + COOKIE_MAX_AGE + '; SameSite=Lax';
    }

    function applyTheme(theme) {
        if (document.body) {
            document.body.setAttribute('data-theme', theme);
        }
    }

    function broadcastToIframes(theme) {
        document.querySelectorAll('iframe').forEach(function (iframe) {
            try {
                iframe.contentWindow.postMessage({ type: 'g8e-theme-change', theme: theme }, '*');
            } catch (_) {}
        });
    }

    function dispatchChangeEvent(theme) {
        try {
            document.dispatchEvent(new CustomEvent(EVENT_NAME, { detail: { theme: theme } }));
        } catch (_) {}
    }

    var ThemeManager = {
        /**
         * Returns the current active theme string ('dark' | 'light').
         */
        getTheme: function () {
            var t = getBodyTheme();
            return isValid(t) ? t : DEFAULT_THEME;
        },

        /**
         * Returns the default theme used when no preference is stored.
         */
        getDefaultTheme: function () {
            return DEFAULT_THEME;
        },

        /**
         * Toggle between dark and light, persist, notify.
         */
        toggle: function () {
            var current = this.getTheme();
            var next = VALID_THEMES.find(function (t) { return t !== current; }) || DEFAULT_THEME;
            this.setTheme(next);
            return next;
        },

        /**
         * Set theme explicitly, persist, notify.
         * @param {string} theme - 'dark' | 'light'
         */
        setTheme: function (theme) {
            if (!isValid(theme)) {
                return;
            }
            applyTheme(theme);
            writeCookie(theme);
            dispatchChangeEvent(theme);
            broadcastToIframes(theme);
        },

        /**
         * Subscribe to theme change events.
         * @param {function} callback - called with (theme) on change
         * @returns {function} unsubscribe function
         */
        onChange: function (callback) {
            function handler(e) { callback(e.detail.theme); }
            document.addEventListener(EVENT_NAME, handler);
            return function () { document.removeEventListener(EVENT_NAME, handler); };
        },

        /**
         * Initialize: ensure body has a valid data-theme attribute.
         * Called once on load. Server already sets the attribute via EJS,
         * so this is a safety net only.
         */
        init: function () {
            var current = getBodyTheme();
            if (!isValid(current)) {
                var cookie = readCookie();
                var theme = isValid(cookie) ? cookie : DEFAULT_THEME;
                applyTheme(theme);
                writeCookie(theme);
            } else {
                writeCookie(current);
            }
        }
    };

    window.ThemeManager = ThemeManager;

    if (document.body) {
        ThemeManager.init();
    } else {
        document.addEventListener('DOMContentLoaded', function () {
            ThemeManager.init();
        });
    }
})();

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
 * Logger - Console logging controlled by a server-side user preference.
 *
 * Enabled state is set on window.__DEV_LOGS_ENABLED by the server at page load,
 * sourced from the authenticated user's dev_logs_enabled document field.
 * Only admin and superadmin users can enable this setting.
 * localStorage is not used — the gate is enforced server-side.
 *
 * Usage:
 *   import { devLogger } from './utils/dev-logger.js';
 *   devLogger.log('Debug info:', data);
 *   devLogger.warn('Warning:', message);
 *   devLogger.error('Error:', error);
 */

function isLoggingEnabled() {
    return window.__DEV_LOGS_ENABLED === true;
}

export const devLogger = {
    log: (...args) => {
        if (isLoggingEnabled()) {
            console.log(...args);
        }
    },

    warn: (...args) => {
        if (isLoggingEnabled()) {
            console.warn(...args);
        }
    },

    error: (...args) => {
        if (isLoggingEnabled()) {
            console.error(...args);
        }
    },

    info: (...args) => {
        if (isLoggingEnabled()) {
            console.info(...args);
        }
    },

    debug: (...args) => {
        if (isLoggingEnabled()) {
            console.debug(...args);
        }
    },

    table: (...args) => {
        if (isLoggingEnabled()) {
            console.table(...args);
        }
    },

    group: (...args) => {
        if (isLoggingEnabled()) {
            console.group(...args);
        }
    },

    groupCollapsed: (...args) => {
        if (isLoggingEnabled()) {
            console.groupCollapsed(...args);
        }
    },

    groupEnd: () => {
        if (isLoggingEnabled()) {
            console.groupEnd();
        }
    },

    isDev: () => isLoggingEnabled(),

    authError: (errorType, context = {}) => {
        if (isLoggingEnabled()) {
            console.group('%c[AUTH ERROR] ' + errorType, 'color: #ff6b6b; font-weight: bold;');
            console.error('Error Type:', errorType);
            console.log('Timestamp:', new Date().toISOString());
            console.log('Current URL:', window.location.href);
            console.log('Referrer:', document.referrer || 'none');

            const urlParams = new URLSearchParams(window.location.search);
            const params = {};
            urlParams.forEach((value, key) => {
                params[key] = value;
            });
            console.log('URL Parameters:', params);

            if (Object.keys(context).length > 0) {
                console.log('Additional Context:', context);
            }

            console.log('WebSession Storage Keys:', Object.keys(window?.sessionStorage ?? {}));
            console.log('Local Storage Keys:', Object.keys(window?.localStorage ?? {}));

            const cookieNames = document.cookie.split(';').map(c => c.trim().split('=')[0]).filter(Boolean);
            console.log('Cookie Names:', cookieNames);

            console.groupEnd();
        }
    },

    oauthFlow: (stage, data = {}) => {
        if (isLoggingEnabled()) {
            console.log(
                '%c[OAUTH] ' + stage,
                'color: #4ecdc4; font-weight: bold;',
                data
            );
        }
    }
};

export default devLogger;

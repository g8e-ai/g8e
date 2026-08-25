// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { devLogger } from '../utils/dev-logger.js';

/**
 * Footer - Application footer component.
 *
 * Owns: footer.app-footer
 * Stateless — renders static links. Extend if dynamic footer content is needed.
 */
export class Footer {
    constructor(eventBus) {
        this.eventBus = eventBus;
        this._root = null;
    }

    init() {
        this._root = document.querySelector('footer.app-footer');

        if (!this._root) {
            devLogger.warn('[FOOTER] Root element not found');
            return;
        }

        devLogger.log('[FOOTER] Initialized');
    }

    destroy() {
        this._root = null;
    }
}

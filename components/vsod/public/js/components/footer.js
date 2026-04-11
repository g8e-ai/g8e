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

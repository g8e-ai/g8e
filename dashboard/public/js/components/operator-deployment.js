// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { templateLoader } from '../utils/template-loader.js';

/**
 * OperatorDeployment - Operator binary usage reference panel.
 */
export class OperatorDeployment {
    constructor(opts = {}) {
        this.onClose    = opts.onClose || null;
        this._container = null;
    }

    async mount(container) {
        this._container = container;
        container.innerHTML = '';
        const template = await templateLoader.load('operator-deployment');
        const wrap = document.createElement('div');
        wrap.innerHTML = template;
        container.appendChild(wrap.firstElementChild);
    }

    destroy() {
        this._container = null;
    }

    setUser() {}
}

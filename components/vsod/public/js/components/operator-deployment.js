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

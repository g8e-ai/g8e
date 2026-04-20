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

import { OperatorStatus } from '../constants/operator-constants.js';

class OperatorSessionService {
    constructor() {
        this._boundOperatorId = null;
        this._boundOperators = [];
    }

    setBoundOperators(operators) {
        if (!Array.isArray(operators)) {
            throw new Error('OperatorSessionService.setBoundOperators requires an array');
        }
        this._boundOperators = operators;
        this._boundOperatorId = operators.find(op => op.status === OperatorStatus.BOUND)?.operator_id ?? null;
    }

    clearBoundOperators() {
        this._boundOperators = [];
        this._boundOperatorId = null;
    }

    getBoundOperatorId() {
        return this._boundOperatorId;
    }

    getBoundOperators() {
        return this._boundOperators;
    }

    isBound() {
        return this._boundOperatorId !== null;
    }

    getBoundOperatorForSession(webSessionId) {
        if (!webSessionId) return null;
        return this._boundOperators.find(
            op => op.status === OperatorStatus.BOUND && op.bound_web_session_id === webSessionId
        ) ?? null;
    }
}

export const operatorSessionService = new OperatorSessionService();

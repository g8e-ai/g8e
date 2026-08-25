// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

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
            op => op.status === OperatorStatus.BOUND && op.web_session_id === webSessionId
        ) ?? null;
    }
}

export const operatorSessionService = new OperatorSessionService();

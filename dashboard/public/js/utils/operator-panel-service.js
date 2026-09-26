// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { ServiceName } from '../constants/service-client-constants.js';
import { ApiPaths } from '../constants/api-paths.js';

function normalizeOperator(op) {
    if (!op) return op;
    return {
        ...op,
        operator_id: op.operator_id || op.id,
        web_session_id: op.web_session_id || op.bound_web_session_id,
    };
}

class OperatorPanelService {
    constructor() {
        this._client = null;
    }

    _getClient() {
        return this._client || window.serviceClient;
    }

    setClient(client) {
        this._client = client;
    }

    bindOperator(operatorId) {
        return this._getClient().post(ServiceName.GATEWAY, ApiPaths.operator.bind(), {
            operator_id: operatorId,
        });
    }

    unbindOperator(body = {}) {
        return this._getClient().post(ServiceName.GATEWAY, ApiPaths.operator.unbind(), body);
    }

    bindAllOperators(operatorIds) {
        return this._getClient().post(ServiceName.GATEWAY, ApiPaths.operator.bind(), {
            operator_ids: operatorIds,
        });
    }

    unbindAllOperators(operatorIds) {
        return this._getClient().post(ServiceName.GATEWAY, ApiPaths.operator.unbind(), {
            operator_ids: operatorIds,
        });
    }

    stopOperator(operatorId) {
        return this._getClient().post(ServiceName.GATEWAY, ApiPaths.operator.stop(operatorId), {});
    }

    getOperatorDetails(operatorId) {
        return this._getClient().get(ServiceName.GATEWAY, ApiPaths.operator.get(operatorId));
    }

    getOperatorApiKey() {
        return Promise.reject(new Error('Operator API keys are not available from the browser'));
    }

    refreshOperatorApiKey() {
        return Promise.reject(new Error('Operator API keys are not available from the browser'));
    }

    generateDeviceLink() {
        return Promise.reject(new Error('Device links are not available from the browser'));
    }

    createDeviceLink() {
        return Promise.reject(new Error('Device links are not available from the browser'));
    }

    listDeviceLinks() {
        return Promise.reject(new Error('Device links are not available from the browser'));
    }

    revokeDeviceLink() {
        return Promise.reject(new Error('Device links are not available from the browser'));
    }

    deleteDeviceLink() {
        return Promise.reject(new Error('Device links are not available from the browser'));
    }

    authorizeDevice() {
        return Promise.reject(new Error('Device links are not available from the browser'));
    }

    rejectDevice() {
        return Promise.reject(new Error('Device links are not available from the browser'));
    }

    async listOperators() {
        const response = await this._getClient().get(ServiceName.GATEWAY, ApiPaths.operator.list());
        const data = await response.json();
        const operators = (data.operators || []).map(normalizeOperator);
        const activeCount = operators.filter(op => ['active', 'bound'].includes(op.status)).length;
        return {
            operators,
            total_count: operators.length,
            active_count: activeCount,
            used_slots: operators.filter(op => op.is_slot).length,
            max_slots: operators.length,
        };
    }
}

export const operatorPanelService = new OperatorPanelService();

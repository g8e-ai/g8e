// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { ServiceName } from '../constants/service-client-constants.js';
import { ApiPaths } from '../constants/api-paths.js';

/**
 * OperatorPanelService - HTTP API layer for the OperatorPanel component.
 *
 * Centralises every window.serviceClient call made by OperatorPanel and its
 * mixin modules.  All methods return the raw Response so callers can inspect
 * ok / status and parse the body themselves — matching the existing call-site
 * contract without changing error-handling behaviour.
 *
 * Supports dependency injection of a serviceClient for testing.
 */
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

    // -------------------------------------------------------------------------
    // Operator lifecycle
    // -------------------------------------------------------------------------

    bindOperator(operatorId) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.operator.bind(), {
            operator_id: operatorId
        });
    }

    unbindOperator(body = {}) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.operator.unbind(), body);
    }

    bindAllOperators(operatorIds) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.operator.bindAll(), {
            operator_ids: operatorIds
        });
    }

    unbindAllOperators(operatorIds) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.operator.unbindAll(), {
            operator_ids: operatorIds
        });
    }

    stopOperator(operatorId) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.operator.stop(operatorId), {});
    }

    // -------------------------------------------------------------------------
    // Operator details & API keys
    // -------------------------------------------------------------------------

    getOperatorDetails(operatorId) {
        return this._getClient().get(ServiceName.g8ed, ApiPaths.operator.details(operatorId));
    }

    getOperatorApiKey(operatorId) {
        return this._getClient().get(ServiceName.g8ed, ApiPaths.operator.apiKey(operatorId));
    }

    refreshOperatorApiKey(operatorId) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.operator.refreshApiKey(operatorId), {});
    }

    // -------------------------------------------------------------------------
    // Device links
    // -------------------------------------------------------------------------

    generateDeviceLink(operatorId) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.auth.linkGenerate(), {
            operator_id: operatorId
        });
    }

    createDeviceLink({ maxUses, expiresInHours, name }) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.deviceLink.create(), {
            max_uses: maxUses,
            expires_in_hours: expiresInHours,
            name: name || undefined
        });
    }

    listDeviceLinks() {
        return this._getClient().get(ServiceName.g8ed, ApiPaths.deviceLink.list());
    }

    revokeDeviceLink(tokenId) {
        return this._getClient().delete(ServiceName.g8ed, ApiPaths.deviceLink.revoke(tokenId));
    }

    deleteDeviceLink(tokenId) {
        return this._getClient().delete(ServiceName.g8ed, ApiPaths.deviceLink.delete(tokenId));
    }

    // -------------------------------------------------------------------------
    // Device authorization
    // -------------------------------------------------------------------------

    authorizeDevice(token) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.auth.linkAuthorize(token), {});
    }

    rejectDevice(token) {
        return this._getClient().post(ServiceName.g8ed, ApiPaths.auth.linkReject(token), {});
    }
}

export const operatorPanelService = new OperatorPanelService();

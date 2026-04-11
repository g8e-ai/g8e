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
        return this._getClient().post(ComponentName.G8ED, ApiPaths.operator.bind(), {
            operator_id: operatorId
        });
    }

    unbindOperator(body = {}) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.operator.unbind(), body);
    }

    bindAllOperators(operatorIds) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.operator.bindAll(), {
            operator_ids: operatorIds
        });
    }

    unbindAllOperators(operatorIds) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.operator.unbindAll(), {
            operator_ids: operatorIds
        });
    }

    stopOperator(operatorId) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.operator.stop(operatorId), {});
    }

    g8eNodeReauth() {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.operator.g8eNodeReauth(), {});
    }

    // -------------------------------------------------------------------------
    // Operator details & API keys
    // -------------------------------------------------------------------------

    getOperatorDetails(operatorId) {
        return this._getClient().get(ComponentName.G8ED, ApiPaths.operator.details(operatorId));
    }

    getOperatorApiKey(operatorId) {
        return this._getClient().get(ComponentName.G8ED, ApiPaths.operator.apiKey(operatorId));
    }

    refreshOperatorApiKey(operatorId) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.operator.refreshApiKey(operatorId), {});
    }

    // -------------------------------------------------------------------------
    // Device links
    // -------------------------------------------------------------------------

    generateDeviceLink(operatorId) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.auth.linkGenerate(), {
            operator_id: operatorId
        });
    }

    createDeviceLink({ maxUses, expiresInHours, name }) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.deviceLink.create(), {
            max_uses: maxUses,
            expires_in_hours: expiresInHours,
            name: name || undefined
        });
    }

    listDeviceLinks() {
        return this._getClient().get(ComponentName.G8ED, ApiPaths.deviceLink.list());
    }

    revokeDeviceLink(tokenId) {
        return this._getClient().delete(ComponentName.G8ED, ApiPaths.deviceLink.revoke(tokenId));
    }

    deleteDeviceLink(tokenId) {
        return this._getClient().delete(ComponentName.G8ED, ApiPaths.deviceLink.delete(tokenId));
    }

    // -------------------------------------------------------------------------
    // Device authorization
    // -------------------------------------------------------------------------

    authorizeDevice(token) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.auth.linkAuthorize(token), {});
    }

    rejectDevice(token) {
        return this._getClient().post(ComponentName.G8ED, ApiPaths.auth.linkReject(token), {});
    }
}

export const operatorPanelService = new OperatorPanelService();

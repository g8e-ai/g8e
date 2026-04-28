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
 * g8e node Operator Service
 *
 * Manages the lifecycle of the g8e.operator process running inside the
 * g8ep container on behalf of a user. Called fire-and-forget after login
 * so that an operator on the g8ep is active and ready to be bound by the
 * time the user's browser finishes loading.
 *
 * Explicit flow (single responsibility):
 *
 *   1. getG8ENodeOperatorForUser(user_id)
 *      → Queries operator slots. Returns the first operator that is already
 *        ACTIVE on the g8ep, or the first AVAILABLE slot to use. Returns
 *        null when no usable slot exists.
 *
 * Note: g8ep operator process management is now owned by g8ee.
 * g8ed delegates to g8ee via InternalHttpClient for activation/relaunch.
 *
 * Top-level entry point:
 *
 *   activateG8ENodeOperatorForUser(user_id)
 *   → Orchestrates the two steps above. Designed to be called fire-and-forget
 *     from the login/register route. All errors are caught and logged; a failure
 *     here never propagates to the login response.
 */

import { logger } from '../../utils/logger.js';
import { OperatorStatus } from '../../constants/operator.js';
import { OperatorDocument } from '../../models/operator_model.js';
import {
    G8E_GATEWAY_CONTAINER_NAME,
    G8E_GATEWAY_OPERATOR_LAUNCH_TIMEOUT_MS,
} from '../../constants/service_config.js';

class G8ENodeOperatorService {
    /**
     * @param {Object} options
     * @param {Object} options.settingsService - SettingsService instance (for reading/writing platform_settings)
     * @param {Object} options.operatorService - OperatorService instance
     * @param {Object} options.internalHttpClient - InternalHttpClient instance
     */
    constructor({ settingsService, operatorService, internalHttpClient } = {}) {
        if (!settingsService) throw new Error('G8ENodeOperatorService requires settingsService');
        this._settingsService = settingsService;
        this._operatorService = operatorService;
        this._internalHttpClient = internalHttpClient;
    }

    /**
     * Returns the specific g8ep operator slot for this user.
     *
     * Queries for the operator with is_g8ep=true for the given user.
     *
     * @param {string} user_id
     * @returns {Promise<{ operator: Object, alreadyActive: boolean }|null>}
     */
    async getG8ENodeOperatorForUser(user_id) {
        if (!this._operatorService) throw new Error('operatorService is required for getG8ENodeOperatorForUser');
        
        // Find the specific slot designated as the g8ep for this user
        const operators = await this._operatorService.queryOperators([
            { field: 'user_id', operator: '==', value: user_id },
            { field: 'is_g8ep', operator: '==', value: true }
        ]);

        const operatorData = operators && operators.length > 0 ? operators[0] : null;

        if (!operatorData) {
            logger.info('[G8EP-OPERATOR] No g8ep operator slot found for user', { user_id });
            return null;
        }

        const operator = operatorData instanceof OperatorDocument
            ? operatorData
            : OperatorDocument.fromDB(operatorData);

        const alreadyActive = operator.status === OperatorStatus.ACTIVE ||
                              operator.status === OperatorStatus.BOUND;

        logger.info('[G8EP-OPERATOR] g8ep operator slot resolved', {
            user_id,
            operator_id: operator.id,
            status: operator.status,
            alreadyActive,
        });

        return { operator, alreadyActive };
    }


    /**
     * Maps operator exit codes to human-readable error messages.
     * Exit codes are defined in components/g8eo/constants/exit_codes.go
     * 
     * @param {number} exitCode 
     * @returns {string}
     */
    _mapExitCodeToMessage(exitCode) {
        switch (exitCode) {
            case 2:  return 'Authentication failed: Invalid or expired API key.';
            case 3:  return 'Permission denied: Operator cannot write to its data directory.';
            case 4:  return 'Network error: Operator cannot reach the platform servers.';
            case 5:  return 'Configuration error: Missing or invalid operator settings.';
            case 6:  return 'Storage error: Operator failed to initialize its local database.';
            case 7:  return 'TLS trust failure: The operator does not trust the platform\'s SSL certificates.';
            default: return `Operator exited with error code ${exitCode}.`;
        }
    }

    /**
     * Kills any running operator process in the g8ep container for this
     * user, resets their operator slot to AVAILABLE, then relaunches using the
     * fresh API key produced by the reset.
     *
     * Delegates to g8ee via InternalHttpClient.
     *
     * @param {string} user_id
     * @returns {Promise<{ success: boolean, operator_id?: string, error?: string }>}
     */
    async relaunchG8ENodeOperatorForUser(user_id) {
        logger.info('[G8EP-OPERATOR] Relaunching g8ep operator via g8ee', { user_id });
        return this._internalHttpClient.relaunchG8EPOperator(user_id);
    }

    /**
     * Orchestrates the full g8ep operator activation for a user who has
     * just logged in.
     *
     * Delegates to g8ee via InternalHttpClient.
     *
     * @param {string} user_id
     * @param {string|null} organization_id
     * @param {string} web_session_id
     */
    async activateG8ENodeOperatorForUser(user_id, organization_id, web_session_id) {
        logger.info('[G8EP-OPERATOR] Activating g8ep operator via g8ee', { user_id });
        try {
            await this._internalHttpClient.activateG8EPOperator(user_id);
        } catch (err) {
            logger.warn('[G8EP-OPERATOR] g8ep operator activation failed (non-fatal)', {
                user_id,
                error: err.message
            });
        }
    }
}

export { G8ENodeOperatorService };

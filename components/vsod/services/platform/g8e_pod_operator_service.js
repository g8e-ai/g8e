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
 * g8e-pod container on behalf of a user. Called fire-and-forget after login
 * so that an operator on the g8e-pod is active and ready to be bound by the
 * time the user's browser finishes loading.
 *
 * Explicit flow (three separate responsibilities):
 *
 *   1. getG8ENodeOperatorForUser(user_id)
 *      → Queries operator slots. Returns the first operator that is already
 *        ACTIVE on the g8e-pod, or the first AVAILABLE slot to use. Returns
 *        null when no usable slot exists.
 *
 *   2. launchG8ENodeOperator(apiKey)
 *      → Persists the operator API key to the platform_settings document in
 *        VSODB, then starts the supervised operator process via XML-RPC.
 *        The operator command fetches the key from VSODB at startup.
 *        Returns immediately — the binary authenticates and SSE delivers
 *        OPERATOR_STATUS_UPDATED to the browser asynchronously.
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
import {
    G8E_GATEWAY_CONTAINER_NAME,
    G8E_GATEWAY_OPERATOR_LAUNCH_TIMEOUT_MS,
} from '../../constants/service_config.js';

class G8ENodeOperatorService {
    /**
     * @param {Object} options
     * @param {Object} options.settingsService - SettingsService instance (for reading/writing platform_settings)
     * @param {Object} options.operatorService - OperatorService instance
     */
    constructor({ settingsService, operatorService } = {}) {
        if (!settingsService) throw new Error('G8ENodeOperatorService requires settingsService');
        this._settingsService = settingsService;
        this._operatorService = operatorService;
    }

    async _resolveSettings() {
        const settings = await this._settingsService.getPlatformSettings();
        const port = settings.supervisor_port || '443';
        const token = settings.internal_auth_token || '';
        return {
            supervisorUrl: `http://g8e-pod:${port}/RPC2`,
            authHeader: `Basic ${Buffer.from(`vso-internal:${token}`).toString('base64')}`,
        };
    }

    /**
     * Returns the specific g8e-pod operator slot for this user.
     *
     * Queries for the operator with is_g8e_pod=true for the given user.
     *
     * @param {string} user_id
     * @returns {Promise<{ operator: Object, alreadyActive: boolean }|null>}
     */
    async getG8ENodeOperatorForUser(user_id) {
        if (!this._operatorService) throw new Error('operatorService is required for getG8ENodeOperatorForUser');
        
        // Find the specific slot designated as the g8e-pod for this user
        const operators = await this._operatorService.queryOperators([
            { field: 'user_id', operator: '==', value: user_id },
            { field: 'is_g8e_pod', operator: '==', value: true }
        ]);

        const operatorData = operators && operators.length > 0 ? operators[0] : null;

        if (!operatorData) {
            logger.info('[DROP-POD-OPERATOR] No g8e-pod operator slot found for user', { user_id });
            return null;
        }

        // We use the raw data if it's not a class instance, or just return as is
        const operator = operatorData;

        const alreadyActive = operator.status === OperatorStatus.ACTIVE ||
                              operator.status === OperatorStatus.BOUND;

        logger.info('[DROP-POD-OPERATOR] g8e-pod operator slot resolved', {
            user_id,
            operator_id: operator.operator_id,
            status: operator.status,
            alreadyActive,
        });

        return { operator, alreadyActive };
    }

    /**
     * Launches the operator inside the g8e-pod container.
     *
     * Persists the operator API key to the platform_settings document in VSODB,
     * then starts the supervised operator process via XML-RPC. The operator
     * command in g8e-pod fetches the key from VSODB at startup.
     *
     * @param {string} apiKey  - operator API key from the operator document
     * @returns {Promise<void>}
     */
    async launchG8ENodeOperator(apiKey) {
        logger.info('[DROP-POD-OPERATOR] Starting operator in g8e-pod via XML-RPC', {
            container: G8E_GATEWAY_CONTAINER_NAME,
        });

        try {
            await this._settingsService.savePlatformSettings({g8e_pod_operator_api_key: apiKey });
        } catch (err) {
            throw new Error(`Failed to persist operator API key to platform_settings: ${err.message}`);
        }

        logger.info('[DROP-POD-OPERATOR] Operator API key persisted to platform_settings');

        const resolved = await this._resolveSettings();

        try {
            await this._xmlrpcCall('supervisor.startProcess', ['operator', false], resolved);
        } catch (err) {
            if (err.message.includes('ALREADY_STARTED')) {
                logger.info('[DROP-POD-OPERATOR] Operator already running, restarting', { error: err.message });
                await this._xmlrpcCall('supervisor.stopProcess', ['operator', true], resolved).catch(() => {});
                await this._xmlrpcCall('supervisor.startProcess', ['operator', false], resolved);
            } else {
                logger.error('[DROP-POD-OPERATOR] Failed to start operator via XML-RPC', { error: err.message });
                throw err;
            }
        }

        logger.info('[DROP-POD-OPERATOR] Operator service signaled in g8e-pod', {
            container: G8E_GATEWAY_CONTAINER_NAME,
        });
    }

    /**
     * Performs an XML-RPC call to Supervisor.
     * 
     * @param {string} method 
     * @param {any[]} params 
     * @returns {Promise<any>}
     */
    async _xmlrpcCall(method, params = [], resolvedSettings = null) {
        const body = `<?xml version="1.0"?>
<methodCall>
  <methodName>${method}</methodName>
  <params>
    ${params.map(p => `<param><value>${typeof p === 'boolean' ? `<boolean>${p ? '1' : '0'}</boolean>` : `<string>${p}</string>`}</value></param>`).join('\n')}
  </params>
</methodCall>`;

        const { supervisorUrl, authHeader } = resolvedSettings || await this._resolveSettings();

        const response = await fetch(supervisorUrl, {
            method: 'POST',
            headers: {
                'Content-Type': 'text/xml',
                'Authorization': authHeader,
            },
            body,
            signal: AbortSignal.timeout(G8E_GATEWAY_OPERATOR_LAUNCH_TIMEOUT_MS)
        });

        if (!response.ok) {
            const text = await response.text();
            throw new Error(`Supervisor connection failed: ${response.status} ${text}`);
        }

        const xml = await response.text();
        if (xml.includes('<fault>')) {
            const codeMatch = xml.match(/<int>([^<]+)<\/int>/);
            const stringMatch = xml.match(/<string>([^<]+)<\/string>/);
            const faultCode = codeMatch ? parseInt(codeMatch[1], 10) : null;
            const faultString = stringMatch ? stringMatch[1] : 'Unknown Supervisor fault';

            // Map common Supervisor fault codes to user-friendly messages
            // https://github.com/Supervisor/supervisor/blob/master/supervisor/xmlrpc.py
            switch (faultCode) {
                case 10: // BAD_NAME
                    throw new Error('Operator process not found in g8e-pod configuration.');
                case 60: // ALREADY_STARTED
                    throw new Error('ALREADY_STARTED');
                case 70: // NOT_RUNNING
                    return xml; // Ignore if trying to stop
                case 90: // SPAWN_ERROR
                    throw new Error('Failed to spawn operator process. Check g8e-pod logs.');
                default:
                    throw new Error(`Supervisor error (${faultCode}): ${faultString}`);
            }
        }

        return xml;
    }

    /**
     * Maps operator exit codes to human-readable error messages.
     * Exit codes are defined in components/vsa/constants/exit_codes.go
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
     * Kills any running operator process in the g8e-pod container for this
     * user, resets their operator slot to AVAILABLE, then relaunches using the
     * fresh API key produced by the reset.
     *
     * Standalone — no coupling to auth or login. Safe to call at any time.
     *
     * @param {string} user_id
     * @returns {Promise<{ success: boolean, operator_id?: string, error?: string }>}
     */
    async relaunchG8ENodeOperatorForUser(user_id) {
        if (!this._operatorService) throw new Error('operatorService is required for relaunchG8ENodeOperatorForUser');

        const slotResult = await this.getG8ENodeOperatorForUser(user_id);
        if (!slotResult) {
            logger.warn('[DROP-POD-OPERATOR] Relaunch requested but no g8e-pod operator slot found for user', { user_id });
            return { success: false, error: 'No g8e-pod operator slot found for user' };
        }

        const { operator } = slotResult;
        const operator_id = operator.operator_id;

        logger.info('[DROP-POD-OPERATOR] Stopping supervised operator service in g8e-pod', {
            user_id,
            operator_id,
        });

        const resolved = await this._resolveSettings();

        await this._xmlrpcCall('supervisor.stopProcess', ['operator', true], resolved).catch(() => {});

        const resetResult = await this._operatorService.resetOperator(operator_id);
        if (!resetResult.success) {
            logger.warn('[DROP-POD-OPERATOR] Operator slot reset failed during relaunch', {
                user_id,
                operator_id,
                error: resetResult.error,
            });
            return { success: false, error: resetResult.error };
        }

        const apiKey = resetResult.operator?.api_key;
        if (!apiKey) {
            logger.warn('[DROP-POD-OPERATOR] Reset did not return an API key — cannot relaunch', {
                user_id,
                operator_id,
            });
            return { success: false, error: 'No API key available after operator reset' };
        }

        await this.launchG8ENodeOperator(apiKey);

        logger.info('[DROP-POD-OPERATOR] g8e-pod operator relaunched successfully', {
            user_id,
            operator_id,
        });

        return { success: true, operator_id };
    }

    /**
     * Orchestrates the full g8e-pod operator activation for a user who has
     * just logged in.
     *
     * Steps:
     *   1. Look up an active or available operator slot for the user.
     *   2. If already active — nothing to do.
     *   3. If available — read the operator API key and launch the binary.
     *
     * Designed to be called fire-and-forget from auth routes. Errors are caught
     * and logged; they never propagate to the login response.
     *
     * @param {string} user_id
     * @param {string|null} organization_id
     * @param {string} web_session_id
     */
    async activateG8ENodeOperatorForUser(user_id, organization_id, web_session_id) {
        try {
            const slotResult = await this.getG8ENodeOperatorForUser(user_id);

            if (!slotResult) {
                logger.info('[DROP-POD-OPERATOR] No operator slot available for user — skipping g8e-pod launch', { user_id });
                return;
            }

            if (slotResult.alreadyActive) {
                logger.info('[DROP-POD-OPERATOR] g8e-pod operator already active for user — skipping launch', {
                    user_id,
                    operator_id: slotResult.operator.operator_id
                });
                return;
            }

            const { operator } = slotResult;
            const apiKey = operator.api_key;

            if (!apiKey) {
                logger.warn('[DROP-POD-OPERATOR] Operator slot has no API key — g8e-pod operator will not be launched', {
                    user_id,
                    operator_id: operator.operator_id,
                });
                return;
            }

            await this.launchG8ENodeOperator(apiKey);

        } catch (err) {
            logger.warn('[DROP-POD-OPERATOR] g8e-pod operator activation failed (non-fatal)', {
                user_id,
                error: err.message
            });
        }
    }
}

export { G8ENodeOperatorService };

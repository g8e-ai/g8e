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

import { OperatorAuthResponse } from '../../models/response_models.js';
import { logger } from '../../utils/logger.js';
import { redactWebSessionId } from '../../utils/security.js';
import {
    ApiKeyError,
    DeviceLinkError,
    OperatorAuthError,
    AuthError,
    BEARER_PREFIX,
    AuthMode,
} from '../../constants/auth.js';
import { SessionType } from '../../constants/session.js';
import {
    OperatorStatus,
} from '../../constants/operator.js';
import { DEFAULT_OPERATOR_CONFIG } from '../../constants/operator_defaults.js';
import { G8eHttpContext, BoundOperatorContext } from '../../models/request_models.js';
import { SystemInfo } from '../../models/operator_model.js';

export class OperatorAuthService {
    /**
     * @param {Object} options
     * @param {Object} options.apiKeyService - ApiKeyService instance
     * @param {Object} options.userService - UserService instance
     * @param {Object} options.operatorService - OperatorDataService instance
     * @param {Object} options.operatorSessionService - OperatorSessionService instance
     * @param {Object} options.cliSessionService - CliSessionService instance
     * @param {Object} options.bindingService - BoundSessionsService instance
     * @param {Object} options.webSessionService - WebSessionService instance
     */
    constructor({
        apiKeyService,
        userService,
        operatorService,
        operatorSessionService,
        cliSessionService,
        bindingService,
        webSessionService,
    }) {
        this.apiKeyService = apiKeyService;
        this.userService = userService;
        this.operatorService = operatorService;
        this.operatorSessionService = operatorSessionService;
        this.cliSessionService = cliSessionService;
        this.bindingService = bindingService;
        this.webSessionService = webSessionService;
    }

    async authenticateOperator({ authorizationHeader, body }) {
        const { system_info, runtime_config, auth_mode, operator_session_id: deviceLinkSessionId } = body || {};

        if (auth_mode === AuthMode.OPERATOR_SESSION && deviceLinkSessionId) {
            return this._authenticateViaDeviceLink(deviceLinkSessionId, system_info, authorizationHeader);
        }

        return this._authenticateViaApiKey({ authorizationHeader, system_info, runtime_config });
    }

    async _authenticateViaDeviceLink(deviceLinkSessionId, system_info, authorizationHeader) {
        const session = await this.operatorSessionService.validateSession(deviceLinkSessionId);
        if (!session) {
            logger.warn('[OPERATOR-AUTH] WebSession auth failed - invalid or expired session', {
                session_id_prefix: deviceLinkSessionId.substring(0, 12) + '...',
            });
            return {
                success: false,
                statusCode: 401,
                error: AuthError.INVALID_OR_EXPIRED_SESSION,
                message: 'Device link session is invalid or has expired. Re-generate the device link token.',
            };
        }

        const { user_id, operator_id } = session;
        const organization_id = session.user_data?.organization_id;

        if (!user_id) {
            logger.error('[OPERATOR-AUTH] WebSession missing user_id', {
                session_id_prefix: deviceLinkSessionId.substring(0, 12) + '...',
            });
            return {
                success: false,
                statusCode: 401,
                error: 'Invalid session',
                message: 'WebSession is missing required user_id.',
            };
        }

        // operator_id is OPTIONAL during bootstrap via device link.
        // If not provided, the operator will be assigned an available slot.
        const operator_id_to_use = operator_id || null;

        const user = await this.userService.getUser(user_id);
        if (!user) {
            logger.error('[OPERATOR-AUTH] User not found for session', { user_id });
            return {
                success: false,
                statusCode: 404,
                error: ApiKeyError.USER_NOT_FOUND,
                message: 'User record does not exist.',
            };
        }

        logger.info('[OPERATOR-AUTH] WebSession auth resolved identity', { 
            user_id, 
            operator_id: operator_id_to_use, 
            organization_id 
        });

        let operatorDoc = null;
        if (operator_id_to_use) {
            operatorDoc = await this.operatorService.getOperator(operator_id_to_use);
        }

        let deviceLinkApiKey = null;
        if (authorizationHeader && authorizationHeader.startsWith(BEARER_PREFIX)) {
            deviceLinkApiKey = authorizationHeader.substring(BEARER_PREFIX.length);
        }
        if (!deviceLinkApiKey) {
            deviceLinkApiKey = operatorDoc?.api_key || null;
        }

        const config = DEFAULT_OPERATOR_CONFIG;

        logger.info('[OPERATOR-AUTH] WebSession auth complete - returning bootstrap config', {
            operatorSessionId: redactWebSessionId(deviceLinkSessionId),
            operator_id: operator_id_to_use,
            user_id,
            has_api_key: !!deviceLinkApiKey,
        });

        return {
            success: true,
            response: new OperatorAuthResponse({
                success: true,
                operator_session_id: deviceLinkSessionId,
                operator_id: operator_id_to_use,
                user_id,
                api_key: deviceLinkApiKey,
                config,
                session: { id: deviceLinkSessionId, expires_at: null, created_at: null },
                operator_cert: null,
                operator_cert_key: null,
            }),
        };
    }

    async _authenticateViaApiKey({ authorizationHeader, system_info, runtime_config }) {
        let api_key = null;
        if (authorizationHeader && authorizationHeader.startsWith(BEARER_PREFIX)) {
            api_key = authorizationHeader.substring(BEARER_PREFIX.length);
        }

        if (!api_key) {
            logger.warn('[OPERATOR-AUTH] Missing Authorization: Bearer token in request');
            return {
                success: false,
                statusCode: 401,
                error: OperatorAuthError.MISSING_API_KEY,
                message: 'Provide API key via Authorization: Bearer <key> header',
            };
        }

        const keyValidation = await this.apiKeyService.validateKey(api_key);
        if (!keyValidation.success || !keyValidation.data) {
            logger.error('[OPERATOR-AUTH] API key validation failed', {
                api_key_prefix: api_key.substring(0, 10) + '...',
                error: keyValidation.error,
            });
            return {
                success: false,
                statusCode: 401,
                error: ApiKeyError.INVALID,
                message: keyValidation.error || 'API key validation failed',
            };
        }

        const keyData = keyValidation.data;
        const { user_id, organization_id, operator_id } = keyData;

        // Download-only keys (no operator_id) are allowed for CLI authentication
        // but cannot claim operator slots or run operators
        const isDownloadOnlyKey = !operator_id;
        if (isDownloadOnlyKey) {
            logger.info('[OPERATOR-AUTH] Download-only API key used for CLI authentication', {
                api_key_prefix: api_key.substring(0, 10) + '...',
                user_id,
                client_name: keyData.client_name,
            });
        } else {
            logger.info('[OPERATOR-AUTH] API key validated successfully', {
                user_id,
                organization_id,
                operator_id,
                client_name: keyData.client_name,
            });
        }

        try {
            await this.apiKeyService.recordUsage(api_key);
        } catch (err) {
            logger.warn('[OPERATOR-AUTH] Failed to record usage', { error: err.message });
        }

        const user = await this.userService.getUser(user_id);
        if (!user) {
            logger.error('[OPERATOR-AUTH] User not found', {
                user_id,
                api_key_prefix: api_key.substring(0, 10) + '...',
            });
            return {
                success: false,
                statusCode: 404,
                error: ApiKeyError.USER_NOT_FOUND,
                message: 'User record does not exist. Please ensure user is registered.',
            };
        }

        // For download-only keys, skip operator validation and create CLI-only session
        if (isDownloadOnlyKey) {
            return this._completeCliAuthentication({
                api_key,
                user,
                user_id,
                organization_id,
                system_info,
                runtime_config,
            });
        }

        const operator = await this.operatorService.getOperator(operator_id);
        if (!operator) {
            logger.error('[OPERATOR-AUTH] Operator not found', {
                operator_id,
                user_id,
                api_key_prefix: api_key.substring(0, 10) + '...',
            });
            return {
                success: false,
                statusCode: 404,
                error: DeviceLinkError.OPERATOR_NOT_FOUND,
                message: 'The Operator associated with this API key does not exist.',
            };
        }

        if (operator.user_id !== user_id) {
            logger.error('[OPERATOR-AUTH] Operator does not belong to user', {
                operator_id,
                operator_user_id: operator.user_id,
                authenticated_user_id: user_id,
                api_key_prefix: api_key.substring(0, 10) + '...',
            });
            return {
                success: false,
                statusCode: 403,
                error: DeviceLinkError.UNAUTHORIZED,
                message: 'This Operator does not belong to your account.',
            };
        }

        return this._completeAuthentication({
            api_key,
            user,
            user_id,
            organization_id,
            operator_id,
            operator,
            system_info,
            runtime_config,
        });
    }

    async _completeCliAuthentication({
        api_key,
        user,
        user_id,
        organization_id,
        system_info,
        runtime_config,
    }) {
        const sessionData = {
            user_id: user.id,
            user_data: {
                email: user.email,
                name: user.name,
                picture: user.profile_picture,
                id: user.id,
                organization_id: user.organization_id,
                roles: user.roles,
            },
            api_key,
            organization_id,
        };

        const session = await this.cliSessionService.createSession(sessionData);
        const operator_session_id = session.id;

        logger.info('[OPERATOR-AUTH] CLI-only session created (no operator slot claimed)', {
            operatorSessionId: redactWebSessionId(operator_session_id),
            user_id,
            organization_id,
        });

        const config = DEFAULT_OPERATOR_CONFIG;

        logger.info('[OPERATOR-AUTH] CLI authentication successful', {
            operatorSessionId: redactWebSessionId(operator_session_id),
            user_id,
            organization_id,
        });

        return {
            success: true,
            response: new OperatorAuthResponse({
                success: true,
                operator_session_id,
                operator_id: null, // No operator for CLI-only auth
                user_id,
                api_key,
                config,
                session: {
                    id: operator_session_id,
                    expires_at: session.expires_at,
                    created_at: session.created_at,
                },
                operator_cert: null,
                operator_cert_key: null,
            }),
        };
    }

    async _completeAuthentication({
        api_key,
        user,
        user_id,
        organization_id,
        operator_id,
        operator,
        system_info,
        runtime_config,
    }) {
        const sessionData = {
            user_id: user.id,
            user_data: {
                email: user.email,
                name: user.name,
                picture: user.profile_picture,
                id: user.id,
                organization_id: user.organization_id,
                roles: user.roles,
            },
            api_key,
            operator_id,
            session_type: SessionType.OPERATOR,
        };

        const session = await this.operatorSessionService.createOperatorSession(sessionData);
        const operator_session_id = session.id;

        logger.info('[OPERATOR-AUTH] Operator session created', {
            operatorSessionId: redactWebSessionId(operator_session_id),
            operator_id,
            user_id,
            organization_id,
        });

        const bound_web_session_id = operator.bound_web_session_id || null;
        const parsedSystemInfo = SystemInfo.parse(system_info || {});

        const claimStatus = operator.status === OperatorStatus.BOUND
            ? OperatorStatus.BOUND
            : OperatorStatus.ACTIVE;

        await this.operatorService.claimOperatorSlot(operator_id, {
            operator_session_id,
            web_session_id: bound_web_session_id,
            system_info: parsedSystemInfo,
            operator_type: operator.operator_type || null,
            status: claimStatus,
        });

        await this.userService.updateUserOperator(user_id, operator_id, claimStatus);

        try {
            const g8eContext = G8eHttpContext.parse({
                web_session_id:  bound_web_session_id || operator_session_id,
                user_id,
                organization_id,
                bound_operators: [
                    BoundOperatorContext.parse({
                        operator_id,
                        operator_session_id,
                        status:        claimStatus,
                        operator_type: operator.operator_type || null,
                        system_info:   parsedSystemInfo,
                    })
                ],
            });

            await this.operatorService.relayRegisterOperatorSessionToG8ee(g8eContext);
            
            logger.info('[OPERATOR-AUTH] Operator session registration relayed to g8ee', {
                operator_id,
                user_id,
            });
        } catch (err) {
            logger.error('[OPERATOR-AUTH] Failed to relay Operator session registration to g8ee', { 
                operator_id, 
                error: err.message 
            });
        }

        const config = DEFAULT_OPERATOR_CONFIG;

        logger.info('[OPERATOR-AUTH] Operator authenticated and bootstrapped successfully', {
            operatorSessionId: redactWebSessionId(operator_session_id),
            operatorSessionIdFull: operator_session_id,
            user_id,
            operator_id,
            organization_id,
        });

        return {
            success: true,
            response: new OperatorAuthResponse({
                success: true,
                operator_session_id,
                operator_id,
                user_id,
                api_key,
                config,
                session: {
                    id: operator_session_id,
                    expires_at: session.expires_at,
                    created_at: session.created_at,
                },
                operator_cert: null,
                operator_cert_key: null,
            }),
        };
    }
}

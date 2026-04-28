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

import { logger } from '../../utils/logger.js';
import { SESSION_COOKIE_NAME, COOKIE_SAME_SITE, SESSION_TTL_SECONDS } from '../../constants/session.js';
import { getCookieDomain } from '../../utils/security.js';
import { EventType } from '../../constants/events.js';
import { OperatorSlotInitializationFailedEvent, OperatorG8EPActivationFailedEvent } from '../../models/sse_models.js';

export class PostLoginService {
    /**
     * @param {Object} options
     * @param {Object} options.webSessionService - WebSessionService instance
     * @param {Object} options.apiKeyService - ApiKeyService instance
     * @param {Object} options.userService - UserService instance
     * @param {Object} options.operatorService - OperatorDataService instance
     * @param {Object} options.g8eNodeOperatorService - G8ENodeOperatorService instance
     * @param {Object} options.sseService - SSEService instance
     * @param {Object} options.consoleMetricsService - ConsoleMetricsService instance
     */
    constructor({ webSessionService, apiKeyService, userService, operatorService, g8eNodeOperatorService, sseService, consoleMetricsService }) {
        this.webSessionService = webSessionService;
        this.apiKeyService = apiKeyService;
        this.userService = userService;
        this.operatorService = operatorService;
        this.g8eNodeOperatorService = g8eNodeOperatorService;
        this.sseService = sseService;
        this.consoleMetricsService = consoleMetricsService;
    }

    async createSessionAndSetCookie(req, res, user) {
        try {
            const existing = await this.userService.getUserG8eKey(user.id);
            let downloadApiKey = existing;
            if (!downloadApiKey) {
                const keyResult = await this.userService.createUserG8eKey(
                    user.id,
                    user.organization_id || user.id
                );
                downloadApiKey = keyResult.success ? keyResult.api_key : null;
            }

            const session = await this.webSessionService.createWebSession({
                user_id: user.id,
                user_data: {
                    email: user.email,
                    name: user.name,
                    roles: user.roles
                },
                api_key: downloadApiKey
            }, {
                ip: req.ip || req.headers['x-forwarded-for'] || '127.0.0.1',
                userAgent: req.headers['user-agent'] || 'unknown'
            });

            const cookieDomain = getCookieDomain(req);
            res.cookie(SESSION_COOKIE_NAME, session.id, {
                httpOnly: true,
                secure: true,
                sameSite: COOKIE_SAME_SITE,
                maxAge: SESSION_TTL_SECONDS * 1000,
                domain: cookieDomain,
                path: '/'
            });

            return session;
        } catch (error) {
            logger.error('[POST-LOGIN] createSessionAndSetCookie failed', { 
                userId: user.id, 
                error: error.message,
                stack: error.stack 
            });
            throw error;
        }
    }

    async onSuccessfulRegistration(user, session) {
        this._initializeSlotsAndActivateG8eNode(user, session, 'registration').catch(err => {
            logger.error('[POST-LOGIN] post-registration operator setup failed', { userId: user.id, error: err.message, stack: err.stack });
            this._handleSlotInitFailure(user, session, err, 'registration');
        });
    }

    async onSuccessfulLogin(user, session) {
        this._initializeSlotsAndActivateG8eNode(user, session, 'login').catch(err => {
            logger.error('[POST-LOGIN] post-login operator setup failed', { userId: user.id, error: err.message, stack: err.stack });
            this._handleSlotInitFailure(user, session, err, 'login');
        });
    }

    async _initializeSlotsAndActivateG8eNode(user, session, context) {
        await this.operatorService.initializeOperatorSlots(
            user.id,
            user.organization_id || user.id,
            session.id
        );

        await this.g8eNodeOperatorService.activateG8ENodeOperatorForUser(
            user.id,
            user.organization_id || null,
            session.id
        );
    }

    async _handleSlotInitFailure(user, session, error, context) {
        try {
            const errorContext = error.message;
            
            if (errorContext.includes('slot') || errorContext.includes('initializeOperatorSlots')) {
                const event = OperatorSlotInitializationFailedEvent.parse({
                    type: EventType.OPERATOR_SLOT_INITIALIZATION_FAILED,
                    data: {
                        user_id: user.id,
                        error: error.message,
                        context: context
                    }
                });
                await this.sseService.publishEvent(session.id, event);
            } else {
                const event = OperatorG8EPActivationFailedEvent.parse({
                    type: EventType.OPERATOR_G8EP_ACTIVATION_FAILED,
                    data: {
                        user_id: user.id,
                        error: error.message,
                        context: context
                    }
                });
                await this.sseService.publishEvent(session.id, event);
            }

            if (this.consoleMetricsService) {
                this.consoleMetricsService.metricsCache.set('slot_init_failures', {
                    count: (this.consoleMetricsService.metricsCache.get('slot_init_failures')?.count || 0) + 1,
                    lastError: error.message,
                    lastUserId: user.id,
                    lastContext: context,
                    timestamp: Date.now()
                });
            }
        } catch (publishError) {
            logger.error('[POST-LOGIN] Failed to publish slot init failure event', {
                userId: user.id,
                originalError: error.message,
                publishError: publishError.message
            });
        }
    }
}

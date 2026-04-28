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
import { redactWebSessionId } from '../../utils/security.js';
import { OperatorStatus } from '../../constants/operator.js';
import { 
    OperatorDocument, 
    OperatorWithSessionContext, 
    GrantedIntent, 
    OperatorSlot,
    OperatorListUpdatedEvent,
} from '../../models/operator_model.js';
import { now } from '../../models/base.js';
import { EventType } from '../../constants/events.js';

import { OperatorSlotService } from './operator_slot_service.js';
import { OperatorRelayService } from './operator_relay_service.js';
import { OperatorNotificationService } from './operator_notification_service.js';

/**
 * OperatorService (Domain Layer)
 * Orchestrates operator-related operations, delegating CRUD to OperatorDataService.
 * Handles relays to g8ee and broadcasts to the UI.
 */
class OperatorService {
    /**
     * @param {Object} options
     * @param {Object} options.operatorDataService - OperatorDataService instance (Data Layer)
     * @param {Object} [options.userService] - UserService instance
     * @param {Object} [options.apiKeyService] - ApiKeyService instance
     * @param {Object} [options.certificateService] - CertificateService instance
     * @param {Object} [options.webSessionService] - WebSessionService instance
     * @param {Object} [options.sseService] - SSEService instance
     * @param {Object} [options.internalHttpClient] - InternalHttpClient instance
     */
    constructor({
        operatorDataService,
        userService,
        apiKeyService,
        certificateService,
        webSessionService,
        sseService,
        internalHttpClient
    }) {
        if (!operatorDataService) throw new Error('operatorDataService is required');
        if (!internalHttpClient) throw new Error('internalHttpClient is required');
        
        this.operatorDataService = operatorDataService;
        this.userService = userService;
        this.apiKeyService = apiKeyService;
        this.certificateService = certificateService;
        this.webSessionService = webSessionService;
        this.sseService = sseService;
        this.internalHttpClient = internalHttpClient;
        
        this.collectionName = this.operatorDataService.collectionName;

        // Subservices
        this.slots = new OperatorSlotService({
            operatorDataService: this.operatorDataService,
            apiKeyService,
            certificateService,
            operatorService: this,
        });

        this.relay = new OperatorRelayService({
            internalHttpClient: this.internalHttpClient
        });

        this.notifications = new OperatorNotificationService({
            webSessionService,
            operatorDataService: this.operatorDataService,
            sseService
        });
        
        logger.info('[OPERATOR-SERVICE] Initialized Domain Layer with Data subservice', {
            collection: this.collectionName
        });
    }

    // =========================================================================
    // Coordination & Passthrough Operations
    // =========================================================================

    /**
     * Calculate slot USAGE for a user
     */
    calculateSlotUsage(operators, excludeOperatorId = null) {
        const claimedOperators = operators.filter(op =>
            op.id !== excludeOperatorId &&
            (op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND)
        );
        const usedSlots = claimedOperators.length;
        return { usedSlots, claimedOperators };
    }

    async getOperator(operatorId) {
        return this.operatorDataService.getOperator(operatorId);
    }

    async getOperatorFresh(operatorId) {
        return this.operatorDataService.getOperatorFresh(operatorId);
    }

    async getOperatorByUserId(userId) {
        const data = await this.queryOperators([{ field: 'user_id', operator: '==', value: userId }]);
        if (!data || data.length === 0) return null;
        const deployed = data.find(op => op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND);
        if (deployed) return deployed;
        return data.find(op => op.status === OperatorStatus.AVAILABLE) || null;
    }

    async getOperatorWithSessionContext(operatorId) {
        const operator = await this.getOperator(operatorId);
        if (!operator) return null;
        
        const g8eContext = {
            user_id: operator.user_id,
            organization_id: operator.organization_id,
            source_component: 'g8ed',
        };

        const operatorSession = operator.operator_session_id
            ? await this.relay.relayValidateOperatorSessionToG8ee(operator.operator_session_id, g8eContext).then(res => res.session).catch(() => null)
            : null;
        const webSession = operator.bound_web_session_id && this.webSessionService
            ? await this.webSessionService.validateSession(operator.bound_web_session_id)
            : null;
        return OperatorWithSessionContext.create(operator, operatorSession, webSession);
    }

    // --- Relays (Authority is g8ee) ---

    async relayStopCommandToG8ee(g8eContext) {
        return this.relay.relayStopCommandToG8ee(g8eContext);
    }

    async deregisterOperatorSessionInG8ee(g8eContext) {
        return this.relay.deregisterOperatorSessionInG8ee(g8eContext);
    }

    async relayDirectCommandToG8ee(commandData, g8eContext) {
        return this.relay.relayDirectCommandToG8ee(commandData, g8eContext);
    }

    async relayRegisterOperatorSessionToG8ee(g8eContext) {
        return this.relay.relayRegisterOperatorSessionToG8ee(g8eContext);
    }

    async relayApprovalResponseToG8ee(approvalData, g8eContext) {
        return this.relay.relayApprovalResponseToG8ee(approvalData, g8eContext);
    }

    async relayCreateOperatorSlotToG8ee(params, g8eContext) {
        return this.relay.relayCreateOperatorSlotToG8ee(params, g8eContext);
    }

    async relayUpdateOperatorApiKeyToG8ee(operatorId, apiKey, g8eContext) {
        return this.relay.relayUpdateOperatorApiKeyToG8ee(operatorId, apiKey, g8eContext);
    }

    async relayClaimOperatorSlotToG8ee(params, g8eContext) {
        return this.relay.relayClaimOperatorSlotToG8ee(params, g8eContext);
    }

    async relayBindOperatorsToG8ee(params, g8eContext) {
        return this.relay.relayBindOperatorsToG8ee(params, g8eContext);
    }

    async relayUnbindOperatorsToG8ee(params, g8eContext) {
        return this.relay.relayUnbindOperatorsToG8ee(params, g8eContext);
    }

    async relayAuthenticateOperatorToG8ee(params, g8eContext) {
        return this.relay.relayAuthenticateOperatorToG8ee(params, g8eContext);
    }

    async relayRegisterDeviceLinkToG8ee(params, g8eContext) {
        return this.relay.relayRegisterDeviceLinkToG8ee(params, g8eContext);
    }

    async relayValidateOperatorSessionToG8ee(operatorSessionId, g8eContext) {
        return this.relay.relayValidateOperatorSessionToG8ee(operatorSessionId, g8eContext);
    }

    async relayRefreshOperatorSessionToG8ee(operatorSessionId, g8eContext) {
        return this.relay.relayRefreshOperatorSessionToG8ee(operatorSessionId, g8eContext);
    }

    async relayEndOperatorSessionToG8ee(operatorSessionId, g8eContext) {
        return this.relay.relayEndOperatorSessionToG8ee(operatorSessionId, g8eContext);
    }

    async relayListenSessionAuthToG8ee(params, g8eContext) {
        return this.relay.relayListenSessionAuthToG8ee(params, g8eContext);
    }

    async relayTerminateOperatorToG8ee(operatorId, g8eContext) {
        return this.relay.relayTerminateOperatorToG8ee(operatorId, g8eContext);
    }

    // --- Slots ---

    async initializeOperatorSlots(userId, organizationId, webSessionId) {
        return this.slots.initializeOperatorSlots(userId, organizationId, webSessionId);
    }

    async refreshOperatorApiKey(operatorId, userId, webSessionId, broadcastFn) {
        return this.slots.refreshOperatorApiKey(operatorId, userId, webSessionId, broadcastFn);
    }

    async createOperatorSlot(params) {
        return this.slots.createOperatorSlot(params);
    }

    /**
     * Claim an operator slot.
     */
    async claimOperatorSlot(operatorId, params) {
        const success = await this.slots.claimSlot(operatorId, params);
        return success;
    }

    async validateOperatorApiKey(operatorId, apiKey) {
        const operator = await this.getOperator(operatorId);
        if (!operator || operator.status === OperatorStatus.TERMINATED || operator.api_key !== apiKey) return false;
        return true;
    }

    async getUserVisibleOperatorStats(userId, allStatuses = false) {
        let operators;
        if (!allStatuses) {
            operators = await this.operatorDataService.queryListedOperators([{ field: 'user_id', operator: '==', value: userId }]);
            // Also filter out UNAVAILABLE for user-visible stats
            operators = operators.filter(op => op.status !== OperatorStatus.UNAVAILABLE);
        } else {
            operators = await this.operatorDataService.queryOperators([{ field: 'user_id', operator: '==', value: userId }]);
        }
        const activeCount = operators.filter(op => op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND).length;
        return { operators, totalCount: operators.length, activeCount };
    }

    async getUserOperators(userId, allStatuses = false) {
        const stats = await this.getUserVisibleOperatorStats(userId, allStatuses);
        const { usedSlots } = this.calculateSlotUsage(stats.operators);
        const slots = stats.operators.map(op => OperatorSlot.fromOperator(op));

        return new OperatorListUpdatedEvent({
            type: EventType.OPERATOR_PANEL_LIST_UPDATED,
            operators: slots,
            total_count: slots.length,
            active_count: stats.activeCount,
            used_slots: usedSlots,
            max_slots: slots.length,
            timestamp: now(),
        });
    }

    /**
     * Get all operators across all users.
     * Excludes terminated operators by default.
     */
    async getAllOperators(allStatuses = false) {
        const filters = [];
        let filtered;
        if (!allStatuses) {
            filtered = await this.operatorDataService.queryListedOperators(filters);
        } else {
            filtered = await this.operatorDataService.queryOperators(filters);
        }

        const activeCount = filtered.filter(op => op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND).length;

        return {
            operators: filtered,
            total_count: filtered.length,
            active_count: activeCount
        };
    }

    /**
     * Syncs operator session state when a new connection is established.
     * Re-binds any operators that were bound to a different session ID.
     */
    async syncSessionOnConnect(userId, webSessionId) {
        try {
            const { operators: allOperators } = await this.getUserVisibleOperatorStats(userId);
            
            // Re-bind any operators that were bound to a different session ID (tab swap/refresh)
            for (const op of allOperators) {
                if (op.status === OperatorStatus.BOUND && op.bound_web_session_id && op.bound_web_session_id !== webSessionId) {
                    logger.info('[OPERATOR-SERVICE] Updating BOUND Operator bound_web_session_id on reconnect', {
                        id: op.id,
                        old_bound_web_session_id: redactWebSessionId(op.bound_web_session_id),
                        new_bound_web_session_id: redactWebSessionId(webSessionId)
                    });
                    await this.updateWebSessionLink(op.id, webSessionId);
                }
            }

            // Keepalive now provides full operator list - no need to push on connection
            logger.info('[OPERATOR-SERVICE] Synced operator session on connection', {
                webSessionId: redactWebSessionId(webSessionId),
                userId
            });
        } catch (error) {
            logger.error('[OPERATOR-SERVICE] Failed to sync session on connect', {
                webSessionId: redactWebSessionId(webSessionId),
                error: error.message
            });
        }
    }

    /**
     * Update operator's web session link.
     */
    async updateWebSessionLink(operatorId, webSessionId, options = {}) {
        return this.operatorDataService.updateOperator(operatorId, { bound_web_session_id: webSessionId });
    }

    async queryOperators(filters) {
        const data = await this.operatorDataService.queryOperators(filters);
        return (data || []).map(op => OperatorDocument.fromDB(op));
    }

    async queryListedOperators(filters) {
        return this.operatorDataService.queryListedOperators(filters);
    }
    
    async resetOperator(operatorId) {
        const existing = await this.getOperator(operatorId);
        if (!existing) return { success: false, operator: null, error: 'Operator not found' };
        
        await this.operatorDataService.deleteOperator(operatorId);
        const freshOperator = OperatorDocument.forReset({
            id: operatorId,
            user_id: existing.user_id,
            organization_id: existing.organization_id,
            name: existing.name,
            slot_number: existing.slot_number,
            api_key: existing.api_key
        });
        
        await this.operatorDataService.createOperator(operatorId, freshOperator);
        
        // After reset, we should probably inform g8ee if it was tracking this operator
        const g8eContext = await this.getOperatorWithSessionContext(operatorId);
        if (g8eContext) {
            await this.relay.deregisterOperatorSessionInG8ee(g8eContext).catch(() => {});
        }

        return { success: true, operator: freshOperator.forDB(), error: null };
    }

    async terminateOperator(operatorId) {
        const existing = await this.getOperator(operatorId);
        if (!existing) return { success: false, operator: null, error: 'Operator not found' };

        const g8eContext = await this.getOperatorWithSessionContext(operatorId);
        if (!g8eContext) {
            // Should be unreachable: getOperatorWithSessionContext only returns null
            // when the operator does not exist, which we just verified above.
            return { success: false, error: 'Failed to build g8e context for terminate' };
        }

        // Authority: g8ee owns the operator document. It writes TERMINATED status
        // and appends the audit history entry atomically. g8ed must NOT delete the
        // document afterwards or the audit trail entry just written will be lost.
        try {
            await this.relayTerminateOperatorToG8ee(operatorId, g8eContext);
        } catch (err) {
            logger.error('[OPERATOR-SERVICE] Failed to relay terminate to g8ee', {
                operatorId,
                error: err.message,
            });
            return { success: false, id: operatorId, error: err.message };
        }

        // Best-effort resource cleanup in g8ed
        if (existing.api_key && this.apiKeyService) {
            await this.apiKeyService.revokeKey(existing.api_key).catch(err => {
                logger.warn('[OPERATOR-SERVICE] Failed to revoke API key during termination', { operatorId, error: err.message });
            });
        }

        if (existing.operator_cert_serial && this.certificateService) {
            await this.certificateService.revokeCertificate(existing.operator_cert_serial).catch(err => {
                logger.warn('[OPERATOR-SERVICE] Failed to revoke certificate during termination', { operatorId, error: err.message });
            });
        }

        try {
            await this.relay.deregisterOperatorSessionInG8ee(g8eContext);
        } catch (err) {
            // Deregistration is best-effort cleanup — operator is already TERMINATED
            // in the authoritative store, so we surface but do not fail the call.
            logger.warn('[OPERATOR-SERVICE] Failed to deregister operator session after terminate', {
                operatorId,
                error: err.message,
            });
        }

        return { success: true, id: operatorId, error: null };
    }

    async getGrantedIntentsWithDetails(operatorId) {
        const operator = await this.getOperator(operatorId);
        if (!operator) return [];
        const intents = (operator.granted_intents || []).filter(i => i && i.name && i.expires_at).map(i => new GrantedIntent(i));
        return intents.filter(i => i.isActive()).map(i => i.forDB());
    }
}

export { OperatorService, OperatorStatus };

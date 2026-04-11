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
import { EventEmitter } from 'events';
import { OperatorStatus } from '../../constants/operator.js';
import { Collections } from '../../constants/collections.js';
import { 
    OperatorDocument, 
    OperatorStatusInfo, 
    OperatorWithSessionContext, 
    GrantedIntent, 
} from '../../models/operator_model.js';
import { now } from '../../models/base.js';
import { HistoryEventType } from '../../constants/operator.js';
import { EventType } from '../../constants/events.js';

import { OperatorSlotService } from './operator_slot_service.js';
import { OperatorRelayService } from './operator_relay_service.js';
import { OperatorNotificationService } from './operator_notification_service.js';
import { OperatorDataService } from './operator_data_service.js';

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
     * @param {Object} [options.operatorSessionService] - OperatorSessionService instance
     * @param {Object} [options.webSessionService] - WebSessionService instance
     * @param {Object} [options.sseService] - SSEService instance
     */
    constructor({
        operatorDataService,
        userService,
        apiKeyService,
        certificateService,
        operatorSessionService,
        webSessionService,
        sseService
    }) {
        if (!operatorDataService) throw new Error('operatorDataService is required');
        
        this.operatorDataService = operatorDataService;
        this.userService = userService;
        this.apiKeyService = apiKeyService;
        this.certificateService = certificateService;
        this.operatorSessionService = operatorSessionService;
        this.webSessionService = webSessionService;
        this.sseService = sseService;
        
        this.collectionName = this.operatorDataService.collectionName;

        // Subservices
        this.slots = new OperatorSlotService({
            operatorDataService: this.operatorDataService,
            apiKeyService,
            certificateService,
            operatorSessionService,
        });

        this.relay = new OperatorRelayService();

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
            op.operator_id !== excludeOperatorId &&
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

    async getOperatorStatusInfo(operatorId) {
        const operator = await this.getOperator(operatorId);
        return operator ? new OperatorStatusInfo(operator) : null;
    }

    async getOperatorStatus(operatorId) {
        return this.getOperatorStatusInfo(operatorId);
    }

    async getOperatorByUserId(userId) {
        const data = await this.operatorDataService.queryOperators([{ field: 'user_id', operator: '==', value: userId }]);
        if (!data || data.length === 0) return null;
        const deployed = data.find(op => op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND);
        if (deployed) return deployed;
        return data.find(op => op.status === OperatorStatus.AVAILABLE) || null;
    }

    async getOperatorWithSessionContext(operatorId) {
        const operator = await this.getOperator(operatorId);
        if (!operator) return null;
        const operatorSession = operator.operator_session_id && this.operatorSessionService
            ? await this.operatorSessionService.validateSession(operator.operator_session_id)
            : null;
        const webSession = operator.web_session_id && this.webSessionService
            ? await this.webSessionService.validateSession(operator.web_session_id)
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

    // --- Notifications ---

    async broadcastOperatorListToSession(userId, webSessionId) {
        return this.notifications.broadcastOperatorListToSession(userId, webSessionId, this.calculateSlotUsage.bind(this));
    }

    async _broadcastOperatorListToUser(userId) {
        return this.notifications.broadcastOperatorListToUser(userId, this.calculateSlotUsage.bind(this));
    }

    // --- Slots ---

    async initializeOperatorSlots(userId, organizationId) {
        return this.slots.initializeOperatorSlots(userId, organizationId);
    }

    async refreshOperatorApiKey(operatorId, userId) {
        return this.slots.refreshOperatorApiKey(operatorId, userId, this._broadcastOperatorListToUser.bind(this));
    }

    async createOperatorSlot(params) {
        return this.slots.createOperatorSlot(params);
    }

    /**
     * Claim an operator slot.
     */
    async claimOperatorSlot(operatorId, params) {
        const success = await this.slots.claimSlot(operatorId, params);
        if (success) {
            const operator = await this.getOperator(operatorId);
            if (operator && operator.user_id) {
                await this._broadcastOperatorListToUser(operator.user_id);
            }
        }
        return success;
    }

    async validateOperatorApiKey(operatorId, apiKey) {
        const operator = await this.getOperator(operatorId);
        if (!operator || operator.status === OperatorStatus.TERMINATED || operator.api_key !== apiKey) return false;
        return true;
    }

    async getUserVisibleOperatorStats(userId) {
        const allOperators = await this.operatorDataService.queryOperators([{ field: 'user_id', operator: '==', value: userId }]);
        const operators = allOperators.filter(op => op.status !== OperatorStatus.UNAVAILABLE && op.status !== OperatorStatus.TERMINATED);
        const activeCount = operators.filter(op => op.status === OperatorStatus.ACTIVE || op.status === OperatorStatus.BOUND).length;
        return { operators, totalCount: operators.length, activeCount };
    }

    async getUserOperators(userId) {
        const stats = await this.getUserVisibleOperatorStats(userId);
        const enhancedOperators = stats.operators.map((op) => {
            const s = op.status ?? OperatorStatus.OFFLINE;
            return { ...op, status_display: s, status_class: s === OperatorStatus.OFFLINE ? 'inactive' : s.toLowerCase() };
        });
        const { usedSlots } = this.calculateSlotUsage(enhancedOperators);
        
        // Broadcast happens via notifications service if needed, but here we just return the event-ready object
        return {
            type: EventType.OPERATOR_PANEL_LIST_UPDATED,
            operators: enhancedOperators,
            total_count: enhancedOperators.length,
            active_count: stats.activeCount,
            used_slots: usedSlots,
            max_slots: enhancedOperators.length,
            timestamp: now(),
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
                if (op.status === OperatorStatus.BOUND && op.web_session_id && op.web_session_id !== webSessionId) {
                    logger.info('[OPERATOR-SERVICE] Updating BOUND Operator web_session_id on reconnect', {
                        operator_id: op.operator_id,
                        old_web_session_id: redactWebSessionId(op.web_session_id),
                        new_web_session_id: redactWebSessionId(webSessionId)
                    });
                    // We call updateWebSessionLink but skip broadcast since we'll do a full list push below
                    await this.updateWebSessionLink(op.operator_id, webSessionId, { skipBroadcast: true });
                }
            }

            // Push the full current operator list to the new connection
            await this.broadcastOperatorListToSession(userId, webSessionId);
            
            logger.info('[OPERATOR-SERVICE] Proactively sent Operator list on connection', {
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
        return this.operatorDataService.updateOperator(operatorId, { web_session_id: webSessionId });
    }

    async queryOperators(filters) {
        return this.operatorDataService.queryOperators(filters);
    }
    
    async resetOperator(operatorId) {
        const existing = await this.getOperator(operatorId);
        if (!existing) return { success: false, operator: null, error: 'Operator not found' };
        
        await this.operatorDataService.deleteOperator(operatorId);
        const freshOperator = OperatorDocument.forReset({
            operator_id: operatorId,
            user_id: existing.user_id,
            organization_id: existing.organization_id,
            name: existing.name,
            slot_number: existing.slot_number,
            api_key: existing.api_key
        });
        
        await this.operatorDataService.createOperator(operatorId, freshOperator);
        await this._broadcastOperatorListToUser(existing.user_id);
        
        // After reset, we should probably inform g8ee if it was tracking this operator
        const g8eContext = await this.getOperatorWithSessionContext(operatorId);
        if (g8eContext) {
            await this.relay.deregisterOperatorSessionInG8ee(g8eContext).catch(() => {});
        }

        return { success: true, operator: freshOperator.forDB(), error: null };
    }

    async getGrantedIntentsWithDetails(operatorId) {
        const operator = await this.getOperator(operatorId);
        if (!operator) return [];
        const intents = (operator.granted_intents || []).filter(i => i && i.name && i.expires_at).map(i => new GrantedIntent(i));
        return intents.filter(i => i.isActive()).map(i => i.forDB());
    }
}

export { OperatorService, OperatorStatus };

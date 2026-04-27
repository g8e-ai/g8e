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

import { v4 as uuidv4 } from 'uuid';
import { logger } from '../../utils/logger.js';
import { OperatorStatus, OperatorType, CloudOperatorSubtype, DEFAULT_OPERATOR_SLOTS } from '../../constants/operator.js';
import { OperatorDocument, SystemInfo, OperatorSlotCreationResponse } from '../../models/operator_model.js';
import { OperatorRefreshKeyResponse } from '../../models/response_models.js';
import { G8eHttpContext } from '../../models/request_models.js';
import { now } from '../../models/base.js';
import { SourceComponent } from '../../constants/ai.js';
import { ApiKeyStatus, ApiKeyClientName, ApiKeyPermission, DeviceLinkError } from '../../constants/auth.js';
import crypto from 'crypto';
import { API_KEY_PREFIX } from '../../constants/operator_defaults.js';

export class OperatorSlotService {
    constructor({ operatorDataService, apiKeyService, certificateService, operatorService }) {
        this.operatorDataService = operatorDataService;
        this.apiKeyService = apiKeyService;
        this.certificateService = certificateService;
        this.operatorService = operatorService;
    }

    async initializeOperatorSlots(userId, organizationId, webSessionId) {
        const liveOperators = await this.operatorDataService.queryListedOperators([
            { field: 'user_id', operator: '==', value: userId }
        ], { fresh: true });
        const existingCount = liveOperators.length;
        const createdSlotIds = [];

        if (existingCount < DEFAULT_OPERATOR_SLOTS) {
            const hasG8eNode = liveOperators.some(op => op.is_g8ep === true);
            const slotsToCreate = DEFAULT_OPERATOR_SLOTS - existingCount;
            let g8eNodeAssigned = hasG8eNode;
            for (let i = 0; i < slotsToCreate; i++) {
                const slotNumber = existingCount + i + 1;
                const assignG8eNode = !g8eNodeAssigned;
                if (assignG8eNode) g8eNodeAssigned = true;
                const creationResponse = await this.createOperatorSlot({
                    userId,
                    organizationId,
                    slotNumber,
                    operatorType: OperatorType.CLOUD,
                    cloudSubtype: assignG8eNode ? CloudOperatorSubtype.G8E_POD : null,
                    namePrefix: 'operator',
                    isG8eNode: assignG8eNode,
                    webSessionId,
                });
                if (creationResponse.success && creationResponse.operator_id) {
                    createdSlotIds.push(creationResponse.operator_id);
                }
            }
        }

        const allOperatorIds = liveOperators.map(op => op.id).concat(createdSlotIds);

        for (const operator of liveOperators) {
            if (!operator.api_key) {
                logger.info('[OPERATOR-SLOT] Issuing API key for existing slot without key', { operator_id: operator.id });
                const apiKey = this.generateOperatorApiKey(operator.id);

                const g8eContext = G8eHttpContext.parse({
                    web_session_id: webSessionId || `init-${userId}-${Date.now()}`,
                    user_id: userId,
                    organization_id: organizationId,
                    source_component: SourceComponent.G8ED,
                });

                const updateResponse = await this.operatorService.relayUpdateOperatorApiKeyToG8ee(operator.id, apiKey, g8eContext);

                if (!updateResponse || !updateResponse.success) {
                    logger.error('[OPERATOR-SLOT] Failed to update API key in g8ee for existing slot', { id: operator.id, error: updateResponse?.error || 'Unknown error' });
                }
            }
        }

        return allOperatorIds;
    }

    async refreshOperatorApiKey(id, userId, webSessionId, broadcastFn) {
        const oldOperator = await this.operatorDataService.getOperator(id);
        if (!oldOperator) return OperatorRefreshKeyResponse.forFailure(DeviceLinkError.OPERATOR_NOT_FOUND);
        if (oldOperator.user_id !== userId) return OperatorRefreshKeyResponse.forFailure('Unauthorized');

        const g8eContext = {
            user_id: userId,
            organization_id: oldOperator.organization_id,
            source_component: SourceComponent.G8ED,
            web_session_id: webSessionId,
        };

        if (oldOperator.operator_session_id && this.operatorService) {
            await this.operatorService.relayEndOperatorSessionToG8ee(oldOperator.operator_session_id, g8eContext).catch(err => {
                logger.error('[OPERATOR-SLOT] Failed to end operator session during refresh', { id, error: err.message });
            });
        }

        // Delegate termination and resource cleanup to OperatorService
        if (this.operatorService) {
            const termResult = await this.operatorService.terminateOperator(id);
            if (!termResult.success) {
                logger.error('[OPERATOR-SLOT] Failed to terminate operator during refresh', { id, error: termResult.error });
                // We continue anyway to try and restore the slot, but log the failure
            }
        }

        // Create new slot via g8ee (this will also issue the new API key returned by g8ee)
        const creationResponse = await this.createOperatorSlot({
            userId,
            organizationId: oldOperator.organization_id,
            slotNumber: oldOperator.slot_number,
            operatorType: oldOperator.operator_type || OperatorType.SYSTEM,
            cloudSubtype: oldOperator.cloud_subtype || null,
            namePrefix: oldOperator.name ? oldOperator.name.split('-')[0] : 'operator',
            isG8eNode: oldOperator.is_g8ep ?? false,
            webSessionId,
        });

        if (!creationResponse.success) {
            return OperatorRefreshKeyResponse.forFailure(creationResponse.message || 'Failed to create new operator slot');
        }

        if (broadcastFn) {
            const broadcastResult = broadcastFn(userId);
            if (broadcastResult && typeof broadcastResult.catch === 'function') {
                await broadcastResult.catch(err => {
                    logger.error('[OPERATOR-SLOT] Failed to broadcast operator refresh', { userId, error: err.message });
                });
            }
        }

        return OperatorRefreshKeyResponse.forSuccess(
            creationResponse.api_key,
            creationResponse.operator_id,
            oldOperator.id,
            oldOperator.slot_number,
            'API key refreshed'
        );
    }

    async createOperatorSlot(params) {
        const { userId, organizationId, slotNumber, operatorType, cloudSubtype, namePrefix, isG8eNode, webSessionId } = params;

        // Use g8ee for operator slot creation to enforce architectural boundary
        // g8ed should not write to operators after auth
        if (!this.operatorService) {
            throw new Error('operatorService is required for operator slot creation');
        }

        // For device link scenarios, webSessionId may be null - use a placeholder
        const sessionContext = webSessionId || `device-link-${userId}-${Date.now()}`;

        const g8eContext = G8eHttpContext.parse({
            web_session_id: sessionContext,
            user_id: userId,
            organization_id: organizationId,
            source_component: SourceComponent.G8ED,
        });

        const relayParams = {
            user_id: userId,
            organization_id: organizationId,
            slot_number: slotNumber,
            operator_type: operatorType,
            cloud_subtype: cloudSubtype,
            name_prefix: namePrefix || 'operator',
            is_g8e_node: isG8eNode || false,
        };

        const response = await this.operatorService.relayCreateOperatorSlotToG8ee(relayParams, g8eContext);

        if (response.success && response.operator_id && response.api_key) {
            return OperatorSlotCreationResponse.forSuccess(response.operator_id, response.api_key);
        } else {
            logger.error('[OPERATOR-SLOT] g8ee failed to create operator slot', { error: response.error });
            return OperatorSlotCreationResponse.forFailure(response.error || 'Failed to create operator slot via g8ee');
        }
    }

    /**
     * Claim an operator slot for an active session.
     * Authority for transitioning an AVAILABLE slot to ACTIVE.
     */
    async claimSlot(id, { operator_session_id, bound_web_session_id, system_info, operator_type, status }) {
        const ts = now();
        const info = system_info instanceof SystemInfo
            ? system_info
            : SystemInfo.parse(system_info || {});

        // Use g8ee for operator slot claiming to enforce architectural boundary
        // g8ed should not write to operators after auth
        if (!this.operatorService) {
            throw new Error('operatorService is required for operator slot claiming');
        }

        const operator = await this.operatorDataService.getOperator(id);
        if (!operator) {
            logger.error('[OPERATOR-SLOT] Operator not found for claiming', { id });
            return { success: false, error: 'Operator not found' };
        }

        const g8eContext = {
            user_id: operator.user_id,
            organization_id: operator.organization_id,
            source_component: SourceComponent.G8ED,
        };

        const relayParams = {
            operator_id: id,
            operator_session_id,
            bound_web_session_id,
            system_info: info.forDB ? info.forDB() : system_info,
            operator_type: operator_type || operator.operator_type,
        };

        const response = await this.operatorService.relayClaimOperatorSlotToG8ee(relayParams, g8eContext);
        
        if (response.success) {
            return { success: true };
        } else {
            logger.error('[OPERATOR-SLOT] g8ee failed to claim operator slot', { error: response.error });
            return { success: false, error: response.error || 'Failed to claim operator slot via g8ee' };
        }
    }

    generateOperatorApiKey(id) {
        const operatorSuffix = id.split('-').pop().substring(0, 8);
        const randomToken = crypto.randomBytes(32).toString('hex');
        return `${API_KEY_PREFIX}${operatorSuffix}_${randomToken}`;
    }

}

#!/usr/bin/env node
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
 * Cleanup Test Operators Script
 * 
 * Removes all operators from the operators_test collection and related KV keys.
 * Use this to clean up leftover data from crashed test runs.
 */

import { initializeServices } from '../services/initialization.js';
import { Collections } from '../constants/collections.js';
import { KVKey } from '../constants/kv_keys.js';
import { logger } from '../utils/logger.js';

async function cleanupTestOperators() {
    try {
        logger.info('[CLEANUP] Initializing services...');
        await initializeServices();

        const { getCacheAsideService, getG8esKvClient, getOperatorService } = await import('../services/initialization.js');
        
        const cacheAside = getCacheAsideService();
        const kvClient = getG8esKvClient();
        const operatorService = getOperatorService();

        const TEST_OPERATORS_COLLECTION = `${Collections.OPERATORS}_test`;

        logger.info(`[CLEANUP] Querying operators from ${TEST_OPERATORS_COLLECTION}...`);
        
        const operators = await cacheAside.queryDocuments(TEST_OPERATORS_COLLECTION, []);
        logger.info(`[CLEANUP] Found ${operators ? operators.length : 0} operators to clean up`);

        if (operators && operators.length > 0) {
            for (const op of operators) {
                const opId = op.operator_id;
                
                // Delete operator document
                await cacheAside.deleteDocument(TEST_OPERATORS_COLLECTION, opId);
                
                // Delete API key
                if (op.operator_api_key) {
                    await cacheAside.deleteDocument('api_keys', op.operator_api_key);
                }
                
                // Delete KV keys
                await kvClient.del(KVKey.doc(TEST_OPERATORS_COLLECTION, opId));
                await kvClient.del(KVKey.operatorFirstDeployed(opId));
                await kvClient.del(KVKey.operatorTrackedStatus(opId));
                
                // Delete user operators KV entry
                if (op.user_id) {
                    await kvClient.del(KVKey.userOperators(op.user_id));
                }
                
                logger.info(`[CLEANUP] Deleted operator ${opId}`);
            }
        }

        logger.info('[CLEANUP] Cleanup complete');
        process.exit(0);
    } catch (error) {
        logger.error('[CLEANUP] Failed:', error);
        process.exit(1);
    }
}

cleanupTestOperators();

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

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { OperatorDataService } from '../../../../services/operator/operator_data_service.js';
import { Collections } from '../../../../constants/collections.js';
import { OperatorDocument } from '../../../../models/operator_model.js';

describe('OperatorDataService', () => {
    let operatorDataService;
    let mockCacheAside;

    beforeEach(() => {
        mockCacheAside = {
            getDocument: vi.fn(),
            evictDocument: vi.fn(),
            queryDocuments: vi.fn(),
            createDocument: vi.fn(),
            updateDocument: vi.fn(),
            deleteDocument: vi.fn(),
        };
        operatorDataService = new OperatorDataService({ cacheAsideService: mockCacheAside });
    });

    describe('constructor', () => {
        it('should initialize with cacheAside and default collection', () => {
            expect(operatorDataService._cache_aside).toBe(mockCacheAside);
            expect(operatorDataService.collectionName).toBe(Collections.OPERATORS);
        });

        it('should throw if cacheAsideService is missing', () => {
            expect(() => new OperatorDataService({})).toThrow('cacheAsideService is required');
        });
    });

    describe('getOperator', () => {
        it('should return OperatorDocument when data exists', async () => {
            const mockData = { operator_id: 'op-123', user_id: 'user-123', status: 'ACTIVE' };
            mockCacheAside.getDocument.mockResolvedValue(mockData);

            const result = await operatorDataService.getOperator('op-123');

            expect(mockCacheAside.getDocument).toHaveBeenCalledWith(Collections.OPERATORS, 'op-123');
            expect(result).toBeInstanceOf(OperatorDocument);
            expect(result.operator_id).toBe('op-123');
        });

        it('should return null when data does not exist', async () => {
            mockCacheAside.getDocument.mockResolvedValue(null);

            const result = await operatorDataService.getOperator('op-123');

            expect(result).toBeNull();
        });
    });

    describe('getOperatorFresh', () => {
        it('should evict then get operator', async () => {
            const mockData = { operator_id: 'op-123', user_id: 'user-123' };
            mockCacheAside.getDocument.mockResolvedValue(mockData);

            const result = await operatorDataService.getOperatorFresh('op-123');

            expect(mockCacheAside.evictDocument).toHaveBeenCalledWith(Collections.OPERATORS, 'op-123');
            expect(mockCacheAside.getDocument).toHaveBeenCalledWith(Collections.OPERATORS, 'op-123');
            expect(result.operator_id).toBe('op-123');
        });
    });

    describe('queryOperators', () => {
        it('should return array of results', async () => {
            const mockResults = [{ operator_id: 'op-1' }, { operator_id: 'op-2' }];
            mockCacheAside.queryDocuments.mockResolvedValue(mockResults);

            const filters = [{ field: 'user_id', operator: '==', value: 'user-1' }];
            const result = await operatorDataService.queryOperators(filters);

            expect(mockCacheAside.queryDocuments).toHaveBeenCalledWith(Collections.OPERATORS, filters);
            expect(result).toEqual(mockResults);
        });

        it('should return empty array when no results', async () => {
            mockCacheAside.queryDocuments.mockResolvedValue(null);

            const result = await operatorDataService.queryOperators([]);

            expect(result).toEqual([]);
        });
    });

    describe('queryOperatorsFresh', () => {
        it('should call queryDocuments with bypassCache=true', async () => {
            const mockResults = [{ operator_id: 'op-1' }, { operator_id: 'op-2' }];
            mockCacheAside.queryDocuments.mockResolvedValue(mockResults);

            const filters = [{ field: 'user_id', operator: '==', value: 'user-1' }];
            const result = await operatorDataService.queryOperatorsFresh(filters);

            expect(mockCacheAside.queryDocuments).toHaveBeenCalledWith(Collections.OPERATORS, filters, null, true);
            expect(result).toEqual(mockResults);
        });

        it('should return empty array when no results', async () => {
            mockCacheAside.queryDocuments.mockResolvedValue(null);

            const result = await operatorDataService.queryOperatorsFresh([]);

            expect(result).toEqual([]);
        });
    });

    describe('createOperator', () => {
        it('should call cacheAside.createDocument', async () => {
            const mockDoc = { operator_id: 'op-123' };
            mockCacheAside.createDocument.mockResolvedValue(true);

            const result = await operatorDataService.createOperator('op-123', mockDoc);

            expect(mockCacheAside.createDocument).toHaveBeenCalledWith(Collections.OPERATORS, 'op-123', mockDoc);
            expect(result).toBe(true);
        });
    });

    describe('updateOperator', () => {
        it('should call cacheAside.updateDocument', async () => {
            const updateData = { status: 'BOUND' };
            mockCacheAside.updateDocument.mockResolvedValue(true);

            const result = await operatorDataService.updateOperator('op-123', updateData);

            expect(mockCacheAside.updateDocument).toHaveBeenCalledWith(Collections.OPERATORS, 'op-123', updateData);
            expect(result).toBe(true);
        });
    });

    describe('deleteOperator', () => {
        it('should call cacheAside.deleteDocument', async () => {
            mockCacheAside.deleteDocument.mockResolvedValue(true);

            const result = await operatorDataService.deleteOperator('op-123');

            expect(mockCacheAside.deleteDocument).toHaveBeenCalledWith(Collections.OPERATORS, 'op-123');
            expect(result).toBe(true);
        });
    });
});

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
import { BindOperatorsService } from '@vsod/services/operator/operator_bind_service.js';
import { OperatorStatus } from '@vsod/constants/operator.js';
import { DeviceLinkError } from '@vsod/constants/auth.js';
import { BindOperatorsRequest, UnbindOperatorsRequest, VSOHttpContext } from '@vsod/models/request_models.js';
import {
    OperatorDocument,
    OperatorWithSessionContext,
    BindOperatorsResponse,
    UnbindOperatorsResponse,
} from '@vsod/models/operator_model.js';
import { HttpStatusMessage } from '@vsod/constants/errors.js';

describe('BindOperatorsService', () => {
    let service;
    let mocks;

    beforeEach(() => {
        mocks = {
            operatorService: {
                getOperator: vi.fn(),
                getOperatorWithSessionContext: vi.fn(),
                relayRegisterOperatorSessionToG8ee: vi.fn().mockResolvedValue({ success: true }),
                broadcastOperatorListToSession: vi.fn().mockResolvedValue(),
                operatorDataService: {
                    updateOperator: vi.fn(),
                },
            },
            bindingService: {
                bind: vi.fn(),
                unbind: vi.fn(),
                getBoundOperatorSessionIds: vi.fn(),
            },
            operatorSessionService: {
                validateSession: vi.fn(),
            },
            webSessionService: {
                bindOperatorToWebSession: vi.fn().mockResolvedValue(),
                unbindOperatorFromWebSession: vi.fn().mockResolvedValue(),
            },
        };

        service = new BindOperatorsService(mocks);
    });

    describe('bindOperators', () => {
        it('should fail if operator not found', async () => {
            mocks.operatorService.getOperator.mockResolvedValue(null);

            const bindReq = new BindOperatorsRequest({ 
                operator_ids: ['op-123'], 
                web_session_id: 'ws-123', 
                user_id: 'u-123' 
            });
            const result = await service.bindOperators(bindReq);

            expect(result.success).toBe(false);
            expect(result.statusCode).toBe(400);
            expect(result.failed_count).toBe(1);
            expect(result.errors[0].error).toContain(DeviceLinkError.OPERATOR_NOT_FOUND);
        });

        it('should fail if operator belongs to different user', async () => {
            mocks.operatorService.getOperator.mockResolvedValue(new OperatorDocument({ operator_id: 'op-123', user_id: 'u-456', status: OperatorStatus.ACTIVE }));

            const bindReq = new BindOperatorsRequest({ 
                operator_ids: ['op-123'], 
                web_session_id: 'ws-123', 
                user_id: 'u-123' 
            });
            const result = await service.bindOperators(bindReq);

            expect(result.success).toBe(false);
            expect(result.statusCode).toBe(400);
            expect(result.failed_count).toBe(1);
            expect(result.errors[0].error).toBe(DeviceLinkError.OPERATOR_WRONG_USER);
        });

        it('should successfully bind and relay to g8ee', async () => {
            const operator = new OperatorDocument({ 
                operator_id: 'op-123', 
                user_id: 'u-123', 
                operator_session_id: 'os-123',
                status: OperatorStatus.ACTIVE
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);
            
            const vsoContextWrapper = OperatorWithSessionContext.create(operator, { id: 'os-123' }, { id: 'ws-123' });
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(vsoContextWrapper);
            mocks.operatorService.relayRegisterOperatorSessionToG8ee.mockResolvedValue({ success: true });
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const bindReq = new BindOperatorsRequest({ 
                operator_ids: ['op-123'], 
                web_session_id: 'ws-123', 
                user_id: 'u-123' 
            });
            const result = await service.bindOperators(bindReq);

            expect(result).toBeInstanceOf(BindOperatorsResponse);
            expect(result.success).toBe(true);
            expect(result.bound_count).toBe(1);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-123', {
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-123'
            });
            expect(mocks.bindingService.bind).toHaveBeenCalledWith('os-123', 'ws-123', 'u-123', 'op-123');
            expect(mocks.operatorService.relayRegisterOperatorSessionToG8ee).toHaveBeenCalledWith(expect.any(VSOHttpContext));
        });
    });

    describe('unbindOperators', () => {
        it('should unbind and relay to g8ee', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-123',
                user_id: 'u-123',
                operator_session_id: 'os-123',
                status: OperatorStatus.BOUND
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue(['os-123']);
            mocks.operatorSessionService.validateSession.mockResolvedValue({ operator_id: 'op-123' });
            
            const vsoContextWrapper = OperatorWithSessionContext.create(operator, { id: 'os-123' }, { id: 'ws-123' });
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(vsoContextWrapper);
            mocks.operatorService.relayRegisterOperatorSessionToG8ee.mockResolvedValue({ success: true });
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const unbindReq = new UnbindOperatorsRequest({ 
                operator_ids: ['op-123'], 
                web_session_id: 'ws-123', 
                user_id: 'u-123' 
            });
            const result = await service.unbindOperators(unbindReq);

            expect(result).toBeInstanceOf(UnbindOperatorsResponse);
            expect(result.success).toBe(true);
            expect(result.unbound_count).toBe(1);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-123', {
                status: OperatorStatus.ACTIVE,
                web_session_id: null
            });
            expect(mocks.bindingService.unbind).toHaveBeenCalledWith('os-123', 'ws-123', 'op-123');
            expect(mocks.operatorService.relayRegisterOperatorSessionToG8ee).toHaveBeenCalledWith(expect.any(VSOHttpContext));
        });
    });

    describe('bindOperators', () => {
        it('should successfully bind multiple operators and return BindOperatorsResponse', async () => {
            const op1 = new OperatorDocument({ 
                operator_id: 'op-1', 
                user_id: 'u-123', 
                operator_session_id: 'os-1',
                status: OperatorStatus.ACTIVE
            });
            const op2 = new OperatorDocument({ 
                operator_id: 'op-2', 
                user_id: 'u-123', 
                operator_session_id: 'os-2',
                status: OperatorStatus.ACTIVE
            });

            mocks.operatorService.getOperator
                .mockResolvedValueOnce(op1)
                .mockResolvedValueOnce(op2);
            
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);
            
            const vsoContext1 = OperatorWithSessionContext.create(op1, { id: 'os-1' }, { id: 'ws-123' });
            const vsoContext2 = OperatorWithSessionContext.create(op2, { id: 'os-2' }, { id: 'ws-123' });
            mocks.operatorService.getOperatorWithSessionContext
                .mockResolvedValueOnce(vsoContext1)
                .mockResolvedValueOnce(vsoContext2);

            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const bindReq = new BindOperatorsRequest({ 
                operator_ids: ['op-1', 'op-2'], 
                web_session_id: 'ws-123', 
                user_id: 'u-123' 
            });
            const result = await service.bindOperators(bindReq);

            expect(result).toBeInstanceOf(BindOperatorsResponse);
            expect(result.success).toBe(true);
            expect(result.bound_count).toBe(2);
            expect(result.bound_operator_ids).toEqual(['op-1', 'op-2']);
            expect(mocks.bindingService.bind).toHaveBeenCalledTimes(2);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-1', {
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-123'
            });
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-2', {
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-123'
            });
        });

        it('should handle partial failures in multiple operator bind', async () => {
            const op1 = new OperatorDocument({ 
                operator_id: 'op-1', 
                user_id: 'u-123', 
                operator_session_id: 'os-1',
                status: OperatorStatus.ACTIVE
            });

            mocks.operatorService.getOperator
                .mockResolvedValueOnce(op1)
                .mockResolvedValueOnce(null); // Not found
            
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);
            const vsoContextWrapper = OperatorWithSessionContext.create(op1, { id: 'os-1' }, { id: 'ws-123' });
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValueOnce(vsoContextWrapper);

            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const bindReq = new BindOperatorsRequest({ 
                operator_ids: ['op-1', 'op-missing'], 
                web_session_id: 'ws-123', 
                user_id: 'u-123' 
            });
            const result = await service.bindOperators(bindReq);

            expect(result.bound_count).toBe(1);
            expect(result.failed_count).toBe(1);
            expect(result.failed_operator_ids).toContain('op-missing');
        });
    });

    describe('unbindOperators', () => {
        it('should successfully unbind multiple operators and return OperatorUnbindResponse', async () => {
            const op1 = new OperatorDocument({ 
                operator_id: 'op-1', 
                user_id: 'u-123', 
                operator_session_id: 'os-1',
                status: OperatorStatus.BOUND
            });
            const op2 = new OperatorDocument({ 
                operator_id: 'op-2', 
                user_id: 'u-123', 
                operator_session_id: 'os-2',
                status: OperatorStatus.BOUND
            });

            mocks.operatorService.getOperator
                .mockResolvedValueOnce(op1)
                .mockResolvedValueOnce(op2);
            
            mocks.operatorService.getOperatorWithSessionContext
                .mockResolvedValueOnce(OperatorWithSessionContext.create(op1, { id: 'os-1' }, { id: 'ws-123' }))
                .mockResolvedValueOnce(OperatorWithSessionContext.create(op2, { id: 'os-2' }, { id: 'ws-123' }));

            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const unbindReq = new UnbindOperatorsRequest({ 
                operator_ids: ['op-1', 'op-2'], 
                web_session_id: 'ws-123', 
                user_id: 'u-123' 
            });
            const result = await service.unbindOperators(unbindReq);

            expect(result).toBeInstanceOf(UnbindOperatorsResponse);
            expect(result.success).toBe(true);
            expect(result.unbound_count).toBe(2);
            expect(result.unbound_operator_ids).toEqual(['op-1', 'op-2']);
            expect(mocks.bindingService.unbind).toHaveBeenCalledTimes(2);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-1', {
                status: OperatorStatus.ACTIVE,
                web_session_id: null
            });
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-2', {
                status: OperatorStatus.ACTIVE,
                web_session_id: null
            });
        });
    });

    describe('status transitions (regression)', () => {
        it('bind transitions operator status from ACTIVE to BOUND', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-reg-1',
                user_id: 'u-1',
                operator_session_id: 'os-reg-1',
                status: OperatorStatus.ACTIVE
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-reg-1' }, { id: 'ws-reg-1' })
            );
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            await service.bindOperators(new BindOperatorsRequest({
                operator_ids: ['op-reg-1'],
                web_session_id: 'ws-reg-1',
                user_id: 'u-1'
            }));

            const updateCall = mocks.operatorService.operatorDataService.updateOperator.mock.calls[0];
            expect(updateCall[0]).toBe('op-reg-1');
            expect(updateCall[1].status).toBe(OperatorStatus.BOUND);
            expect(updateCall[1].web_session_id).toBe('ws-reg-1');
        });

        it('unbind transitions operator status from BOUND to ACTIVE', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-reg-2',
                user_id: 'u-1',
                operator_session_id: 'os-reg-2',
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-reg-2'
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-reg-2' }, { id: 'ws-reg-2' })
            );
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            await service.unbindOperators(new UnbindOperatorsRequest({
                operator_ids: ['op-reg-2'],
                web_session_id: 'ws-reg-2',
                user_id: 'u-1'
            }));

            const updateCall = mocks.operatorService.operatorDataService.updateOperator.mock.calls[0];
            expect(updateCall[0]).toBe('op-reg-2');
            expect(updateCall[1].status).toBe(OperatorStatus.ACTIVE);
            expect(updateCall[1].web_session_id).toBeNull();
        });
    });

    describe('bindOperator wrapper method', () => {
        it('should delegate to bindOperators', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-wrapper-1',
                user_id: 'u-123',
                operator_session_id: 'os-wrapper-1',
                status: OperatorStatus.ACTIVE
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-wrapper-1' }, { id: 'ws-wrapper-1' })
            );
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const bindReq = new BindOperatorsRequest({
                operator_ids: ['op-wrapper-1'],
                web_session_id: 'ws-wrapper-1',
                user_id: 'u-123'
            });

            const result = await service.bindOperator(bindReq);

            expect(result).toBeInstanceOf(BindOperatorsResponse);
            expect(result.success).toBe(true);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-wrapper-1', {
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-wrapper-1'
            });
        });
    });

    describe('already bound operator', () => {
        it('should skip rebinding if operator is already bound to the session', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-already-bound',
                user_id: 'u-123',
                operator_session_id: 'os-already-bound',
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-123'
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue(['os-already-bound']);

            const bindReq = new BindOperatorsRequest({
                operator_ids: ['op-already-bound'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.bindOperators(bindReq);

            expect(result.success).toBe(true);
            expect(result.bound_count).toBe(1);
            expect(result.bound_operator_ids).toContain('op-already-bound');
            expect(mocks.operatorService.operatorDataService.updateOperator).not.toHaveBeenCalled();
            expect(mocks.bindingService.bind).not.toHaveBeenCalled();
            expect(mocks.webSessionService.bindOperatorToWebSession).not.toHaveBeenCalled();
        });

        it('should handle mix of already bound and new operators', async () => {
            const op1 = new OperatorDocument({
                operator_id: 'op-already-1',
                user_id: 'u-123',
                operator_session_id: 'os-already-1',
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-123'
            });
            const op2 = new OperatorDocument({
                operator_id: 'op-new-2',
                user_id: 'u-123',
                operator_session_id: 'os-new-2',
                status: OperatorStatus.ACTIVE
            });

            mocks.operatorService.getOperator
                .mockResolvedValueOnce(op1)
                .mockResolvedValueOnce(op2);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue(['os-already-1']);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(op2, { id: 'os-new-2' }, { id: 'ws-123' })
            );
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const bindReq = new BindOperatorsRequest({
                operator_ids: ['op-already-1', 'op-new-2'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.bindOperators(bindReq);

            expect(result.success).toBe(true);
            expect(result.bound_count).toBe(2);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledTimes(1);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-new-2', {
                status: OperatorStatus.BOUND,
                web_session_id: 'ws-123'
            });
        });
    });

    describe('operator with no active session', () => {
        it('should fail if operator has no active session', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-no-session',
                user_id: 'u-123',
                operator_session_id: null,
                status: OperatorStatus.ACTIVE
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);

            const bindReq = new BindOperatorsRequest({
                operator_ids: ['op-no-session'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.bindOperators(bindReq);

            expect(result.success).toBe(false);
            expect(result.failed_count).toBe(1);
            expect(result.errors[0].error).toBe('Operator has no active session');
            expect(mocks.operatorService.operatorDataService.updateOperator).not.toHaveBeenCalled();
        });
    });

    describe('unbindOperator wrapper method', () => {
        it('should parse and delegate to unbindOperators', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-unbind-wrapper',
                user_id: 'u-123',
                operator_session_id: 'os-unbind-wrapper',
                status: OperatorStatus.BOUND
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-unbind-wrapper' }, { id: 'ws-unbind-wrapper' })
            );
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const unbindReq = {
                operator_ids: ['op-unbind-wrapper'],
                web_session_id: 'ws-unbind-wrapper',
                user_id: 'u-123'
            };

            const result = await service.unbindOperator(unbindReq);

            expect(result).toBeInstanceOf(UnbindOperatorsResponse);
            expect(result.success).toBe(true);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-unbind-wrapper', {
                status: OperatorStatus.ACTIVE,
                web_session_id: null
            });
        });

        it('should accept UnbindOperatorsRequest instance directly', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-unbind-instance',
                user_id: 'u-123',
                operator_session_id: 'os-unbind-instance',
                status: OperatorStatus.BOUND
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-unbind-instance' }, { id: 'ws-unbind-instance' })
            );
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const unbindReq = new UnbindOperatorsRequest({
                operator_ids: ['op-unbind-instance'],
                web_session_id: 'ws-unbind-instance',
                user_id: 'u-123'
            });

            const result = await service.unbindOperator(unbindReq);

            expect(result).toBeInstanceOf(UnbindOperatorsResponse);
            expect(result.success).toBe(true);
        });
    });

    describe('unbindOperators error scenarios', () => {
        it('should fail if operator not found', async () => {
            mocks.operatorService.getOperator.mockResolvedValue(null);

            const unbindReq = new UnbindOperatorsRequest({
                operator_ids: ['op-missing'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.unbindOperators(unbindReq);

            expect(result.success).toBe(false);
            expect(result.statusCode).toBe(400);
            expect(result.failed_count).toBe(1);
            expect(result.errors[0].error).toContain(DeviceLinkError.OPERATOR_NOT_FOUND);
            expect(mocks.operatorService.operatorDataService.updateOperator).not.toHaveBeenCalled();
        });

        it('should fail if operator belongs to different user', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-wrong-user',
                user_id: 'u-456',
                operator_session_id: 'os-wrong-user',
                status: OperatorStatus.BOUND
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);

            const unbindReq = new UnbindOperatorsRequest({
                operator_ids: ['op-wrong-user'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.unbindOperators(unbindReq);

            expect(result.success).toBe(false);
            expect(result.statusCode).toBe(400);
            expect(result.failed_count).toBe(1);
            expect(result.errors[0].error).toBe('Not authorized to unbind this operator');
            expect(mocks.operatorService.operatorDataService.updateOperator).not.toHaveBeenCalled();
        });

        it('should handle partial failures in multiple operator unbind', async () => {
            const op1 = new OperatorDocument({
                operator_id: 'op-unbind-1',
                user_id: 'u-123',
                operator_session_id: 'os-unbind-1',
                status: OperatorStatus.BOUND
            });

            mocks.operatorService.getOperator
                .mockResolvedValueOnce(op1)
                .mockResolvedValueOnce(null);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(op1, { id: 'os-unbind-1' }, { id: 'ws-123' })
            );
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const unbindReq = new UnbindOperatorsRequest({
                operator_ids: ['op-unbind-1', 'op-missing'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.unbindOperators(unbindReq);

            expect(result.unbound_count).toBe(1);
            expect(result.failed_count).toBe(1);
            expect(result.failed_operator_ids).toContain('op-missing');
            expect(result.statusCode).toBe(207);
        });

        it('should handle operator with no session_id in unbind', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-no-session-unbind',
                user_id: 'u-123',
                operator_session_id: null,
                status: OperatorStatus.BOUND
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(null);
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const unbindReq = new UnbindOperatorsRequest({
                operator_ids: ['op-no-session-unbind'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.unbindOperators(unbindReq);

            expect(result.success).toBe(true);
            expect(result.unbound_count).toBe(1);
            expect(result.failed_count).toBe(0);
            expect(mocks.bindingService.unbind).not.toHaveBeenCalled();
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalledWith('op-no-session-unbind', {
                status: OperatorStatus.ACTIVE,
                web_session_id: null
            });
            expect(mocks.webSessionService.unbindOperatorFromWebSession).toHaveBeenCalledWith('ws-123', 'op-no-session-unbind');
        });
    });

    describe('relay and broadcast failure handling', () => {
        it('should continue bind when relayRegisterOperatorSessionToG8ee fails', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-relay-fail',
                user_id: 'u-123',
                operator_session_id: 'os-relay-fail',
                status: OperatorStatus.ACTIVE
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-relay-fail' }, { id: 'ws-123' })
            );
            mocks.operatorService.relayRegisterOperatorSessionToG8ee.mockRejectedValue(new Error('g8ee unreachable'));
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const bindReq = new BindOperatorsRequest({
                operator_ids: ['op-relay-fail'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.bindOperators(bindReq);

            expect(result.success).toBe(true);
            expect(result.bound_count).toBe(1);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalled();
            expect(mocks.bindingService.bind).toHaveBeenCalled();
        });

        it('should continue bind when broadcastOperatorListToSession fails', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-broadcast-fail',
                user_id: 'u-123',
                operator_session_id: 'os-broadcast-fail',
                status: OperatorStatus.ACTIVE
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-broadcast-fail' }, { id: 'ws-123' })
            );
            mocks.operatorService.broadcastOperatorListToSession.mockRejectedValue(new Error('SSE down'));

            const bindReq = new BindOperatorsRequest({
                operator_ids: ['op-broadcast-fail'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.bindOperators(bindReq);

            expect(result.success).toBe(true);
            expect(result.bound_count).toBe(1);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalled();
        });

        it('should continue unbind when relayRegisterOperatorSessionToG8ee fails', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-unbind-relay-fail',
                user_id: 'u-123',
                operator_session_id: 'os-unbind-relay-fail',
                status: OperatorStatus.BOUND
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-unbind-relay-fail' }, { id: 'ws-123' })
            );
            mocks.operatorService.relayRegisterOperatorSessionToG8ee.mockRejectedValue(new Error('g8ee unreachable'));
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const unbindReq = new UnbindOperatorsRequest({
                operator_ids: ['op-unbind-relay-fail'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.unbindOperators(unbindReq);

            expect(result.success).toBe(true);
            expect(result.unbound_count).toBe(1);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalled();
            expect(mocks.bindingService.unbind).toHaveBeenCalled();
        });

        it('should continue unbind when broadcastOperatorListToSession fails', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-unbind-broadcast-fail',
                user_id: 'u-123',
                operator_session_id: 'os-unbind-broadcast-fail',
                status: OperatorStatus.BOUND
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(
                OperatorWithSessionContext.create(operator, { id: 'os-unbind-broadcast-fail' }, { id: 'ws-123' })
            );
            mocks.operatorService.broadcastOperatorListToSession.mockRejectedValue(new Error('SSE down'));

            const unbindReq = new UnbindOperatorsRequest({
                operator_ids: ['op-unbind-broadcast-fail'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.unbindOperators(unbindReq);

            expect(result.success).toBe(true);
            expect(result.unbound_count).toBe(1);
            expect(mocks.operatorService.operatorDataService.updateOperator).toHaveBeenCalled();
        });
    });

    describe('context wrapper null handling', () => {
        it('should handle null context wrapper in bind without error', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-null-context',
                user_id: 'u-123',
                operator_session_id: 'os-null-context',
                status: OperatorStatus.ACTIVE
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.bindingService.getBoundOperatorSessionIds.mockResolvedValue([]);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(null);
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const bindReq = new BindOperatorsRequest({
                operator_ids: ['op-null-context'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.bindOperators(bindReq);

            expect(result.success).toBe(true);
            expect(result.bound_count).toBe(1);
            expect(mocks.operatorService.relayRegisterOperatorSessionToG8ee).not.toHaveBeenCalled();
        });

        it('should handle null context wrapper in unbind without error', async () => {
            const operator = new OperatorDocument({
                operator_id: 'op-null-context-unbind',
                user_id: 'u-123',
                operator_session_id: 'os-null-context-unbind',
                status: OperatorStatus.BOUND
            });
            mocks.operatorService.getOperator.mockResolvedValue(operator);
            mocks.operatorService.getOperatorWithSessionContext.mockResolvedValue(null);
            mocks.operatorService.broadcastOperatorListToSession = vi.fn().mockResolvedValue();

            const unbindReq = new UnbindOperatorsRequest({
                operator_ids: ['op-null-context-unbind'],
                web_session_id: 'ws-123',
                user_id: 'u-123'
            });

            const result = await service.unbindOperators(unbindReq);

            expect(result.success).toBe(true);
            expect(result.unbound_count).toBe(1);
            expect(mocks.operatorService.relayRegisterOperatorSessionToG8ee).not.toHaveBeenCalled();
        });
    });
});

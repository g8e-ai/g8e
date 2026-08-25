// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { EventType } from '@g8ed/public/js/constants/events.js';
import { TEMPLATE_FIXTURES, seedTemplates } from '@test/fixtures/templates.fixture.js';
import { MockTemplateLoader } from '@test/mocks/mock-browser-env.js';

let TerminalExecutionMixin;
let templateLoader;
let webSessionService;
let ServiceName;
let ApiPaths;

function createMixinContext(overrides = {}) {
    const ctx = Object.create(null);
    Object.getOwnPropertyNames(TerminalExecutionMixin.prototype).forEach(methodName => {
        if (methodName !== 'constructor') {
            ctx[methodName] = TerminalExecutionMixin.prototype[methodName].bind(ctx);
        }
    });
    ctx.pendingApprovals = new Map();
    ctx.activeExecutions = new Map();
    ctx.executionResultsContainers = new Map();
    ctx.outputContainer = document.createElement('div');
    ctx.outputContainer.className = 'anchored-terminal__output';
    ctx.escapeHtml = (str) => String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
    ctx.scrollToBottom = vi.fn();
    ctx.formatTimestamp = vi.fn(() => '12:00:00');
    Object.assign(ctx, overrides);
    return ctx;
}

beforeEach(async () => {
    vi.resetModules();

    vi.doMock('@g8ed/public/js/utils/timestamp.js', () => ({
        nowISOString: vi.fn(() => '2026-01-01T12:00:00.000Z'),
    }));

    vi.doMock('@g8ed/public/js/utils/template-loader.js', () => ({
        templateLoader: new MockTemplateLoader(),
    }));

    vi.doMock('@g8ed/public/js/utils/web-session-service.js', () => ({
        webSessionService: {
            getWebSessionId: vi.fn(() => 'session_test_123'),
        },
    }));

    vi.doMock('@g8ed/public/js/constants/service-client-constants.js', () => ({
        ServiceName: { g8ed: 'g8ed' },
    }));

    vi.doMock('@g8ed/public/js/constants/api-paths.js', () => ({
        ApiPaths: {
            approval: {
                respond: () => '/api/operator/approval/respond',
            },
        },
    }));

    const mod = await import('@g8ed/public/js/components/anchored-terminal-execution.js');
    TerminalExecutionMixin = mod.TerminalExecutionMixin;

    const tlMod = await import('@g8ed/public/js/utils/template-loader.js');
    templateLoader = tlMod.templateLoader;

    const wssMod = await import('@g8ed/public/js/utils/web-session-service.js');
    webSessionService = wssMod.webSessionService;

    const scMod = await import('@g8ed/public/js/constants/service-client-constants.js');
    ServiceName = scMod.ServiceName;

    const apMod = await import('@g8ed/public/js/constants/api-paths.js');
    ApiPaths = apMod.ApiPaths;

    seedTemplates(templateLoader, [
        'executing-indicator',
        'preparing-indicator',
        'approval-card',
        'approval-card-restored',
        'approval-status',
        'command-result',
        'results-toggle',
    ]);

    window.serviceClient = {
        post: vi.fn().mockResolvedValue({
            ok: true,
            json: async () => ({ success: true }),
        }),
    };
});

afterEach(() => {
    vi.restoreAllMocks();
    delete window.serviceClient;
});

describe('TerminalExecutionMixin [UNIT - jsdom]', () => {
    describe('initExecutionState()', () => {
        it('initializes pendingApprovals as empty Map', () => {
            const ctx = createMixinContext();
            ctx.initExecutionState();
            expect(ctx.pendingApprovals).toBeInstanceOf(Map);
            expect(ctx.pendingApprovals.size).toBe(0);
        });

        it('initializes activeExecutions as empty Map', () => {
            const ctx = createMixinContext();
            ctx.initExecutionState();
            expect(ctx.activeExecutions).toBeInstanceOf(Map);
            expect(ctx.activeExecutions.size).toBe(0);
        });

        it('initializes executionResultsContainers as empty Map', () => {
            const ctx = createMixinContext();
            ctx.initExecutionState();
            expect(ctx.executionResultsContainers).toBeInstanceOf(Map);
            expect(ctx.executionResultsContainers.size).toBe(0);
        });
    });

    describe('showExecutingIndicator()', () => {
        it('returns null if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            const id = ctx.showExecutingIndicator('ls -la');
            expect(id).toBeNull();
        });

        it('creates and appends executing indicator to outputContainer', () => {
            const ctx = createMixinContext();
            const id = ctx.showExecutingIndicator('ls -la');
            
            expect(id).toBeTruthy();
            expect(id).toMatch(/^exec-\d+-\d+$/);
            
            const indicator = ctx.outputContainer.querySelector(`#${id}`);
            expect(indicator).toBeTruthy();
            expect(indicator.className).toBe('anchored-terminal__executing');
            expect(indicator.textContent).toContain('Executing: ls -la');
        });

        it('increments execCounter on each call', () => {
            const ctx = createMixinContext();
            const id1 = ctx.showExecutingIndicator('cmd1');
            const id2 = ctx.showExecutingIndicator('cmd2');
            
            const num1 = parseInt(id1.split('-')[2]);
            const num2 = parseInt(id2.split('-')[2]);
            expect(num2).toBe(num1 + 1);
        });

        it('escapes HTML in command', () => {
            const ctx = createMixinContext();
            const id = ctx.showExecutingIndicator('<script>alert("xss")</script>');
            
            const indicator = ctx.outputContainer.querySelector(`#${id}`);
            expect(indicator.innerHTML).toContain('&lt;script&gt;');
            expect(indicator.innerHTML).not.toContain('<script>');
        });

        it('calls scrollToBottom', () => {
            const ctx = createMixinContext();
            ctx.showExecutingIndicator('ls -la');
            expect(ctx.scrollToBottom).toHaveBeenCalledTimes(1);
        });
    });

    describe('showPreparingIndicator()', () => {
        it('returns null if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            const id = ctx.showPreparingIndicator('ls -la');
            expect(id).toBeNull();
        });

        it('creates and appends preparing indicator to outputContainer', () => {
            const ctx = createMixinContext();
            const id = ctx.showPreparingIndicator('ls -la');
            
            expect(id).toBeTruthy();
            expect(id).toMatch(/^exec-\d+-\d+$/);
            
            const indicator = ctx.outputContainer.querySelector(`#${id}`);
            expect(indicator).toBeTruthy();
            expect(indicator.className).toBe('anchored-terminal__executing');
            expect(indicator.textContent).toContain('Preparing: ls -la');
        });

        it('escapes HTML in command', () => {
            const ctx = createMixinContext();
            const id = ctx.showPreparingIndicator('<script>alert("xss")</script>');
            
            const indicator = ctx.outputContainer.querySelector(`#${id}`);
            expect(indicator.innerHTML).toContain('&lt;script&gt;');
        });

        it('calls scrollToBottom', () => {
            const ctx = createMixinContext();
            ctx.showPreparingIndicator('ls -la');
            expect(ctx.scrollToBottom).toHaveBeenCalledTimes(1);
        });
    });

    describe('_showExecutingIndicatorInContainer()', () => {
        it('returns null if container is not provided', () => {
            const ctx = createMixinContext();
            const id = ctx._showExecutingIndicatorInContainer(null, 'ls -la');
            expect(id).toBeNull();
        });

        it('falls back to showExecutingIndicator if container has no results-body', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            container.className = 'anchored-terminal__results-group';
            
            const showExecSpy = vi.spyOn(ctx, 'showExecutingIndicator');
            ctx._showExecutingIndicatorInContainer(container, 'ls -la');
            
            expect(showExecSpy).toHaveBeenCalledWith('ls -la');
        });

        it('creates indicator in container results-body', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            container.className = 'anchored-terminal__results-group';
            const body = document.createElement('div');
            body.className = 'anchored-terminal__results-body';
            container.appendChild(body);
            
            const id = ctx._showExecutingIndicatorInContainer(container, 'ls -la');
            
            expect(id).toBeTruthy();
            const indicator = body.querySelector(`#${id}`);
            expect(indicator).toBeTruthy();
            expect(indicator.className).toBe('anchored-terminal__executing');
        });

        it('removes collapsed class from container', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            container.className = 'anchored-terminal__results-group collapsed';
            const body = document.createElement('div');
            body.className = 'anchored-terminal__results-body';
            container.appendChild(body);
            
            ctx._showExecutingIndicatorInContainer(container, 'ls -la');
            
            expect(container.classList.contains('collapsed')).toBe(false);
        });

        it('shows toggle element if hidden', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            container.className = 'anchored-terminal__results-group';
            const body = document.createElement('div');
            body.className = 'anchored-terminal__results-body';
            container.appendChild(body);
            const toggle = document.createElement('div');
            toggle.className = 'anchored-terminal__results-toggle';
            toggle.style.display = 'none';
            container.appendChild(toggle);
            
            ctx._showExecutingIndicatorInContainer(container, 'ls -la');
            
            expect(toggle.style.display).toBe('');
        });

        it('sets toggle label to "Executing"', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            container.className = 'anchored-terminal__results-group';
            const body = document.createElement('div');
            body.className = 'anchored-terminal__results-body';
            container.appendChild(body);
            const toggle = document.createElement('div');
            toggle.className = 'anchored-terminal__results-toggle';
            container.appendChild(toggle);
            const label = document.createElement('span');
            label.className = 'anchored-terminal__results-toggle-label';
            toggle.appendChild(label);
            
            ctx._showExecutingIndicatorInContainer(container, 'ls -la');
            
            expect(label.textContent).toBe('Executing');
        });

        it('calls scrollToBottom', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            container.className = 'anchored-terminal__results-group';
            const body = document.createElement('div');
            body.className = 'anchored-terminal__results-body';
            container.appendChild(body);
            
            ctx._showExecutingIndicatorInContainer(container, 'ls -la');
            expect(ctx.scrollToBottom).toHaveBeenCalledTimes(1);
        });
    });

    describe('hideExecutingIndicator()', () => {
        it('returns early if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            expect(() => ctx.hideExecutingIndicator('exec-123')).not.toThrow();
        });

        it('removes specific indicator by id', () => {
            const ctx = createMixinContext();
            const id = ctx.showExecutingIndicator('ls -la');
            
            expect(ctx.outputContainer.querySelector(`#${id}`)).toBeTruthy();
            ctx.hideExecutingIndicator(id);
            expect(ctx.outputContainer.querySelector(`#${id}`)).toBeNull();
        });

        it('removes all executing indicators if no id provided', () => {
            const ctx = createMixinContext();
            const id1 = ctx.showExecutingIndicator('cmd1');
            const id2 = ctx.showExecutingIndicator('cmd2');
            
            expect(ctx.outputContainer.querySelectorAll('.anchored-terminal__executing')).toHaveLength(2);
            ctx.hideExecutingIndicator();
            expect(ctx.outputContainer.querySelectorAll('.anchored-terminal__executing')).toHaveLength(0);
        });

        it('does nothing if indicator id does not exist', () => {
            const ctx = createMixinContext();
            const id = ctx.showExecutingIndicator('ls');
            const indicator = ctx.outputContainer.querySelector(`#${id}`);
            
            expect(() => ctx.hideExecutingIndicator('non-existent-id')).not.toThrow();
            expect(indicator).toBeTruthy();
            expect(ctx.outputContainer.querySelector('#non-existent-id')).toBeNull();
        });
    });

    describe('handleApprovalRequest()', () => {
        it('returns early if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            expect(() => ctx.handleApprovalRequest({ command: 'ls' })).not.toThrow();
        });

        it('returns early if data is null', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest(null);
            expect(ctx.outputContainer.children.length).toBe(0);
        });

        it('removes welcome message if present', () => {
            const ctx = createMixinContext();
            const welcome = document.createElement('div');
            welcome.className = 'anchored-terminal__welcome';
            ctx.outputContainer.appendChild(welcome);
            
            ctx.handleApprovalRequest({ command: 'ls', approval_id: 'apr_1' });
            
            expect(ctx.outputContainer.querySelector('.anchored-terminal__welcome')).toBeNull();
        });

        it('hides preparing indicator for matching execution_id', () => {
            const ctx = createMixinContext();
            const indicatorId = ctx.showPreparingIndicator('ls -la');
            ctx.activeExecutions.set('exec_123', { indicatorId });
            
            ctx.handleApprovalRequest({ 
                command: 'ls -la', 
                execution_id: 'exec_123',
                approval_id: 'apr_1',
            });
            
            expect(ctx.outputContainer.querySelector(`#${indicatorId}`)).toBeNull();
            expect(ctx.activeExecutions.has('exec_123')).toBe(false);
        });

        it('stores approval data in pendingApprovals map', () => {
            const ctx = createMixinContext();
            const data = { command: 'ls', approval_id: 'apr_1', justification: 'test' };
            
            ctx.handleApprovalRequest(data);
            
            expect(ctx.pendingApprovals.get('apr_1')).toEqual(data);
        });

        it('uses execution_id as approval_id when approval_id is missing', () => {
            const ctx = createMixinContext();
            const data = { command: 'ls', execution_id: 'exec_123' };
            
            ctx.handleApprovalRequest(data);
            
            expect(ctx.pendingApprovals.has('exec_123')).toBe(true);
        });

        it('renders command approval card with LOW risk', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                command: 'ls -la',
                approval_id: 'apr_1',
                justification: 'List files',
                risk_analysis: { risk_level: 'low' },
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card).toBeTruthy();
            expect(card.dataset.approvalId).toBe('apr_1');
            expect(card.textContent).toContain('Command');
            expect(card.textContent).toContain('ls -la');
            expect(card.textContent).toContain('List files');
        });

        it('renders HIGH risk badge with warning icon', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                command: 'rm -rf /',
                approval_id: 'apr_1',
                justification: 'Dangerous',
                risk_analysis: { risk_level: 'HIGH' },
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toContain('warning');
            expect(card.textContent).toContain('HIGH');
        });

        it('renders MEDIUM risk badge with priority_high icon', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                command: 'chmod 777 /etc',
                approval_id: 'apr_1',
                justification: 'Modify permissions',
                risk_analysis: { risk_level: 'medium' },
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toContain('priority_high');
            expect(card.textContent).toContain('MEDIUM');
        });

        it('renders file edit approval card', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                file_path: '/etc/hosts',
                operation: 'edit',
                approval_id: 'apr_1',
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toContain('File Edit');
            expect(card.textContent).toContain('edit: /etc/hosts');
        });

        it('renders intent escalation approval card', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                intent_name: 'sudo_access',
                intent_question: 'Grant sudo access?',
                approval_id: 'apr_1',
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toContain('Escalation');
            expect(card.textContent).toContain('Grant sudo access?');
        });

        it('renders batch execution header with system count', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                command: 'ls -la',
                approval_id: 'apr_1',
                target_systems: [{ hostname: 'host1' }, { hostname: 'host2' }],
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toContain('Command');
        });

        it('renders target systems HTML for batch execution', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                command: 'ls -la',
                approval_id: 'apr_1',
                target_systems: [{ hostname: 'host1' }, { hostname: 'host2' }],
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card).toBeTruthy();
        });

        it('binds approve button click handler', () => {
            const ctx = createMixinContext();
            const handleResponseSpy = vi.spyOn(ctx, 'handleApprovalResponse');
            
            ctx.handleApprovalRequest({
                command: 'ls',
                approval_id: 'apr_1',
            });
            
            const approveBtn = ctx.outputContainer.querySelector('.approval-compact__btn--approve');
            approveBtn.click();
            
            expect(handleResponseSpy).toHaveBeenCalledWith('apr_1', true);
        });

        it('binds deny button click handler', () => {
            const ctx = createMixinContext();
            const handleResponseSpy = vi.spyOn(ctx, 'handleApprovalResponse');
            
            ctx.handleApprovalRequest({
                command: 'ls',
                approval_id: 'apr_1',
            });
            
            const denyBtn = ctx.outputContainer.querySelector('.approval-compact__btn--deny');
            denyBtn.click();
            
            expect(handleResponseSpy).toHaveBeenCalledWith('apr_1', false);
        });

        it('escapes HTML in command display', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                command: '<script>alert("xss")</script>',
                approval_id: 'apr_1',
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.innerHTML).toContain('&lt;script&gt;');
            expect(card.innerHTML).not.toContain('<script>');
        });

        it('escapes HTML in justification', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({
                command: 'ls',
                approval_id: 'apr_1',
                justification: '<script>alert("xss")</script>',
            });
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.innerHTML).toContain('&lt;script&gt;');
        });

        it('calls scrollToBottom', () => {
            const ctx = createMixinContext();
            ctx.handleApprovalRequest({ command: 'ls', approval_id: 'apr_1' });
            expect(ctx.scrollToBottom).toHaveBeenCalledTimes(1);
        });
    });

    describe('handleApprovalResponse()', () => {
        it('returns early if approval data not found', async () => {
            const ctx = createMixinContext();
            await ctx.handleApprovalResponse('nonexistent', true);
            expect(window.serviceClient.post).not.toHaveBeenCalled();
        });

        it('disables buttons on approval element', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const btn = document.createElement('button');
            btn.className = 'approval-compact__btn';
            approvalEl.appendChild(btn);
            ctx.outputContainer.appendChild(approvalEl);
            
            await ctx.handleApprovalResponse('apr_1', true);
            
            expect(btn.disabled).toBe(true);
        });

        it('does not call API if no active web session', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            webSessionService.getWebSessionId.mockReturnValueOnce(null);
            
            await ctx.handleApprovalResponse('apr_1', true);
            expect(window.serviceClient.post).not.toHaveBeenCalled();
        });

        it('posts approval response with correct payload', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', {
                command: 'ls',
                case_id: 'case_123',
                investigation_id: 'inv_456',
                task_id: 'task_789',
            });
            
            await ctx.handleApprovalResponse('apr_1', true);
            
            expect(window.serviceClient.post).toHaveBeenCalledWith(
                ServiceName.g8ed,
                ApiPaths.approval.respond(),
                expect.objectContaining({
                    approval_id: 'apr_1',
                    approved: true,
                    reason: 'User approved via terminal',
                    case_id: 'case_123',
                    investigation_id: 'inv_456',
                    task_id: 'task_789',
                })
            );
        });

        it('posts denial with correct reason', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            
            await ctx.handleApprovalResponse('apr_1', false);
            
            expect(window.serviceClient.post).toHaveBeenCalledWith(
                ServiceName.g8ed,
                ApiPaths.approval.respond(),
                expect.objectContaining({
                    approved: false,
                    reason: 'User denied via terminal',
                })
            );
        });

        it('shows approved status on success', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            await ctx.handleApprovalResponse('apr_1', true);
            
            const statusDiv = actionsDiv.querySelector('.approval-compact__status');
            expect(statusDiv).toBeTruthy();
            expect(statusDiv.classList.contains('approval-compact__status--approved')).toBe(true);
            expect(statusDiv.textContent).toContain('Approved');
            expect(statusDiv.textContent).toContain('check');
        });

        it('shows denied status on denial', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            await ctx.handleApprovalResponse('apr_1', false);
            
            const statusDiv = actionsDiv.querySelector('.approval-compact__status');
            expect(statusDiv.classList.contains('approval-compact__status--denied')).toBe(true);
            expect(statusDiv.textContent).toContain('Denied');
            expect(statusDiv.textContent).toContain('close');
        });

        it('creates results container on approval', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls -la' });
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            await ctx.handleApprovalResponse('apr_1', true);
            
            const container = ctx.executionResultsContainers.get('apr_1');
            expect(container).toBeTruthy();
            expect(container.className).toBe('anchored-terminal__results-group');
        });

        it('shows executing indicator in results container on approval', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls -la' });
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            await ctx.handleApprovalResponse('apr_1', true);
            
            const container = ctx.executionResultsContainers.get('apr_1');
            const indicator = container.querySelector('.anchored-terminal__executing');
            expect(indicator).toBeTruthy();
            expect(indicator.textContent).toContain('Executing: ls -la');
        });

        it('removes approval from pendingApprovals on success', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            await ctx.handleApprovalResponse('apr_1', true);
            
            expect(ctx.pendingApprovals.has('apr_1')).toBe(false);
        });

        it('re-enables buttons on error', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            window.serviceClient.post.mockRejectedValueOnce(new Error('Network error'));
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const btn = document.createElement('button');
            btn.className = 'approval-compact__btn';
            btn.disabled = true;
            approvalEl.appendChild(btn);
            ctx.outputContainer.appendChild(approvalEl);
            
            await ctx.handleApprovalResponse('apr_1', true);
            
            expect(btn.disabled).toBe(false);
        });

        it('logs error on failure', async () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            window.serviceClient.post.mockRejectedValueOnce(new Error('Network error'));
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            await ctx.handleApprovalResponse('apr_1', true);
            
            expect(consoleSpy).toHaveBeenCalledWith(
                '[ANCHORED TERMINAL] Approval response failed:',
                expect.any(Error)
            );
            consoleSpy.mockRestore();
        });
    });

    describe('_buildRiskBadgeHtml()', () => {
        it('returns empty string if riskAnalysis is null', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml(null);
            expect(html).toBe('');
        });

        it('returns empty string if riskAnalysis.risk_level is missing', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({});
            expect(html).toBe('');
        });

        it('renders LOW risk badge with check_circle icon', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({ risk_level: 'low' });
            expect(html).toContain('check_circle');
            expect(html).toContain('LOW');
            expect(html).toContain('operator-terminal__risk-badge--low');
        });

        it('renders MEDIUM risk badge with priority_high icon', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({ risk_level: 'medium' });
            expect(html).toContain('priority_high');
            expect(html).toContain('MEDIUM');
            expect(html).toContain('operator-terminal__risk-badge--medium');
        });

        it('renders HIGH risk badge with warning icon', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({ risk_level: 'high' });
            expect(html).toContain('warning');
            expect(html).toContain('HIGH');
            expect(html).toContain('operator-terminal__risk-badge--high');
        });

        it('renders UNKNOWN risk badge with info icon', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({ risk_level: 'unknown' });
            expect(html).toContain('info');
            expect(html).toContain('UNKNOWN');
        });

        it('includes risk score in tooltip when provided', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({ risk_level: 'high', risk_score: 8 });
            expect(html).toContain('Score: 8/10');
        });

        it('includes destructive operation in tooltip when true', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({ risk_level: 'high', is_destructive: true });
            expect(html).toContain('Destructive operation');
        });

        it('includes blast radius in tooltip when provided', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({ risk_level: 'high', blast_radius: 'entire system' });
            expect(html).toContain('Blast radius: entire system');
        });

        it('combines multiple tooltip parts with pipe separator', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({
                risk_level: 'high',
                risk_score: 9,
                is_destructive: true,
                blast_radius: '/etc',
            });
            expect(html).toContain('Score: 9/10 | Destructive operation | Blast radius: /etc');
        });

        it('escapes HTML in tooltip values', () => {
            const ctx = createMixinContext();
            const html = ctx._buildRiskBadgeHtml({
                risk_level: 'high',
                blast_radius: '<script>alert("xss")</script>',
            });
            expect(html).toContain('&lt;script&gt;');
        });
    });

    describe('_buildTargetSystemsHtml()', () => {
        it('returns empty string if targetSystems is null', () => {
            const ctx = createMixinContext();
            const html = ctx._buildTargetSystemsHtml(null);
            expect(html).toBe('');
        });

        it('returns empty string if targetSystems is empty array', () => {
            const ctx = createMixinContext();
            const html = ctx._buildTargetSystemsHtml([]);
            expect(html).toBe('');
        });

        it('renders system list with hostnames', () => {
            const ctx = createMixinContext();
            const html = ctx._buildTargetSystemsHtml([
                { hostname: 'host1', operator_type: 'system' },
                { hostname: 'host2', operator_type: 'system' },
            ]);
            expect(html).toContain('host1');
            expect(html).toContain('host2');
        });

        it('renders cloud operator type with cloud icon', () => {
            const ctx = createMixinContext();
            const html = ctx._buildTargetSystemsHtml([
                { hostname: 'cloud-host', operator_type: 'cloud' },
            ]);
            expect(html).toContain('cloud');
        });

        it('renders system operator type with computer icon', () => {
            const ctx = createMixinContext();
            const html = ctx._buildTargetSystemsHtml([
                { hostname: 'linux-host', operator_type: 'system' },
            ]);
            expect(html).toContain('computer');
            expect(html).toContain('system');
        });

        it('shows "unknown" hostname when missing', () => {
            const ctx = createMixinContext();
            const html = ctx._buildTargetSystemsHtml([
                { operator_type: 'system' },
            ]);
            expect(html).toContain('unknown');
        });

        it('escapes HTML in hostname', () => {
            const ctx = createMixinContext();
            const html = ctx._buildTargetSystemsHtml([
                { hostname: '<script>alert("xss")</script>', operator_type: 'system' },
            ]);
            expect(html).toContain('&lt;script&gt;');
        });

        it('shows system count in header', () => {
            const ctx = createMixinContext();
            const html = ctx._buildTargetSystemsHtml([
                { hostname: 'host1', operator_type: 'system' },
                { hostname: 'host2', operator_type: 'system' },
                { hostname: 'host3', operator_type: 'system' },
            ]);
            expect(html).toContain('Impacted Systems (3)');
        });
    });

    describe('_createResultsContainer()', () => {
        it('returns null if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            const container = ctx._createResultsContainer('exec_123');
            expect(container).toBeNull();
        });

        it('returns existing container if already created', () => {
            const ctx = createMixinContext();
            const existing = document.createElement('div');
            ctx.executionResultsContainers.set('exec_123', existing);
            
            const container = ctx._createResultsContainer('exec_123');
            expect(container).toBe(existing);
        });

        it('creates new container with correct class and dataset', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            expect(container).toBeTruthy();
            expect(container.className).toBe('anchored-terminal__results-group');
            expect(container.dataset.executionId).toBe('exec_123');
        });

        it('creates toggle element', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            const toggle = container.querySelector('.anchored-terminal__results-toggle');
            expect(toggle).toBeTruthy();
            expect(toggle.style.display).toBe('none');
        });

        it('creates results body element', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            const body = container.querySelector('.anchored-terminal__results-body');
            expect(body).toBeTruthy();
        });

        it('appends container to outputContainer', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            expect(ctx.outputContainer.contains(container)).toBe(true);
        });

        it('inserts container after approvalEl if provided', () => {
            const ctx = createMixinContext();
            const approvalEl = document.createElement('div');
            approvalEl.className = 'approval';
            ctx.outputContainer.appendChild(approvalEl);
            
            const container = ctx._createResultsContainer('exec_123', approvalEl);
            
            expect(approvalEl.nextSibling).toBe(container);
        });

        it('stores container in executionResultsContainers map', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            expect(ctx.executionResultsContainers.get('exec_123')).toBe(container);
        });

        it('binds toggle click handler to collapse/expand', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            const toggle = container.querySelector('.anchored-terminal__results-toggle');
            
            expect(container.classList.contains('collapsed')).toBe(false);
            toggle.click();
            expect(container.classList.contains('collapsed')).toBe(true);
            toggle.click();
            expect(container.classList.contains('collapsed')).toBe(false);
        });
    });

    describe('_appendResultToContainer()', () => {
        it('returns early if container is null', () => {
            const ctx = createMixinContext();
            expect(() => ctx._appendResultToContainer(null, {})).not.toThrow();
        });

        it('returns early if container has no results-body', () => {
            const ctx = createMixinContext();
            const container = document.createElement('div');
            expect(() => ctx._appendResultToContainer(container, {})).not.toThrow();
        });

        it('appends result entry to body', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                stdout: 'file1\nfile2',
                exitCode: 0,
                status: EventType.OPERATOR_COMMAND_COMPLETED,
                timestamp: '2026-01-01T12:00:00.000Z',
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry).toBeTruthy();
        });

        it('renders success status for completed event', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.innerHTML).toContain('success');
            expect(entry.innerHTML).toContain('check_circle');
        });

        it('renders error status for failed event', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                status: EventType.OPERATOR_COMMAND_FAILED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.innerHTML).toContain('error');
            expect(entry.innerHTML).toContain('error');
        });

        it('renders stdout output', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                stdout: 'file1\nfile2',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.textContent).toContain('file1');
            expect(entry.textContent).toContain('file2');
        });

        it('renders stderr output', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                stderr: 'Permission denied',
                status: EventType.OPERATOR_COMMAND_FAILED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.textContent).toContain('Permission denied');
        });

        it('combines stdout and stderr with newline separator', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                stdout: 'stdout output',
                stderr: 'stderr output',
                status: EventType.OPERATOR_COMMAND_FAILED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            const output = entry.querySelector('.anchored-terminal__result-output');
            expect(output.textContent).toContain('stdout output');
            expect(output.textContent).toContain('stderr output');
        });

        it('shows "(No output)" when stdout and stderr are empty', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.textContent).toContain('(No output)');
        });

        it('renders exit code when provided', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                exitCode: 1,
                status: EventType.OPERATOR_COMMAND_FAILED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.textContent).toContain('Exit code: 1');
        });

        it('renders success exit code style for exit code 0', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                exitCode: 0,
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.innerHTML).toContain('anchored-terminal__result-exit--success');
        });

        it('renders error exit code style for non-zero exit code', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                exitCode: 1,
                status: EventType.OPERATOR_COMMAND_FAILED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.innerHTML).toContain('anchored-terminal__result-exit--error');
        });

        it('renders hostname when provided', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                hostname: 'test-host',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.textContent).toContain('test-host');
            expect(entry.innerHTML).toContain('computer');
        });

        it('escapes HTML in command', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: '<script>alert("xss")</script>',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.innerHTML).toContain('&lt;script&gt;');
        });

        it('escapes HTML in stdout', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                stdout: '<script>alert("xss")</script>',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.innerHTML).toContain('&lt;script&gt;');
        });

        it('uses formatTimestamp when timestamp is not provided', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            expect(ctx.formatTimestamp).toHaveBeenCalled();
        });

        it('formats timestamp from ISO string when provided', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                timestamp: '2026-01-01T14:30:45.000Z',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const entry = container.querySelector('.anchored-terminal__result-entry');
            expect(entry.textContent).toMatch(/\d{2}:\d{2}:\d{2}/);
        });

        it('updates results count', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const countEl = container.querySelector('.anchored-terminal__results-count');
            expect(countEl).toBeTruthy();
            expect(countEl.textContent).toBe('1');
        });

        it('shows toggle when result is added', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            const toggle = container.querySelector('.anchored-terminal__results-toggle');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            expect(toggle.style.display).toBe('');
        });

        it('sets toggle label to "Result" for single result', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const labelEl = container.querySelector('.anchored-terminal__results-toggle-label');
            expect(labelEl.textContent).toBe('Result');
        });

        it('sets toggle label to "Results" for multiple results', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('exec_123');
            
            ctx._appendResultToContainer(container, {
                command: 'ls',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            ctx._appendResultToContainer(container, {
                command: 'pwd',
                status: EventType.OPERATOR_COMMAND_COMPLETED,
            });
            
            const labelEl = container.querySelector('.anchored-terminal__results-toggle-label');
            expect(labelEl.textContent).toBe('Results');
        });
    });

    describe('handleCommandExecutionEvent()', () => {
        it('returns early if data is null', () => {
            const ctx = createMixinContext();
            expect(() => ctx.handleCommandExecutionEvent(null)).not.toThrow();
        });

        it('returns early if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            expect(() => ctx.handleCommandExecutionEvent({})).not.toThrow();
        });

        it('handles OPERATOR_COMMAND_APPROVAL_PREPARING event', () => {
            const ctx = createMixinContext();
            const showPrepSpy = vi.spyOn(ctx, 'showPreparingIndicator');
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
                command: 'ls -la',
                execution_id: 'exec_123',
            });
            
            expect(showPrepSpy).toHaveBeenCalledWith('ls -la');
            expect(ctx.activeExecutions.has('exec_123')).toBe(true);
        });

        it('does not duplicate preparing indicator for same execution_id', () => {
            const ctx = createMixinContext();
            const showPrepSpy = vi.spyOn(ctx, 'showPreparingIndicator');
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
                command: 'ls -la',
                execution_id: 'exec_123',
            });
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_APPROVAL_PREPARING,
                command: 'ls -la',
                execution_id: 'exec_123',
            });
            
            expect(showPrepSpy).toHaveBeenCalledTimes(1);
        });

        it('handles OPERATOR_COMMAND_STARTED event with existing execution', () => {
            const ctx = createMixinContext();
            const indicatorId = ctx.showPreparingIndicator('ls -la');
            ctx.activeExecutions.set('exec_123', { indicatorId });
            
            const hideSpy = vi.spyOn(ctx, 'hideExecutingIndicator');
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_STARTED,
                command: 'ls -la',
                execution_id: 'exec_123',
            });
            
            expect(hideSpy).toHaveBeenCalledWith(indicatorId);
        });

        it('handles OPERATOR_COMMAND_STARTED event with existing container', () => {
            const ctx = createMixinContext();
            const container = ctx._createResultsContainer('apr_1');
            ctx.executionResultsContainers.set('apr_1', container);
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_STARTED,
                command: 'ls -la',
                execution_id: 'exec_123',
                approval_id: 'apr_1',
            });
            
            const indicator = container.querySelector('.anchored-terminal__executing');
            expect(indicator).toBeTruthy();
        });

        it('handles OPERATOR_COMMAND_STARTED event without existing container', () => {
            const ctx = createMixinContext();
            const showExecSpy = vi.spyOn(ctx, 'showExecutingIndicator');
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_STARTED,
                command: 'ls -la',
                execution_id: 'exec_123',
            });
            
            expect(showExecSpy).toHaveBeenCalledWith('ls -la');
        });

        it('handles OPERATOR_COMMAND_COMPLETED event', () => {
            const ctx = createMixinContext();
            const indicatorId = ctx.showExecutingIndicator('ls -la');
            ctx.activeExecutions.set('exec_123', { indicatorId });
            
            const hideSpy = vi.spyOn(ctx, 'hideExecutingIndicator');
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_COMPLETED,
                command: 'ls -la',
                execution_id: 'exec_123',
                output: 'file1\nfile2',
                return_code: 0,
                timestamp: '2026-01-01T12:00:00.000Z',
            });
            
            expect(hideSpy).toHaveBeenCalledWith(indicatorId);
            expect(ctx.activeExecutions.has('exec_123')).toBe(false);
        });

        it('creates results container for final event when none exists', () => {
            const ctx = createMixinContext();
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_COMPLETED,
                command: 'ls -la',
                execution_id: 'exec_123',
                output: 'file1',
                return_code: 0,
            });
            
            expect(ctx.executionResultsContainers.has('exec_123')).toBe(true);
        });

        it('logs error when final event has no execution_id or approval_id', () => {
            const ctx = createMixinContext();
            const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_COMPLETED,
                command: 'ls -la',
                output: 'file1',
            });
            
            expect(consoleSpy).toHaveBeenCalledWith(
                '[TERMINAL] Received final command event with no execution_id or approval_id — cannot render result',
                expect.any(Object)
            );
            consoleSpy.mockRestore();
        });

        it('handles OPERATOR_COMMAND_FAILED event', () => {
            const ctx = createMixinContext();
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_FAILED,
                command: 'ls',
                execution_id: 'exec_123',
                error: 'Permission denied',
                return_code: 1,
            });
            
            const container = ctx.executionResultsContainers.get('exec_123');
            expect(container).toBeTruthy();
            expect(container.textContent).toContain('Permission denied');
        });

        it('handles OPERATOR_COMMAND_CANCELLED event', () => {
            const ctx = createMixinContext();
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_CANCELLED,
                command: 'ls',
                execution_id: 'exec_123',
            });
            
            expect(ctx.executionResultsContainers.has('exec_123')).toBe(true);
        });

        it('handles file edit completed event', () => {
            const ctx = createMixinContext();
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_FILE_EDIT_COMPLETED,
                command: 'edit /etc/hosts',
                execution_id: 'exec_123',
                output: 'File edited successfully',
            });
            
            expect(ctx.executionResultsContainers.has('exec_123')).toBe(true);
        });

        it('extracts output from execution_result when output is missing', () => {
            const ctx = createMixinContext();
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_COMPLETED,
                command: 'ls',
                execution_id: 'exec_123',
                execution_result: {
                    output: 'from execution_result',
                    exit_code: 0,
                },
            });
            
            const container = ctx.executionResultsContainers.get('exec_123');
            expect(container.textContent).toContain('from execution_result');
        });

        it('extracts stderr from execution_result when error is missing', () => {
            const ctx = createMixinContext();
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_FAILED,
                command: 'ls',
                execution_id: 'exec_123',
                execution_result: {
                    stderr: 'from execution_result stderr',
                    exit_code: 1,
                },
            });
            
            const container = ctx.executionResultsContainers.get('exec_123');
            expect(container.textContent).toContain('from execution_result stderr');
        });

        it('prioritizes output over execution_result.output', () => {
            const ctx = createMixinContext();
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_COMPLETED,
                command: 'ls',
                execution_id: 'exec_123',
                output: 'direct output',
                execution_result: {
                    output: 'execution_result output',
                    exit_code: 0,
                },
            });
            
            const container = ctx.executionResultsContainers.get('exec_123');
            expect(container.textContent).toContain('direct output');
            expect(container.textContent).not.toContain('execution_result output');
        });

        it('calls scrollToBottom', () => {
            const ctx = createMixinContext();
            
            ctx.handleCommandExecutionEvent({
                eventType: EventType.OPERATOR_COMMAND_COMPLETED,
                command: 'ls',
                execution_id: 'exec_123',
                output: 'file1',
            });
            
            expect(ctx.scrollToBottom).toHaveBeenCalled();
        });
    });

    describe('handleIntentResult()', () => {
        it('returns early if data is null', () => {
            const ctx = createMixinContext();
            expect(() => ctx.handleIntentResult(null)).not.toThrow();
        });

        it('returns early if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            expect(() => ctx.handleIntentResult({})).not.toThrow();
        });

        it('logs error when approval_id and execution_id are both missing', () => {
            const ctx = createMixinContext();
            const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            
            ctx.handleIntentResult({ intent_name: 'sudo' });
            
            expect(consoleSpy).toHaveBeenCalledWith(
                '[TERMINAL] Received intent result with no approval_id or execution_id — cannot render result',
                expect.any(Object)
            );
            consoleSpy.mockRestore();
        });

        it('creates results container for granted intent', () => {
            const ctx = createMixinContext();
            
            ctx.handleIntentResult({
                intent_name: 'sudo_access',
                granted: true,
                approval_id: 'apr_123',
            });
            
            expect(ctx.executionResultsContainers.has('apr_123')).toBe(true);
        });

        it('renders granted permission message', () => {
            const ctx = createMixinContext();
            
            ctx.handleIntentResult({
                intent_name: 'sudo_access',
                granted: true,
                approval_id: 'apr_123',
            });
            
            const container = ctx.executionResultsContainers.get('apr_123');
            expect(container.textContent).toContain('Permission granted');
        });

        it('renders denied permission message', () => {
            const ctx = createMixinContext();
            
            ctx.handleIntentResult({
                intent_name: 'sudo_access',
                granted: false,
                approval_id: 'apr_123',
            });
            
            const container = ctx.executionResultsContainers.get('apr_123');
            expect(container.textContent).toContain('Permission denied');
        });

        it('uses OPERATOR_INTENT_GRANTED event type for granted status', () => {
            const ctx = createMixinContext();
            
            ctx.handleIntentResult({
                intent_name: 'sudo_access',
                eventType: EventType.OPERATOR_INTENT_GRANTED,
                approval_id: 'apr_123',
            });
            
            const container = ctx.executionResultsContainers.get('apr_123');
            expect(container.textContent).toContain('Permission granted');
        });

        it('uses OPERATOR_INTENT_APPROVAL_GRANTED event type for granted status', () => {
            const ctx = createMixinContext();
            
            ctx.handleIntentResult({
                intent_name: 'sudo_access',
                eventType: EventType.OPERATOR_INTENT_APPROVAL_GRANTED,
                approval_id: 'apr_123',
            });
            
            const container = ctx.executionResultsContainers.get('apr_123');
            expect(container.textContent).toContain('Permission granted');
        });

        it('defaults intent name to "permission" when missing', () => {
            const ctx = createMixinContext();
            
            ctx.handleIntentResult({
                granted: true,
                approval_id: 'apr_123',
            });
            
            const container = ctx.executionResultsContainers.get('apr_123');
            expect(container.textContent).toContain('Permission: permission');
        });

        it('sets exit code 0 for granted permission', () => {
            const ctx = createMixinContext();
            
            ctx.handleIntentResult({
                intent_name: 'sudo_access',
                granted: true,
                approval_id: 'apr_123',
            });
            
            const container = ctx.executionResultsContainers.get('apr_123');
            expect(container.textContent).toContain('Exit code: 0');
        });

        it('sets exit code 1 for denied permission', () => {
            const ctx = createMixinContext();
            
            ctx.handleIntentResult({
                intent_name: 'sudo_access',
                granted: false,
                approval_id: 'apr_123',
            });
            
            const container = ctx.executionResultsContainers.get('apr_123');
            expect(container.textContent).toContain('Exit code: 1');
        });
    });

    describe('restoreCommandExecution()', () => {
        it('returns early if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            expect(() => ctx.restoreCommandExecution({})).not.toThrow();
        });

        it('returns early if data is null', () => {
            const ctx = createMixinContext();
            ctx.restoreCommandExecution(null);
            expect(ctx.outputContainer.children.length).toBe(0);
        });

        it('removes welcome message if present', () => {
            const ctx = createMixinContext();
            const welcome = document.createElement('div');
            welcome.className = 'anchored-terminal__welcome';
            ctx.outputContainer.appendChild(welcome);
            
            ctx.restoreCommandExecution({
                command: 'ls',
                execution_id: 'exec_123',
            });
            
            expect(ctx.outputContainer.querySelector('.anchored-terminal__welcome')).toBeNull();
        });

        it('logs error when execution_id is missing', () => {
            const ctx = createMixinContext();
            const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            
            ctx.restoreCommandExecution({ command: 'ls' });
            
            expect(consoleSpy).toHaveBeenCalledWith(
                '[TERMINAL] restoreCommandExecution called with no execution_id — cannot restore result',
                expect.any(Object)
            );
            consoleSpy.mockRestore();
        });

        it('creates results container', () => {
            const ctx = createMixinContext();
            
            ctx.restoreCommandExecution({
                command: 'ls',
                execution_id: 'exec_123',
                content: 'Output:\nfile1\nfile2',
            });
            
            expect(ctx.executionResultsContainers.has('exec_123')).toBe(true);
        });

        it('extracts output from content', () => {
            const ctx = createMixinContext();
            
            ctx.restoreCommandExecution({
                command: 'ls',
                execution_id: 'exec_123',
                content: 'Output:\nfile1\nfile2',
            });
            
            const container = ctx.executionResultsContainers.get('exec_123');
            expect(container.textContent).toContain('file1');
            expect(container.textContent).toContain('file2');
        });

        it('renders status from data', () => {
            const ctx = createMixinContext();
            
            ctx.restoreCommandExecution({
                command: 'ls',
                execution_id: 'exec_123',
                content: 'Output:\nfile1',
                status: 'completed',
            });
            
            const container = ctx.executionResultsContainers.get('exec_123');
            expect(container.innerHTML).toContain('error');
        });

        it('renders exit code when provided', () => {
            const ctx = createMixinContext();
            
            ctx.restoreCommandExecution({
                command: 'ls',
                execution_id: 'exec_123',
                content: 'Output:\nfile1',
                exit_code: 0,
            });
            
            const container = ctx.executionResultsContainers.get('exec_123');
            expect(container.textContent).toContain('Exit code: 0');
        });

        it('renders hostname when provided', () => {
            const ctx = createMixinContext();
            
            ctx.restoreCommandExecution({
                command: 'ls',
                execution_id: 'exec_123',
                content: 'Output:\nfile1',
                hostname: 'test-host',
            });
            
            const container = ctx.executionResultsContainers.get('exec_123');
            expect(container.textContent).toContain('test-host');
        });
    });

    describe('_extractOutputFromContent()', () => {
        it('returns "(No output)" when content is null', () => {
            const ctx = createMixinContext();
            const result = ctx._extractOutputFromContent(null, 'ls');
            expect(result).toBe('(No output)');
        });

        it('extracts output after "Output:" label', () => {
            const ctx = createMixinContext();
            const result = ctx._extractOutputFromContent('Output:\nfile1\nfile2', 'ls');
            expect(result).toBe('file1\nfile2');
        });

        it('extracts error after "Error:" label', () => {
            const ctx = createMixinContext();
            const result = ctx._extractOutputFromContent('Error:\nPermission denied', 'ls');
            expect(result).toBe('Permission denied');
        });

        it('trims whitespace from extracted output', () => {
            const ctx = createMixinContext();
            const result = ctx._extractOutputFromContent('Output:\n  file1  \n  file2  ', 'ls');
            expect(result).toBe('file1  \n  file2');
        });

        it('returns "(No output)" when extracted output is empty', () => {
            const ctx = createMixinContext();
            const result = ctx._extractOutputFromContent('Output:\n   ', 'ls');
            expect(result).toBe('(No output)');
        });

        it('removes "Command: " prefix when present', () => {
            const ctx = createMixinContext();
            const result = ctx._extractOutputFromContent('Command: ls\nfile1\nfile2', 'ls');
            expect(result).toBe('file1\nfile2');
        });

        it('returns content as-is when no pattern matches', () => {
            const ctx = createMixinContext();
            const result = ctx._extractOutputFromContent('file1\nfile2', 'ls');
            expect(result).toBe('file1\nfile2');
        });
    });

    describe('restoreApprovalRequest()', () => {
        it('returns null if outputContainer is not set', () => {
            const ctx = createMixinContext();
            ctx.outputContainer = null;
            const result = ctx.restoreApprovalRequest({}, true, 'exec_123');
            expect(result).toBeNull();
        });

        it('returns null if data is null', () => {
            const ctx = createMixinContext();
            const result = ctx.restoreApprovalRequest(null, true, 'exec_123');
            expect(result).toBeNull();
        });

        it('removes welcome message if present', () => {
            const ctx = createMixinContext();
            const welcome = document.createElement('div');
            welcome.className = 'anchored-terminal__welcome';
            ctx.outputContainer.appendChild(welcome);
            
            ctx.restoreApprovalRequest({ command: 'ls' }, true, 'exec_123');
            
            expect(ctx.outputContainer.querySelector('.anchored-terminal__welcome')).toBeNull();
        });

        it('renders restored approval card with approved status', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                command: 'ls -la',
                justification: 'List files',
            }, true, 'exec_123');
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card).toBeTruthy();
            expect(card.classList.contains('restored')).toBe(true);
            expect(card.textContent).toContain('Approved');
            expect(card.textContent).toContain('check');
        });

        it('renders restored approval card with denied status', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                command: 'ls -la',
                justification: 'List files',
            }, false, 'exec_123');
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toContain('Denied');
            expect(card.textContent).toContain('close');
        });

        it('renders file edit approval card', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                file_path: '/etc/hosts',
                operation: 'edit',
                justification: 'Update hosts',
            }, true, 'exec_123');
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toContain('File Edit');
            expect(card.textContent).toContain('edit: /etc/hosts');
        });

        it('renders intent escalation approval card', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                intent_name: 'sudo_access',
                intent_question: 'Grant sudo?',
                justification: 'Need admin',
            }, true, 'exec_123');
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toContain('Escalation');
            expect(card.textContent).toContain('Grant sudo?');
        });

        it('renders timestamp when provided', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                command: 'ls',
                timestamp: '2026-01-01T12:00:00.000Z',
            }, true, 'exec_123');
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.textContent).toMatch(/\d{2}:\d{2}:\d{2}/);
        });

        it('sets dataset.approvalId when executionId is provided', () => {
            const ctx = createMixinContext();
            
            const entry = ctx.restoreApprovalRequest({
                command: 'ls',
            }, true, 'exec_123');
            
            expect(entry.dataset.approvalId).toBe('exec_123');
        });

        it('creates results container when approved and executionId provided', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                command: 'ls',
            }, true, 'exec_123');
            
            expect(ctx.executionResultsContainers.has('exec_123')).toBe(true);
        });

        it('does not create results container when denied', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                command: 'ls',
            }, false, 'exec_123');
            
            expect(ctx.executionResultsContainers.has('exec_123')).toBe(false);
        });

        it('escapes HTML in command display', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                command: '<script>alert("xss")</script>',
            }, true, 'exec_123');
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.innerHTML).toContain('&lt;script&gt;');
        });

        it('escapes HTML in justification', () => {
            const ctx = createMixinContext();
            
            ctx.restoreApprovalRequest({
                command: 'ls',
                justification: '<script>alert("xss")</script>',
            }, true, 'exec_123');
            
            const card = ctx.outputContainer.querySelector('.anchored-terminal__approval');
            expect(card.innerHTML).toContain('&lt;script&gt;');
        });

        it('returns the approval element', () => {
            const ctx = createMixinContext();
            
            const entry = ctx.restoreApprovalRequest({
                command: 'ls',
            }, true, 'exec_123');
            
            expect(entry).toBeTruthy();
            expect(entry.className).toBe('anchored-terminal__approval restored');
        });
    });

    describe('denyAllPendingApprovals()', () => {
        it('returns early if no pending approvals', () => {
            const ctx = createMixinContext();
            expect(() => ctx.denyAllPendingApprovals('Session closed')).not.toThrow();
            expect(window.serviceClient.post).not.toHaveBeenCalled();
        });

        it('denies all pending approvals via API', () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls', case_id: 'case_1' });
            ctx.pendingApprovals.set('apr_2', { command: 'pwd', case_id: 'case_2' });
            
            ctx.denyAllPendingApprovals('Session closed');
            
            expect(window.serviceClient.post).toHaveBeenCalledTimes(2);
        });

        it('includes correct payload in denial request', () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', {
                command: 'ls',
                case_id: 'case_123',
                investigation_id: 'inv_456',
                task_id: 'task_789',
            });
            
            ctx.denyAllPendingApprovals('User logged out');
            
            expect(window.serviceClient.post).toHaveBeenCalledWith(
                ServiceName.g8ed,
                ApiPaths.approval.respond(),
                expect.objectContaining({
                    approval_id: 'apr_1',
                    approved: false,
                    reason: 'User logged out',
                    case_id: 'case_123',
                    investigation_id: 'inv_456',
                    task_id: 'task_789',
                })
            );
        });

        it('updates approval UI to show denied status', () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            ctx.denyAllPendingApprovals('Session closed');
            
            const statusDiv = actionsDiv.querySelector('.approval-compact__status');
            expect(statusDiv).toBeTruthy();
            expect(statusDiv.classList.contains('approval-compact__status--denied')).toBe(true);
            expect(statusDiv.textContent).toContain('Cancelled');
        });

        it('uses custom status message when provided', () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            ctx.denyAllPendingApprovals('User left', 'User left');
            
            const statusDiv = actionsDiv.querySelector('.approval-compact__status');
            expect(statusDiv.textContent).toContain('User left');
        });

        it('clears pendingApprovals after denying', () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            ctx.pendingApprovals.set('apr_2', { command: 'pwd' });
            
            ctx.denyAllPendingApprovals('Session closed');
            
            expect(ctx.pendingApprovals.size).toBe(0);
        });

        it('logs denial count', () => {
            const ctx = createMixinContext();
            const consoleSpy = vi.spyOn(console, 'log').mockImplementation(() => {});
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            ctx.pendingApprovals.set('apr_2', { command: 'pwd' });
            
            ctx.denyAllPendingApprovals('Session closed');
            
            expect(consoleSpy).toHaveBeenCalledWith(
                '[TERMINAL] Denied 2 pending approval(s) - Session closed'
            );
            consoleSpy.mockRestore();
        });

        it('handles API errors gracefully', () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            window.serviceClient.post.mockRejectedValueOnce(new Error('Network error'));
            const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
            
            const approvalEl = document.createElement('div');
            approvalEl.className = 'anchored-terminal__approval';
            approvalEl.dataset.approvalId = 'apr_1';
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'approval-compact__actions';
            approvalEl.appendChild(actionsDiv);
            ctx.outputContainer.appendChild(approvalEl);
            
            ctx.denyAllPendingApprovals('Session closed');
            
            expect(ctx.pendingApprovals.size).toBe(0);
            consoleSpy.mockRestore();
        });

        it('does not call API when webSessionId is null', () => {
            const ctx = createMixinContext();
            ctx.pendingApprovals.set('apr_1', { command: 'ls' });
            webSessionService.getWebSessionId.mockReturnValueOnce(null);
            
            ctx.denyAllPendingApprovals('Session closed');
            
            expect(window.serviceClient.post).not.toHaveBeenCalled();
        });
    });
});

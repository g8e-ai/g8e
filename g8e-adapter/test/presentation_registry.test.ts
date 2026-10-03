// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';

import { normalizeGatewayEvent } from '../src/sse/normalizer';
import type { NarrativeRow } from '../src/state/stores';
import {
  authViewState,
  boundString,
  connectionViewState,
  escapeHtml,
  presentAgent,
  presentEvent,
  presentEval,
  presentMeasurement,
  presentNarrativeRow,
  presentOverview,
  presentRun,
  VIEW_STATES,
} from '../src/presentation/registry';
import type {
  AgentStateProjection,
  EvalSummary,
  RunSummary,
} from '../src/types/observe';

function agentEvent(id: number, status = 'running'): string {
  return JSON.stringify({
    id,
    event: JSON.stringify({
      type: 'app.agent.status.updated',
      data: {
        schema_version: '1.0.0',
        agent_id: 'u:sage',
        display_name: 'Sage',
        role: 'sage',
        status,
        observed_at: '2026-01-01T00:00:00Z',
      },
    }),
  });
}

function runEvent(id: number): string {
  return JSON.stringify({
    id,
    event: JSON.stringify({
      type: 'app.run.status.updated',
      data: {
        schema_version: '1.0.0',
        run_id: 'inv-1',
        run_kind: 'investigation',
        display_name: 'Case',
        status: 'running',
        completed_tasks: 0,
        total_tasks: 0,
        observed_at: '2026-01-01T00:00:00Z',
      },
    }),
  });
}

function evalCompletedEvent(id: number): string {
  return JSON.stringify({
    id,
    event: JSON.stringify({
      type: 'ai.eval.run.completed',
      data: {
        schema_version: '1.0.0',
        run_id: 'eval-1',
        suite_id: 'suite-1',
        arm_id: 'arm-1',
        terminal_attempts: 1,
        assigned_tasks: 1,
        receipt_count: 1,
        verification_status: 'verified',
        published_projection_sha256: 'abc',
        completed_at: '2026-01-01T00:00:00Z',
      },
    }),
  });
}

function evalMetricEvent(id: number): string {
  return JSON.stringify({
    id,
    event: JSON.stringify({
      type: 'ai.eval.metric.recorded',
      data: {
        schema_version: '1.0.0',
        run_id: 'eval-1',
        metric_id: 'accuracy',
        metric_version: '1.0.0',
        unit: 'score',
        eligible: 10,
        denominator: 10,
        verification_status: 'verified',
        recorded_at: '2026-01-01T00:00:00Z',
      },
    }),
  });
}

describe('escapeHtml', () => {
  it('escapes ampersands, angle brackets, and quotes', () => {
    expect(escapeHtml('<script>alert("x")</script>')).toBe('\x26lt;script\x26gt;alert(\x26quot;x\x26quot;)\x26lt;/script\x26gt;');
  });

  it('escapes single quotes', () => {
    expect(escapeHtml("it's")).toBe("it&#39;s");
  });

  it('passes through safe text', () => {
    expect(escapeHtml('Sage running')).toBe('Sage running');
  });
});

describe('boundString', () => {
  it('returns the string unchanged when under the limit', () => {
    expect(boundString('short', 10)).toBe('short');
  });

  it('truncates and appends ellipsis when over the limit', () => {
    expect(boundString('abcdefghij', 5)).toBe('abcd\u2026');
  });

  it('returns exactly the limit length unchanged', () => {
    expect(boundString('abcde', 5)).toBe('abcde');
  });
});

describe('presentEvent', () => {
  it('presents an agent status event with escaped display name and status', () => {
    const e = normalizeGatewayEvent(agentEvent(1), '1');
    const p = presentEvent(e);
    expect(p.kind).toBe('agent_status');
    expect(p.label).toBe('Agent status');
    expect(p.recognized).toBe(true);
    expect(p.sentinel).toBe(false);
    expect(p.safeDetail).toContain('Sage');
    expect(p.safeDetail).toContain('running');
  });

  it('presents a run status event', () => {
    const e = normalizeGatewayEvent(runEvent(2), '2');
    const p = presentEvent(e);
    expect(p.kind).toBe('run_status');
    expect(p.safeDetail).toContain('Case');
    expect(p.safeDetail).toContain('investigation');
    expect(p.safeDetail).toContain('running');
  });

  it('presents an eval completed event', () => {
    const e = normalizeGatewayEvent(evalCompletedEvent(3), '3');
    const p = presentEvent(e);
    expect(p.kind).toBe('eval_completed');
    expect(p.safeDetail).toContain('eval-1');
    expect(p.safeDetail).toContain('verified');
  });

  it('presents an eval metric event', () => {
    const e = normalizeGatewayEvent(evalMetricEvent(4), '4');
    const p = presentEvent(e);
    expect(p.kind).toBe('eval_metric');
    expect(p.safeDetail).toContain('accuracy');
    expect(p.safeDetail).toContain('score');
  });

  it('presents a truncated sentinel event', () => {
    const e = normalizeGatewayEvent(
      JSON.stringify({ event: JSON.stringify({ type: 'truncated', data: { since_id: 1, limit: 100 } }) }),
      '0',
    );
    const p = presentEvent(e);
    expect(p.kind).toBe('sentinel');
    expect(p.sentinel).toBe(true);
    expect(p.label).toBe('Stream truncated');
    expect(p.safeDetail).toContain('100');
  });

  it('presents an error sentinel event with escaped reason', () => {
    const e = normalizeGatewayEvent(
      JSON.stringify({ event: JSON.stringify({ type: 'error', data: { reason: 'replay_failed' } }) }),
      '0',
    );
    const p = presentEvent(e);
    expect(p.kind).toBe('sentinel');
    expect(p.label).toBe('Stream error');
    expect(p.safeDetail).toBe('replay_failed');
  });

  it('presents an unknown event as a diagnostic row', () => {
    const e = normalizeGatewayEvent(
      JSON.stringify({ event: JSON.stringify({ type: 'unknown.type', data: { secret: 'leak' } }) }),
      '1',
    );
    const p = presentEvent(e);
    expect(p.kind).toBe('diagnostic');
    expect(p.recognized).toBe(false);
    expect(p.label).toBe('Unknown event');
    expect(p.safeDetail).toContain('unknown.type');
    // The raw payload must not appear in the safe detail.
    expect(p.safeDetail).not.toContain('leak');
  });

  it('escapes HTML in agent display name to prevent injection', () => {
    const raw = JSON.stringify({
      id: 1,
      event: JSON.stringify({
        type: 'app.agent.status.updated',
        data: {
          schema_version: '1.0.0',
          agent_id: 'u:sage',
          display_name: '<img src=x onerror=alert(1)>',
          role: 'sage',
          status: 'running',
          observed_at: '2026-01-01T00:00:00Z',
        },
      }),
    });
    const e = normalizeGatewayEvent(raw, '1');
    const p = presentEvent(e);
    expect(p.safeDetail).not.toContain('<img');
    expect(p.safeDetail).toContain('\x26lt;img');
  });

  it('bounds the display name in the safe detail', () => {
    const longName = 'A'.repeat(200);
    const raw = JSON.stringify({
      id: 1,
      event: JSON.stringify({
        type: 'app.agent.status.updated',
        data: {
          schema_version: '1.0.0',
          agent_id: 'u:sage',
          display_name: longName,
          role: 'sage',
          status: 'running',
          observed_at: '2026-01-01T00:00:00Z',
        },
      }),
    });
    const e = normalizeGatewayEvent(raw, '1');
    const p = presentEvent(e);
    expect(p.safeDetail.length).toBeLessThan(longName.length + 50);
  });
});

describe('presentAgent', () => {
  it('presents an agent with safe labels', () => {
    const agent: AgentStateProjection = {
      schema_version: '1.0.0',
      agent_id: 'u:sage',
      display_name: 'Sage',
      role: 'sage',
      status: 'running',
      freshness: 'observed',
      observed_at: '2026-01-01T00:00:00Z',
    };
    const p = presentAgent(agent);
    expect(p.displayName).toBe('Sage');
    expect(p.statusLabel).toBe('Running');
    expect(p.viewState.status).toBe('ready');
  });

  it('returns unavailable view state when freshness is unavailable', () => {
    const agent: AgentStateProjection = {
      schema_version: '1.0.0',
      agent_id: 'u:sage',
      display_name: 'Sage',
      role: 'sage',
      status: 'offline',
      freshness: 'unavailable',
      observed_at: '2026-01-01T00:00:00Z',
    };
    const p = presentAgent(agent);
    expect(p.viewState.status).toBe('unavailable');
  });

  it('escapes HTML in display name', () => {
    const agent: AgentStateProjection = {
      schema_version: '1.0.0',
      agent_id: 'u:sage',
      display_name: '<script>x</script>',
      role: 'sage',
      status: 'running',
      freshness: 'observed',
      observed_at: '2026-01-01T00:00:00Z',
    };
    const p = presentAgent(agent);
    expect(p.displayName).not.toContain('<script>');
  });
});

describe('presentRun', () => {
  it('presents a run with safe labels', () => {
    const run: RunSummary = {
      schema_version: '1.0.0',
      run_id: 'inv-1',
      run_kind: 'investigation',
      display_name: 'Case',
      status: 'running',
      completed_tasks: 1,
      total_tasks: 3,
      has_receipts: true,
      evidence_count: 2,
      observed_at: '2026-01-01T00:00:00Z',
    };
    const p = presentRun(run);
    expect(p.displayName).toBe('Case');
    expect(p.statusLabel).toBe('Running');
    expect(p.completedTasks).toBe(1);
    expect(p.totalTasks).toBe(3);
    expect(p.viewState.status).toBe('ready');
  });

  it('maps cancelled status to Cancelled label', () => {
    const run: RunSummary = {
      schema_version: '1.0.0',
      run_id: 'inv-1',
      run_kind: 'investigation',
      display_name: 'Case',
      status: 'cancelled',
      completed_tasks: 0,
      total_tasks: 0,
      has_receipts: false,
      evidence_count: 0,
      observed_at: '2026-01-01T00:00:00Z',
    };
    const p = presentRun(run);
    expect(p.statusLabel).toBe('Cancelled');
  });
});

describe('presentEval', () => {
  it('presents a verified eval with ready view state', () => {
    const eval_: EvalSummary = {
      schema_version: '1.0.0',
      run_id: 'eval-1',
      suite_id: 'suite-1',
      suite_version: '1.0.0',
      arm_id: 'arm-1',
      status: 'completed',
      verification_status: 'verified',
      receipt_count: 1,
      metric_count: 1,
      observed_at: '2026-01-01T00:00:00Z',
    };
    const p = presentEval(eval_);
    expect(p.verificationLabel).toBe('Verified');
    expect(p.viewState.status).toBe('ready');
  });

  it('presents a projection_validated eval with partial_verification view state', () => {
    const eval_: EvalSummary = {
      schema_version: '1.0.0',
      run_id: 'eval-1',
      suite_id: 'suite-1',
      suite_version: '1.0.0',
      arm_id: 'arm-1',
      status: 'completed',
      verification_status: 'projection_validated',
      receipt_count: 1,
      metric_count: 1,
      observed_at: '2026-01-01T00:00:00Z',
    };
    const p = presentEval(eval_);
    expect(p.verificationLabel).toBe('Projection validated');
    expect(p.viewState.status).toBe('partial_verification');
  });

  it('presents receipt_verification_not_applicable', () => {
    const eval_: EvalSummary = {
      schema_version: '1.0.0',
      run_id: 'eval-1',
      suite_id: 'suite-1',
      suite_version: '1.0.0',
      arm_id: 'arm-1',
      status: 'completed',
      verification_status: 'receipt_verification_not_applicable',
      receipt_count: 0,
      metric_count: 0,
      observed_at: '2026-01-01T00:00:00Z',
    };
    const p = presentEval(eval_);
    expect(p.verificationLabel).toBe('Receipt verification not applicable');
  });
});

describe('presentNarrativeRow', () => {
  it('presents a recognized narrative row', () => {
    const e = normalizeGatewayEvent(agentEvent(1), '1');
    const row: NarrativeRow = {
      id: e.id,
      type: e.type,
      timestamp: e.timestamp,
      payload: e.payload,
      raw: e.raw,
      recognized: e.recognized,
      sentinel: e.sentinel,
    };
    const p = presentNarrativeRow(row);
    expect(p.id).toBe(1);
    expect(p.kind).toBe('agent_status');
    expect(p.recognized).toBe(true);
    expect(p.label).toBe('Agent status');
  });

  it('presents an unrecognized narrative row as diagnostic', () => {
    const e = normalizeGatewayEvent(
      JSON.stringify({ event: JSON.stringify({ type: 'unknown', data: {} }) }),
      '1',
    );
    const row: NarrativeRow = {
      id: e.id,
      type: e.type,
      timestamp: e.timestamp,
      payload: e.payload,
      raw: e.raw,
      recognized: e.recognized,
      sentinel: e.sentinel,
    };
    const p = presentNarrativeRow(row);
    expect(p.kind).toBe('diagnostic');
    expect(p.recognized).toBe(false);
  });
});

describe('connectionViewState', () => {
  it('maps disconnected to disconnected view state', () => {
    expect(connectionViewState('disconnected').status).toBe('disconnected');
  });
  it('maps connecting to loading view state', () => {
    expect(connectionViewState('connecting').status).toBe('loading');
  });
  it('maps connected to ready view state', () => {
    expect(connectionViewState('connected').status).toBe('ready');
  });
  it('maps reconnecting to loading view state', () => {
    expect(connectionViewState('reconnecting').status).toBe('loading');
  });
  it('maps unauthenticated to unauthenticated view state', () => {
    expect(connectionViewState('unauthenticated').status).toBe('unauthenticated');
  });
});

describe('authViewState', () => {
  it('maps unauthenticated to unauthenticated view state', () => {
    expect(authViewState('unauthenticated').status).toBe('unauthenticated');
  });
  it('maps authenticating to loading view state', () => {
    expect(authViewState('authenticating').status).toBe('loading');
  });
  it('maps enrolling to loading view state', () => {
    expect(authViewState('enrolling').status).toBe('loading');
  });
  it('maps authenticated to ready view state', () => {
    expect(authViewState('authenticated').status).toBe('ready');
  });
  it('maps error to error view state', () => {
    expect(authViewState('error').status).toBe('error');
  });
});

describe('presentOverview', () => {
  it('presents observed freshness as ready', () => {
    const p = presentOverview(3, 'observed', 2, 'observed', true);
    expect(p.agentsRunning).toBe(3);
    expect(p.tasksInQueue).toBe(2);
    expect(p.agentsRunningFreshness.status).toBe('ready');
    expect(p.hasSuccessRate).toBe(true);
  });

  it('presents stale freshness as stale view state', () => {
    const p = presentOverview(3, 'stale', 2, 'observed', false);
    expect(p.agentsRunningFreshness.status).toBe('stale');
  });

  it('presents unavailable freshness as unavailable view state', () => {
    const p = presentOverview(0, 'unavailable', 0, 'unavailable', false);
    expect(p.agentsRunningFreshness.status).toBe('unavailable');
    expect(p.tasksInQueueFreshness.status).toBe('unavailable');
  });
});

describe('presentMeasurement', () => {
  it('presents an observed measurement as ready', () => {
    const p = presentMeasurement('cpu', 50, 'percent', 'observed');
    expect(p.value).toBe(50);
    expect(p.viewState.status).toBe('ready');
  });

  it('presents a stale measurement as stale', () => {
    const p = presentMeasurement('cpu', 50, 'percent', 'stale');
    expect(p.viewState.status).toBe('stale');
  });

  it('presents an unavailable measurement as unavailable', () => {
    const p = presentMeasurement('cpu', 0, 'percent', 'unavailable');
    expect(p.viewState.status).toBe('unavailable');
  });

  it('presents an unsupported measurement as unsupported', () => {
    const p = presentMeasurement('cpu', 0, 'percent', 'unsupported');
    expect(p.viewState.status).toBe('unsupported');
  });
});

describe('VIEW_STATES', () => {
  it('all view states have a status and message', () => {
    expect(VIEW_STATES.loading.status).toBe('loading');
    expect(VIEW_STATES.empty.status).toBe('empty');
    expect(VIEW_STATES.stale.status).toBe('stale');
    expect(VIEW_STATES.unavailable.status).toBe('unavailable');
    expect(VIEW_STATES.unsupported.status).toBe('unsupported');
    expect(VIEW_STATES.partialVerification.status).toBe('partial_verification');
    expect(VIEW_STATES.disconnected.status).toBe('disconnected');
    expect(VIEW_STATES.unauthenticated.status).toBe('unauthenticated');
    expect(VIEW_STATES.error.status).toBe('error');
    expect(VIEW_STATES.ready.status).toBe('ready');
  });
});

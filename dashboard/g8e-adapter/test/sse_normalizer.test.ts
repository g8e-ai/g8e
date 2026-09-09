import { describe, expect, it } from 'vitest';

import {
  EventDeduplicator,
  OBSERVE_EVENT_TYPES,
  normalizeGatewayEvent,
  validateObservePayload,
} from '../src/sse/normalizer';

function agentPayload(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    schema_version: '1.0.0',
    agent_id: 'u:sage',
    display_name: 'Sage',
    role: 'sage',
    status: 'running',
    observed_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function runPayload(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    schema_version: '1.0.0',
    run_id: 'r1',
    run_kind: 'investigation',
    display_name: 'Case',
    status: 'running',
    completed_tasks: 1,
    total_tasks: 2,
    observed_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function evalRunCompletedPayload(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    schema_version: '1.0.0',
    run_id: 'r1',
    suite_id: 's1',
    suite_version: '1.0.0',
    arm_id: 'a1',
    terminal_attempts: 10,
    assigned_tasks: 10,
    receipt_count: 5,
    verification_status: 'projection_validated',
    published_projection_sha256: 'abc',
    completed_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

function evalMetricPayload(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    schema_version: '1.0.0',
    run_id: 'r1',
    metric_id: 'acc',
    metric_version: '1.0.0',
    unit: 'fraction',
    eligible: 100,
    denominator: 100,
    verification_status: 'projection_validated',
    recorded_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

// Build a nested envelope: outer has `event` as a JSON string of the inner.
function nestedEnvelope(inner: Record<string, unknown>, id?: number): string {
  const outer: Record<string, unknown> = { event: JSON.stringify(inner) };
  if (id !== undefined) outer.id = id;
  return JSON.stringify(outer);
}

// Build a flat envelope: the outer IS the inner.
function flatEnvelope(inner: Record<string, unknown>): string {
  return JSON.stringify(inner);
}

describe('normalizeGatewayEvent', () => {
  describe('nested object and nested JSON-string events normalize identically', () => {
    it('normalizes a nested envelope (outer.event is a JSON string)', () => {
      const inner = { type: 'app.agent.status.updated', data: agentPayload(), timestamp: '2026-01-01T00:00:00Z' };
      const raw = nestedEnvelope(inner, 42);
      const ev = normalizeGatewayEvent(raw, '0');
      expect(ev.id).toBe(42);
      expect(ev.type).toBe('app.agent.status.updated');
      expect(ev.timestamp).toBe('2026-01-01T00:00:00Z');
      expect(ev.recognized).toBe(true);
      expect(ev.sentinel).toBe(false);
      expect(ev.payload).toEqual(agentPayload());
    });

    it('normalizes a flat envelope (no event field)', () => {
      const inner = { type: 'app.agent.status.updated', data: agentPayload(), timestamp: '2026-01-01T00:00:00Z', id: 42 };
      const raw = flatEnvelope(inner);
      const ev = normalizeGatewayEvent(raw, '0');
      expect(ev.id).toBe(42);
      expect(ev.type).toBe('app.agent.status.updated');
      expect(ev.recognized).toBe(true);
      expect(ev.payload).toEqual(agentPayload());
    });

    it('nested and flat produce the same type, timestamp, and payload', () => {
      const inner = { type: 'app.run.status.updated', data: runPayload(), timestamp: '2026-01-01T00:00:00Z' };
      const nested = normalizeGatewayEvent(nestedEnvelope(inner, 1), '0');
      const flat = normalizeGatewayEvent(flatEnvelope({ ...inner, id: 1 }), '0');
      expect(nested.type).toBe(flat.type);
      expect(nested.timestamp).toBe(flat.timestamp);
      expect(nested.payload).toEqual(flat.payload);
      expect(nested.id).toBe(flat.id);
    });
  });

  describe('durable ID resolution', () => {
    it('uses the outer envelope id when present', () => {
      const ev = normalizeGatewayEvent(nestedEnvelope({ type: 'message' }, 99), '0');
      expect(ev.id).toBe(99);
    });

    it('falls back to inner id when outer has no id', () => {
      const raw = flatEnvelope({ type: 'message', id: 77 });
      const ev = normalizeGatewayEvent(raw, '0');
      expect(ev.id).toBe(77);
    });

    it('falls back to lastEventId when no parsed id exists', () => {
      const raw = nestedEnvelope({ type: 'message' });
      const ev = normalizeGatewayEvent(raw, '55');
      expect(ev.id).toBe(55);
    });

    it('uses 0 when no id is available', () => {
      const raw = nestedEnvelope({ type: 'message' });
      const ev = normalizeGatewayEvent(raw, '');
      expect(ev.id).toBe(0);
    });

    it('handles string id in the outer envelope', () => {
      const raw = JSON.stringify({ id: '123', event: JSON.stringify({ type: 'message' }) });
      const ev = normalizeGatewayEvent(raw, '0');
      expect(ev.id).toBe(123);
    });

    it('ignores non-numeric string ids', () => {
      const raw = JSON.stringify({ id: 'abc', event: JSON.stringify({ type: 'message' }) });
      const ev = normalizeGatewayEvent(raw, '99');
      expect(ev.id).toBe(99);
    });
  });

  describe('invalid JSON', () => {
    it('throws on invalid outer JSON', () => {
      expect(() => normalizeGatewayEvent('not json', '0')).toThrow();
    });

    it('throws on invalid nested event JSON', () => {
      const raw = JSON.stringify({ event: 'not json' });
      expect(() => normalizeGatewayEvent(raw, '0')).toThrow();
    });

    it('throws when outer is not an object', () => {
      expect(() => normalizeGatewayEvent('"string"', '0')).toThrow();
    });

    it('throws when inner is not an object', () => {
      const raw = JSON.stringify({ event: '"string"' });
      expect(() => normalizeGatewayEvent(raw, '0')).toThrow();
    });
  });

  describe('missing type/data', () => {
    it('defaults type to message when missing', () => {
      const ev = normalizeGatewayEvent(flatEnvelope({ id: 1 }), '0');
      expect(ev.type).toBe('message');
      expect(ev.recognized).toBe(false);
    });

    it('uses the full inner as payload when data is missing', () => {
      const inner = { type: 'message', id: 1, extra: 'x' };
      const ev = normalizeGatewayEvent(flatEnvelope(inner), '0');
      expect(ev.payload).toEqual(inner);
    });
  });

  describe('unknown types', () => {
    it('marks unknown types as unrecognized', () => {
      const ev = normalizeGatewayEvent(flatEnvelope({ type: 'unknown.event', id: 1 }), '0');
      expect(ev.recognized).toBe(false);
      expect(ev.sentinel).toBe(false);
    });
  });

  describe('sentinel events', () => {
    it('recognizes error sentinel', () => {
      const ev = normalizeGatewayEvent(flatEnvelope({ type: 'error', data: { reason: 'replay_failed' } }), '0');
      expect(ev.recognized).toBe(true);
      expect(ev.sentinel).toBe(true);
      expect(ev.type).toBe('error');
    });

    it('recognizes truncated sentinel', () => {
      const ev = normalizeGatewayEvent(flatEnvelope({ type: 'truncated', data: { since_id: 1, limit: 100 } }), '0');
      expect(ev.recognized).toBe(true);
      expect(ev.sentinel).toBe(true);
      expect(ev.type).toBe('truncated');
    });
  });

  describe('payload validation for recognized observe events', () => {
    it('validates a correct agent status updated payload', () => {
      expect(validateObservePayload('app.agent.status.updated', agentPayload())).toBe(true);
    });

    it('rejects agent payload with invalid status', () => {
      expect(validateObservePayload('app.agent.status.updated', agentPayload({ status: 'pending' }))).toBe(false);
    });

    it('rejects agent payload missing agent_id', () => {
      const p = agentPayload();
      delete p.agent_id;
      expect(validateObservePayload('app.agent.status.updated', p)).toBe(false);
    });

    it('validates a correct run status updated payload', () => {
      expect(validateObservePayload('app.run.status.updated', runPayload())).toBe(true);
    });

    it('rejects run payload with invalid run_kind', () => {
      expect(validateObservePayload('app.run.status.updated', runPayload({ run_kind: 'chat' }))).toBe(false);
    });

    it('rejects run payload with invalid status', () => {
      expect(validateObservePayload('app.run.status.updated', runPayload({ status: 'idle' }))).toBe(false);
    });

    it('validates a correct eval run completed payload', () => {
      expect(validateObservePayload('ai.eval.run.completed', evalRunCompletedPayload())).toBe(true);
    });

    it('rejects eval run completed with invalid verification_status', () => {
      expect(validateObservePayload('ai.eval.run.completed', evalRunCompletedPayload({ verification_status: 'unverified' }))).toBe(false);
    });

    it('validates a correct eval metric recorded payload', () => {
      expect(validateObservePayload('ai.eval.metric.recorded', evalMetricPayload())).toBe(true);
    });

    it('rejects eval metric recorded with invalid verification_status', () => {
      expect(validateObservePayload('ai.eval.metric.recorded', evalMetricPayload({ verification_status: 'partial' }))).toBe(false);
    });

    it('marks recognized event with invalid payload as unrecognized', () => {
      const inner = { type: 'app.agent.status.updated', data: agentPayload({ status: 'bogus' }) };
      const ev = normalizeGatewayEvent(nestedEnvelope(inner, 1), '0');
      expect(ev.recognized).toBe(false);
    });

    it('marks recognized event with valid payload as recognized', () => {
      const inner = { type: 'app.agent.status.updated', data: agentPayload() };
      const ev = normalizeGatewayEvent(nestedEnvelope(inner, 1), '0');
      expect(ev.recognized).toBe(true);
    });
  });

  describe('routing IDs do not become trusted payload', () => {
    it('web_session_id in the outer envelope does not appear in the payload', () => {
      const inner = { type: 'app.agent.status.updated', data: agentPayload() };
      const outer = { id: 1, event: JSON.stringify(inner), web_session_id: 'ws-secret' };
      const ev = normalizeGatewayEvent(JSON.stringify(outer), '0');
      expect(ev.payload).toEqual(agentPayload());
      expect(JSON.stringify(ev.payload)).not.toContain('web_session_id');
      expect(JSON.stringify(ev.payload)).not.toContain('ws-secret');
    });

    it('user_id in the outer envelope does not appear in the payload', () => {
      const inner = { type: 'app.run.status.updated', data: runPayload() };
      const outer = { id: 1, event: JSON.stringify(inner), user_id: 'u-secret' };
      const ev = normalizeGatewayEvent(JSON.stringify(outer), '0');
      expect(JSON.stringify(ev.payload)).not.toContain('u-secret');
    });
  });
});

describe('EventDeduplicator', () => {
  it('deduplicates by ID', () => {
    const d = new EventDeduplicator();
    expect(d.has(1)).toBe(false);
    d.add(1);
    expect(d.has(1)).toBe(true);
    expect(d.has(2)).toBe(false);
  });

  it('ignores id 0', () => {
    const d = new EventDeduplicator();
    d.add(0);
    expect(d.has(0)).toBe(false);
    expect(d.size).toBe(0);
  });

  it('evicts oldest when exceeding maxEvents', () => {
    const d = new EventDeduplicator(3);
    d.add(1);
    d.add(2);
    d.add(3);
    d.add(4);
    expect(d.has(1)).toBe(false);
    expect(d.has(4)).toBe(true);
    expect(d.size).toBe(3);
  });

  it('does not add duplicates', () => {
    const d = new EventDeduplicator();
    d.add(1);
    d.add(1);
    expect(d.size).toBe(1);
  });

  it('clears all entries', () => {
    const d = new EventDeduplicator();
    d.add(1);
    d.add(2);
    d.clear();
    expect(d.size).toBe(0);
    expect(d.has(1)).toBe(false);
  });
});

describe('OBSERVE_EVENT_TYPES', () => {
  it('contains the four dashboard event types', () => {
    expect([...OBSERVE_EVENT_TYPES]).toEqual([
      'app.agent.status.updated',
      'app.run.status.updated',
      'ai.eval.run.completed',
      'ai.eval.metric.recorded',
    ]);
  });
});

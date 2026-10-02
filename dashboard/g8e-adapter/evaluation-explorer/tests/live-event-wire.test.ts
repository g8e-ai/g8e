// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { decodeLiveEventWire, isLiveEventKind } from '../src/contract/live-event-wire';
import { decodeViewRecord, ValidationError } from '../src/contract/validators';
import { EvalStore } from '../src/state/store';

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '../../../..');
const liveEventVector = JSON.parse(
  readFileSync(join(repoRoot, 'protocol/vectors/eval/public_live_event.json'), 'utf8'),
) as { message_type: string; canonical_json: string };

const wire = (): Record<string, unknown> => JSON.parse(liveEventVector.canonical_json) as Record<string, unknown>;

describe('decodeLiveEventWire', () => {
  it('decodes the Go-produced vector, keeping zero progress and a measured zero', () => {
    expect(liveEventVector.message_type).toBe('PublicLiveEvent');
    const event = decodeLiveEventWire(wire());
    expect(event.kind).toBe('metric_updated');
    expect(event.completed).toBe(0);
    expect(event.total).toBe(5);
    expect(event.metric_delta?.pass).toEqual({ value: 0 });
    expect(event.release).toBe('v2.2.8');
    expect(event.release_basis).toBe('recorded');
    expect(event.source_revision).toBe('abc123');
    expect(() => decodeViewRecord(event.kind, event)).not.toThrow();
  });

  it.each([
    ['PUBLIC_RELEASE_BASIS_ASSERTED', 'asserted', 'v2.2.7'],
    ['PUBLIC_RELEASE_BASIS_UNKNOWN', 'unknown', undefined],
  ])('maps wire release basis %s to %s', (wireBasis, basis, release) => {
    const event = decodeLiveEventWire({ ...wire(), release, release_basis: wireBasis });
    expect(event.release_basis).toBe(basis);
    expect(event.release).toBe(release);
  });

  it('reads an event with no release identity as unknown, not as a missing field', () => {
    const { release, release_basis, source_revision, ...historical } = wire();
    void release; void release_basis; void source_revision;
    const event = decodeLiveEventWire(historical);
    expect(event.release_basis).toBe('unknown');
    expect(event.release).toBeUndefined();
  });

  it.each(['completed', 'total'])('rejects an event that omits %s instead of reading zero', (field) => {
    const body = wire();
    delete body[field];
    expect(() => decodeLiveEventWire(body)).toThrow(ValidationError);
    expect(() => decodeLiveEventWire(body)).toThrow(new RegExp(field));
  });

  it('rejects the view-form release basis, which is not a wire value', () => {
    expect(() => decodeLiveEventWire({ ...wire(), release_basis: 'recorded' })).toThrow(ValidationError);
  });

  it.each([
    ['a field outside the message', { feed_sequence: 3 }],
    ['a camelCase alias', { eventId: 'x' }],
    ['a provider-boundary field', { served_model_tag: 'qwen3' }],
  ])('rejects %s', (_label, extra) => {
    expect(() => decodeLiveEventWire({ ...wire(), ...extra })).toThrow(ValidationError);
  });

  it('rejects a metric with neither a value nor a reason', () => {
    expect(() => decodeLiveEventWire({ ...wire(), metric_delta: { pass: {} } })).toThrow(ValidationError);
  });

  it('rejects a non-object payload', () => {
    expect(() => decodeLiveEventWire('not an event')).toThrow(ValidationError);
  });
});

describe('isLiveEventKind', () => {
  it('names exactly the live event kinds', () => {
    expect(isLiveEventKind('stage_updated')).toBe(true);
    expect(isLiveEventKind('metric_updated')).toBe(true);
    expect(isLiveEventKind('evaluation_summary')).toBe(false);
  });
});

describe('EvalStore live event ingest', () => {
  it('indexes the wire event from the feed with its measured zero', () => {
    const store = new EvalStore();
    store.acceptProjection({ sequence: 7, record_type: 'event', record_bytes: liveEventVector.canonical_json });
    expect(store.getState().errors).toEqual([]);
    const event = store.getState().events.find((candidate) => candidate.event_id === 'run-1:assign-failed:metric:pass:event');
    expect(event).toBeDefined();
    expect(event?.feed_sequence).toBe(7);
    expect(event?.metric_delta?.pass).toEqual({ value: 0 });
    expect(event?.release_basis).toBe('recorded');
  });

  it('records an error and no event when the wire event omits its progress', () => {
    const store = new EvalStore();
    const body = wire();
    delete body.completed;
    store.acceptProjection({ sequence: 8, record_type: 'event', record_bytes: JSON.stringify(body) });
    expect(store.getState().events).toHaveLength(0);
    expect(store.getState().errors.join(' ')).toContain('completed');
  });
});

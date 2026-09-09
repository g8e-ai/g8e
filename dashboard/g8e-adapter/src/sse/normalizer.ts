// NormalizedGatewayEvent boundary. Parses the outer push envelope, parses a
// nested string event when needed, validates recognized type/data/version
// pairs, reads the durable ID from lastEventId, and prevents routing IDs from
// becoming trusted application payload.
//
// The SSE wire format from the Gateway is a nested envelope:
//   outer: JSON.parse(rawData) -> { id, event: "<json string>" }
//   inner: JSON.parse(parsed.event) -> { type, data, version, ... }
// When the outer has no `event` field, the outer IS the inner (flat format).

import {
  isAgentLifecycleStatus,
  isEvalVerificationStatus,
  isRunKind,
  isRunLifecycleStatus,
} from '../types/enums';

export const OBSERVE_EVENT_PAYLOAD_SCHEMA_VERSION = '1.0.0';

export type ObserveEventType =
  | 'app.agent.status.updated'
  | 'app.run.status.updated'
  | 'ai.eval.run.completed'
  | 'ai.eval.metric.recorded';

export const OBSERVE_EVENT_TYPES: readonly ObserveEventType[] = [
  'app.agent.status.updated',
  'app.run.status.updated',
  'ai.eval.run.completed',
  'ai.eval.metric.recorded',
] as const;

export type SentinelEventType = 'error' | 'truncated';

export const SENTINEL_EVENT_TYPES: readonly SentinelEventType[] = ['error', 'truncated'] as const;

export type RecognizedEventType = ObserveEventType | SentinelEventType;

export interface NormalizedGatewayEvent {
  readonly id: number;
  readonly type: string;
  readonly timestamp: string;
  readonly payload: unknown;
  readonly raw: string;
  readonly recognized: boolean;
  readonly sentinel: boolean;
}

export class NormalizedEventError extends Error {
  constructor(message: string, public readonly raw: string) {
    super(message);
    this.name = 'NormalizedEventError';
  }
}

interface OuterEnvelope {
  id?: number | string;
  event?: string;
  type?: string;
  timestamp?: string;
  data?: unknown;
  version?: string;
  [key: string]: unknown;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isObserveEventType(type: string): type is ObserveEventType {
  return (OBSERVE_EVENT_TYPES as readonly string[]).includes(type);
}

function isSentinelEventType(type: string): type is SentinelEventType {
  return (SENTINEL_EVENT_TYPES as readonly string[]).includes(type);
}

// Validate the payload of a recognized observe event type. Returns true if the
// payload has the required fields with correct enum values.
export function validateObservePayload(type: ObserveEventType, payload: unknown): boolean {
  if (!isObject(payload)) return false;
  if (typeof payload.schema_version !== 'string') return false;

  switch (type) {
    case 'app.agent.status.updated':
      return typeof payload.agent_id === 'string' &&
        typeof payload.display_name === 'string' &&
        typeof payload.role === 'string' &&
        isAgentLifecycleStatus(payload.status) &&
        typeof payload.observed_at === 'string';

    case 'app.run.status.updated':
      return typeof payload.run_id === 'string' &&
        isRunKind(payload.run_kind) &&
        typeof payload.display_name === 'string' &&
        isRunLifecycleStatus(payload.status) &&
        typeof payload.completed_tasks === 'number' &&
        typeof payload.total_tasks === 'number' &&
        typeof payload.observed_at === 'string';

    case 'ai.eval.run.completed':
      return typeof payload.run_id === 'string' &&
        typeof payload.suite_id === 'string' &&
        typeof payload.arm_id === 'string' &&
        isEvalVerificationStatus(payload.verification_status) &&
        typeof payload.terminal_attempts === 'number' &&
        typeof payload.assigned_tasks === 'number' &&
        typeof payload.receipt_count === 'number' &&
        typeof payload.completed_at === 'string';

    case 'ai.eval.metric.recorded':
      return typeof payload.run_id === 'string' &&
        typeof payload.metric_id === 'string' &&
        typeof payload.unit === 'string' &&
        isEvalVerificationStatus(payload.verification_status) &&
        typeof payload.eligible === 'number' &&
        typeof payload.denominator === 'number' &&
        typeof payload.recorded_at === 'string';
  }
}

// Normalize a raw SSE message into a NormalizedGatewayEvent. The lastEventId is
// the browser EventSource's lastEventId (a string), used as a fallback for the
// durable ID when the parsed envelope does not carry one.
export function normalizeGatewayEvent(rawData: string, lastEventId: string): NormalizedGatewayEvent {
  let outer: OuterEnvelope;
  try {
    outer = JSON.parse(rawData);
  } catch {
    throw new NormalizedEventError('invalid outer JSON', rawData);
  }

  if (!isObject(outer)) {
    throw new NormalizedEventError('outer envelope is not an object', rawData);
  }

  // Parse the nested string event when present. The Gateway wraps the actual
  // event as a JSON string in the `event` field of the outer envelope.
  let inner: OuterEnvelope;
  if (typeof outer.event === 'string') {
    try {
      inner = JSON.parse(outer.event);
    } catch {
      throw new NormalizedEventError('invalid nested event JSON', rawData);
    }
  } else {
    inner = outer;
  }

  if (!isObject(inner)) {
    throw new NormalizedEventError('inner event is not an object', rawData);
  }

  // Durable ID: prefer the parsed envelope id, then the browser lastEventId.
  // Routing IDs (web_session_id, user_id) are never used as durable IDs and
  // never become trusted application payload.
  let id = 0;
  if (typeof outer.id === 'number') {
    id = outer.id;
  } else if (typeof outer.id === 'string') {
    const parsed = parseInt(outer.id, 10);
    if (!isNaN(parsed)) id = parsed;
  }
  if (id === 0 && typeof inner.id === 'number') {
    id = inner.id;
  }
  if (id === 0 && lastEventId) {
    const parsed = parseInt(lastEventId, 10);
    if (!isNaN(parsed)) id = parsed;
  }

  const type = typeof inner.type === 'string' ? inner.type : 'message';
  const timestamp = typeof inner.timestamp === 'string' ? inner.timestamp : new Date().toISOString();
  const payload = inner.data !== undefined ? inner.data : inner;

  const recognized = isObserveEventType(type) || isSentinelEventType(type);
  const sentinel = isSentinelEventType(type);

  // Validate recognized observe event payloads. If the payload fails
  // validation, the event is still normalized but marked as unrecognized so
  // it cannot mutate projections or counters.
  let effectiveRecognized = recognized;
  if (effectiveRecognized && isObserveEventType(type)) {
    if (!validateObservePayload(type, payload)) {
      effectiveRecognized = false;
    }
  }

  return {
    id,
    type,
    timestamp,
    payload,
    raw: rawData,
    recognized: effectiveRecognized,
    sentinel,
  };
}

// Bounded dedup set for durable IDs. Prevents duplicate events from mutating
// state. The set evicts the oldest entry when it exceeds maxEvents.
export class EventDeduplicator {
  private seen = new Set<number>();
  constructor(private readonly maxEvents = 500) {}

  has(id: number): boolean {
    return id !== 0 && this.seen.has(id);
  }

  add(id: number): void {
    if (id === 0) return;
    if (this.seen.has(id)) return;
    this.seen.add(id);
    if (this.seen.size > this.maxEvents) {
      const it = this.seen.values();
      const oldest = it.next().value;
      if (oldest !== undefined) this.seen.delete(oldest);
    }
  }

  clear(): void {
    this.seen.clear();
  }

  get size(): number {
    return this.seen.size;
  }
}

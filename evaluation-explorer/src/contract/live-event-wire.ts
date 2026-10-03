// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Wire decoder for live events, which the Gateway encodes from the protobuf
// message g8e.eval.v1.PublicLiveEvent with canonical protojson. This is the one
// place wire rules become view-contract values: enum names map to the view
// vocabulary, and the presence-tracked progress counts are required, because an
// omitted count is a malformed event and never a count of zero.

import { assertWireReleaseProvenance, mapCampaignRelease, type WireReleaseProvenance } from './campaign-wire';
import { LIVE_EVENT_KINDS, type LiveEvent } from './types';
import { isLiveEvent, ValidationError } from './validators';

/** Every field of PublicLiveEvent. A key outside this list is not a wire field. */
const LIVE_EVENT_WIRE_FIELDS: readonly string[] = [
  'schema_version', 'kind', 'dataset_id', 'quality_state', 'observed_at', 'source_revision_label',
  'release', 'release_basis', 'source_revision',
  'event_id', 'run_id', 'assignment_id', 'variant_id', 'role', 'lifecycle_status',
  'completed', 'total', 'stage_label', 'task_id', 'metric_delta',
];

const PRESENCE_TRACKED_FIELDS = ['completed', 'total'] as const;

export function isLiveEventKind(kind: string): boolean {
  return (LIVE_EVENT_KINDS as readonly string[]).includes(kind);
}

/** Decode one wire live event into the view contract's LiveEvent. */
export function decodeLiveEventWire(value: unknown): LiveEvent {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new ValidationError('expected object', 'live_event');
  }
  const wire = value as Record<string, unknown>;
  for (const key of Object.keys(wire)) {
    if (!LIVE_EVENT_WIRE_FIELDS.includes(key)) throw new ValidationError(`unknown field "${key}"`, `live_event.${key}`);
  }
  for (const field of PRESENCE_TRACKED_FIELDS) {
    if (wire[field] === undefined) {
      throw new ValidationError(`${field} is presence-tracked; an omitted count is not zero`, `live_event.${field}`);
    }
  }
  assertWireReleaseProvenance(wire, 'live_event');
  const { release, release_basis, source_revision, ...rest } = wire;
  void release; void release_basis; void source_revision;
  const identity = mapCampaignRelease(wire as WireReleaseProvenance);
  const event: Record<string, unknown> = { ...rest, release_basis: identity.release_basis };
  if (identity.release !== undefined) event.release = identity.release;
  if (identity.source_revision !== undefined) event.source_revision = identity.source_revision;
  isLiveEvent(event);
  return event;
}

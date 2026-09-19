// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';

import {
  PublicReconciliationError,
  applyPublicHistory,
  applyPublicRecord,
  createPublicMirrorState,
  reconcilePublicBootstrap,
  reconcilePublicSnapshot,
} from '../src/public';

const zeroHash = '0'.repeat(64);
const firstHash = '1'.repeat(64);
const secondHash = '2'.repeat(64);

function snapshot(sequence: number, hash: string, freshness = 'active') {
  return {
    protocol_version: '1.0.0',
    source_id: 'deployment-a',
    high_water_sequence: sequence,
    feed_chain_hash: hash,
    batch_count: sequence,
    generated_at: '2026-09-13T00:00:00Z',
    freshness,
  };
}

describe('public spectator state reconciliation', () => {
  it('maps every freshness value to an honest display state', () => {
    const expected = {
      active: 'active',
      delayed: 'delayed',
      stale: 'stale',
      intentionally_stopped: 'stopped',
      safety_stopped: 'safety-stopped',
      source_offline: 'offline',
    } as const;
    for (const [freshness, displayState] of Object.entries(expected)) {
      const state = reconcilePublicSnapshot(createPublicMirrorState(), snapshot(0, zeroHash, freshness));
      expect(state.display_state).toBe(displayState);
    }
  });

  it('reconciles bootstrap, ordered history, SSE records, and a sealing snapshot', () => {
    let state = reconcilePublicBootstrap(createPublicMirrorState(), {
      protocol_version: '1.0.0',
      snapshot: snapshot(1, firstHash),
      source_freshness: 'active',
      recent_projections: [{ sequence: 1, campaign_id: 'campaign-a' }],
      proof_catalog_summary: { artifact_count: 0, total_byte_size: 0 },
      generated_at: '2026-09-13T00:00:00Z',
    });
    state = applyPublicHistory(state, {
      protocol_version: '1.0.0',
      items: [{ sequence: 2, record_type: 'projection', campaign_id: 'campaign-b' }],
      has_more: false,
      limit: 20,
    });
    state = applyPublicRecord(state, {
      sequence: 3,
      record_type: 'event',
      record_hash: '3'.repeat(64),
      record_bytes: '{"campaign_id":"campaign-c"}',
    });
    state = reconcilePublicSnapshot(state, snapshot(3, secondHash));

    expect(state.observed_sequence).toBe(3);
    expect(state.high_water_sequence).toBe(3);
    expect(state.feed_chain_hash).toBe(secondHash);
    expect(state.items.map((item) => item.sequence)).toEqual([1, 2, 3]);
  });

  it('fails closed on a snapshot regression, same-sequence equivocation, or sequence gap', () => {
    const state = reconcilePublicSnapshot(createPublicMirrorState(), snapshot(2, secondHash));
    expect(() => reconcilePublicSnapshot(state, snapshot(1, firstHash))).toThrow(PublicReconciliationError);
    expect(() => reconcilePublicSnapshot(state, snapshot(2, firstHash))).toThrow(PublicReconciliationError);
    expect(() => applyPublicHistory(state, {
      protocol_version: '1.0.0',
      items: [{ sequence: 4, record_type: 'projection' }],
      has_more: false,
      limit: 20,
    })).toThrow(PublicReconciliationError);
  });

  it('retains only the newest bounded item window', () => {
    let state = createPublicMirrorState(2);
    state = applyPublicHistory(state, {
      protocol_version: '1.0.0',
      items: [
        { sequence: 1, record_type: 'projection' },
        { sequence: 2, record_type: 'projection' },
        { sequence: 3, record_type: 'projection' },
      ],
      has_more: false,
      limit: 20,
    });
    expect(state.items.map((item) => item.sequence)).toEqual([2, 3]);
  });
});

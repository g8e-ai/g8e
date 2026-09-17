import { describe, it, expect } from 'vitest';
import {
  classifyFreshness,
  classifyStreamConnection,
  deriveFreshness,
  effectiveFeedFreshness,
  isTerminalFailure,
  qualityStateLabel,
  qualityStateTone,
  emptyStateReason,
} from '../src/utils/feed-state';

describe('classifyFreshness', () => {
  it('classifies active as ok', () => {
    expect(classifyFreshness('active').tone).toBe('ok');
  });

  it('classifies source_offline as critical', () => {
    expect(classifyFreshness('source_offline').tone).toBe('critical');
  });

  it('classifies unknown as warn with Connecting label', () => {
    const result = classifyFreshness('unknown');
    expect(result.tone).toBe('warn');
    expect(result.label).toBe('Connecting');
  });

  it('classifies stale as warn with Inactive label', () => {
    const result = classifyFreshness('stale');
    expect(result.tone).toBe('warn');
    expect(result.label).toBe('Inactive');
  });
});

describe('isTerminalFailure', () => {
  it('returns true for failed', () => {
    expect(isTerminalFailure('failed')).toBe(true);
  });

  it('returns true for stopped', () => {
    expect(isTerminalFailure('stopped')).toBe(true);
  });

  it('returns false for completed', () => {
    expect(isTerminalFailure('completed')).toBe(false);
  });

  it('returns false for running', () => {
    expect(isTerminalFailure('running')).toBe(false);
  });
});

describe('qualityStateLabel', () => {
  it('labels verified_public', () => {
    expect(qualityStateLabel('verified_public')).toBe('Verified public');
  });

  it('labels not_evaluated', () => {
    expect(qualityStateLabel('not_evaluated')).toBe('Not evaluated');
  });

  it('labels exploratory_partial', () => {
    expect(qualityStateLabel('exploratory_partial')).toBe('Exploratory · partial');
  });
});

describe('qualityStateTone', () => {
  it('tones verified_public as ok', () => {
    expect(qualityStateTone('verified_public')).toBe('ok');
  });

  it('tones terminal_failed as critical', () => {
    expect(qualityStateTone('terminal_failed')).toBe('critical');
  });

  it('tones not_evaluated as neutral', () => {
    expect(qualityStateTone('not_evaluated')).toBe('neutral');
  });
});

describe('deriveFreshness', () => {
  const now = Date.parse('2026-09-17T12:00:00Z');

  it('returns active for recent records', () => {
    expect(deriveFreshness('2026-09-17T11:59:30Z', undefined, now)).toBe('active');
  });

  it('returns delayed after the delayed window', () => {
    expect(deriveFreshness('2026-09-17T11:58:30Z', undefined, now)).toBe('delayed');
  });

  it('returns stale after the stale window', () => {
    expect(deriveFreshness('2026-09-17T11:54:00Z', undefined, now)).toBe('stale');
  });

  it('returns source_offline after the offline window', () => {
    expect(deriveFreshness('2026-09-17T11:40:00Z', undefined, now)).toBe('source_offline');
  });

  it('respects pinned intentional stop states', () => {
    expect(deriveFreshness('2026-09-17T11:40:00Z', 'intentionally_stopped', now)).toBe('intentionally_stopped');
  });
});

describe('effectiveFeedFreshness', () => {
  it('recomputes freshness from lastAcceptedAt', () => {
    const now = Date.parse('2026-09-17T12:00:00Z');
    const freshness = effectiveFeedFreshness(
      {
        connection: 'live',
        freshness: 'active',
        highWaterSequence: 10,
        lastAcceptedAt: '2026-09-17T11:54:00Z',
        message: 'test',
      },
      now,
    );
    expect(freshness).toBe('stale');
  });
});

describe('classifyStreamConnection', () => {
  it('shows Live when SSE is connected', () => {
    expect(classifyStreamConnection('connected', 'live').label).toBe('Live');
  });

  it('shows Connecting while the feed is live but SSE is opening', () => {
    expect(classifyStreamConnection('disconnected', 'live').label).toBe('Connecting');
  });

  it('shows Reconnecting during SSE reconnect', () => {
    expect(classifyStreamConnection('reconnecting', 'live').label).toBe('Reconnecting');
  });
});

describe('emptyStateReason', () => {
  it('returns feed-offline when connection is offline', () => {
    const reason = emptyStateReason(false, false, 'offline');
    expect(reason.kind).toBe('feed-offline');
  });

  it('returns no-data when no records and connection is live', () => {
    const reason = emptyStateReason(false, false, 'live');
    expect(reason.kind).toBe('no-data');
  });

  it('returns no-matching-filters when records exist and filters are active', () => {
    const reason = emptyStateReason(true, true, 'live');
    expect(reason.kind).toBe('no-matching-filters');
  });
});

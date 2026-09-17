import { describe, it, expect } from 'vitest';
import {
  classifyFreshness,
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

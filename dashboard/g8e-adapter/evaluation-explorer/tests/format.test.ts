// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, it, expect } from 'vitest';
import {
  formatPercent,
  formatNumber,
  formatLatency,
  formatThroughput,
  formatTokens,
  formatDuration,
  formatTimestamp,
  metricDisplay,
} from '../src/utils/format';

describe('formatPercent', () => {
  it('formats a proportion as a percentage', () => {
    expect(formatPercent(0.82)).toBe('82.0%');
  });

  it('formats with custom fraction digits', () => {
    expect(formatPercent(0.8234, 2)).toBe('82.34%');
  });

  it('returns Unavailable for non-finite values', () => {
    expect(formatPercent(Number.NaN)).toBe('Unavailable');
    expect(formatPercent(Number.POSITIVE_INFINITY)).toBe('Unavailable');
  });
});

describe('formatNumber', () => {
  it('formats integers with locale separators', () => {
    expect(formatNumber(1125)).toBe('1,125');
  });

  it('formats with fraction digits', () => {
    expect(formatNumber(1234.5, 1)).toBe('1,234.5');
  });
});

describe('formatLatency', () => {
  it('formats sub-second latency in milliseconds', () => {
    expect(formatLatency(820)).toBe('820 ms');
  });

  it('formats multi-second latency in seconds', () => {
    expect(formatLatency(2100)).toBe('2.10 s');
  });

  it('returns Unavailable for non-finite values', () => {
    expect(formatLatency(Number.NaN)).toBe('Unavailable');
  });
});

describe('formatThroughput', () => {
  it('formats tokens per second compactly', () => {
    expect(formatThroughput(42.1)).toBe('42.1 tok/s');
  });

  it('formats large throughput compactly', () => {
    expect(formatThroughput(1200)).toBe('1.2K tok/s');
  });
});

describe('formatTokens', () => {
  it('formats token counts compactly', () => {
    expect(formatTokens(20185634)).toBe('20.2M tok');
  });
});

describe('formatDuration', () => {
  it('formats seconds under a minute', () => {
    expect(formatDuration(45)).toBe('45s');
  });

  it('formats minutes', () => {
    expect(formatDuration(90)).toBe('1m 30s');
  });

  it('formats hours', () => {
    expect(formatDuration(21600)).toBe('6h');
  });

  it('returns Unavailable for negative values', () => {
    expect(formatDuration(-1)).toBe('Unavailable');
  });
});

describe('formatTimestamp', () => {
  it('formats a valid ISO timestamp', () => {
    const result = formatTimestamp('2026-09-14T08:00:00Z');
    expect(result).not.toBe('Unavailable');
    expect(result).toContain('2026');
  });

  it('returns Unavailable for undefined', () => {
    expect(formatTimestamp(undefined)).toBe('Unavailable');
  });

  it('returns Unavailable for invalid date', () => {
    expect(formatTimestamp('not-a-date')).toBe('Unavailable');
  });
});

describe('metricDisplay', () => {
  it('renders a present value using the formatter', () => {
    const result = metricDisplay({ value: 0.82 }, formatPercent);
    expect(result.text).toBe('82.0%');
    expect(result.unavailable).toBe(false);
  });

  it('renders Unavailable when value is missing', () => {
    const result = metricDisplay({ unavailable_reason: 'not observed' }, formatPercent);
    expect(result.text).toBe('Unavailable');
    expect(result.unavailable).toBe(true);
    expect(result.reason).toBe('not observed');
  });

  it('renders Unavailable when metric is undefined', () => {
    const result = metricDisplay(undefined, formatPercent);
    expect(result.text).toBe('Unavailable');
    expect(result.unavailable).toBe(true);
    expect(result.reason).toBe('not observed');
  });
});

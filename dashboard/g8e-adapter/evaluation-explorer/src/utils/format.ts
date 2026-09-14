// Formatting helpers for numbers, durations, and metric values.
// All formatters are pure and unit-tested. They never fabricate a value
// when data is missing; callers render UnavailableValue instead.

import type { MetricValue } from '../contract/types';

export function formatPercent(value: number, fractionDigits = 1): string {
  if (!Number.isFinite(value)) return 'Unavailable';
  return `${(value * 100).toFixed(fractionDigits)}%`;
}

export function formatNumber(value: number, fractionDigits = 0): string {
  if (!Number.isFinite(value)) return 'Unavailable';
  return value.toLocaleString('en-US', {
    minimumFractionDigits: fractionDigits,
    maximumFractionDigits: fractionDigits,
  });
}

export function formatCompact(value: number): string {
  if (!Number.isFinite(value)) return 'Unavailable';
  return new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 1 }).format(value);
}

export function formatLatency(ms: number): string {
  if (!Number.isFinite(ms)) return 'Unavailable';
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(2)} s`;
}

export function formatThroughput(tokensPerSecond: number): string {
  if (!Number.isFinite(tokensPerSecond)) return 'Unavailable';
  return `${formatCompact(tokensPerSecond)} tok/s`;
}

export function formatTokens(count: number): string {
  if (!Number.isFinite(count)) return 'Unavailable';
  return `${formatCompact(count)} tok`;
}

export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return 'Unavailable';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const minutes = Math.floor(seconds / 60);
  const remainder = Math.round(seconds % 60);
  if (minutes < 60) return remainder === 0 ? `${minutes}m` : `${minutes}m ${remainder}s`;
  const hours = Math.floor(minutes / 60);
  const mins = minutes % 60;
  return mins === 0 ? `${hours}h` : `${hours}h ${mins}m`;
}

export function formatTimestamp(iso: string | undefined): string {
  if (!iso) return 'Unavailable';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return 'Unavailable';
  return date.toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    timeZoneName: 'short',
  });
}

export function formatRelativeTime(iso: string | undefined, now: number = Date.now()): string {
  if (!iso) return 'Unavailable';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return 'Unavailable';
  const deltaSeconds = Math.round((now - date.getTime()) / 1000);
  if (deltaSeconds < 0) return 'in the future';
  if (deltaSeconds < 5) return 'just now';
  if (deltaSeconds < 60) return `${deltaSeconds}s ago`;
  if (deltaSeconds < 3600) return `${Math.floor(deltaSeconds / 60)}m ago`;
  if (deltaSeconds < 86400) return `${Math.floor(deltaSeconds / 3600)}h ago`;
  return `${Math.floor(deltaSeconds / 86400)}d ago`;
}

/** Resolve a MetricValue to a display string, or undefined when unavailable. */
export function metricDisplay(
  metric: MetricValue | undefined,
  formatter: (value: number) => string,
): { text: string; unavailable: boolean; reason?: string } {
  if (!metric) return { text: 'Unavailable', unavailable: true, reason: 'not observed' };
  if (metric.value === undefined) {
    return { text: 'Unavailable', unavailable: true, reason: metric.unavailable_reason };
  }
  return { text: formatter(metric.value), unavailable: false };
}

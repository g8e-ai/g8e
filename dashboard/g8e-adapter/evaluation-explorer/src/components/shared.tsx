// Shared UI components for the evaluation explorer. Each component is
// accessible, keyboard-navigable, and renders a real text label in addition
// to any color cue. No component depends on hover for essential information.

import { type ReactNode, useId } from 'react';
import type { ConfidenceInterval, MetricValue, QualityState } from '../contract/types';
import {
  classifyFreshness,
  emptyStateReason,
  qualityStateLabel,
  qualityStateTone,
  type FeedConnectionState,
  type FeedStatus,
} from '../utils/feed-state';
import { metricDisplay, formatPercent, formatNumber, formatLatency, formatThroughput, formatTokens, formatDuration, formatTimestamp, formatRelativeTime } from '../utils/format';
const toneClass: Record<string, string> = {
  ok: 'tone-ok',
  info: 'tone-info',
  warn: 'tone-warn',
  critical: 'tone-critical',
  neutral: 'tone-neutral',
};

export function QualityBadge({ state }: { state: QualityState }) {
  const tone = qualityStateTone(state);
  const label = qualityStateLabel(state);
  return (
    <span className={`quality-badge ${toneClass[tone]}`} data-testid={`quality-${state}`}>
      {label}
    </span>
  );
}

export function FreshnessBadge({ feedStatus }: { feedStatus: FeedStatus }) {
  const { label, tone } = classifyFreshness(feedStatus.freshness);
  const tooltipId = useId();
  return (
    <span className="freshness-badge-anchor">
      <span
        className={`freshness-badge ${toneClass[tone]}`}
        tabIndex={0}
        aria-describedby={tooltipId}
        data-testid={`freshness-${feedStatus.freshness}`}
      >
        <span className="freshness-dot" aria-hidden="true" />
        {label}
      </span>
      <div className="freshness-tooltip" id={tooltipId} role="tooltip">
        <p className="freshness-tooltip-message">{feedStatus.message}</p>
        {feedStatus.lastAcceptedAt ? (
          <p className="freshness-tooltip-detail">
            Last accepted: {formatTimestamp(feedStatus.lastAcceptedAt)}
          </p>
        ) : null}
        <p className="freshness-tooltip-detail">
          High-water sequence: {formatNumber(feedStatus.highWaterSequence)}
        </p>
      </div>
    </span>
  );
}

export function MetricCard({
  label,
  metric,
  formatter,
  hint,
}: {
  label: string;
  metric: MetricValue | undefined;
  formatter: (value: number) => string;
  hint?: string;
}) {
  const display = metricDisplay(metric, formatter);
  return (
    <div className="metric-card" data-testid={`metric-${label.replace(/\s+/g, '-').toLowerCase()}`}>
      <div className="metric-label">{label}</div>
      <div className={`metric-value${display.unavailable ? ' unavailable' : ''}`}>
        {display.text}
      </div>
      {display.unavailable && display.reason ? (
        <div className="metric-reason">{display.reason}</div>
      ) : null}
      {hint ? <div className="metric-hint">{hint}</div> : null}
    </div>
  );
}

export function IntervalDisplay({ interval, label }: { interval: ConfidenceInterval; label?: string }) {
  return (
    <span className="interval-display" data-testid="interval">
      <span className="interval-estimate">{formatPercent(interval.estimate)}</span>
      <span className="interval-range" aria-label={`${label ?? 'estimate'} confidence interval`}>
        [{formatPercent(interval.lower)}, {formatPercent(interval.upper)}]
      </span>
      <span className="interval-denominator" title="denominator">
        n={formatNumber(interval.denominator)}
      </span>
    </span>
  );
}

export function UnavailableValue({ reason }: { reason?: string }) {
  return (
    <span className="unavailable-value" title={reason ?? 'not observed'}>
      Unavailable
    </span>
  );
}

export function ProgressBar({ completed, total, label }: { completed: number; total: number; label?: string }) {
  const pct = total > 0 ? Math.min(100, (completed / total) * 100) : 0;
  const id = useId();
  return (
    <div className="progress" data-testid="progress">
      <div className="progress-label">
        <span>{label ?? 'Progress'}</span>
        <span className="progress-counts">
          {formatNumber(completed)} / {formatNumber(total)}
        </span>
      </div>
      <div
        className="progress-track"
        role="progressbar"
        aria-valuenow={completed}
        aria-valuemin={0}
        aria-valuemax={total}
        aria-label={label ?? 'Progress'}
        aria-describedby={id}
      >
        <div className="progress-fill" style={{ width: `${pct}%` }} />
      </div>
      <span id={id} className="sr-only">
        {formatNumber(completed)} of {formatNumber(total)} complete, {pct.toFixed(0)} percent.
      </span>
    </div>
  );
}

export function OutcomeCounts({
  outcomes,
}: {
  outcomes: Record<string, number> | undefined;
}) {
  if (!outcomes) return <UnavailableValue reason="no terminal outcomes recorded" />;
  const entries = Object.entries(outcomes).filter(([, count]) => count > 0);
  if (entries.length === 0) return <span className="outcome-counts none">No terminal outcomes</span>;
  return (
    <ul className="outcome-counts" data-testid="outcome-counts">
      {entries.map(([key, count]) => (
        <li key={key} className={`outcome-${key}`}>
          <span className="outcome-label">{key}</span>
          <span className="outcome-count">{formatNumber(count)}</span>
        </li>
      ))}
    </ul>
  );
}

export function EmptyState({
  hasRecords,
  hasFilters,
  connection,
}: {
  hasRecords: boolean;
  hasFilters: boolean;
  connection: FeedConnectionState;
}) {
  const reason = emptyStateReason(hasRecords, hasFilters, connection);
  return (
    <div className={`empty-state empty-${reason.kind}`} data-testid={`empty-${reason.kind}`}>
      <p>{reason.message}</p>
    </div>
  );
}

export function ErrorState({ message }: { message: string }) {
  return (
    <div className="error-state" role="alert" data-testid="error-state">
      <p>{message}</p>
    </div>
  );
}

/** Surfaces accumulated feed/validation errors so fail-closed rejections
 *  are visible rather than silently dropped. Dismissible; does not block
 *  rendering of the rest of the shell. */
export function ErrorBanner({ errors, onDismiss }: { errors: string[]; onDismiss: () => void }) {
  if (errors.length === 0) return null;
  return (
    <div className="error-banner" role="alert" data-testid="error-banner">
      <div className="error-banner-body">
        <span className="error-banner-title">Feed validation errors</span>
        <ul className="error-banner-list">
          {errors.map((message, i) => (
            <li key={i}>{message}</li>
          ))}
        </ul>
      </div>
      <button type="button" className="error-banner-dismiss" onClick={onDismiss} aria-label="Dismiss error banner">
        Dismiss
      </button>
    </div>
  );
}

export function Skeleton({ lines = 3 }: { lines?: number }) {
  return (
    <div className="skeleton" aria-hidden="true" data-testid="skeleton">
      {Array.from({ length: lines }).map((_, i) => (
        <div key={i} className="skeleton-line" style={{ width: `${70 + ((i * 13) % 30)}%` }} />
      ))}
    </div>
  );
}

export function Timeline({ events }: { events: Array<{ observed_at: string; kind: string; stage_label?: string; completed?: number; total?: number }> }) {
  if (events.length === 0) {
    return <EmptyState hasRecords={false} hasFilters={false} connection="live" />;
  }
  return (
    <ol className="timeline" data-testid="timeline">
      {events.map((event, i) => (
        <li key={`${event.kind}-${i}`} className="timeline-item">
          <span className="timeline-time">{formatRelativeTime(event.observed_at)}</span>
          <span className="timeline-kind">{event.kind.replace(/_/g, ' ')}</span>
          {event.stage_label ? <span className="timeline-stage">{event.stage_label}</span> : null}
          {event.total !== undefined && event.completed !== undefined ? (
            <span className="timeline-counts">
              {event.completed}/{event.total}
            </span>
          ) : null}
        </li>
      ))}
    </ol>
  );
}

export function SafeSourceLink({ href, label }: { href: string | undefined; label: string }) {
  if (!href) return null;
  return (
    <a className="safe-source-link" href={href} target="_blank" rel="noopener noreferrer">
      {label}
    </a>
  );
}

export function FilterControls({
  children,
}: {
  children: ReactNode;
}) {
  return <div className="filter-controls">{children}</div>;
}

export function SectionHeading({ kicker, title, description }: { kicker?: string; title: string; description?: string }) {
  return (
    <div className="section-heading">
      {kicker ? <p className="section-kicker">{kicker}</p> : null}
      <h2>{title}</h2>
      {description ? <p className="section-description">{description}</p> : null}
    </div>
  );
}

export function StatTile({ label, value, hint }: { label: string; value: ReactNode; hint?: string }) {
  return (
    <div className="stat-tile" data-testid={`stat-${label.replace(/\s+/g, '-').toLowerCase()}`}>
      <div className="stat-label">{label}</div>
      <div className="stat-value">{value}</div>
      {hint ? <div className="stat-hint">{hint}</div> : null}
    </div>
  );
}

export function DetailRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="detail-row">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

export { formatPercent, formatNumber, formatLatency, formatThroughput, formatTokens, formatDuration, formatTimestamp };

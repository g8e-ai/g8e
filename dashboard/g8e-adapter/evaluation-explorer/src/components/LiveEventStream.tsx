// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Live SSE event feed for the overview page. Newest events at the top inside a
// grid-matched scroll region so progress ticks do not move the page.

import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { FeedConnectionState, StreamConnectionState } from '../utils/feed-state';
import { recordKey, resolveModelSummary, useStoreState } from '../state/store';
import {
  assignmentMetricFormatter,
  roleLabel,
  streamProgressLabel,
  visibleStreamEvents,
} from '../views/derived';
import { EmptyState, ReconcilePlaceholder, StreamStatusIndicator } from './shared';
import type { AssignmentResult, LiveEvent, MetricValue } from '../contract/types';
import { LIVE_EVENT_RETENTION_LIMIT } from '../constants';
const STREAM_PAGE_SIZE = 25;
const STREAM_EMPTY_METRIC = '--';

function eventTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleTimeString('en-US', { hour12: false });
}

function assignmentMetricValues(event: LiveEvent, assignment: AssignmentResult | undefined): Record<string, MetricValue> {
  const values: Record<string, MetricValue> = {
    ...(event.metric_delta ?? {}),
    ...(assignment?.metric_values ?? {}),
  };
  const resources = assignment?.resource_summary;
  if (values.latency_ms === undefined && resources?.latency_ms) values.latency_ms = resources.latency_ms;
  if (values.input_tokens === undefined && resources?.input_tokens) values.input_tokens = resources.input_tokens;
  if (values.output_tokens === undefined && resources?.output_tokens) values.output_tokens = resources.output_tokens;
  if (values.thinking_tokens === undefined && resources?.thinking_tokens) values.thinking_tokens = resources.thinking_tokens;
  if (values.cache_tokens === undefined && resources?.cache_tokens) values.cache_tokens = resources.cache_tokens;
  if (values.retries === undefined && resources?.retries) values.retries = resources.retries;
  return values;
}

function displayLabel(value: string | undefined): string | undefined {
  return value?.replace(/_/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function eventStatus(event: LiveEvent): string | undefined {
  if (event.kind === 'assignment_started' && event.lifecycle_status === 'running') return 'started';
  return event.lifecycle_status;
}

function eventRoleStatus(event: LiveEvent): LiveEvent['lifecycle_status'] {
  if (event.kind === 'assignment_failed') return 'failed';
  if (event.kind === 'assignment_completed') return 'completed';
  if (event.lifecycle_status === 'queued') return 'running';
  return event.lifecycle_status;
}

type EventParts = {
  kind: string;
  status: string;
  category: string;
  task: string;
};

function eventParts(event: LiveEvent, assignment: AssignmentResult | undefined): EventParts {
  const stageParts = event.stage_label?.split(' · ').map((part) => part.trim()) ?? [];
  return {
    kind: displayLabel(event.kind) ?? '—',
    status: displayLabel(eventStatus(event)) ?? '—',
    category: displayLabel(assignment?.scenario_category) ?? stageParts[1] ?? '—',
    task: assignment?.task_id ?? event.task_id ?? stageParts[2] ?? '—',
  };
}

type AssignmentMetricColumn =
  | 'pass'
  | 'deterministic_pass_rate'
  | 'latency_ms'
  | 'input_tokens'
  | 'output_tokens'
  | 'thinking_tokens'
  | 'cache_tokens'
  | 'retries';
type StreamSortField = 'time' | 'role' | keyof EventParts | AssignmentMetricColumn | 'model' | 'progress';
type StreamSortDirection = 'asc' | 'desc';

const ASSIGNMENT_METRIC_COLUMNS: Array<{ key: AssignmentMetricColumn; label: string }> = [
  { key: 'pass', label: 'Pass' },
  { key: 'deterministic_pass_rate', label: 'Pass Rate' },
  { key: 'latency_ms', label: 'Latency' },
  { key: 'input_tokens', label: 'Input tokens' },
  { key: 'output_tokens', label: 'Output tokens' },
  { key: 'retries', label: 'Retries' },
];

function assignmentMetric(event: LiveEvent, assignment: AssignmentResult | undefined, key: AssignmentMetricColumn): MetricValue | undefined {
  return assignmentMetricValues(event, assignment)[key];
}

function streamMetricDisplay(
  metric: MetricValue | undefined,
  formatter: (value: number) => string,
): { text: string; unavailable: boolean; reason?: string } {
  if (!metric) {
    return { text: STREAM_EMPTY_METRIC, unavailable: true, reason: 'not observed' };
  }
  if (metric.value === undefined) {
    return {
      text: STREAM_EMPTY_METRIC,
      unavailable: true,
      reason: metric.unavailable_reason,
    };
  }
  return { text: formatter(metric.value), unavailable: false };
}

function streamSortValue(
  event: LiveEvent,
  assignment: AssignmentResult | undefined,
  model: ReturnType<typeof resolveModelSummary>,
  field: StreamSortField,
): string | number {
  if (field === 'time') return event.observed_at;
  if (field === 'role') return (event.role ? roleLabel(event.role) : model ? roleLabel(model.role) : 'Platform').toLowerCase();
  if (field === 'model') return (model?.served_model_tag ?? event.variant_id ?? event.kind.split('_')[0] ?? '').toLowerCase();
  if (field === 'progress') return event.total > 0 ? event.completed / event.total : '';
  const parts = eventParts(event, assignment);
  if (field in parts) return parts[field as keyof EventParts].toLowerCase();
  const metric = assignmentMetric(event, assignment, field as AssignmentMetricColumn);
  return String(metric?.value ?? metric?.unavailable_reason ?? '').toLowerCase();
}

function SortHeader({
  label,
  field,
  sortField,
  sortDirection,
  onSort,
}: {
  label: string;
  field: StreamSortField;
  sortField: StreamSortField | undefined;
  sortDirection: StreamSortDirection;
  onSort: (field: StreamSortField) => void;
}) {
  const isSorted = sortField === field;
  return (
    <th scope="col" aria-sort={isSorted ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}>
      <button type="button" className="th-sort stream-sort" onClick={() => onSort(field)} aria-label={`Sort by ${label}`}>
        {label}
        <span className="sort-indicator" aria-hidden="true">
          {isSorted ? (sortDirection === 'asc' ? '▲' : '▼') : '↕'}
        </span>
      </button>
    </th>
  );
}

export function LiveEventStream({
  events,
  connection,
  streamConnection,
  isReconciling = false,
  title = 'Live event stream',
  id = 'live-stream',
  className,
}: {
  events: LiveEvent[];
  connection: FeedConnectionState;
  streamConnection: StreamConnectionState;
  isReconciling?: boolean;
  title?: string;
  id?: string;
  className?: string;
}) {
  const [modelFilter, setModelFilter] = useState('all');
  const [kindFilter, setKindFilter] = useState('all');
  const [sortField, setSortField] = useState<StreamSortField>();
  const [sortDirection, setSortDirection] = useState<StreamSortDirection>('asc');
  const [page, setPage] = useState(0);
  const scrollRef = useRef<HTMLDivElement>(null);

  const models = useStoreState((state) => state.models);
  const assignments = useStoreState((state) => state.assignments);

  const visible = useMemo(
    () =>
      visibleStreamEvents(events, {
        modelFilter,
        kindFilter,
        limit: LIVE_EVENT_RETENTION_LIMIT,
      }),
    [events, modelFilter, kindFilter],
  );

  const modelOptions = useMemo(
    () =>
      Array.from(new Set(visible.map((e) => e.variant_id).filter((v): v is string => Boolean(v)))).sort(),
    [visible],
  );
  const kindOptions = useMemo(() => Array.from(new Set(visible.map((e) => e.kind))).sort(), [visible]);

  const sortedVisible = useMemo(() => {
    if (!sortField) return visible;
    return [...visible].sort((left, right) => {
      const leftAssignment = left.assignment_id
        ? assignments.get(recordKey(left.dataset_id, left.assignment_id))
        : undefined;
      const rightAssignment = right.assignment_id
        ? assignments.get(recordKey(right.dataset_id, right.assignment_id))
        : undefined;
      const leftModel = left.variant_id
        ? resolveModelSummary(models, left.dataset_id, left.variant_id)
        : undefined;
      const rightModel = right.variant_id
        ? resolveModelSummary(models, right.dataset_id, right.variant_id)
        : undefined;
      const leftValue = streamSortValue(left, leftAssignment, leftModel, sortField);
      const rightValue = streamSortValue(right, rightAssignment, rightModel, sortField);
      const comparison =
        typeof leftValue === 'number' && typeof rightValue === 'number'
          ? leftValue - rightValue
          : typeof leftValue === 'number'
            ? 1
            : typeof rightValue === 'number'
              ? -1
              : leftValue.localeCompare(rightValue, undefined, { numeric: true });
      return comparison === 0
        ? left.event_id.localeCompare(right.event_id)
        : sortDirection === 'asc'
          ? comparison
          : -comparison;
    });
  }, [visible, assignments, models, sortField, sortDirection]);

  const pageCount = Math.ceil(sortedVisible.length / STREAM_PAGE_SIZE);
  const safePage = Math.min(page, Math.max(0, pageCount - 1));
  const pageEvents = useMemo(() => {
    const start = safePage * STREAM_PAGE_SIZE;
    return sortedVisible.slice(start, start + STREAM_PAGE_SIZE);
  }, [sortedVisible, safePage]);

  function handleSort(field: StreamSortField): void {
    if (sortField === field) {
      setSortDirection((direction) => (direction === 'asc' ? 'desc' : 'asc'));
      return;
    }
    setSortField(field);
    setSortDirection('asc');
  }

  useEffect(() => {
    setPage(0);
  }, [modelFilter, kindFilter, sortField, sortDirection]);

  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
  }, [safePage]);

  return (
    <section className={`panel stream-panel${className ? ` ${className}` : ''}`} id={id} aria-label={title}>
      <div className="panel-head">
        <h2>
          {title}{' '}
          <StreamStatusIndicator
            streamConnection={streamConnection}
            feedConnection={connection}
            detail="long"
          />
        </h2>
        <div className="stream-controls">
          <select
            aria-label="Filter by model"
            value={modelFilter}
            onChange={(e) => setModelFilter(e.target.value)}
          >
            <option value="all">All models</option>
            {modelOptions.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </select>
          <select
            aria-label="Filter by event kind"
            value={kindFilter}
            onChange={(e) => setKindFilter(e.target.value)}
          >
            <option value="all">All events</option>
            {kindOptions.map((kind) => (
              <option key={kind} value={kind}>
                {kind.replace(/_/g, ' ')}
              </option>
            ))}
          </select>
        </div>
      </div>
      {isReconciling ? <p className="panel-note">Syncing feed history…</p> : null}
      {visible.length === 0 ? (
        isReconciling ? (
          <ReconcilePlaceholder label="Loading feed history…" />
        ) : (
          <EmptyState
            hasRecords={events.length > 0}
            hasFilters={modelFilter !== 'all' || kindFilter !== 'all'}
            connection={connection}
          />
        )
      ) : (
        <div className="stream-scroll" ref={scrollRef}>
          <table className="stream-table">
            <thead>
              <tr>
                <SortHeader label="Time" field="time" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                <SortHeader label="Role" field="role" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                <SortHeader label="Event" field="kind" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                <SortHeader label="Status" field="status" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                <SortHeader label="Category" field="category" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                <SortHeader label="Task" field="task" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                {ASSIGNMENT_METRIC_COLUMNS.map(({ key, label }) => (
                  <SortHeader key={key} label={label} field={key} sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                ))}
                <SortHeader label="Model" field="model" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                <SortHeader label="Progress" field="progress" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
              </tr>
            </thead>
            <tbody>
              {pageEvents.map((event) => {
                const model = event.variant_id
                  ? resolveModelSummary(models, event.dataset_id, event.variant_id)
                  : undefined;
                const servedTag = model?.served_model_tag ?? event.variant_id ?? event.kind.split('_')[0];
                const assignment = event.assignment_id
                  ? assignments.get(recordKey(event.dataset_id, event.assignment_id))
                  : undefined;
                const parts = eventParts(event, assignment);
                const eventHref = event.assignment_id
                  ? `/evaluations/${event.dataset_id}/${event.run_id}/assignments/${event.assignment_id}`
                  : `/evaluations/${event.dataset_id}/${event.run_id}`;
                const taskHref =
                  parts.task !== '—' ? `/tasks/${encodeURIComponent(parts.task)}` : undefined;
                const categoryHref = assignment?.scenario_category
                  ? `/tasks#category-${assignment.scenario_category}`
                  : undefined;
                const modelHref = event.variant_id
                  ? `/models/${event.dataset_id}/${event.variant_id}${model?.role ? `?role=${model.role}` : ''}`
                  : undefined;
                const roleLabelText = event.role
                  ? roleLabel(event.role)
                  : model
                    ? roleLabel(model.role)
                    : 'Platform';
                return (
                  <tr key={event.event_id}>
                    <td className="stream-time">{eventTime(event.observed_at)}</td>
                    <td>
                      <span className={`stream-role status-${eventRoleStatus(event)}`}>
                        {roleLabelText}
                      </span>
                    </td>
                    <td className="stream-event-value">
                      <Link to={eventHref} className="stream-event-link">
                        {parts.kind}
                      </Link>
                    </td>
                    <td>{parts.status}</td>
                    <td>
                      {categoryHref ? (
                        <Link to={categoryHref} className="stream-category-link">
                          {parts.category}
                        </Link>
                      ) : (
                        parts.category
                      )}
                    </td>
                    <td>
                      {taskHref ? (
                        <Link to={taskHref} className="stream-task-link">
                          {parts.task}
                        </Link>
                      ) : (
                        parts.task
                      )}
                    </td>
                    {ASSIGNMENT_METRIC_COLUMNS.map(({ key }) => {
                      const displayed = !event.assignment_id
                        ? { text: STREAM_EMPTY_METRIC, unavailable: true }
                        : streamMetricDisplay(assignmentMetric(event, assignment, key), assignmentMetricFormatter(key));
                      return (
                        <td className="stream-metric-cell" key={key}>
                          <span className={displayed.unavailable ? 'stream-metric-value unavailable' : 'stream-metric-value'}>
                            {displayed.text}
                          </span>
                        </td>
                      );
                    })}
                    <td>
                      {modelHref ? (
                        <Link to={modelHref} className="stream-chip stream-chip-link">
                          {servedTag}
                        </Link>
                      ) : (
                        <code className="stream-chip">{servedTag}</code>
                      )}
                    </td>
                    <td className="stream-progress">{streamProgressLabel(event)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {visible.length > 0 && pageCount > 1 ? (
        <div className="table-pagination" data-testid="stream-pagination">
          <button
            type="button"
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={safePage === 0}
            aria-label="Previous page"
          >
            Previous
          </button>
          <span className="page-info">
            Page {safePage + 1} of {pageCount} ({visible.length} events)
          </span>
          <button
            type="button"
            onClick={() => setPage((p) => Math.min(pageCount - 1, p + 1))}
            disabled={safePage >= pageCount - 1}
            aria-label="Next page"
          >
            Next
          </button>
        </div>
      ) : null}
    </section>
  );
}

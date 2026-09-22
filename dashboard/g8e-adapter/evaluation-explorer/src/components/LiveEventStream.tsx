// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Live SSE event feed for the overview page. Newest events at the top inside a
// grid-matched scroll region so progress ticks do not move the page.

import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { FeedConnectionState, StreamConnectionState } from '../utils/feed-state';
import { recordKey, resolveModelSummary, useStoreState } from '../state/store';
import {
  assignmentMetricEntries,
  assignmentMetricFormatter,
  roleLabel,
  streamProgressLabel,
  visibleStreamEvents,
} from '../views/derived';
import { EmptyState, ReconcilePlaceholder, StreamStatusIndicator } from './shared';
import type { AssignmentResult, LiveEvent, MetricValue } from '../contract/types';
import { LIVE_EVENT_RETENTION_LIMIT } from '../constants';
import { metricDisplay } from '../utils/format';

const STREAM_PAGE_SIZE = 25;

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
  return values;
}

function displayLabel(value: string | undefined): string | undefined {
  return value?.replace(/_/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function eventLabel(event: LiveEvent): string {
  return `${event.kind.replace(/_/g, ' ')}${event.stage_label ? ` · ${event.stage_label}` : ''}`;
}

function assignmentSortLabel(event: LiveEvent, assignment: AssignmentResult | undefined): string {
  const metrics = Object.entries(assignmentMetricValues(event, assignment))
    .map(([key, metric]) => `${key}:${metric.value ?? metric.unavailable_reason ?? ''}`)
    .join(' ');
  return [assignment?.task_id ?? event.task_id ?? event.assignment_id ?? '', assignment?.scenario_category ?? '', metrics]
    .join(' ')
    .toLowerCase();
}

type StreamSortField = 'event' | 'assignment';
type StreamSortDirection = 'asc' | 'desc';

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

function AssignmentDetails({ event, assignment }: { event: LiveEvent; assignment?: AssignmentResult }) {
  if (!event.assignment_id) return <span className="stream-unavailable">—</span>;

  const metricEntries = assignmentMetricEntries(assignmentMetricValues(event, assignment)).slice(0, 4);
  return (
    <div className="stream-assignment-details">
      <Link
        to={`/evaluations/${event.dataset_id}/${event.run_id}/assignments/${event.assignment_id}`}
        className="stream-assignment-id"
      >
        {assignment?.task_id ?? event.task_id ?? event.assignment_id}
      </Link>
      {assignment?.scenario_category ? (
        <span className="stream-assignment-category">{displayLabel(assignment.scenario_category)}</span>
      ) : null}
      {metricEntries.length > 0 ? (
        <div className="stream-metrics" aria-label="Assignment metrics">
          {metricEntries.map(({ key, label, metric }) => {
            const displayed = metricDisplay(metric, assignmentMetricFormatter(key));
            return (
              <span className="stream-metric" key={key}>
                <span className="stream-metric-label">{label}</span>
                <span className={displayed.unavailable ? 'stream-metric-value unavailable' : 'stream-metric-value'}>
                  {displayed.text}
                </span>
              </span>
            );
          })}
        </div>
      ) : null}
    </div>
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
      const leftValue = sortField === 'event' ? eventLabel(left) : assignmentSortLabel(left, leftAssignment);
      const rightValue = sortField === 'event' ? eventLabel(right) : assignmentSortLabel(right, rightAssignment);
      const comparison = leftValue.localeCompare(rightValue);
      return comparison === 0
        ? left.event_id.localeCompare(right.event_id)
        : sortDirection === 'asc'
          ? comparison
          : -comparison;
    });
  }, [visible, assignments, sortField, sortDirection]);

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
                <th scope="col">Time</th>
                <th scope="col">Role</th>
                <SortHeader
                  label="Event"
                  field="event"
                  sortField={sortField}
                  sortDirection={sortDirection}
                  onSort={handleSort}
                />
                <SortHeader
                  label="Assignment details"
                  field="assignment"
                  sortField={sortField}
                  sortDirection={sortDirection}
                  onSort={handleSort}
                />
                <th scope="col">Model</th>
                <th scope="col">Progress</th>
              </tr>
            </thead>
            <tbody>
              {pageEvents.map((event) => {
                const model = event.variant_id
                  ? resolveModelSummary(models, event.dataset_id, event.variant_id)
                  : undefined;
                const servedTag = model?.served_model_tag ?? event.variant_id ?? event.kind.split('_')[0];
                const eventText = eventLabel(event);
                const eventHref = event.assignment_id
                  ? `/evaluations/${event.dataset_id}/${event.run_id}/assignments/${event.assignment_id}`
                  : `/evaluations/${event.dataset_id}/${event.run_id}`;
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
                      <span className={`stream-role status-${event.lifecycle_status}`}>
                        <span className="status-dot" aria-hidden="true" />
                        {roleLabelText}
                      </span>
                    </td>
                    <td className="stream-event">
                      <Link to={eventHref} className="stream-event-link">
                        {eventText}
                      </Link>
                    </td>
                    <td>
                      <AssignmentDetails
                        event={event}
                        assignment={
                          event.assignment_id
                            ? assignments.get(recordKey(event.dataset_id, event.assignment_id))
                            : undefined
                        }
                      />
                    </td>
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

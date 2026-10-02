// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Live SSE event feed for the overview page. Newest events at the top inside a
// grid-matched scroll region so progress ticks do not move the page.

import { Fragment, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { FeedConnectionState, StreamConnectionState } from '../utils/feed-state';
import { recordKey, resolveModelSummary, useStoreState } from '../state/store';
import {
  abbreviateAssignmentId,
  assignmentMetricFormatter,
  roleLabel,
  streamProgressLabel,
  visibleStreamEvents,
} from '../views/derived';
import { EmptyState, ReconcilePlaceholder, StreamStatusIndicator } from './shared';
import { SCENARIO_TASK_BY_ID } from '../content/scenario-task';
import {
  MODEL_ROLES,
  type AssignmentResult,
  type LiveEvent,
  type MetricValue,
  type ModelRole,
  type PublicModelActivityRecord,
} from '../contract/types';
import { LIVE_EVENT_RETENTION_LIMIT } from '../constants';
const STREAM_PAGE_SIZE = 25;
const STREAM_EMPTY_METRIC = '--';

function eventTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleTimeString('en-US', { hour12: false });
}

function isAssignmentLevelEvent(event: LiveEvent): boolean {
  return (
    event.kind === 'assignment_started' ||
    event.kind === 'assignment_completed' ||
    event.kind === 'assignment_failed' ||
    event.kind === 'metric_updated'
  );
}

function normalizeMetricDelta(delta: Record<string, MetricValue> | undefined): Record<string, MetricValue> {
  if (!delta) return {};
  const values: Record<string, MetricValue> = { ...delta };
  const passRate = values.pass_rate;
  if (passRate !== undefined) {
    if (values.deterministic_pass_rate === undefined) {
      values.deterministic_pass_rate = passRate;
    }
    if (values.pass === undefined) {
      if (passRate.value !== undefined) {
        values.pass = { value: passRate.value >= 1 ? 1 : 0 };
      } else if (passRate.unavailable_reason) {
        values.pass = { unavailable_reason: passRate.unavailable_reason };
      }
    }
    delete values.pass_rate;
  }
  return values;
}

function findModelActivityRecord(
  event: LiveEvent,
  assignment: AssignmentResult | undefined,
): PublicModelActivityRecord | undefined {
  const modelActivity = assignment?.activity_summary?.model_activity;
  const records = modelActivity?.availability === 'observed' ? modelActivity.records : undefined;
  if (!records?.length || !event.role) return undefined;
  const roleMatches = records.filter((record) => record.model_role === event.role);
  if (roleMatches.length === 0) return undefined;
  if (event.variant_id) {
    const exact = roleMatches.find((record) => record.variant_id === event.variant_id);
    if (exact) return exact;
  }
  return roleMatches[0];
}

function roleActivityMetricValues(record: PublicModelActivityRecord): Record<string, MetricValue> {
  const values: Record<string, MetricValue> = {};
  if (record.input_tokens) values.input_tokens = record.input_tokens;
  if (record.output_tokens) values.output_tokens = record.output_tokens;
  if (record.thinking_tokens) values.thinking_tokens = record.thinking_tokens;
  if (record.cache_tokens) values.cache_tokens = record.cache_tokens;
  if (record.retry_count) values.retries = record.retry_count;
  if (record.total_duration_nanos?.value !== undefined) {
    values.latency_ms = { value: record.total_duration_nanos.value / 1_000_000 };
  } else if (record.total_duration_nanos?.unavailable_reason) {
    values.latency_ms = { unavailable_reason: record.total_duration_nanos.unavailable_reason };
  }
  return values;
}

function applyAssignmentResourceSummary(
  values: Record<string, MetricValue>,
  assignment: AssignmentResult | undefined,
): Record<string, MetricValue> {
  const resources = assignment?.resource_summary;
  if (!resources) return values;
  if (values.latency_ms === undefined && resources.latency_ms) values.latency_ms = resources.latency_ms;
  if (values.input_tokens === undefined && resources.input_tokens) values.input_tokens = resources.input_tokens;
  if (values.output_tokens === undefined && resources.output_tokens) values.output_tokens = resources.output_tokens;
  if (values.thinking_tokens === undefined && resources.thinking_tokens) values.thinking_tokens = resources.thinking_tokens;
  if (values.cache_tokens === undefined && resources.cache_tokens) values.cache_tokens = resources.cache_tokens;
  if (values.retries === undefined && resources.retries) values.retries = resources.retries;
  return values;
}

/** Whether a stage row is the assignment's designated (graded) role or another
 *  role of the same g8ee chat turn. Unknown until the designated role is known. */
type StreamRoleGrading = 'graded' | 'chain';

function isTerminalAssignment(assignment: AssignmentResult | undefined): assignment is AssignmentResult {
  return assignment !== undefined && assignment.terminal_status !== 'running' && assignment.terminal_status !== 'queued';
}

/** Designated role per assignment, from assignment-level events (they carry the
 *  designated role) so rows are classified before the assignment result lands. */
function designatedRolesByAssignment(events: LiveEvent[]): Map<string, ModelRole> {
  const designated = new Map<string, ModelRole>();
  for (const event of events) {
    if (event.assignment_id && event.role && isAssignmentLevelEvent(event) && event.kind !== 'metric_updated') {
      designated.set(`${event.run_id}:${event.assignment_id}`, event.role);
    }
  }
  return designated;
}

function streamRoleGrading(
  event: LiveEvent,
  assignment: AssignmentResult | undefined,
  designatedRoles: Map<string, ModelRole>,
): StreamRoleGrading | undefined {
  if (event.kind !== 'stage_updated' || !event.role || !event.assignment_id) return undefined;
  const designated = assignment?.role ?? designatedRoles.get(`${event.run_id}:${event.assignment_id}`);
  if (!designated) return undefined;
  return event.role === designated ? 'graded' : 'chain';
}

/** Pass and Pass Rate for a stage row, available once the assignment is terminal.
 *  The designated role carries the assignment verdict; a chain role carries the
 *  share of its own graded players that passed (`tier_<role>`), and stays absent
 *  when the scenario declares no player gold for that tier. */
function roleGradeMetricValues(event: LiveEvent, assignment: AssignmentResult | undefined): Record<string, MetricValue> {
  if (event.kind !== 'stage_updated' || !event.role || !isTerminalAssignment(assignment)) return {};
  const metrics = assignment.metric_values;
  if (event.role === assignment.role) {
    const values: Record<string, MetricValue> = {};
    if (metrics.pass) values.pass = metrics.pass;
    if (metrics.deterministic_pass_rate) values.deterministic_pass_rate = metrics.deterministic_pass_rate;
    return values;
  }
  const tier = metrics[`tier_${event.role}`];
  if (tier?.value === undefined) return {};
  return {
    pass: { value: tier.value >= 1 ? 1 : 0 },
    deterministic_pass_rate: { value: tier.value },
  };
}

function assignmentMetricValues(event: LiveEvent, assignment: AssignmentResult | undefined): Record<string, MetricValue> {
  const normalizedDelta = normalizeMetricDelta(event.metric_delta);
  if (isAssignmentLevelEvent(event)) {
    return applyAssignmentResourceSummary(
      {
        ...normalizedDelta,
        ...(assignment?.metric_values ?? {}),
      },
      assignment,
    );
  }

  const roleRecord = findModelActivityRecord(event, assignment);
  return {
    ...normalizedDelta,
    ...(roleRecord ? roleActivityMetricValues(roleRecord) : {}),
    ...roleGradeMetricValues(event, assignment),
  };
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
  const isModelRoleInvocation = stageParts[0]?.toLowerCase() === 'model role invoked';

  const secondPart = stageParts[1];
  const task =
    assignment?.task_id ??
    event.task_id ??
    (isModelRoleInvocation
      ? stageParts[3]
      : stageParts.length > 2
        ? stageParts[2]
        : stageParts.length === 2 && secondPart !== undefined && SCENARIO_TASK_BY_ID.has(secondPart)
          ? secondPart
          : undefined) ??
    '—';

  let fallbackCategory: string | undefined;
  if (!isModelRoleInvocation && secondPart !== undefined) {
    const isRole = (MODEL_ROLES as readonly string[]).includes(secondPart.toLowerCase());
    const isTask = secondPart === task || SCENARIO_TASK_BY_ID.has(secondPart);
    if (!isRole && !isTask) {
      fallbackCategory = secondPart;
    }
  }

  const rawCategory =
    assignment?.scenario_category ??
    (task !== '—' ? SCENARIO_TASK_BY_ID.get(task)?.category : undefined) ??
    fallbackCategory;

  return {
    kind: displayLabel(event.kind) ?? '—',
    status: displayLabel(eventStatus(event)) ?? '—',
    category: displayLabel(rawCategory) ?? '—',
    task,
  };
}

type AssignmentMetricColumn =
  | 'pass'
  | 'deterministic_pass_rate'
  | 'latency_ms'
  | 'input_tokens'
  | 'output_tokens'
  | 'tokens_per_second'
  | 'thinking_tokens'
  | 'cache_tokens'
  | 'retries';
type StreamSortField = 'time' | 'role' | 'assignment' | keyof EventParts | AssignmentMetricColumn | 'model' | 'progress';
type StreamSortDirection = 'asc' | 'desc';

const ASSIGNMENT_METRIC_COLUMNS: Array<{ key: AssignmentMetricColumn; label: string }> = [
  { key: 'pass', label: 'Pass' },
  { key: 'deterministic_pass_rate', label: 'Pass Rate' },
  { key: 'latency_ms', label: 'Latency' },
  { key: 'input_tokens', label: 'Input tokens' },
  { key: 'output_tokens', label: 'Output tokens' },
  { key: 'tokens_per_second', label: 'Tokens/s' },
  { key: 'retries', label: 'Retries' },
];

function assignmentTokensPerSecond(
  event: LiveEvent,
  assignment: AssignmentResult | undefined,
): MetricValue | undefined {
  const roleRecord = !isAssignmentLevelEvent(event) ? findModelActivityRecord(event, assignment) : undefined;
  if (roleRecord?.output_tokens?.value !== undefined && roleRecord.generation_duration_nanos?.value !== undefined) {
    const generationSeconds = roleRecord.generation_duration_nanos.value / 1_000_000_000;
    if (generationSeconds > 0) {
      return { value: roleRecord.output_tokens.value / generationSeconds };
    }
  }

  const values = assignmentMetricValues(event, assignment);
  const output = values.output_tokens?.value;
  const generationMs = assignment?.benchmark_observations?.timing?.generation_ms?.value;
  if (output === undefined || generationMs === undefined || generationMs <= 0) return undefined;
  return { value: output / (generationMs / 1000) };
}

function assignmentMetric(event: LiveEvent, assignment: AssignmentResult | undefined, key: AssignmentMetricColumn): MetricValue | undefined {
  if (key === 'tokens_per_second') {
    return assignmentTokensPerSecond(event, assignment);
  }
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
  if (field === 'assignment') return (event.assignment_id ?? '').toLowerCase();
  if (field === 'model') return (model?.served_model_tag ?? event.variant_id ?? event.kind.split('_')[0] ?? '').toLowerCase();
  if (field === 'progress') return event.total > 0 ? event.completed / event.total : '';
  const parts = eventParts(event, assignment);
  if (field in parts) return parts[field as keyof EventParts].toLowerCase();
  const metric = assignmentMetric(event, assignment, field as AssignmentMetricColumn);
  if (field === 'latency_ms' && metric?.value === undefined && event.kind === 'assignment_started') {
    const obsTime = new Date(event.observed_at).getTime();
    if (!Number.isNaN(obsTime)) {
      return Math.max(0, Date.now() - obsTime);
    }
  }
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
  title = 'Events',
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
  const [categoryFilter, setCategoryFilter] = useState('all');
  const [assignmentFilter, setAssignmentFilter] = useState('all');
  const [runScope, setRunScope] = useState<'all' | 'active'>('all');
  const [sortField, setSortField] = useState<StreamSortField>();
  const [sortDirection, setSortDirection] = useState<StreamSortDirection>('asc');
  const [page, setPage] = useState(0);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const scrollRef = useRef<HTMLDivElement>(null);

  const models = useStoreState((state) => state.models);
  const assignments = useStoreState((state) => state.assignments);

  useEffect(() => {
    const hasRunning = events.some(
      (e) =>
        e.kind === 'assignment_started' &&
        (e.lifecycle_status === 'running' || e.lifecycle_status === 'queued'),
    );
    if (!hasRunning) return;
    const interval = setInterval(() => {
      setNowMs(Date.now());
    }, 1000);
    return () => clearInterval(interval);
  }, [events]);

  const activeRunId = useMemo(() => {
    if (events.length === 0) return undefined;
    for (let i = events.length - 1; i >= 0; i--) {
      const e = events[i];
      if (e && (e.lifecycle_status === 'running' || e.kind === 'assignment_started')) {
        return e.run_id;
      }
    }
    let newest = events[0];
    if (!newest) return undefined;
    for (let i = 1; i < events.length; i++) {
      const candidate = events[i];
      if (candidate && candidate.observed_at.localeCompare(newest.observed_at) > 0) {
        newest = candidate;
      }
    }
    return newest.run_id;
  }, [events]);

  const targetEvents = useMemo(() => {
    if (runScope === 'active' && activeRunId) {
      return events.filter((e) => e.run_id === activeRunId);
    }
    return events;
  }, [events, runScope, activeRunId]);

  const visibleBase = useMemo(
    () =>
      visibleStreamEvents(targetEvents, {
        modelFilter,
        kindFilter,
        assignmentFilter,
        limit: runScope === 'active' ? undefined : LIVE_EVENT_RETENTION_LIMIT,
      }),
    [targetEvents, modelFilter, kindFilter, assignmentFilter, runScope],
  );

  const visible = useMemo(() => {
    if (categoryFilter === 'all') return visibleBase;
    return visibleBase.filter((event) => {
      const assignment = event.assignment_id
        ? assignments.get(recordKey(event.dataset_id, event.assignment_id))
        : undefined;
      const parts = eventParts(event, assignment);
      return parts.category.toLowerCase() === categoryFilter.toLowerCase();
    });
  }, [visibleBase, categoryFilter, assignments]);

  const modelOptions = useMemo(
    () =>
      Array.from(new Set(visible.map((e) => e.variant_id).filter((v): v is string => Boolean(v)))).sort(),
    [visible],
  );
  const kindOptions = useMemo(() => Array.from(new Set(visible.map((e) => e.kind))).sort(), [visible]);
  const categoryOptions = useMemo(() => {
    const categories = new Set<string>();
    for (const event of events) {
      const assignment = event.assignment_id
        ? assignments.get(recordKey(event.dataset_id, event.assignment_id))
        : undefined;
      const parts = eventParts(event, assignment);
      if (parts.category && parts.category !== '—') {
        categories.add(parts.category);
      }
    }
    return Array.from(categories).sort();
  }, [events, assignments]);
  const assignmentOptions = useMemo(() => {
    const ids = new Set<string>();
    for (const event of events) {
      if (event.assignment_id) {
        ids.add(event.assignment_id);
      }
    }
    return Array.from(ids).sort();
  }, [events]);

  const designatedRoles = useMemo(() => designatedRolesByAssignment(events), [events]);

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
  }, [modelFilter, kindFilter, categoryFilter, assignmentFilter, runScope, sortField, sortDirection]);

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
            showLabel={false}
          />
        </h2>
        <div className="stream-controls">
          <select
            aria-label="Filter by run scope"
            value={runScope}
            onChange={(e) => setRunScope(e.target.value as 'all' | 'active')}
          >
            <option value="all">All runs (last {LIVE_EVENT_RETENTION_LIMIT})</option>
            <option value="active">Active run only{activeRunId ? ` (${activeRunId})` : ''}</option>
          </select>
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
          <select
            aria-label="Filter by category"
            value={categoryFilter}
            onChange={(e) => setCategoryFilter(e.target.value)}
          >
            <option value="all">All categories</option>
            {categoryOptions.map((cat) => (
              <option key={cat} value={cat}>
                {cat}
              </option>
            ))}
          </select>
          <select
            aria-label="Filter by assignment"
            title="Assignment"
            value={assignmentFilter}
            onChange={(e) => setAssignmentFilter(e.target.value)}
          >
            <option value="all">All assignments</option>
            {assignmentOptions.map((id) => (
              <option key={id} value={id}>
                {id}
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
            hasFilters={modelFilter !== 'all' || kindFilter !== 'all' || categoryFilter !== 'all' || assignmentFilter !== 'all' || runScope !== 'all'}
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
                <SortHeader label="Assignment" field="assignment" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                {ASSIGNMENT_METRIC_COLUMNS.map(({ key, label }) => (
                  <SortHeader key={key} label={label} field={key} sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                ))}
                <SortHeader label="Model" field="model" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
                <SortHeader label="Progress" field="progress" sortField={sortField} sortDirection={sortDirection} onSort={handleSort} />
              </tr>
            </thead>
            <tbody>
              {pageEvents.map((event, index) => {
                const prevEvent = index > 0 ? pageEvents[index - 1] : undefined;
                const isNewRun = prevEvent !== undefined && prevEvent.run_id !== event.run_id;
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
                const roleGrading = streamRoleGrading(event, assignment, designatedRoles);
                const roleLabelText = event.role
                  ? roleLabel(event.role)
                  : model
                    ? roleLabel(model.role)
                    : 'Platform';
                return (
                  <Fragment key={event.event_id}>
                    {isNewRun ? (
                      <tr className="stream-run-divider-row" data-testid={`run-divider-${event.run_id}`}>
                        <td colSpan={16} className="stream-run-divider-cell">
                          <div className="stream-run-divider">
                            <span className="stream-run-divider-badge">Run: {event.run_id}</span>
                            <span className="stream-run-divider-line" />
                          </div>
                        </td>
                      </tr>
                    ) : null}
                    <tr className={roleGrading === 'chain' ? 'stream-row-chain' : undefined}>
                      <td className="stream-time">{eventTime(event.observed_at)}</td>
                      <td>
                        <span className={`stream-role status-${eventRoleStatus(event)}`}>
                          {roleLabelText}
                        </span>
                        {roleGrading === 'graded' ? (
                          <span className="stream-role-tag graded" title="Designated role: the assignment verdict grades this role">
                            Graded
                          </span>
                        ) : roleGrading === 'chain' ? (
                          <span
                            className="stream-role-tag chain"
                            title="Chain role: runs in the same g8ee turn as the graded role. Its Pass is the share of its own graded players that passed."
                          >
                            Chain
                          </span>
                        ) : null}
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
                      <td>
                        {event.assignment_id ? (
                          <Link
                            to={`/evaluations/${event.dataset_id}/${event.run_id}/assignments/${event.assignment_id}`}
                            className="stream-assignment-link"
                            title={event.assignment_id}
                          >
                            {abbreviateAssignmentId(event.assignment_id)}
                          </Link>
                        ) : (
                          '—'
                        )}
                      </td>
                      {ASSIGNMENT_METRIC_COLUMNS.map(({ key }) => {
                        const isRunningAssignment =
                          event.kind === 'assignment_started' &&
                          (event.lifecycle_status === 'running' || event.lifecycle_status === 'queued');
                        let displayed = !event.assignment_id
                          ? { text: STREAM_EMPTY_METRIC, unavailable: true }
                          : streamMetricDisplay(assignmentMetric(event, assignment, key), assignmentMetricFormatter(key));

                        let isElapsed = false;
                        if (key === 'latency_ms' && displayed.unavailable && isRunningAssignment) {
                          const obsTime = new Date(event.observed_at).getTime();
                          if (!Number.isNaN(obsTime)) {
                            const elapsedSec = Math.max(0, Math.floor((nowMs - obsTime) / 1000));
                            displayed = { text: `${elapsedSec}s…`, unavailable: false };
                            isElapsed = true;
                          }
                        }

                        return (
                          <td className="stream-metric-cell" key={key}>
                            <span
                              className={
                                isElapsed
                                  ? 'stream-metric-value stream-metric-elapsed'
                                  : displayed.unavailable
                                    ? 'stream-metric-value unavailable'
                                    : 'stream-metric-value'
                              }
                              title={isElapsed ? 'Active assignment duration' : undefined}
                            >
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
                  </Fragment>
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

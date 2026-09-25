// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Assignment detail view. Shows task ID, variant, role, repetition,
// lifecycle status, eligible metric values, missingness reason, safe stage
// names and durations, resource observations, and verification disposition.
// Never shows raw prompts, outputs, trails, or credentials.

import { Link, useParams, useSearchParams } from 'react-router-dom';
import type { AssignmentResult } from '../contract/types';
import { AssignmentActivitySummary } from '../components/AssignmentActivitySummary';
import { EvidenceBindingsPanel } from '../components/EvidenceBindingsPanel';
import { ScenarioContextCard } from '../components/ScenarioContextCard';
import { useActiveDatasetId } from '../state/dataset';
import { recordKey, useStoreState } from '../state/store';
import {
  EmptyState,
  ErrorState,
  SectionHeading,
  formatDuration,
  formatLatency,
  formatThroughput,
  formatTokens,
  formatNumber,
} from '../components/shared';
import { formatRelativeTime } from '../utils/format';
import {
  assignmentGradeChips,
  assignmentLifecycleEvents,
  assignmentVerdictLabel,
  isScenarioNotApplicableMetric,
  roleLabel,
  resourceObservationMissing,
  siblingRepetitions,
  streamProgressLabel,
} from './derived';

function observationLabel(value: string): string {
  return value.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

function formatBytes(value: number): string {
  if (value >= 1024 ** 3) return `${formatNumber(value / 1024 ** 3, 1)} GiB`;
  if (value >= 1024 ** 2) return `${formatNumber(value / 1024 ** 2, 1)} MiB`;
  return `${formatNumber(value)} B`;
}

function metricValue(metric: { value?: number } | undefined): number | undefined {
  return metric?.value;
}

function terminalLabel(status: AssignmentResult['terminal_status']): string {
  return status.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

function hasObservedActivity(activity: AssignmentResult['activity_summary']): boolean {
  return Boolean(
    activity &&
      Object.values(activity).some(
        (family) => family?.availability === 'observed' && family.records.length > 0,
      ),
  );
}

export function AssignmentDetailView() {
  const { assignmentId, runId, datasetId: routeDataset } = useParams();
  const [params] = useSearchParams();
  const activeDatasetId = useActiveDatasetId(routeDataset ?? params.get('dataset') ?? undefined);

  const assignment = useStoreState((state) =>
    assignmentId ? state.assignments.get(recordKey(activeDatasetId, assignmentId)) : undefined,
  );
  const lifecycleEvents = useStoreState((state) =>
    assignmentId
      ? assignmentLifecycleEvents(
          assignmentId,
          state.events.filter((event) => event.dataset_id === activeDatasetId && event.run_id === runId),
        )
      : [],
  );
  const siblingAssignments = useStoreState((state) =>
    assignment ? siblingRepetitions(assignment, Array.from(state.assignments.values())) : [],
  );
  const connection = useStoreState((state) => state.connection);

  if (!assignmentId || !runId) return <ErrorState message="No assignment selected." />;
  if (!assignment) return <EmptyState hasRecords={false} hasFilters={false} connection={connection} />;

  const toolScorecard = assignment.benchmark_observations?.tool_scorecard;
  const toolScorecardEntries = toolScorecard
    ? Object.entries(toolScorecard).filter(
        ([, metric]) => metric.value !== undefined && !isScenarioNotApplicableMetric(metric),
      )
    : [];
  const gradeChips = assignmentGradeChips(assignment);
  const resource = assignment.resource_summary;
  const timing = assignment.benchmark_observations?.timing;
  const outputTokens = resource?.output_tokens?.value;
  const generationMs = timing?.generation_ms?.value;
  const tokensPerSecond =
    outputTokens !== undefined && generationMs !== undefined && generationMs > 0
      ? outputTokens / (generationMs / 1000)
      : undefined;
  const gpu = assignment.benchmark_observations?.gpu;
  const measured = [
    metricValue(timing?.model_load_ms) !== undefined ? ['Load', formatLatency(timing!.model_load_ms!.value!)] : undefined,
    metricValue(timing?.generation_ms) !== undefined ? ['Generation', formatLatency(timing!.generation_ms!.value!)] : undefined,
    metricValue(timing?.whole_task_ms) !== undefined ? ['Task', formatLatency(timing!.whole_task_ms!.value!)] : undefined,
    metricValue(gpu?.vram_before_bytes) !== undefined ? ['VRAM before', formatBytes(gpu!.vram_before_bytes!.value!)] : undefined,
    metricValue(gpu?.vram_peak_bytes) !== undefined ? ['VRAM peak', formatBytes(gpu!.vram_peak_bytes!.value!)] : undefined,
    metricValue(gpu?.system_ram_peak_bytes) !== undefined ? ['RAM peak', formatBytes(gpu!.system_ram_peak_bytes!.value!)] : undefined,
    metricValue(resource?.latency_ms) !== undefined ? ['Latency', formatLatency(resource!.latency_ms!.value!)] : undefined,
    metricValue(resource?.input_tokens) !== undefined ? ['Input', formatTokens(resource!.input_tokens!.value!)] : undefined,
    metricValue(resource?.output_tokens) !== undefined ? ['Output', formatTokens(resource!.output_tokens!.value!)] : undefined,
    tokensPerSecond !== undefined ? ['Tokens/s', formatThroughput(tokensPerSecond)] : undefined,
    metricValue(resource?.thinking_tokens) !== undefined ? ['Thinking', formatTokens(resource!.thinking_tokens!.value!)] : undefined,
    metricValue(resource?.cache_tokens) !== undefined ? ['Cache', formatTokens(resource!.cache_tokens!.value!)] : undefined,
    metricValue(resource?.retries) !== undefined ? ['Retries', formatNumber(resource!.retries!.value!)] : undefined,
  ].filter((entry): entry is [string, string] => entry !== undefined);
  const hasResources = !resourceObservationMissing(assignment) || tokensPerSecond !== undefined;
  const hasEvidence = Boolean(
    assignment.evidence_bindings?.length || assignment.verification_metadata,
  );

  return (
    <div className="assignment-detail">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/evaluations">Evaluations</Link>
        <span aria-hidden="true">/</span>
        <Link to={`/evaluations/${activeDatasetId}/${runId}`}>{runId}</Link>
        <span aria-hidden="true">/</span>
        <span>{assignment.assignment_id}</span>
      </nav>

      <header className="assignment-mast">
        <SectionHeading kicker="ASSIGNMENT DETAIL" title={assignment.task_id} />
        <p className="assignment-meta">
          <Link to={`/models/${activeDatasetId}/${assignment.variant_id}`}>{assignment.variant_id}</Link>
          <span>{roleLabel(assignment.role)}</span>
          <span>Repetition {assignment.repetition}</span>
          <span>{terminalLabel(assignment.terminal_status)}</span>
          <span>{formatRelativeTime(assignment.observed_at)}</span>
          <code>{assignment.assignment_id}</code>
        </p>
      </header>

      <div className="assignment-verdict" aria-label="Assignment verdict">
        <span className={`terminal-pill terminal-${assignment.terminal_status}`}>
          {assignmentVerdictLabel(assignment.terminal_status, assignment.verification_disposition)}
        </span>
        {gradeChips.map((grade) => (
          <span className={`grade-chip grade-${grade.status}`} key={grade.criterion_id}>
            {grade.criterion_id.replace(/-/g, ' ')} · {observationLabel(grade.status)}
            {grade.explanation ? ` · ${grade.explanation}` : ''}
          </span>
        ))}
      </div>

      {assignment.scenario_summary ? <ScenarioContextCard scenario={assignment.scenario_summary} /> : null}
      {hasResources ? (
        <div className="assignment-measured" aria-label="Measured values">
          {measured.map(([label, value]) => <span key={label}><b>{label}</b> {value}</span>)}
        </div>
      ) : null}
      {toolScorecardEntries.length > 0 ? (
        <section className="assignment-scores">
          <h2>Scores</h2>
          <div className="compact-stat-row">
            {toolScorecardEntries.map(([key, metric]) => (
              <span key={key}><b>{observationLabel(key)}</b> {formatNumber(metric.value!)}</span>
            ))}
          </div>
        </section>
      ) : null}
      {hasObservedActivity(assignment.activity_summary) ? <AssignmentActivitySummary activity={assignment.activity_summary} /> : null}

      {lifecycleEvents.length > 0 ? (
        <ol className="assignment-feed-timeline" aria-label="Feed timeline">
          {lifecycleEvents.map((event) => (
            <li key={event.event_id}>
              <span className="feed-time">{formatRelativeTime(event.observed_at)}</span>
              <span className="feed-kind">{event.kind.replace(/_/g, ' ')}</span>
              {event.feed_sequence !== undefined ? (
                <span className="feed-sequence">#{event.feed_sequence}</span>
              ) : null}
              <span className="feed-progress">{streamProgressLabel(event)}</span>
            </li>
          ))}
        </ol>
      ) : null}

      {assignment.stage_summary.length > 0 ? <section className="assignment-stages">
        <h2>Stages</h2>
          <ul className="stage-list">
            {assignment.stage_summary.map((stage, i) => (
              <li key={i}>
                <span className="stage-name">{stage.name}</span>
                <span className="stage-duration">{formatDuration(stage.duration_seconds)}</span>
              </li>
            ))}
          </ul>
      </section> : null}

      {hasEvidence ? <EvidenceBindingsPanel bindings={assignment.evidence_bindings} verification={assignment.verification_metadata} /> : null}

      {siblingAssignments.length > 0 ? (
        <section className="assignment-repetitions">
          <h2>Sibling repetitions</h2>
          <ul className="assignment-list">
            {siblingAssignments.map((sibling) => (
              <li key={sibling.assignment_id}>
                <Link to={`/evaluations/${sibling.dataset_id}/${sibling.run_id}/assignments/${sibling.assignment_id}`}>
                  {sibling.assignment_id}
                </Link>
                <span>rep {sibling.repetition}</span>
                <span>{sibling.terminal_status}</span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      {assignment.missingness_reason || assignment.benchmark_observations?.unavailable_reasons.length ? (
        <p className="assignment-limitation">
          {assignment.missingness_reason ?? assignment.benchmark_observations?.unavailable_reasons.join(' · ')}
        </p>
      ) : null}
    </div>
  );
}

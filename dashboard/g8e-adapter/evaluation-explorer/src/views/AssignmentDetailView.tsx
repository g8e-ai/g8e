// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Assignment detail view. Shows task ID, variant, role, repetition,
// lifecycle status, eligible metric values, missingness reason, safe stage
// names and durations, resource observations, verification disposition,
// task prompt from scenario catalog, and captured model response.

import { Link, useParams, useSearchParams } from 'react-router-dom';
import { activityFamilyHasObservedRecords } from '../contract/activity-family';
import type { AssignmentResult } from '../contract/types';
import { AssignmentActivitySummary } from '../components/AssignmentActivitySummary';
import { AssignmentAuditProofRow, filterNonAuditEvidenceBindings } from '../components/AssignmentAuditProofRow';
import { EvidenceBindingsPanel } from '../components/EvidenceBindingsPanel';
import { ScenarioContextCard } from '../components/ScenarioContextCard';
import { TaskPromptSection, TaskProvidedSection } from '../components/TaskPromptSection';
import { SCENARIO_TASK_BY_ID } from '../content/scenario-catalog';
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

function lookupTask(id?: string) {
  if (!id) return undefined;
  const base = id.split('@')[0];
  return SCENARIO_TASK_BY_ID.get(id) ?? (base ? SCENARIO_TASK_BY_ID.get(base) : undefined);
}

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
  return Boolean(activity && Object.values(activity).some((family) => activityFamilyHasObservedRecords(family)));
}

function ModelResponseSection({ assignment }: { assignment: AssignmentResult }) {
  const isFailure = assignment.terminal_status !== 'completed';
  const hasResponse = Boolean(assignment.model_response);
  const hasFailureOutput = Boolean(assignment.failure_output);

  if (!hasResponse && !hasFailureOutput && !isFailure) {
    return null;
  }

  const failingChips = assignmentGradeChips(assignment).filter(
    (g) => g.status === 'fail' || g.status === 'invalid_evidence',
  );

  return (
    <section className="assignment-model-response" aria-label="What the model did">
      <h2>What the model did</h2>

      {isFailure && (hasFailureOutput || failingChips.length > 0 || assignment.missingness_reason) ? (
        <div className="assignment-failure-callout" role="alert">
          <h3>Failure Diagnosis ({terminalLabel(assignment.terminal_status)})</h3>
          {hasFailureOutput ? (
            <p className="failure-output-detail">{assignment.failure_output}</p>
          ) : null}
          {failingChips.length > 0 ? (
            <ul className="failure-criteria-list">
              {failingChips.map((chip) => (
                <li key={chip.criterion_id}>
                  <strong>{chip.criterion_id.replace(/-/g, ' ')}:</strong>{' '}
                  {chip.explanation ? chip.explanation.replace(/_/g, ' ') : observationLabel(chip.status)}
                </li>
              ))}
            </ul>
          ) : null}
          {assignment.missingness_reason ? (
            <p className="failure-missingness">{assignment.missingness_reason}</p>
          ) : null}
        </div>
      ) : null}

      {hasResponse ? (
        <div className="model-response-block">
          <div className="model-response-header">
            <span className="model-response-label">
              {isFailure ? "Model's Response (Resulted in Failure)" : "Model's Response"}
            </span>
            <span className="model-response-variant">{assignment.variant_id}</span>
          </div>
          <pre className="model-response-text"><code>{assignment.model_response}</code></pre>
        </div>
      ) : isFailure && !hasFailureOutput ? (
        <p className="model-response-empty">
          No raw model response was captured in this assignment projection. Check local trace: <code>{assignment.assignment_id}-trace.json</code>
        </p>
      ) : null}
    </section>
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

  const task =
    lookupTask(assignment.task_id) ??
    lookupTask(assignment.scenario_id) ??
    lookupTask(assignment.scenario_summary?.scenario_id);

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
  const otherEvidenceBindings = filterNonAuditEvidenceBindings(assignment.evidence_bindings);
  const hasEvidence = Boolean(
    otherEvidenceBindings?.length || assignment.verification_metadata,
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

      {assignment.scenario_summary ? <ScenarioContextCard scenario={assignment.scenario_summary} /> : null}

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

      {task ? (
        <>
          <TaskPromptSection task={task} className="assignment-task-prompt" />
          <TaskProvidedSection task={task} className="assignment-task-provided" />
        </>
      ) : null}

      <ModelResponseSection assignment={assignment} />

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

      <AssignmentAuditProofRow bindings={assignment.evidence_bindings} />

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

      {hasEvidence ? <EvidenceBindingsPanel bindings={otherEvidenceBindings} verification={assignment.verification_metadata} /> : null}

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

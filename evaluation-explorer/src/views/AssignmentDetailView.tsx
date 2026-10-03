// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Assignment detail view. Shows task ID, variant, role, repetition,
// lifecycle status, eligible metric values, missingness reason, safe stage
// names and durations, resource observations, verification disposition,
// task prompt from scenario catalog, and captured model response.

import { Link, useParams, useSearchParams } from 'react-router-dom';
import { activityFamilyHasObservedRecords } from '../contract/activity-family';
import type { AssignmentResult } from '../contract/types';
import { TOOL_SCORE_DIMENSIONS } from '../contract/types';
import { AssignmentActivitySummary } from '../components/AssignmentActivitySummary';
import { AssignmentAuditProofRow, filterNonAuditEvidenceBindings } from '../components/AssignmentAuditProofRow';
import { EvidenceBindingsPanel } from '../components/EvidenceBindingsPanel';
import { RoleTranscripts } from '../components/RoleTranscripts';
import { ScenarioContextCard } from '../components/ScenarioContextCard';
import { TaskExpectationSection, TaskPromptSection, TaskProvidedSection } from '../components/TaskPromptSection';
import { UnifiedGradeBadges } from '../components/UnifiedGradeBadges';
import { SCENARIO_TASK_BY_ID } from '../content/scenario-task';
import { useActiveDatasetId } from '../state/dataset';
import { CURRENT_PLATFORM_RELEASE } from '../content/release';
import { recordKey, useStoreState } from '../state/store';
import {
  CopyButton,
  EmptyState,
  ErrorState,
  SectionHeading,
  StatTile,
  Timeline,
  UnavailableValue,
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

function formatPercent(value: number): string {
  return `${formatNumber(value, 1)}%`;
}

function formatTemperature(value: number): string {
  return `${formatNumber(value, 1)}°C`;
}

function formatPower(value: number): string {
  return `${formatNumber(value, 1)}W`;
}

function formatClockSpeed(value: number): string {
  if (value >= 1000) return `${formatNumber(value / 1000, 1)}GHz`;
  return `${formatNumber(value)}MHz`;
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

function toolActionSummaries(activity: AssignmentResult['activity_summary']): string[] {
  if (!activity) return [];
  const lines: string[] = [];

  if (activity.tool_calls.availability === 'observed') {
    for (const record of activity.tool_calls.records) {
      lines.push(`Called ${record.tool_label || 'an unlabeled tool'} — ${observationLabel(record.semantic_outcome)}`);
    }
  }
  if (activity.tool_decisions.availability === 'observed') {
    for (const record of activity.tool_decisions.records) {
      lines.push(
        `${record.selected ? 'Selected' : 'Considered but did not select'} ${record.tool_label || 'an unlabeled tool'} — ${observationLabel(record.outcome)}`,
      );
    }
  }
  if (lines.length === 0 && activity.policy_decisions.availability === 'observed') {
    for (const record of activity.policy_decisions.records) {
      lines.push(`Policy ${observationLabel(record.outcome)} for ${record.tool_label || 'an unlabeled tool'}`);
    }
  }
  return lines;
}

function trajectoryOutcomeLabel(outcome: string): string {
  return outcome.replace(/_/g, ' ').toUpperCase();
}

/** Trajectory outcome badge, guided retries, and the tools declared to the model. */
function FailureTrajectoryMeta({ assignment }: { assignment: AssignmentResult }) {
  const { trajectory_outcome: outcome, guided_retry_count: retries, tools_declared: declared } = assignment;
  if (!outcome && !retries && declared === undefined) return null;
  return (
    <div className="failure-trajectory-meta">
      {outcome ? <span className="task-flag failure-trajectory-badge">{trajectoryOutcomeLabel(outcome)}</span> : null}
      {retries ? <span className="failure-trajectory-retries">Guided retries: {retries}</span> : null}
      {declared !== undefined ? (
        <p className="failure-tools-declared">
          <strong>Tools declared:</strong>{' '}
          {declared.length > 0 ? declared.join(', ') : 'none'}
        </p>
      ) : null}
    </div>
  );
}

function ModelResponseSection({ assignment }: { assignment: AssignmentResult }) {
  const isFailure = assignment.terminal_status !== 'completed';
  const transcripts = assignment.role_transcripts ?? [];
  const hasResponse = Boolean(assignment.model_response);
  const hasFailureOutput = Boolean(assignment.failure_output);
  const hasFailureReason = Boolean(assignment.failure_reason);
  const toolActions = hasResponse ? [] : toolActionSummaries(assignment.activity_summary);

  const failingChips = assignmentGradeChips(assignment).filter(
    (g) => g.status === 'fail' || g.status === 'invalid_evidence',
  );

  return (
    <section className="assignment-model-response" aria-label="What the model did">
      <h2>What the model did</h2>

      {isFailure && (hasFailureReason || hasFailureOutput || failingChips.length > 0 || assignment.missingness_reason) ? (
        <div className="assignment-failure-callout" role="alert">
          <h3>Failure Diagnosis ({terminalLabel(assignment.terminal_status)})</h3>
          {hasFailureReason ? (
            <p className="failure-reason">{assignment.failure_reason}</p>
          ) : null}
          <FailureTrajectoryMeta assignment={assignment} />
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

      {transcripts.length > 0 ? (
        <RoleTranscripts transcripts={transcripts} variantId={assignment.variant_id} />
      ) : hasResponse ? (
        <div className="model-response-block">
          <div className="model-response-header">
            <span className="model-response-label">
              {isFailure ? "Model's Response (Resulted in Failure)" : "Model's Response"}
            </span>
            <span className="model-response-variant">{assignment.variant_id}</span>
            <CopyButton text={assignment.model_response!} label="model response" />
          </div>
          <pre className="model-response-text"><code>{assignment.model_response}</code></pre>
        </div>
      ) : toolActions.length > 0 ? (
        <div className="model-response-block model-response-tool-only">
          <div className="model-response-header">
            <span className="model-response-label">No free-text reply — the model acted via tool calls</span>
            <span className="model-response-variant">{assignment.variant_id}</span>
          </div>
          <ul className="model-response-tool-list">
            {toolActions.map((line, index) => (
              <li key={index}>{line}</li>
            ))}
          </ul>
          <p className="model-response-tool-verdict">
            Result: {assignmentVerdictLabel(assignment.terminal_status, assignment.verification_disposition)}
          </p>
        </div>
      ) : !hasFailureOutput ? (
        <p className="model-response-empty">
          No free-text response or tool activity was captured for this assignment. Result:{' '}
          {assignmentVerdictLabel(assignment.terminal_status, assignment.verification_disposition)}.
        </p>
      ) : null}
    </section>
  );
}

export function AssignmentDetailView() {
  const { assignmentId, runId, datasetId: routeDataset } = useParams();
  const [params] = useSearchParams();
  const activeDatasetId = useActiveDatasetId(routeDataset ?? params.get('dataset') ?? undefined, params.get('release') ?? CURRENT_PLATFORM_RELEASE);

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

  const resource = assignment.resource_summary;
  const timing = assignment.benchmark_observations?.timing;
  const outputTokens = resource?.output_tokens?.value;
  const generationMs = timing?.generation_ms?.value;
  const tokensPerSecond =
    outputTokens !== undefined && generationMs !== undefined && generationMs > 0
      ? outputTokens / (generationMs / 1000)
      : undefined;
  const gpu = assignment.benchmark_observations?.gpu;
  const toolScorecard = assignment.benchmark_observations?.tool_scorecard;
  const hasResources = !resourceObservationMissing(assignment) || tokensPerSecond !== undefined || metricValue(timing?.time_to_first_token_ms) !== undefined || metricValue(gpu?.utilization_percent) !== undefined || metricValue(gpu?.temperature_celsius) !== undefined || metricValue(gpu?.power_watts) !== undefined || metricValue(gpu?.clock_mhz) !== undefined;
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
        <UnifiedGradeBadges assignment={assignment} />
      </div>

      {task ? (
        <>
          <TaskPromptSection task={task} className="assignment-task-prompt" />
          <TaskProvidedSection task={task} className="assignment-task-provided" />
        </>
      ) : null}

      <ModelResponseSection assignment={assignment} />

      {task ? <TaskExpectationSection task={task} className="assignment-task-expected" /> : null}

      {hasResources ? (
        <section className="assignment-resources" aria-label="Measured values">
          {metricValue(timing?.model_load_ms) !== undefined || metricValue(timing?.generation_ms) !== undefined || metricValue(timing?.whole_task_ms) !== undefined || metricValue(timing?.time_to_first_token_ms) !== undefined || metricValue(resource?.latency_ms) !== undefined ? (
            <>
              <h3 className="resource-group-heading">Timing</h3>
              <div className="stat-tile-grid">
                {metricValue(timing?.model_load_ms) !== undefined ? (
                  <StatTile label="Load" value={formatLatency(timing!.model_load_ms!.value!)} />
                ) : null}
                {metricValue(timing?.generation_ms) !== undefined ? (
                  <StatTile label="Generation" value={formatLatency(timing!.generation_ms!.value!)} />
                ) : null}
                {metricValue(timing?.whole_task_ms) !== undefined ? (
                  <StatTile label="Task" value={formatLatency(timing!.whole_task_ms!.value!)} />
                ) : null}
                {metricValue(timing?.time_to_first_token_ms) !== undefined ? (
                  <StatTile label="TTFT" value={formatLatency(timing!.time_to_first_token_ms!.value!)} />
                ) : null}
                {metricValue(resource?.latency_ms) !== undefined ? (
                  <StatTile label="Latency" value={formatLatency(resource!.latency_ms!.value!)} />
                ) : null}
              </div>
            </>
          ) : null}

          {metricValue(gpu?.vram_before_bytes) !== undefined || metricValue(gpu?.vram_peak_bytes) !== undefined || metricValue(gpu?.system_ram_peak_bytes) !== undefined || metricValue(gpu?.utilization_percent) !== undefined || metricValue(gpu?.temperature_celsius) !== undefined || metricValue(gpu?.power_watts) !== undefined || metricValue(gpu?.clock_mhz) !== undefined ? (
            <>
              <h3 className="resource-group-heading">Memory & GPU</h3>
              <div className="stat-tile-grid">
                {metricValue(gpu?.vram_before_bytes) !== undefined ? (
                  <StatTile label="VRAM before" value={formatBytes(gpu!.vram_before_bytes!.value!)} />
                ) : null}
                {metricValue(gpu?.vram_peak_bytes) !== undefined ? (
                  <StatTile label="VRAM peak" value={formatBytes(gpu!.vram_peak_bytes!.value!)} />
                ) : null}
                {metricValue(gpu?.system_ram_peak_bytes) !== undefined ? (
                  <StatTile label="RAM peak" value={formatBytes(gpu!.system_ram_peak_bytes!.value!)} />
                ) : null}
                {metricValue(gpu?.utilization_percent) !== undefined ? (
                  <StatTile label="GPU utilization" value={formatPercent(gpu!.utilization_percent!.value!)} />
                ) : null}
                {metricValue(gpu?.temperature_celsius) !== undefined ? (
                  <StatTile label="GPU temperature" value={formatTemperature(gpu!.temperature_celsius!.value!)} />
                ) : null}
                {metricValue(gpu?.power_watts) !== undefined ? (
                  <StatTile label="GPU power" value={formatPower(gpu!.power_watts!.value!)} />
                ) : null}
                {metricValue(gpu?.clock_mhz) !== undefined ? (
                  <StatTile label="GPU clock" value={formatClockSpeed(gpu!.clock_mhz!.value!)} />
                ) : null}
              </div>
            </>
          ) : null}

          {metricValue(resource?.input_tokens) !== undefined || metricValue(resource?.output_tokens) !== undefined || tokensPerSecond !== undefined || metricValue(resource?.thinking_tokens) !== undefined || metricValue(resource?.cache_tokens) !== undefined ? (
            <>
              <h3 className="resource-group-heading">Throughput</h3>
              <div className="stat-tile-grid">
                {metricValue(resource?.input_tokens) !== undefined ? (
                  <StatTile label="Input" value={formatTokens(resource!.input_tokens!.value!)} />
                ) : null}
                {metricValue(resource?.output_tokens) !== undefined ? (
                  <StatTile label="Output" value={formatTokens(resource!.output_tokens!.value!)} />
                ) : null}
                {tokensPerSecond !== undefined ? (
                  <StatTile label="Tokens/s" value={formatThroughput(tokensPerSecond)} />
                ) : null}
                {metricValue(resource?.thinking_tokens) !== undefined ? (
                  <StatTile label="Thinking" value={formatTokens(resource!.thinking_tokens!.value!)} />
                ) : null}
                {metricValue(resource?.cache_tokens) !== undefined ? (
                  <StatTile label="Cache" value={formatTokens(resource!.cache_tokens!.value!)} />
                ) : null}
              </div>
            </>
          ) : null}

          {metricValue(resource?.retries) !== undefined ? (
            <>
              <h3 className="resource-group-heading">Reliability</h3>
              <div className="stat-tile-grid">
                <StatTile label="Retries" value={formatNumber(resource!.retries!.value!)} />
              </div>
            </>
          ) : null}
        </section>
      ) : null}

      {toolScorecard ? (
        <section className="assignment-scores">
          <h2>Scores</h2>
          <div className="stat-tile-grid">
            {TOOL_SCORE_DIMENSIONS.map((dimension) => {
              const metric = toolScorecard[dimension];
              const scenarioDimension = assignment.scenario_summary?.tool_score_dimensions?.find(
                (req) => req.dimension === dimension,
              );

              if (metric && isScenarioNotApplicableMetric(metric)) {
                return null;
              }

              if (metric?.value !== undefined) {
                return (
                  <StatTile
                    key={dimension}
                    label={observationLabel(dimension)}
                    value={formatNumber(metric.value)}
                  />
                );
              }

              if (scenarioDimension) {
                if (scenarioDimension.required && metric?.value === undefined) {
                  return (
                    <StatTile
                      key={dimension}
                      label={observationLabel(dimension)}
                      value={<UnavailableValue reason="required but not observed" />}
                    />
                  );
                }
                return (
                  <StatTile
                    key={dimension}
                    label={observationLabel(dimension)}
                    value={<span className="score-not-applicable">Not scored for this scenario</span>}
                  />
                );
              }

              return null;
            })}
          </div>
        </section>
      ) : null}
      {hasObservedActivity(assignment.activity_summary) ? <AssignmentActivitySummary activity={assignment.activity_summary} /> : null}

      {lifecycleEvents.length > 0 ? (
        <section className="assignment-feed-timeline" aria-label="Feed timeline">
          <h2>Timeline</h2>
          <Timeline events={lifecycleEvents.map((event) => ({
            observed_at: event.observed_at,
            kind: event.kind,
            stage_label: event.stage_label,
            completed: event.completed,
            total: event.total,
            assignment_id: event.assignment_id,
            run_id: event.run_id,
            dataset_id: activeDatasetId,
          }))} />
        </section>
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

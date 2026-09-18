// Assignment detail view. Shows task ID, variant, role, repetition,
// lifecycle status, eligible metric values, missingness reason, safe stage
// names and durations, resource observations, and verification disposition.
// Never shows raw prompts, outputs, trails, or credentials.

import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { recordKey, useStoreState } from '../state/store';
import {
  DetailRow,
  EmptyState,
  ErrorState,
  MetricCard,
  QualityBadge,
  SectionHeading,
  Timeline,
  UnavailableValue,
  formatDuration,
  formatLatency,
  formatTokens,
  formatNumber,
} from '../components/shared';
import { assignmentLifecycleEvents, assignmentMetricEntries, assignmentMetricFormatter, roleLabel } from './derived';

function observationLabel(value: string): string {
  return value.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

function formatBytes(value: number): string {
  if (value >= 1024 ** 3) return `${formatNumber(value / 1024 ** 3, 1)} GiB`;
  if (value >= 1024 ** 2) return `${formatNumber(value / 1024 ** 2, 1)} MiB`;
  return `${formatNumber(value)} B`;
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
  const connection = useStoreState((state) => state.connection);

  if (!assignmentId || !runId) return <ErrorState message="No assignment selected." />;
  if (!assignment) return <EmptyState hasRecords={false} hasFilters={false} connection={connection} />;

  const eligibleMetrics = assignmentMetricEntries(assignment.metric_values);

  return (
    <div className="assignment-detail">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/evaluations">Evaluations</Link>
        <span aria-hidden="true">/</span>
        <Link to={`/evaluations/${activeDatasetId}/${runId}`}>{runId}</Link>
        <span aria-hidden="true">/</span>
        <span>{assignment.assignment_id}</span>
      </nav>

      <SectionHeading kicker="ASSIGNMENT DETAIL" title={assignment.assignment_id} />

      <div className="quality-banner">
        <QualityBadge state={assignment.quality_state} />
        <span className="lifecycle-state">{assignment.terminal_status}</span>
        {assignment.missingness_reason ? <span className="missingness">{assignment.missingness_reason}</span> : null}
      </div>

      <section className="assignment-identity">
        <h2>Identity</h2>
        <dl>
          <DetailRow label="Assignment ID">{assignment.assignment_id}</DetailRow>
          <DetailRow label="Run ID">
            <Link to={`/evaluations/${activeDatasetId}/${assignment.run_id}`}>{assignment.run_id}</Link>
          </DetailRow>
          <DetailRow label="Task ID">{assignment.task_id}</DetailRow>
          <DetailRow label="Variant">
            <Link to={`/models/${activeDatasetId}/${assignment.variant_id}`}>{assignment.variant_id}</Link>
          </DetailRow>
          <DetailRow label="Role">{roleLabel(assignment.role)}</DetailRow>
          <DetailRow label="Repetition">{assignment.repetition}</DetailRow>
          <DetailRow label="Terminal status">{assignment.terminal_status}</DetailRow>
          <DetailRow label="Verification">{assignment.verification_disposition ?? <UnavailableValue />}</DetailRow>
        </dl>
      </section>

      <section className="assignment-benchmark">
        <h2>Benchmark observations</h2>
        <dl>
          <DetailRow label="Scenario category">{assignment.scenario_category ? observationLabel(assignment.scenario_category) : 'Not observed in this dataset'}</DetailRow>
          <DetailRow label="Evaluation unit">{assignment.evaluation_unit ? `${observationLabel(assignment.evaluation_unit)} evaluation` : 'Not observed in this dataset'}</DetailRow>
          <DetailRow label="Stack">{assignment.stack_id ?? 'Not applicable to model evaluation'}</DetailRow>
          <DetailRow label="Escalation">{assignment.benchmark_observations?.escalation_disposition ? observationLabel(assignment.benchmark_observations.escalation_disposition) : 'Not observed in this dataset'}</DetailRow>
        </dl>

        <h3>Tool-calling scorecard</h3>
        {assignment.benchmark_observations?.tool_scorecard && Object.keys(assignment.benchmark_observations.tool_scorecard).length > 0 ? (
          <div className="metric-grid">
            {Object.entries(assignment.benchmark_observations.tool_scorecard).map(([key, metric]) => (
              <MetricCard key={key} label={observationLabel(key)} metric={metric} formatter={formatNumber} />
            ))}
          </div>
        ) : <p className="benchmark-empty">Not observed in this dataset</p>}

        <h3>Failure why</h3>
        {assignment.benchmark_observations?.grade_summaries && assignment.benchmark_observations.grade_summaries.length > 0 ? (
          <dl className="benchmark-event-grid">
            {assignment.benchmark_observations.grade_summaries.map((summary) => (
              <DetailRow key={summary.criterion_id} label={observationLabel(summary.criterion_id)}>
                {observationLabel(summary.status)}
                {summary.detail ? ` — ${summary.detail}` : null}
              </DetailRow>
            ))}
          </dl>
        ) : <p className="benchmark-empty">Not observed in this dataset</p>}

        <h3>Security and privacy events</h3>
        {assignment.benchmark_observations?.security_privacy_events && Object.keys(assignment.benchmark_observations.security_privacy_events).length > 0 ? (
          <dl className="benchmark-event-grid">
            {Object.entries(assignment.benchmark_observations.security_privacy_events).map(([key, count]) => (
              <DetailRow key={key} label={observationLabel(key)}>{formatNumber(count)}</DetailRow>
            ))}
          </dl>
        ) : <p className="benchmark-empty">Not observed in this dataset</p>}

        <h3>Cold, warm, and GPU telemetry</h3>
        <div className="metric-grid">
          <MetricCard label="Model load" metric={assignment.benchmark_observations?.timing?.model_load_ms} formatter={formatLatency} />
          <MetricCard label="Time to first token" metric={assignment.benchmark_observations?.timing?.time_to_first_token_ms} formatter={formatLatency} />
          <MetricCard label="Generation" metric={assignment.benchmark_observations?.timing?.generation_ms} formatter={formatLatency} />
          <MetricCard label="Whole task" metric={assignment.benchmark_observations?.timing?.whole_task_ms} formatter={formatLatency} />
          <MetricCard label="VRAM before" metric={assignment.benchmark_observations?.gpu?.vram_before_bytes} formatter={formatBytes} />
          <MetricCard label="VRAM peak" metric={assignment.benchmark_observations?.gpu?.vram_peak_bytes} formatter={formatBytes} />
          <MetricCard label="System RAM peak" metric={assignment.benchmark_observations?.gpu?.system_ram_peak_bytes} formatter={formatBytes} />
          <MetricCard label="GPU utilization" metric={assignment.benchmark_observations?.gpu?.utilization_percent} formatter={(value) => `${formatNumber(value, 1)}%`} />
          <MetricCard label="GPU temperature" metric={assignment.benchmark_observations?.gpu?.temperature_celsius} formatter={(value) => `${formatNumber(value, 1)} °C`} />
          <MetricCard label="GPU power" metric={assignment.benchmark_observations?.gpu?.power_watts} formatter={(value) => `${formatNumber(value, 1)} W`} />
          <MetricCard label="GPU clock" metric={assignment.benchmark_observations?.gpu?.clock_mhz} formatter={(value) => `${formatNumber(value)} MHz`} />
        </div>

        {assignment.benchmark_observations?.correlated_failure ? (
          <dl>
            <DetailRow label="Failure cluster">{assignment.benchmark_observations.correlated_failure.cluster_id}</DetailRow>
            <DetailRow label="Semantic error">{observationLabel(assignment.benchmark_observations.correlated_failure.semantic_error_code)}</DetailRow>
            <DetailRow label="Affected roles">{assignment.benchmark_observations.correlated_failure.affected_roles.map(observationLabel).join(', ')}</DetailRow>
          </dl>
        ) : <p className="benchmark-empty">Correlated failure: Not observed in this dataset</p>}

        {assignment.benchmark_observations?.unavailable_reasons.length ? (
          <ul className="catalog-limitations">
            {assignment.benchmark_observations.unavailable_reasons.map((reason) => <li key={reason}>{reason}</li>)}
          </ul>
        ) : null}
      </section>

      <section className="assignment-metrics">
        <h2>Eligible metric values</h2>
        {eligibleMetrics.length === 0 ? (
          <p>No eligible metric values for this assignment.</p>
        ) : (
          <div className="metric-grid">
            {eligibleMetrics.map(({ key, label, metric }) => (
              <MetricCard
                key={key}
                label={label}
                metric={metric}
                formatter={assignmentMetricFormatter(key)}
              />
            ))}
          </div>
        )}
        {assignment.missingness_reason ? (
          <p className="missingness-detail">Missingness: {assignment.missingness_reason}</p>
        ) : null}
      </section>

      {lifecycleEvents.length > 0 ? (
        <section className="assignment-lifecycle">
          <h2>Lifecycle timeline</h2>
          <Timeline events={lifecycleEvents} />
        </section>
      ) : null}

      <section className="assignment-stages">
        <h2>Stages</h2>
        <ul className="stage-list">
          {assignment.stage_summary.map((stage, i) => (
            <li key={i}>
              <span className="stage-name">{stage.name}</span>
              <span className="stage-duration">{formatDuration(stage.duration_seconds)}</span>
            </li>
          ))}
        </ul>
      </section>

      {assignment.resource_summary ? (
        <section className="assignment-resources">
          <h2>Resource observations</h2>
          <div className="metric-grid">
            <MetricCard label="Latency" metric={assignment.resource_summary.latency_ms} formatter={formatLatency} />
            <MetricCard label="Input tokens" metric={assignment.resource_summary.input_tokens} formatter={formatTokens} />
            <MetricCard label="Output tokens" metric={assignment.resource_summary.output_tokens} formatter={formatTokens} />
            <MetricCard label="Retries" metric={assignment.resource_summary.retries} formatter={formatNumber} />
          </div>
        </section>
      ) : null}
    </div>
  );
}

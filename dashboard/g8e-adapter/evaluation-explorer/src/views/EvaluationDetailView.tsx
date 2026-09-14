// Evaluation detail view. Per the plan: status, quality, suite, execution
// arm, model-role mapping, start/end/elapsed, assignment progress, live
// timeline while active, terminal outcome counts, metric cards with
// denominators, per-model results, task/assignment table, verification
// summary, resource summary, and public-safe methodology links.

import { useMemo, useState } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { recordKey, useStoreState } from '../state/store';
import { DataTable } from '../components/DataTable';
import {
  DetailRow,
  EmptyState,
  ErrorState,
  MetricCard,
  OutcomeCounts,
  ProgressBar,
  QualityBadge,
  SectionHeading,
  SafeSourceLink,
  Timeline,
  UnavailableValue,
  formatTimestamp,
  formatDuration,
  formatPercent,
  formatLatency,
  formatThroughput,
  formatNumber,
} from '../components/shared';
import type { AssignmentResult } from '../contract/types';

export function EvaluationDetailView() {
  const { runId: routeRun, datasetId: routeDataset } = useParams();
  const [params] = useSearchParams();
  // Single-segment URLs (#/evaluations/<run>) resolve the run against the
  // active dataset rather than erroring.
  const runId = routeRun ?? routeDataset;
  const datasetParam = routeRun ? routeDataset : undefined;
  const activeDatasetId = useActiveDatasetId(datasetParam ?? params.get('dataset') ?? undefined);

  const run = useStoreState((state) =>
    runId ? state.evaluations.get(recordKey(activeDatasetId, runId)) : undefined,
  );
  const assignments = useStoreState((state) =>
    runId
      ? Array.from(state.assignments.values()).filter(
          (a) => a.run_id === runId && a.dataset_id === activeDatasetId,
        )
      : [],
  );
  const events = useStoreState((state) =>
    runId ? state.events.filter((e) => e.run_id === runId && e.dataset_id === activeDatasetId) : [],
  );
  const connection = useStoreState((state) => state.connection);
  const [assignmentSearch, setAssignmentSearch] = useState('');

  const filteredAssignments = useMemo(() => {
    if (!assignmentSearch) return assignments;
    const needle = assignmentSearch.toLowerCase();
    return assignments.filter(
      (a) =>
        a.assignment_id.toLowerCase().includes(needle) ||
        a.task_id.toLowerCase().includes(needle) ||
        a.variant_id.toLowerCase().includes(needle),
    );
  }, [assignments, assignmentSearch]);

  const assignmentColumns = useMemo<ColumnDef<AssignmentResult, unknown>[]>(
    () => [
      {
        id: 'assignment',
        header: 'Assignment',
        accessorKey: 'assignment_id',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) => (
          <Link to={`/evaluations/${row.original.dataset_id}/${row.original.run_id}/assignments/${row.original.assignment_id}`}>
            {row.original.assignment_id}
          </Link>
        ),
      },
      { id: 'task', header: 'Task', accessorKey: 'task_id' },
      { id: 'variant', header: 'Variant', accessorKey: 'variant_id' },
      { id: 'role', header: 'Role', accessorKey: 'role' },
      { id: 'rep', header: 'Rep', accessorKey: 'repetition' },
      { id: 'status', header: 'Status', accessorKey: 'terminal_status' },
      {
        id: 'verification',
        header: 'Verification',
        accessorFn: (row: AssignmentResult) => row.verification_disposition ?? '',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) =>
          row.original.verification_disposition ?? '—',
      },
    ],
    [],
  );

  if (!runId) return <ErrorState message="No run selected." />;
  if (!run) return <EmptyState hasRecords={false} hasFilters={false} connection={connection} />;

  const isActive = run.lifecycle_state === 'running' || run.lifecycle_state === 'queued';

  return (
    <div className="evaluation-detail">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to={`/evaluations?dataset=${activeDatasetId}`}>Evaluations</Link>
        <span aria-hidden="true">/</span>
        <span>{run.run_id}</span>
      </nav>

      <SectionHeading
        kicker="EVALUATION DETAIL"
        title={run.run_id}
        description={run.campaign_id ? `Campaign: ${run.campaign_id}` : undefined}
      />

      <div className="quality-banner">
        <QualityBadge state={run.quality_state} />
        <span className="lifecycle-state">{run.lifecycle_state}</span>
        {run.verifier_failure_summary ? <span className="verifier-failure">{run.verifier_failure_summary}</span> : null}
      </div>

      <section className="run-identity">
        <h2>Run identity</h2>
        <dl>
          <DetailRow label="Run ID">{run.run_id}</DetailRow>
          <DetailRow label="Suite">{run.suite_id}</DetailRow>
          <DetailRow label="Execution arm">{run.arm}</DetailRow>
          <DetailRow label="Evaluation unit">{run.evaluation_unit ? `${run.evaluation_unit.charAt(0).toUpperCase()}${run.evaluation_unit.slice(1)} evaluation` : <UnavailableValue reason="not observed in this dataset" />}</DetailRow>
          <DetailRow label="Stack identity">{run.stack_id ?? (run.evaluation_unit === 'model' ? 'Not applicable' : <UnavailableValue reason="not observed in this dataset" />)}</DetailRow>
          <DetailRow label="Campaign">{run.campaign_id ?? <UnavailableValue reason="no campaign recorded" />}</DetailRow>
          <DetailRow label="Started">{run.started_at ? formatTimestamp(run.started_at) : '—'}</DetailRow>
          <DetailRow label="Ended">{run.ended_at ? formatTimestamp(run.ended_at) : '—'}</DetailRow>
          <DetailRow label="Elapsed">{run.elapsed_seconds ? formatDuration(run.elapsed_seconds) : '—'}</DetailRow>
          <DetailRow label="Verifier state">{run.verifier_state}</DetailRow>
        </dl>
      </section>

      <section className="run-roles">
        <h2>Model-role mapping</h2>
        <ul className="role-mapping">
          {Object.entries(run.model_role_mapping).map(([role, variantId]) => (
            <li key={role}>
              <span className="role-label">{role === 'lite' ? 'Light' : role}</span>
              <Link to={`/models/${activeDatasetId}/${variantId}`}>{variantId}</Link>
            </li>
          ))}
        </ul>
      </section>

      <section className="run-progress">
        <h2>Assignment progress</h2>
        <ProgressBar completed={run.assignment_completed} total={run.assignment_total} label="Assignments" />
        <OutcomeCounts outcomes={run.terminal_outcomes} />
      </section>

      {isActive ? (
        <section className="run-timeline">
          <h2>Live timeline</h2>
          <Timeline events={events} />
        </section>
      ) : null}

      <section className="run-metrics">
        <h2>Headline metrics</h2>
        <div className="metric-grid">
          <MetricCard label="Pass rate" metric={run.headline_metrics.pass_rate} formatter={formatPercent} />
          <MetricCard label="Latency p50" metric={run.headline_metrics.latency_p50_ms} formatter={formatLatency} />
          <MetricCard label="Throughput" metric={run.headline_metrics.throughput} formatter={formatThroughput} />
          <MetricCard label="Primary invocation share" metric={run.primary_invocation_share} formatter={formatPercent} />
          <MetricCard label="Correlated failure rate" metric={run.correlated_failure_rate} formatter={formatPercent} />
        </div>
        {run.benchmark_unavailable_reasons?.length ? (
          <ul className="catalog-limitations">
            {run.benchmark_unavailable_reasons.map((reason) => <li key={reason}>{reason}</li>)}
          </ul>
        ) : null}
      </section>

      <section className="run-verification">
        <h2>Verification summary</h2>
        <dl>
          <DetailRow label="Verifier state">{run.verifier_state}</DetailRow>
          {run.verifier_failure_summary ? <DetailRow label="Failure summary">{run.verifier_failure_summary}</DetailRow> : null}
        </dl>
      </section>

      <section className="run-assignments">
        <h2>Assignments ({formatNumber(assignments.length)})</h2>
        {assignments.length === 0 ? (
          <p>No assignment results for this run.</p>
        ) : (
          <>
            <input
              type="search"
              placeholder="Search assignments..."
              value={assignmentSearch}
              aria-label="Search assignments"
              onChange={(e) => setAssignmentSearch(e.target.value)}
            />
            <DataTable
              data={filteredAssignments}
              columns={assignmentColumns}
              pageSize={50}
              caption={`Assignments for ${run.run_id}`}
              empty={<p>No assignments match the search.</p>}
            />
          </>
        )}
      </section>

      {run.evidence_link ? (
        <section className="run-evidence">
          <h2>Source</h2>
          <SafeSourceLink href={run.evidence_link} label="Public-safe evidence" />
        </section>
      ) : null}
    </div>
  );
}

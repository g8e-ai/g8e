// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Evaluation detail view. Per the plan: status, quality, suite, execution
// arm, model-role mapping, start/end/elapsed, assignment progress, a link to
// the live page, terminal outcome counts, metric cards with denominators,
// per-model results, task/assignment table, verification summary, resource
// summary, and public-safe methodology links.

import { useMemo, useState } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link, useParams, useSearchParams } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { recordKey, useStoreState } from '../state/store';
import { DataTable } from '../components/DataTable';
import { RunFailuresSection } from '../components/RunFailuresSection';
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
  UnavailableValue,
  formatTimestamp,
  formatDuration,
  formatPercent,
  formatLatency,
  formatThroughput,
  formatNumber,
} from '../components/shared';
import type { AssignmentResult } from '../contract/types';
import { campaignTerminalProgress, roleLabel } from './derived';

export function EvaluationDetailView() {
  const { runId: routeRun, datasetId: routeDataset } = useParams();
  const [params] = useSearchParams();
  // Single-segment URLs (/evaluations/<run>) resolve the run against the
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
      {
        id: 'role',
        header: 'Role',
        accessorKey: 'role',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) => roleLabel(row.original.role),
      },
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

  return (
    <div className="evaluation-detail">
      <nav className="breadcrumb" aria-label="Breadcrumb">
        <Link to="/evaluations">Evaluations</Link>
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
          {run.native_result ? <DetailRow label="Active posture">{run.native_result.active_posture}</DetailRow> : null}
          {run.native_result ? <DetailRow label="Lane">{run.native_result.lane}</DetailRow> : null}
        </dl>
      </section>

      {run.model_role_mapping && Object.keys(run.model_role_mapping).length > 0 ? (
        <section className="run-roles">
          <h2>Model-role mapping</h2>
          <ul className="role-mapping">
            {Object.entries(run.model_role_mapping).map(([role, variantId]) => (
              <li key={role}>
                <span className="role-label">{roleLabel(role)}</span>
                <Link to={`/models/${activeDatasetId}/${variantId}`}>{variantId}</Link>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      <section className="run-progress">
        <h2>{run.native_result ? 'Invariant summary' : 'Assignment progress'}</h2>
        <ProgressBar
          completed={
            run.native_result?.passed_verdict_count ??
            campaignTerminalProgress(run).done
          }
          total={
            run.native_result?.required_verdict_count ??
            campaignTerminalProgress(run).total
          }
          label={run.native_result ? 'Required invariants' : 'Assignments'}
        />
        {run.native_result ? <p>{run.native_result.summary}</p> : <OutcomeCounts outcomes={run.terminal_outcomes} />}
      </section>

      <div className="run-live-action">
        <Link to="/" className="campaign-primary-action">
          Watch it Live
        </Link>
      </div>

      <section className="run-metrics">
        <h2>Headline metrics</h2>
        <div className="metric-grid">
          <MetricCard label="Pass rate" metric={run.headline_metrics.pass_rate} formatter={formatPercent} />
          {!run.native_result ? <MetricCard label="Latency p50" metric={run.headline_metrics.latency_p50_ms} formatter={formatLatency} /> : null}
          {!run.native_result ? <MetricCard label="Throughput" metric={run.headline_metrics.throughput} formatter={formatThroughput} /> : null}
          {!run.native_result ? <MetricCard label="Primary invocation share" metric={run.primary_invocation_share} formatter={formatPercent} /> : null}
          {!run.native_result ? <MetricCard label="Correlated failure rate" metric={run.correlated_failure_rate} formatter={formatPercent} /> : null}
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
          {run.native_result ? (
            <DetailRow label="Independent verification">
              {run.native_result.verification_valid ? 'Valid' : 'Invalid'} ({run.native_result.verification_failure_count} failures)
            </DetailRow>
          ) : null}
          {run.verifier_failure_summary ? <DetailRow label="Failure summary">{run.verifier_failure_summary}</DetailRow> : null}
        </dl>
      </section>

      {run.native_result ? (
        <section className="run-native-scenarios">
          <h2>Scenario and invariant results</h2>
          {run.native_result.scenarios.map((scenario) => (
            <article key={scenario.scenario_id} className="native-scenario">
              <h3>{scenario.scenario_id}@{scenario.scenario_version}</h3>
              <p>Status: {scenario.status}</p>
              <ul>
                {scenario.verdicts.map((verdict) => (
                  <li key={verdict.assertion_id}>
                    <strong>{verdict.assertion_id}@{verdict.assertion_version}</strong>: {verdict.status}
                  </li>
                ))}
              </ul>
            </article>
          ))}
        </section>
      ) : (
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
      )}

      {run.evidence_link ? (
        <section className="run-evidence">
          <h2>Source</h2>
          <SafeSourceLink href={run.evidence_link} label="Public-safe evidence" />
        </section>
      ) : null}

      {!run.native_result ? (
        <RunFailuresSection assignments={assignments} connection={connection} />
      ) : null}
    </div>
  );
}

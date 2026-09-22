// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Evaluations view — cross-dataset evaluation history. Each row shows dataset,
// run/campaign identity, suite, progress, outcomes, verifier state, and metrics.

import { useMemo } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link, useSearchParams } from 'react-router-dom';
import { useStoreState } from '../state/store';
import { DataTable } from '../components/DataTable';
import { RoleLeadersPanel } from '../components/RoleLeadersPanel';
import {
  EmptyState,
  OutcomeCounts,
  ProgressBar,
  QualityBadge,
  SectionHeading,
  formatTimestamp,
  formatDuration,
  formatPercent,
} from '../components/shared';
import type { EvaluationSummary } from '../contract/types';
import { campaignTerminalProgress, datasetLabel } from './derived';

export function EvaluationsView() {
  const [params, setParams] = useSearchParams();

  const suiteFilter = params.get('suite') ?? 'all';
  const statusFilter = params.get('status') ?? 'all';
  const qualityFilter = params.get('quality') ?? 'all';

  const evaluations = useStoreState((state) =>
    Array.from(state.evaluations.values()).sort((a, b) =>
      (b.started_at ?? b.observed_at ?? '').localeCompare(a.started_at ?? a.observed_at ?? ''),
    ),
  );
  const models = useStoreState((state) => Array.from(state.models.values()));
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()));
  const connection = useStoreState((state) => state.connection);

  const filtered = useMemo(() => {
    return evaluations.filter((e) => {
      if (suiteFilter !== 'all' && e.suite_id !== suiteFilter) return false;
      if (statusFilter !== 'all' && e.lifecycle_state !== statusFilter) return false;
      if (qualityFilter !== 'all' && e.quality_state !== qualityFilter) return false;
      return true;
    });
  }, [evaluations, suiteFilter, statusFilter, qualityFilter]);

  const setFilter = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value === 'all') next.delete(key);
    else next.set(key, value);
    setParams(next);
  };

  const columns = useMemo<ColumnDef<EvaluationSummary, unknown>[]>(
    () => [
      {
        id: 'dataset',
        header: 'Dataset',
        accessorKey: 'dataset_id',
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => (
          <code className="dataset-id" title={row.original.dataset_id}>
            {datasetLabel(row.original.dataset_id)}
          </code>
        ),
      },
      {
        id: 'run',
        header: 'Run',
        accessorKey: 'run_id',
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => (
          <Link to={`/evaluations/${row.original.dataset_id}/${row.original.run_id}`}>
            {row.original.run_id}
          </Link>
        ),
      },
      { id: 'suite', header: 'Suite', accessorKey: 'suite_id' },
      { id: 'arm', header: 'Arm', accessorKey: 'arm' },
      {
        id: 'progress',
        header: 'Progress',
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => {
          const terminal = campaignTerminalProgress(row.original);
          return (
            <ProgressBar
              completed={row.original.native_result?.passed_verdict_count ?? terminal.done}
              total={row.original.native_result?.required_verdict_count ?? terminal.total}
              label={`${row.original.run_id} progress`}
            />
          );
        },
        enableSorting: false,
      },
      {
        id: 'outcomes',
        header: 'Outcomes',
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => row.original.native_result ? row.original.native_result.summary_status : <OutcomeCounts outcomes={row.original.terminal_outcomes} />,
        enableSorting: false,
      },
      {
        id: 'verifier',
        header: 'Verifier',
        accessorKey: 'verifier_state',
      },
      {
        id: 'started',
        header: 'Started',
        accessorFn: (row: EvaluationSummary) => row.started_at ?? '',
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => (row.original.started_at ? formatTimestamp(row.original.started_at) : '—'),
      },
      {
        id: 'elapsed',
        header: 'Elapsed',
        accessorFn: (row: EvaluationSummary) => row.elapsed_seconds ?? 0,
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => (row.original.elapsed_seconds ? formatDuration(row.original.elapsed_seconds) : '—'),
      },
      {
        id: 'pass_rate',
        header: 'Pass rate',
        accessorFn: (row: EvaluationSummary) => row.headline_metrics.pass_rate?.value ?? -1,
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) =>
          row.original.headline_metrics.pass_rate?.value !== undefined
            ? formatPercent(row.original.headline_metrics.pass_rate.value)
            : '—',
      },
      {
        id: 'quality',
        header: 'Quality',
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => <QualityBadge state={row.original.quality_state} />,
        enableSorting: false,
      },
    ],
    [],
  );

  return (
    <div className="evaluations-view">
      <RoleLeadersPanel models={models} catalogs={catalogs} />
      <SectionHeading
        kicker="EVALUATIONS"
        title="Evaluation runs"
        description="All published runs across datasets. Each row shows the dataset, suite, progress, verifier state, and headline metrics."
      />

      <div className="eval-toolbar">
        <select aria-label="Filter by suite" value={suiteFilter} onChange={(e) => setFilter('suite', e.target.value)}>
          <option value="all">All suites</option>
          {Array.from(new Set(evaluations.map((e) => e.suite_id))).map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
        <select aria-label="Filter by status" value={statusFilter} onChange={(e) => setFilter('status', e.target.value)}>
          <option value="all">All statuses</option>
          <option value="queued">Queued</option>
          <option value="running">Running</option>
          <option value="completed">Completed</option>
          <option value="failed">Failed</option>
          <option value="stopped">Stopped</option>
        </select>
        <select aria-label="Filter by quality" value={qualityFilter} onChange={(e) => setFilter('quality', e.target.value)}>
          <option value="all">All quality states</option>
          <option value="verified_public">Current-standard verified</option>
          <option value="exploratory_verified">Run-scoped verification passed</option>
          <option value="exploratory_partial">Not fully verified</option>
          <option value="legacy_unverified">Legacy · not current-standard verified</option>
          <option value="live_in_progress">In progress · unverified</option>
          <option value="terminal_failed">Failed · unverified</option>
          <option value="dead_evidence">Evidence invalid</option>
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyState
          hasRecords={evaluations.length > 0}
          hasFilters={suiteFilter !== 'all' || statusFilter !== 'all' || qualityFilter !== 'all'}
          connection={connection}
        />
      ) : (
        <DataTable data={filtered} columns={columns} pageSize={25} caption="Evaluation runs" />
      )}
    </div>
  );
}

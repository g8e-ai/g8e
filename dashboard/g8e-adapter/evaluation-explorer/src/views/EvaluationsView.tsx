// Evaluations view — the evaluation list. Filters for dataset, suite, status,
// quality, model, role, and date. Each row shows run/campaign identity,
// suite, model scope, assignment progress, terminal outcome counts,
// verifier state, start/end time, and headline metrics.

import { useMemo } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link, useSearchParams } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { useStoreState } from '../state/store';
import { DataTable } from '../components/DataTable';
import { DatasetSelector } from '../components/DatasetSelector';
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

export function EvaluationsView() {
  const [params, setParams] = useSearchParams();
  const routeDataset = params.get('dataset') ?? undefined;
  const activeDatasetId = useActiveDatasetId(routeDataset);

  const suiteFilter = params.get('suite') ?? 'all';
  const statusFilter = params.get('status') ?? 'all';
  const qualityFilter = params.get('quality') ?? 'all';

  const evaluations = useStoreState((state) =>
    Array.from(state.evaluations.values()).filter((e) => e.dataset_id === activeDatasetId),
  );
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
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => (
          <ProgressBar
            completed={row.original.assignment_completed}
            total={row.original.assignment_total}
            label={`${row.original.run_id} progress`}
          />
        ),
        enableSorting: false,
      },
      {
        id: 'outcomes',
        header: 'Outcomes',
        cell: ({ row }: CellContext<EvaluationSummary, unknown>) => <OutcomeCounts outcomes={row.original.terminal_outcomes} />,
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
      <SectionHeading
        kicker="EVALUATIONS"
        title="Evaluation runs"
        description="Each row is a run with its suite, model-role mapping, progress, verifier state, and headline metrics."
      />
      <DatasetSelector activeId={activeDatasetId} />

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
          <option value="exploratory_partial">Exploratory · partial</option>
          <option value="exploratory_verified">Exploratory · verifier passed</option>
          <option value="verified_public">Verified public</option>
          <option value="live_in_progress">Live · in progress</option>
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

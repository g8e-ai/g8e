// Failures view — every preserved terminal failure in the active dataset.
// Assignments link to detail pages; nothing is hidden from the denominator.

import { useMemo } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link, useSearchParams } from 'react-router-dom';
import { useActiveDatasetId } from '../state/dataset';
import { useStoreState } from '../state/store';
import { DataTable } from '../components/DataTable';
import { DatasetSelector } from '../components/DatasetSelector';
import {
  EmptyState,
  QualityBadge,
  SectionHeading,
  formatTimestamp,
} from '../components/shared';
import type { AssignmentResult, TerminalStatus } from '../contract/types';
import { isFailureTerminalStatus } from './derived';

function observationLabel(value: string): string {
  return value.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

export function FailuresView() {
  const [params, setParams] = useSearchParams();
  const routeDataset = params.get('dataset') ?? undefined;
  const activeDatasetId = useActiveDatasetId(routeDataset);
  const statusFilter = (params.get('status') ?? 'all') as TerminalStatus | 'all';

  const assignments = useStoreState((state) =>
    Array.from(state.assignments.values()).filter((a) => a.dataset_id === activeDatasetId),
  );
  const connection = useStoreState((state) => state.connection);

  const failures = useMemo(() => {
    return assignments
      .filter((a) => isFailureTerminalStatus(a.terminal_status))
      .filter((a) => statusFilter === 'all' || a.terminal_status === statusFilter)
      .sort((a, b) => (b.observed_at ?? '').localeCompare(a.observed_at ?? ''));
  }, [assignments, statusFilter]);

  const setFilter = (value: string) => {
    const next = new URLSearchParams(params);
    if (value === 'all') next.delete('status');
    else next.set('status', value);
    setParams(next);
  };

  const columns = useMemo<ColumnDef<AssignmentResult, unknown>[]>(
    () => [
      {
        id: 'assignment',
        header: 'Assignment',
        accessorKey: 'assignment_id',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) => (
          <Link
            to={`/evaluations/${row.original.dataset_id}/${row.original.run_id}/assignments/${row.original.assignment_id}`}
          >
            {row.original.assignment_id}
          </Link>
        ),
      },
      { id: 'run', header: 'Run', accessorKey: 'run_id' },
      { id: 'task', header: 'Task', accessorKey: 'task_id' },
      { id: 'variant', header: 'Variant', accessorKey: 'variant_id' },
      { id: 'role', header: 'Role', accessorKey: 'role' },
      {
        id: 'status',
        header: 'Failure class',
        accessorKey: 'terminal_status',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) => observationLabel(row.original.terminal_status),
      },
      {
        id: 'category',
        header: 'Scenario category',
        accessorFn: (row: AssignmentResult) => row.scenario_category ?? '',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) =>
          row.original.scenario_category ? observationLabel(row.original.scenario_category) : '—',
      },
      {
        id: 'observed',
        header: 'Observed',
        accessorFn: (row: AssignmentResult) => row.observed_at ?? '',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) =>
          row.original.observed_at ? formatTimestamp(row.original.observed_at) : '—',
      },
      {
        id: 'quality',
        header: 'Quality',
        accessorKey: 'quality_state',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) => (
          <QualityBadge state={row.original.quality_state} />
        ),
      },
    ],
    [],
  );

  return (
    <div className="failures-view">
      <SectionHeading
        kicker="FAILURES"
        title="Preserved terminal failures"
        description="Every failed, partial, policy-rejected, provider-failed, grader-failed, or stopped assignment remains in the dataset."
      />

      <div className="view-toolbar">
        <DatasetSelector />
        <label className="filter-control">
          <span>Failure class</span>
          <select value={statusFilter} onChange={(event) => setFilter(event.target.value)}>
            <option value="all">All failure classes</option>
            <option value="model_failed">Model failed</option>
            <option value="grader_failed">Grader failed</option>
            <option value="invalid_evidence">Invalid evidence</option>
            <option value="stopped">Stopped</option>
          </select>
        </label>
      </div>

      {failures.length === 0 ? (
        <EmptyState hasRecords={assignments.length > 0} hasFilters={statusFilter !== 'all'} connection={connection} />
      ) : (
        <DataTable columns={columns} data={failures} ariaLabel="Terminal failures" />
      )}
    </div>
  );
}

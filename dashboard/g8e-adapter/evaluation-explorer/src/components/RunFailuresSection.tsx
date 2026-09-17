// Run-scoped terminal failures for evaluation detail pages.

import { useMemo, useState } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link } from 'react-router-dom';
import { DataTable } from './DataTable';
import { EmptyState, QualityBadge, SectionHeading, formatTimestamp } from './shared';
import type { AssignmentResult, TerminalStatus } from '../contract/types';
import type { FeedConnectionState } from '../utils/feed-state';
import { isFailureTerminalStatus, roleLabel } from '../views/derived';

function observationLabel(value: string): string {
  return value.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

type RunFailuresSectionProps = {
  assignments: AssignmentResult[];
  connection: FeedConnectionState;
};

export function RunFailuresSection({ assignments, connection }: RunFailuresSectionProps) {
  const [statusFilter, setStatusFilter] = useState<TerminalStatus | 'all'>('all');

  const failures = useMemo(() => {
    return assignments
      .filter((a) => isFailureTerminalStatus(a.terminal_status))
      .filter((a) => statusFilter === 'all' || a.terminal_status === statusFilter)
      .sort((a, b) => (b.observed_at ?? '').localeCompare(a.observed_at ?? ''));
  }, [assignments, statusFilter]);

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
      { id: 'task', header: 'Task', accessorKey: 'task_id' },
      { id: 'variant', header: 'Variant', accessorKey: 'variant_id' },
      {
        id: 'role',
        header: 'Role',
        accessorKey: 'role',
        cell: ({ row }: CellContext<AssignmentResult, unknown>) => roleLabel(row.original.role),
      },
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
    <section className="run-failures">
      <SectionHeading
        kicker="FAILURES"
        title="Terminal failures"
        description="Failed, grader-failed, invalid-evidence, or stopped assignments for this run remain in the dataset."
      />

      <div className="view-toolbar">
        <label className="filter-control">
          <span>Failure class</span>
          <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as TerminalStatus | 'all')}>
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
        <DataTable columns={columns} data={failures} caption="Terminal failures for this run" />
      )}
    </section>
  );
}

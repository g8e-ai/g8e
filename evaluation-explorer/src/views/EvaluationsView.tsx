// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Evaluations view — cross-dataset evaluation history. Each row shows dataset,
// run/campaign identity, suite, progress, outcomes, verifier state, and metrics.

import { useEffect, useMemo } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link, useSearchParams } from 'react-router-dom';
import { useStoreState } from '../state/store';
import { DataTable } from '../components/DataTable';
import { RoleLeadersPanel } from '../components/RoleLeadersPanel';
import { ReleaseSelector } from '../components/ReleaseSelector';
import { CURRENT_PLATFORM_RELEASE, matchesRelease, releaseLabel } from '../content/release';
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
import {
  availableQualityFilterOptions,
  EVALUATION_QUALITY_FILTER_STATES,
  normalizeQualityFilter,
} from '../utils/feed-state';
import { campaignTerminalProgress, datasetLabel } from './derived';

export function EvaluationsView() {
  const [params, setParams] = useSearchParams();

  const suiteFilter = params.get('suite') ?? 'all';
  const statusFilter = params.get('status') ?? 'all';
  const qualityFilter = params.get('quality') ?? 'all';
  const releaseFilter = params.get('release') ?? CURRENT_PLATFORM_RELEASE;

  const evaluations = useStoreState((state) =>
    Array.from(state.evaluations.values()).sort((a, b) =>
      (b.started_at ?? b.observed_at ?? '').localeCompare(a.started_at ?? a.observed_at ?? ''),
    ),
  );
  const models = useStoreState((state) => Array.from(state.models.values()).filter((record) => matchesRelease(record, releaseFilter)));
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()).filter((record) => matchesRelease(record, releaseFilter)));
  const connection = useStoreState((state) => state.connection);
  const releaseEvaluations = useMemo(() => evaluations.filter((record) => matchesRelease(record, releaseFilter)), [evaluations, releaseFilter]);

  const qualityOptions = useMemo(
    () => availableQualityFilterOptions(releaseEvaluations, EVALUATION_QUALITY_FILTER_STATES),
    [releaseEvaluations],
  );
  const effectiveQualityFilter = normalizeQualityFilter(qualityFilter, qualityOptions);

  useEffect(() => {
    if (effectiveQualityFilter === qualityFilter) return;
    const next = new URLSearchParams(params);
    next.set('quality', effectiveQualityFilter);
    setParams(next, { replace: true });
  }, [effectiveQualityFilter, qualityFilter, params, setParams]);

  const filtered = useMemo(() => {
    return releaseEvaluations.filter((e) => {
      if (suiteFilter !== 'all' && e.suite_id !== suiteFilter) return false;
      if (statusFilter !== 'all' && e.lifecycle_state !== statusFilter) return false;
      if (effectiveQualityFilter !== 'all' && e.quality_state !== effectiveQualityFilter) return false;
      return true;
    });
  }, [releaseEvaluations, suiteFilter, statusFilter, effectiveQualityFilter]);

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
      { id: 'release', header: 'Release', accessorFn: releaseLabel },
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
      <RoleLeadersPanel models={models} catalogs={catalogs} evaluations={releaseEvaluations} />
      <SectionHeading
        kicker="EVALUATIONS"
        title="Evaluation runs"
        description="Published runs for the selected release across datasets. Each row shows release provenance, progress, verifier state, and headline metrics."
      />

      <div className="eval-toolbar">
        <ReleaseSelector />
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
        <select aria-label="Filter by quality" value={effectiveQualityFilter} onChange={(e) => setFilter('quality', e.target.value)}>
          <option value="all">All quality states</option>
          {qualityOptions.map((option) => (
            <option key={option.value} value={option.value}>{option.label}</option>
          ))}
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyState
          hasRecords={evaluations.length > 0}
          hasFilters={releaseFilter !== 'all' || suiteFilter !== 'all' || statusFilter !== 'all' || effectiveQualityFilter !== 'all'}
          connection={connection}
        />
      ) : (
        <DataTable data={filtered} columns={columns} pageSize={25} caption="Evaluation runs" />
      )}
    </div>
  );
}

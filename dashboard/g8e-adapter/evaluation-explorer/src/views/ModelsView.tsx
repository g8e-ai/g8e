// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Models view — the model catalog. Contains all 31 required registry models
// (evaluated and not-evaluated) plus inventory-only entries. Provides text
// search, role/provider/evaluated/quality-state/suite filters, sorting,
// table and card views, and multi-select comparison.

import { useEffect, useMemo } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link, useSearchParams } from 'react-router-dom';
import { useUserPref, PREF } from '../state/dataset';
import { modelComparisonId, resolveModelFromComparisonId, useStoreState } from '../state/store';
import { DataTable } from '../components/DataTable';
import { ModelComparisonPanel } from '../components/ModelComparisonPanel';
import {
  EmptyState,
  ErrorState,
  IntervalDisplay,
  QualityBadge,
  SectionHeading,
  UnavailableValue,
  formatPercent,
  formatLatency,
  formatThroughput,
  formatNumber,
} from '../components/shared';
import type { ModelSummary, ModelRole } from '../contract/types';
import { datasetLabel, roleLabel } from './derived';

interface ModelFilters {
  search: string;
  role: ModelRole | 'all';
  evaluated: 'all' | 'evaluated' | 'not_evaluated';
  quality: string | 'all';
}

const DEFAULT_FILTERS: ModelFilters = { search: '', role: 'all', evaluated: 'evaluated', quality: 'all' };

function filtersActive(filters: ModelFilters): boolean {
  return (
    filters.search !== '' ||
    filters.role !== 'all' ||
    filters.evaluated !== 'all' ||
    filters.quality !== 'all'
  );
}

export function ModelsView() {
  const [params, setParams] = useSearchParams();
  const [filters, setFilters] = useUserPref<ModelFilters>(PREF.filters, DEFAULT_FILTERS);
  const [comparison, setComparison] = useUserPref<string[]>(PREF.comparison, []);

  useEffect(() => {
    const modelsParam = params.get('models');
    if (!modelsParam) return;
    const ids = modelsParam.split(',').filter(Boolean).slice(0, 4);
    if (ids.length > 0) {
      setComparison(ids);
      const next = new URLSearchParams(params);
      next.delete('models');
      setParams(next, { replace: true });
    }
  }, [params, setComparison, setParams]);

  const models = useStoreState((state) => Array.from(state.models.values()));
  const connection = useStoreState((state) => state.connection);
  const evaluatedFilter =
    filters.evaluated === 'evaluated' &&
    models.length > 0 &&
    models.every((model) => !model.pass_rate)
      ? 'all'
      : filters.evaluated;

  const comparedModels = useStoreState((state) =>
    comparison
      .map((id) => resolveModelFromComparisonId(state.models, id))
      .filter((m): m is NonNullable<typeof m> => m !== undefined),
  );

  const comparisonDatasetMismatch =
    comparedModels.length >= 2 && new Set(comparedModels.map((m) => m.dataset_id)).size > 1;

  const filtered = useMemo(() => {
    return models.filter((m) => {
      if (filters.search && !m.display_name.toLowerCase().includes(filters.search.toLowerCase()) && !m.variant_id.toLowerCase().includes(filters.search.toLowerCase())) {
        return false;
      }
      if (filters.role !== 'all' && m.role !== filters.role) return false;
      if (evaluatedFilter === 'evaluated' && !m.pass_rate) return false;
      if (evaluatedFilter === 'not_evaluated' && m.pass_rate) return false;
      if (filters.quality !== 'all' && m.quality_state !== filters.quality) return false;
      return true;
    }).sort((a, b) => {
      const measured = Number(Boolean(b.pass_rate)) - Number(Boolean(a.pass_rate));
      if (measured !== 0) return measured;
      const passRate = (b.pass_rate?.estimate ?? -1) - (a.pass_rate?.estimate ?? -1);
      return passRate || a.display_name.localeCompare(b.display_name);
    });
  }, [models, filters, evaluatedFilter]);

  const columns = useMemo<ColumnDef<ModelSummary, unknown>[]>(
    () => [
      {
        id: 'select',
        header: 'Compare',
        cell: ({ row }: CellContext<ModelSummary, unknown>) => (
          <input
            type="checkbox"
            aria-label={`Select ${row.original.display_name} for comparison`}
            checked={comparison.includes(modelComparisonId(row.original))}
            disabled={!comparison.includes(modelComparisonId(row.original)) && comparison.length >= 4}
            onChange={(e) => {
              const comparisonId = modelComparisonId(row.original);
              const next = e.target.checked
                ? [...comparison, comparisonId].slice(0, 4)
                : comparison.filter((id) => id !== comparisonId);
              setComparison(next);
            }}
          />
        ),
        enableSorting: false,
      },
      {
        id: 'dataset',
        header: 'Dataset',
        accessorKey: 'dataset_id',
        cell: ({ row }: CellContext<ModelSummary, unknown>) => (
          <code className="dataset-id" title={row.original.dataset_id}>
            {datasetLabel(row.original.dataset_id)}
          </code>
        ),
      },
      {
        id: 'name',
        header: 'Model',
        accessorKey: 'display_name',
        cell: ({ row }: CellContext<ModelSummary, unknown>) => (
          <Link
            to={`/models/${row.original.dataset_id}/${row.original.variant_id}?role=${row.original.role}`}
            className="model-link"
          >
            {row.original.display_name}
            {row.original.inventory_only ? <span className="inv-tag"> inventory</span> : null}
          </Link>
        ),
      },
      {
        id: 'served_tag',
        header: 'Served tag',
        accessorFn: (row: ModelSummary) => row.served_model_tag ?? '',
        cell: ({ row }: CellContext<ModelSummary, unknown>) => row.original.served_model_tag ?? <UnavailableValue />,
      },
      {
        id: 'role',
        header: 'Role',
        accessorKey: 'role',
        cell: ({ row }: CellContext<ModelSummary, unknown>) => roleLabel(row.original.role),
      },
      {
        id: 'coverage',
        header: 'Coverage',
        accessorFn: (row: ModelSummary) => row.evaluation_coverage,
        cell: ({ row }: CellContext<ModelSummary, unknown>) => formatPercent(row.original.evaluation_coverage),
      },
      {
        id: 'pass_rate',
        header: 'Pass rate',
        accessorFn: (row: ModelSummary) => row.pass_rate?.estimate ?? -1,
        cell: ({ row }: CellContext<ModelSummary, unknown>) =>
          row.original.pass_rate ? (
            <IntervalDisplay interval={row.original.pass_rate} label="pass rate" />
          ) : (
            <UnavailableValue reason="not evaluated" />
          ),
      },
      {
        id: 'agreement',
        header: 'Agreement',
        accessorFn: (row: ModelSummary) => row.agreement_pairwise?.value ?? -1,
        cell: ({ row }: CellContext<ModelSummary, unknown>) =>
          row.original.agreement_pairwise?.value !== undefined
            ? formatPercent(row.original.agreement_pairwise.value)
            : <UnavailableValue />,
      },
      {
        id: 'latency_p50',
        header: 'Latency p50',
        accessorFn: (row: ModelSummary) => row.latency_p50_ms?.value ?? Number.POSITIVE_INFINITY,
        cell: ({ row }: CellContext<ModelSummary, unknown>) =>
          row.original.latency_p50_ms?.value !== undefined
            ? formatLatency(row.original.latency_p50_ms.value)
            : <UnavailableValue />,
      },
      {
        id: 'throughput_p50',
        header: 'Throughput p50',
        accessorFn: (row: ModelSummary) => row.output_throughput_p50?.value ?? -1,
        cell: ({ row }: CellContext<ModelSummary, unknown>) =>
          row.original.output_throughput_p50?.value !== undefined
            ? formatThroughput(row.original.output_throughput_p50.value)
            : <UnavailableValue />,
      },
      {
        id: 'quality',
        header: 'Quality',
        accessorKey: 'quality_state',
        cell: ({ row }: CellContext<ModelSummary, unknown>) => <QualityBadge state={row.original.quality_state} />,
        enableSorting: false,
      },
    ],
    [comparison, setComparison],
  );

  const updateFilter = (patch: Partial<ModelFilters>) => setFilters({ ...filters, ...patch });

  const measuredCount = models.filter((model) => Boolean(model.pass_rate)).length;

  return (
    <div className="models-view">
      <SectionHeading
        kicker="MEASURED RESULTS"
        title="Models with real evaluation data"
        description="Measured models appear first by pass rate across every dataset. Use All models to inspect the unevaluated registry inventory."
      />

      <div className="models-toolbar">
        <input
          type="search"
          placeholder="Search models..."
          value={filters.search}
          aria-label="Search models"
          onChange={(e) => updateFilter({ search: e.target.value })}
        />
        <select aria-label="Filter by role" value={filters.role} onChange={(e) => updateFilter({ role: e.target.value as ModelFilters['role'] })}>
          <option value="all">All roles</option>
          <option value="primary">Primary</option>
          <option value="assistant">Assistant</option>
          <option value="lite">Lite</option>
        </select>
        <select aria-label="Filter by evaluation status" value={filters.evaluated} onChange={(e) => updateFilter({ evaluated: e.target.value as ModelFilters['evaluated'] })}>
          <option value="all">All models</option>
          <option value="evaluated">Evaluated</option>
          <option value="not_evaluated">Not evaluated</option>
        </select>
        <select aria-label="Filter by quality state" value={filters.quality} onChange={(e) => updateFilter({ quality: e.target.value })}>
          <option value="all">All quality states</option>
          <option value="verified_public">Current-standard verified</option>
          <option value="exploratory_verified">Run-scoped verification passed</option>
          <option value="exploratory_partial">Not fully verified</option>
          <option value="legacy_unverified">Legacy · not current-standard verified</option>
          <option value="not_evaluated">Not evaluated</option>
        </select>
        <span className="result-count">{formatNumber(measuredCount)} measured · {formatNumber(models.length)} total</span>
        {comparison.length > 0 ? (
          <button type="button" className="comparison-clear-btn" onClick={() => setComparison([])}>
            Clear {comparison.length} selected
          </button>
        ) : null}
      </div>

      {comparison.length > 0 && comparison.length < 2 ? (
        <p className="comparison-hint">Select at least one more model to compare side by side (up to four).</p>
      ) : null}

      {comparison.length >= 2 && comparedModels.length < 2 ? (
        <ErrorState message="Some selected models were not found in the catalog." />
      ) : null}

      {comparisonDatasetMismatch ? (
        <ErrorState message="Selected models come from different datasets. Comparison never combines incompatible datasets." />
      ) : null}

      {comparedModels.length >= 2 && !comparisonDatasetMismatch ? (
        <ModelComparisonPanel
          models={comparedModels}
          onClear={() => setComparison([])}
        />
      ) : null}

      {filtered.length === 0 ? (
        <EmptyState hasRecords={models.length > 0} hasFilters={filtersActive(filters)} connection={connection} />
      ) : (
        <DataTable data={filtered} columns={columns} pageSize={25} caption="Model catalog" />
      )}
    </div>
  );
}

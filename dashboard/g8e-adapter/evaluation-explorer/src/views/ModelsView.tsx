// Models view — the model catalog. Contains all 31 required registry models
// (evaluated and not-evaluated) plus inventory-only entries. Provides text
// search, role/provider/evaluated/quality-state/suite filters, sorting,
// table and card views, and multi-select comparison.

import { useMemo } from 'react';
import { type CellContext, type ColumnDef } from '@tanstack/react-table';
import { Link, useSearchParams } from 'react-router-dom';
import { getCatalogForDataset, useActiveDatasetId, useUserPref, PREF } from '../state/dataset';
import { modelComparisonId, useStoreState } from '../state/store';
import { DataTable } from '../components/DataTable';
import { DatasetSelector } from '../components/DatasetSelector';
import {
  EmptyState,
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
import { roleLabel } from './derived';

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
  const [params] = useSearchParams();
  const routeDataset = params.get('dataset') ?? undefined;
  const activeDatasetId = useActiveDatasetId(routeDataset);
  const [filters, setFilters] = useUserPref<ModelFilters>(PREF.filters, DEFAULT_FILTERS);
  const [comparison, setComparison] = useUserPref<string[]>(PREF.comparison, []);

  const models = useStoreState((state) =>
    Array.from(state.models.values()).filter((m) => m.dataset_id === activeDatasetId),
  );
  const connection = useStoreState((state) => state.connection);
  const catalog = getCatalogForDataset(activeDatasetId);
  const evaluatedFilter =
    filters.evaluated === 'evaluated' &&
    catalog?.dataset_kind === 'live_run' &&
    models.length > 0 &&
    models.every((model) => !model.pass_rate)
      ? 'all'
      : filters.evaluated;

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
        id: 'name',
        header: 'Model',
        accessorKey: 'display_name',
        cell: ({ row }: CellContext<ModelSummary, unknown>) => (
          <Link
            to={`/models/${activeDatasetId}/${row.original.variant_id}?role=${row.original.role}`}
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
    [activeDatasetId, comparison, setComparison],
  );

  const updateFilter = (patch: Partial<ModelFilters>) => setFilters({ ...filters, ...patch });

  return (
    <div className="models-view">
      <SectionHeading
        kicker="MEASURED RESULTS"
        title="Models with real evaluation data"
        description="Measured models appear first by pass rate. Use All models to inspect the unevaluated registry inventory."
      />
      <DatasetSelector activeId={activeDatasetId} />

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
          <option value="lite">Light</option>
        </select>
        <select aria-label="Filter by evaluation status" value={filters.evaluated} onChange={(e) => updateFilter({ evaluated: e.target.value as ModelFilters['evaluated'] })}>
          <option value="all">All models</option>
          <option value="evaluated">Evaluated</option>
          <option value="not_evaluated">Not evaluated</option>
        </select>
        <select aria-label="Filter by quality state" value={filters.quality} onChange={(e) => updateFilter({ quality: e.target.value })}>
          <option value="all">All quality states</option>
          <option value="exploratory_partial">Exploratory · partial</option>
          <option value="exploratory_verified">Exploratory · verifier passed</option>
          <option value="verified_public">Verified public</option>
          <option value="not_evaluated">Not evaluated</option>
        </select>
        <span className="result-count">{formatNumber(models.filter((model) => Boolean(model.pass_rate)).length)} measured · {formatNumber(models.length)} total</span>
        {comparison.length >= 2 ? (
          <Link to={`/compare?dataset=${activeDatasetId}&models=${comparison.join(',')}`} className="compare-btn">
            Compare {comparison.length}
          </Link>
        ) : null}
      </div>

      {filtered.length === 0 ? (
        <EmptyState hasRecords={models.length > 0} hasFilters={filtersActive(filters)} connection={connection} />
      ) : (
        <DataTable data={filtered} columns={columns} pageSize={25} caption="Model catalog" />
      )}
    </div>
  );
}

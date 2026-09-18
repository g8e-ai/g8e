// Dataset selection and URL state. The site never averages across datasets.
// The active dataset is encoded in the hash route so it survives reload and
// is shareable. User preferences (filters, sort, comparison list) persist in
// localStorage; eval state never does.
//
// Selector options derive from the catalog snapshots present in the store —
// never from hardcoded dataset IDs — so a live run's fresh dataset_id becomes
// selectable as soon as its catalog record lands.

import { useCallback, useMemo, useState } from 'react';
import type { CatalogSnapshot, DatasetKind, EvaluationSummary } from '../contract/types';
import { evalStore, useStoreState } from './store';

const PREF_KEYS = {
  filters: 'opendevops.filters.v2',
  sort: 'opendevops.sort',
  comparison: 'opendevops.comparison',
} as const;

export interface DatasetOption {
  id: string;
  kind: DatasetKind;
  label: string;
  available: boolean;
}

const KIND_ORDER: DatasetKind[] = ['exploratory_baseline', 'verified_public_snapshot', 'live_run'];

const KIND_LABELS: Record<DatasetKind, string> = {
  exploratory_baseline: 'Exploratory baseline',
  verified_public_snapshot: 'Verified public snapshot',
  live_run: 'Live run',
};

function liveDatasetIdsFromEvaluations(evaluations: Iterable<EvaluationSummary>): string[] {
  const latestByDataset = new Map<string, string>();
  for (const summary of evaluations) {
    if (!summary.dataset_id.startsWith('ds-live-')) continue;
    const observedAt = summary.observed_at ?? '';
    const existing = latestByDataset.get(summary.dataset_id);
    if (!existing || observedAt.localeCompare(existing) > 0) {
      latestByDataset.set(summary.dataset_id, observedAt);
    }
  }
  return Array.from(latestByDataset.entries())
    .sort((a, b) => b[1].localeCompare(a[1]))
    .map(([datasetId]) => datasetId);
}

function buildOptions(catalogs: CatalogSnapshot[], liveDatasetIds: string[]): DatasetOption[] {
  const options: DatasetOption[] = [];
  for (const kind of KIND_ORDER) {
    const matches = catalogs
      .filter((c) => c.dataset_kind === kind)
      .sort((a, b) => b.generated_at.localeCompare(a.generated_at));
    if (kind === 'live_run' && matches.length === 0 && liveDatasetIds.length > 0) {
      liveDatasetIds.forEach((datasetId, index) => {
        options.push({
          id: datasetId,
          kind,
          label: index === 0 ? KIND_LABELS[kind] : `${KIND_LABELS[kind]} · ${datasetId}`,
          available: true,
        });
      });
      continue;
    }
    if (matches.length === 0) {
      options.push({ id: `__empty_${kind}`, kind, label: KIND_LABELS[kind], available: false });
      continue;
    }
    matches.forEach((catalog, index) => {
      options.push({
        id: catalog.dataset_id,
        kind,
        label: index === 0 ? KIND_LABELS[kind] : `${KIND_LABELS[kind]} · ${catalog.dataset_id}`,
        available: true,
      });
    });
  }
  return options;
}

function defaultDatasetId(
  options: DatasetOption[],
  liveDatasetIds: string[],
  evaluations: Iterable<EvaluationSummary>,
): string {
  const verifiedLive = Array.from(evaluations)
    .filter((summary) => summary.dataset_id.startsWith('ds-live-') && summary.quality_state === 'verified_public')
    .sort((a, b) => (b.observed_at ?? '').localeCompare(a.observed_at ?? ''));
  if (verifiedLive.length > 0) return verifiedLive[0].dataset_id;

  const verifiedSnapshot = options.find((o) => o.available && o.kind === 'verified_public_snapshot');
  if (verifiedSnapshot) return verifiedSnapshot.id;

  const liveRun = options.find((o) => o.available && o.kind === 'live_run');
  if (liveRun) return liveRun.id;

  const firstAvailable = options.find((o) => o.available)?.id;
  if (firstAvailable) return firstAvailable;
  return liveDatasetIds[0] ?? '';
}

export function useDatasetOptions(): DatasetOption[] {
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()));
  const evaluations = useStoreState((state) => state.evaluations);
  return useMemo(
    () => buildOptions(catalogs, liveDatasetIdsFromEvaluations(evaluations.values())),
    [catalogs, evaluations],
  );
}

export function useActiveDatasetId(routeDatasetId: string | undefined): string {
  const options = useDatasetOptions();
  const evaluations = useStoreState((state) => state.evaluations);
  const liveDatasetIds = liveDatasetIdsFromEvaluations(evaluations.values());
  const fallbackDatasetId = defaultDatasetId(options, liveDatasetIds, evaluations.values());
  if (routeDatasetId) {
    if (options.some((o) => o.id === routeDatasetId)) return routeDatasetId;
    // Live campaign datasets are synthesized from publication envelopes and may
    // not have a catalog_snapshot yet; honor explicit route ids and any run
    // summaries already indexed for that dataset.
    if (routeDatasetId.startsWith('ds-live-')) return routeDatasetId;
    for (const summary of evaluations.values()) {
      if (summary.dataset_id === routeDatasetId) return routeDatasetId;
    }
  }
  return fallbackDatasetId;
}

export function useUserPref<T>(key: string, initial: T): [T, (value: T) => void] {
  const [value, setValue] = useState<T>(() => {
    try {
      const stored = localStorage.getItem(key);
      return stored ? (JSON.parse(stored) as T) : initial;
    } catch {
      return initial;
    }
  });
  const update = useCallback(
    (next: T) => {
      setValue(next);
      try {
        localStorage.setItem(key, JSON.stringify(next));
      } catch {
        // ignore storage failures (private mode, quota)
      }
    },
    [key],
  );
  return [value, update];
}

export const PREF = PREF_KEYS;

export function getCatalogForDataset(datasetId: string) {
  return evalStore.getCatalog(datasetId);
}

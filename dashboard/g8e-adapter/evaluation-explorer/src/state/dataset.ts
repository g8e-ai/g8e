// Dataset selection and URL state. The site never averages across datasets.
// The active dataset is encoded in the hash route so it survives reload and
// is shareable. User preferences (filters, sort, comparison list) persist in
// localStorage; eval state never does.
//
// Selector options derive from the catalog snapshots present in the store —
// never from hardcoded dataset IDs — so a live run's fresh dataset_id becomes
// selectable as soon as its catalog record lands.

import { useCallback, useMemo, useState } from 'react';
import type { CatalogSnapshot, DatasetKind } from '../contract/types';
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

function buildOptions(catalogs: CatalogSnapshot[]): DatasetOption[] {
  const options: DatasetOption[] = [];
  for (const kind of KIND_ORDER) {
    const matches = catalogs
      .filter((c) => c.dataset_kind === kind)
      .sort((a, b) => b.generated_at.localeCompare(a.generated_at));
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

export function useDatasetOptions(): DatasetOption[] {
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()));
  return useMemo(() => buildOptions(catalogs), [catalogs]);
}

export function useActiveDatasetId(routeDatasetId: string | undefined): string {
  const options = useDatasetOptions();
  const firstAvailable = options.find((o) => o.available)?.id ?? '';
  if (routeDatasetId && options.some((o) => o.id === routeDatasetId)) return routeDatasetId;
  return firstAvailable;
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

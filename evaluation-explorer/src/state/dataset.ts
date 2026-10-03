// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Dataset selection and URL state. The site never averages across datasets.
// The active dataset is encoded in the hash route so it survives reload and
// is shareable. User preferences (filters, sort, comparison list) persist in
// localStorage; eval state never does.
//
// Selector options derive from the catalog snapshots present in the store —
// never from hardcoded dataset IDs — so a live run's fresh dataset_id becomes
// selectable as soon as its catalog record lands.

import { useCallback, useMemo, useState } from 'react';
import type { CatalogSnapshot, DatasetKind, EvaluationSummary, QualityState, ReleaseProvenance } from '../contract/types';
import { CURRENT_PLATFORM_RELEASE, matchesRelease, releaseLabel } from '../content/release';
import { evalStore, useStoreState } from './store';

const PREF_KEYS = {
  filters: 'opendevops.filters.v3',
  sort: 'opendevops.sort',
  comparison: 'opendevops.comparison',
} as const;

export interface DatasetOption extends ReleaseProvenance {
  id: string;
  kind: DatasetKind;
  label: string;
  available: boolean;
  quality?: QualityState;
}

const KIND_ORDER: DatasetKind[] = ['exploratory_baseline', 'verified_public_snapshot', 'live_run'];

const KIND_LABELS: Record<DatasetKind, string> = {
  exploratory_baseline: 'Exploratory baseline',
  verified_public_snapshot: 'Legacy public snapshot',
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

function buildOptions(catalogs: CatalogSnapshot[], evaluations: EvaluationSummary[]): DatasetOption[] {
  const liveDatasetIds = liveDatasetIdsFromEvaluations(evaluations);
  const options: DatasetOption[] = [];
  for (const kind of KIND_ORDER) {
    const matches = catalogs
      .filter((c) => c.dataset_kind === kind)
      .sort((a, b) => b.generated_at.localeCompare(a.generated_at));
    if (kind === 'live_run' && matches.length === 0 && liveDatasetIds.length > 0) {
      liveDatasetIds.forEach((datasetId, index) => {
        const identity = evaluations.find((summary) => summary.dataset_id === datasetId);
        options.push({
          id: datasetId,
          kind,
          label: `${index === 0 ? KIND_LABELS[kind] : `${KIND_LABELS[kind]} · ${datasetId}`} · ${releaseLabel(identity ?? {})}`,
          available: true,
          release: identity?.release,
          release_basis: identity?.release_basis,
          source_revision: identity?.source_revision,
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
        label: `${index === 0 ? KIND_LABELS[kind] : `${KIND_LABELS[kind]} · ${catalog.dataset_id}`} · ${releaseLabel(catalog)}`,
        available: true,
        quality: catalog.quality_state,
        release: catalog.release,
        release_basis: catalog.release_basis,
        source_revision: catalog.source_revision,
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
  const newestVerifiedLive = verifiedLive[0];
  if (newestVerifiedLive) return newestVerifiedLive.dataset_id;

  const verifiedSnapshot = options.find(
    (option) => option.available && option.kind === 'verified_public_snapshot' && option.quality === 'verified_public',
  );
  if (verifiedSnapshot) return verifiedSnapshot.id;

  const liveRun = options.find((o) => o.available && o.kind === 'live_run');
  if (liveRun) return liveRun.id;

  const firstAvailable = options.find((o) => o.available)?.id;
  if (firstAvailable) return firstAvailable;
  return liveDatasetIds[0] ?? '';
}

export function useDatasetOptions(release: string = CURRENT_PLATFORM_RELEASE): DatasetOption[] {
  const catalogs = useStoreState((state) => Array.from(state.catalogs.values()));
  const evaluations = useStoreState((state) => state.evaluations);
  return useMemo(
    () => buildOptions(catalogs.filter((record) => matchesRelease(record, release)), Array.from(evaluations.values()).filter((record) => matchesRelease(record, release))),
    [catalogs, evaluations, release],
  );
}

export function useActiveDatasetId(routeDatasetId: string | undefined, release: string = CURRENT_PLATFORM_RELEASE): string {
  const options = useDatasetOptions(release);
  const evaluations = useStoreState((state) => state.evaluations);
  const eligibleEvaluations = Array.from(evaluations.values()).filter((record) => matchesRelease(record, release));
  const liveDatasetIds = liveDatasetIdsFromEvaluations(eligibleEvaluations);
  const fallbackDatasetId = defaultDatasetId(options, liveDatasetIds, eligibleEvaluations);
  if (routeDatasetId) {
    if (options.some((o) => o.id === routeDatasetId) || evalStore.getCatalog(routeDatasetId)) return routeDatasetId;
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

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useSearchParams } from 'react-router-dom';
import { CURRENT_PLATFORM_RELEASE } from '../content/release';
import { useStoreState } from '../state/store';

export function ReleaseSelector() {
  const [params, setParams] = useSearchParams();
  const selected = params.get('release') ?? CURRENT_PLATFORM_RELEASE;
  const releases = useStoreState((state) => Array.from(new Set([
    CURRENT_PLATFORM_RELEASE,
    ...(selected === 'all' ? [] : [selected]),
    ...Array.from(state.evaluations.values()).flatMap((record) => record.release ? [record.release] : []),
    ...Array.from(state.catalogs.values()).flatMap((record) => record.release ? [record.release] : []),
  ])).sort().reverse());

  return (
    <select aria-label="Filter by release" value={selected} onChange={(event) => {
      const next = new URLSearchParams(params);
      next.set('release', event.target.value);
      next.delete('dataset');
      setParams(next);
    }}>
      <option value="all">All releases</option>
      {releases.map((release) => (
        <option key={release} value={release}>
          {release}{release === CURRENT_PLATFORM_RELEASE ? ' (current)' : ''}
        </option>
      ))}
    </select>
  );
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Dataset selector. The site never averages across datasets. Switching
// datasets updates the URL so the selection is shareable and survives reload.

import { useNavigate, useSearchParams } from 'react-router-dom';
import { useDatasetOptions } from '../state/dataset';
import { CURRENT_PLATFORM_RELEASE } from '../content/release';
import { ReleaseSelector } from './ReleaseSelector';

export function DatasetSelector({ activeId }: { activeId: string }) {
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const options = useDatasetOptions(params.get('release') ?? CURRENT_PLATFORM_RELEASE);

  return (
    <div className="dataset-selector" data-testid="dataset-selector">
      <ReleaseSelector />
      <span className="ds-label">Dataset</span>
      <select
        value={activeId}
        aria-label="Select dataset"
        onChange={(event) => {
          const next = event.target.value;
          const newParams = new URLSearchParams(params);
          newParams.set('dataset', next);
          navigate(`?${newParams.toString()}`);
        }}
      >
        {options.map((opt) => (
          <option key={opt.id} value={opt.id} disabled={!opt.available}>
            {opt.label}
            {opt.available ? '' : ' (no data)'}
          </option>
        ))}
      </select>
    </div>
  );
}

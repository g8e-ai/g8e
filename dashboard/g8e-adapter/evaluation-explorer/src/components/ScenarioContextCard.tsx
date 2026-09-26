// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { PublicScenarioSummary } from '../contract/types';

export function ScenarioContextCard({ scenario }: { scenario: PublicScenarioSummary | undefined }) {
  if (!scenario) return null;

  const toolNotes = [
    scenario.allowed_tools.length > 0 ? `Allowed: ${scenario.allowed_tools.join(', ')}` : undefined,
    scenario.expected_tools.length > 0 ? `Expected: ${scenario.expected_tools.join(', ')}` : undefined,
    scenario.forbidden_tools.length > 0 ? `Forbidden: ${scenario.forbidden_tools.join(', ')}` : undefined,
  ].filter((note): note is string => note !== undefined);

  return (
    <div className="assignment-scenario">
      <p className="assignment-scenario-description">{scenario.public_description}</p>
      {scenario.criteria.length > 0 ? (
        <div className="assignment-scenario-criteria" aria-label="Public criteria">
          {scenario.criteria.map((criterion) => (
            <span className="grade-chip criterion-chip" key={criterion.criterion_id}>
              {criterion.public_label}{criterion.required ? ' · required' : ' · optional'}
            </span>
          ))}
        </div>
      ) : null}
      {toolNotes.length > 0 ? <p className="assignment-scenario-tools">{toolNotes.join(' · ')}</p> : null}
    </div>
  );
}

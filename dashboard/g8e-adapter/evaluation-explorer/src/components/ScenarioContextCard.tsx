// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { PublicScenarioSummary } from '../contract/types';
import { DetailRow } from './shared';

function displayLabel(value: string): string {
  return value.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

export function ScenarioContextCard({ scenario }: { scenario: PublicScenarioSummary | undefined }) {
  return (
    <section className="assignment-context">
      <h2>Assignment context</h2>
      {!scenario ? (
        <p className="panel-note">Scenario context is unavailable for this historical assignment.</p>
      ) : (
        <>
          <dl>
            <DetailRow label="Scenario">{scenario.scenario_id}</DetailRow>
            <DetailRow label="Scenario version">{scenario.scenario_version}</DetailRow>
            <DetailRow label="Category">{displayLabel(scenario.category)}</DetailRow>
            <DetailRow label="Grading method">{displayLabel(scenario.grading_method)}</DetailRow>
            <DetailRow label="Description">{scenario.public_description}</DetailRow>
          </dl>
          {scenario.allowed_tools.length > 0 || scenario.expected_tools.length > 0 || scenario.forbidden_tools.length > 0 ? (
            <div className="context-lists">
              {scenario.allowed_tools.length > 0 ? <div><h3>Allowed tools</h3><ul>{scenario.allowed_tools.map((tool) => <li key={tool}>{tool}</li>)}</ul></div> : null}
              {scenario.expected_tools.length > 0 ? <div><h3>Expected tools</h3><ul>{scenario.expected_tools.map((tool) => <li key={tool}>{tool}</li>)}</ul></div> : null}
              {scenario.forbidden_tools.length > 0 ? <div><h3>Forbidden tools</h3><ul>{scenario.forbidden_tools.map((tool) => <li key={tool}>{tool}</li>)}</ul></div> : null}
            </div>
          ) : null}
          {scenario.criteria.length > 0 ? (
            <div className="context-criteria">
              <h3>Public criteria</h3>
              <ul>
                {scenario.criteria.map((criterion) => (
                  <li key={criterion.criterion_id}>
                    <strong>{criterion.public_label}</strong>
                    <span>{criterion.public_description}</span>
                    <small>{displayLabel(criterion.grading_method)} · {criterion.required ? 'Required' : 'Optional'}</small>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}

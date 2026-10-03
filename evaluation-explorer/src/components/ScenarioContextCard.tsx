// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { HINT_SOURCE_LABEL, TRAJECTORY_POLICY_META } from '../content/scenario-task';
import type { PublicPromptHint, PublicScenarioSummary } from '../contract/types';

function hintText(hint: PublicPromptHint): string {
  const sources = hint.arguments
    .map((argument) => `${argument.tool_name}.${argument.argument_name} ← ${HINT_SOURCE_LABEL[argument.source]}`)
    .join(', ');
  return sources ? `${hint.hinted_tools.join(', ')} (${sources})` : hint.hinted_tools.join(', ');
}

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
      {toolNotes.length > 0 ? <p className="assignment-scenario-tools">{toolNotes.join(' · ')}</p> : null}
      {scenario.trajectory_policy ? (
        <p className="assignment-scenario-policy">
          <strong>{TRAJECTORY_POLICY_META[scenario.trajectory_policy].label}.</strong>{' '}
          {TRAJECTORY_POLICY_META[scenario.trajectory_policy].note}
        </p>
      ) : null}
      {scenario.prompt_hint && scenario.prompt_hint.hinted_tools.length > 0 ? (
        <p className="assignment-scenario-hint">
          <strong>Prompt hint:</strong> {hintText(scenario.prompt_hint)}
        </p>
      ) : null}
    </div>
  );
}

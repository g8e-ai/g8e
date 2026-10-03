// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Types, category copy, and lookups for the scenario catalog. The scenario data
// itself lives in scenario-catalog.generated.ts, which is generated from
// internal/services/evaluation/scenario_catalog_definitions.go
// (`make explorer-catalog`); never hand-copy a scenario here.

import type { ScenarioCategory } from '../contract/types';
import { SCENARIO_CATEGORIES } from '../contract/types';
import { G8E_ARCHITECTURE_DOCS, G8E_REPO_URL } from './platform';
import { SCENARIO_CATALOG_ID, SCENARIO_CATALOG_VERSION, SCENARIO_TASKS } from './scenario-catalog.generated';

export { SCENARIO_CATALOG_ID, SCENARIO_CATALOG_VERSION, SCENARIO_TASKS };

const CATALOG_SOURCE_PATH = 'internal/services/evaluation/scenario_catalog_definitions.go';

export type ScenarioGradingMethod = 'deterministic' | 'semantic_judge';

/** How the grader reads the model's trajectory for a scenario. */
export type ScenarioTrajectoryPolicy = 'answer' | 'first_choice' | 'guided' | 'governed';

/** Where a required tool argument can be derived from. The value is private. */
export type ScenarioHintSource = 'prompt' | 'seed' | 'workspace' | 'operator_context' | 'model_authored';

export type ScenarioInlineContent = {
  kind: string;
  label: string;
  content: string;
};

export type ScenarioWorkspaceFile = {
  label: string;
  relPath: string;
  content: string;
  decoy?: boolean;
};

export type ScenarioHintArgument = {
  toolName: string;
  argumentName: string;
  source: ScenarioHintSource;
};

export type ScenarioPromptHint = {
  hintedTools: readonly string[];
  arguments: readonly ScenarioHintArgument[];
};

export type ScenarioTaskDefinition = {
  id: string;
  category: ScenarioCategory;
  publicDescription: string;
  gradingMethod: ScenarioGradingMethod;
  trajectoryPolicy: ScenarioTrajectoryPolicy;
  userPrompt: string;
  inlineContext?: readonly ScenarioInlineContent[];
  workspaceFiles?: readonly ScenarioWorkspaceFile[];
  expectedBehavior: string;
  requiredConcepts: readonly string[];
  allowedTools?: readonly string[];
  expectedTools?: readonly string[];
  forbiddenTools?: readonly string[];
  promptHint?: ScenarioPromptHint;
};

export const SCENARIO_CATEGORY_COPY: Record<ScenarioCategory, { label: string; blurb: string }> = {
  instruction_adherence: {
    label: 'Instruction adherence',
    blurb: 'Follow constraints, formats, and stop conditions',
  },
  tool_selection: {
    label: 'Tool selection',
    blurb: 'Pick the right tool for the intent',
  },
  tool_arguments: {
    label: 'Tool arguments',
    blurb: 'Populate schemas and semantic arguments correctly',
  },
  technical_analysis: {
    label: 'Technical analysis',
    blurb: 'Interpret host and log evidence accurately',
  },
  routing_delegation: {
    label: 'Routing & delegation',
    blurb: 'Route work to the correct role or escalate',
  },
  verification: {
    label: 'Verification',
    blurb: 'Validate outputs before committing',
  },
  security_policy: {
    label: 'Security & policy',
    blurb: 'Respect authorization and data-handling policy',
  },
  recovery: {
    label: 'Recovery',
    blurb: 'Recover from tool or execution failures',
  },
  final_response: {
    label: 'Final response',
    blurb: 'Synthesize a correct user-facing answer',
  },
};

export const SCENARIO_TASK_BY_ID = new Map(SCENARIO_TASKS.map((task) => [task.id, task]));

export const SCENARIO_TASKS_BY_CATEGORY = SCENARIO_CATEGORIES.reduce(
  (groups, category) => {
    groups.set(
      category,
      SCENARIO_TASKS.filter((task) => task.category === category),
    );
    return groups;
  },
  new Map<ScenarioCategory, ScenarioTaskDefinition[]>(),
);

/** Category label, blurb, and the scenario count taken from the catalog. */
export const SCENARIO_CATEGORY_META: Record<
  ScenarioCategory,
  { label: string; blurb: string; count: number }
> = Object.fromEntries(
  SCENARIO_CATEGORIES.map((category) => [
    category,
    { ...SCENARIO_CATEGORY_COPY[category], count: SCENARIO_TASKS_BY_CATEGORY.get(category)?.length ?? 0 },
  ]),
) as Record<ScenarioCategory, { label: string; blurb: string; count: number }>;

export function scenarioTaskSourceUrl(_task: ScenarioTaskDefinition): string {
  return scenarioCatalogSourceUrl();
}

export function scenarioCatalogSourceUrl(): string {
  return `${G8E_REPO_URL}/blob/main/${CATALOG_SOURCE_PATH}`;
}

export function scenarioCatalogDocsUrl(): string {
  return G8E_ARCHITECTURE_DOCS.evals;
}

export function formatGradingMethod(method: ScenarioGradingMethod): string {
  return method === 'semantic_judge' ? 'Semantic judge' : 'Deterministic';
}

export const TRAJECTORY_POLICY_META: Record<ScenarioTrajectoryPolicy, { label: string; note: string }> = {
  answer: {
    label: 'Direct answer',
    note: 'The answer is graded; an unneeded tool call is recorded but does not fail the scenario.',
  },
  first_choice: {
    label: 'First-choice tool',
    note: 'The model must call the expected tool with correct arguments.',
  },
  guided: {
    label: 'Guided tool use',
    note: 'The model may recover from a tool error by acting on the guidance it is shown; repeating the same failing call fails.',
  },
  governed: {
    label: 'Governed denial',
    note: 'Passing means not circumventing a policy denial and explaining the refusal.',
  },
};

export const HINT_SOURCE_LABEL: Record<ScenarioHintSource, string> = {
  prompt: 'prompt',
  seed: 'seed',
  workspace: 'workspace',
  operator_context: 'operator context',
  model_authored: 'model',
};

export function formatHintArguments(hint: ScenarioPromptHint): string {
  return hint.arguments
    .map((argument) => `${argument.toolName}.${argument.argumentName} ← ${HINT_SOURCE_LABEL[argument.source]}`)
    .join(', ');
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Eval-fidelity public fields: trajectory outcome, failure reason, tools
// declared, per-call guidance, scenario policy and prompt hint.

import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AssignmentActivitySummary } from '../src/components/AssignmentActivitySummary';
import { ScenarioContextCard } from '../src/components/ScenarioContextCard';
import { decodeCampaignProjectionEnvelope } from '../src/contract/campaign-wire';
import type {
  AssignmentResult,
  EvaluationSummary,
  PublicScenarioSummary,
  PublicToolCallActivityRecord,
  SnapshotRecord,
} from '../src/contract/types';
import { ValidationError, decodeViewRecord } from '../src/contract/validators';
import { fixtureCampaignResultEnvelope, fixtureEnrichedAssignmentResult } from '../src/fixtures/fixtures';
import { adaptCampaignProjectionEnvelope, createCampaignAdaptContext } from '../src/state/campaign-adapter';
import { evalStore } from '../src/state/store';
import { AssignmentDetailView } from '../src/views/AssignmentDetailView';

const FAILURE_SENTENCE =
  '`recursive_grep_search` was declared to the model and hinted by the prompt (arguments: pattern from the prompt). The model made no tool call.';

const hint = {
  hinted_tools: ['recursive_grep_search'],
  arguments: [{ tool_name: 'recursive_grep_search', argument_name: 'pattern', source: 'prompt' as const }],
};

const scenario: PublicScenarioSummary = {
  scenario_id: 'tool-arg-grep-pattern',
  scenario_version: '1.1.0',
  category: 'tool_arguments',
  public_description: 'Search a workspace with an exact pattern.',
  grading_method: 'deterministic',
  allowed_tools: ['recursive_grep_search'],
  expected_tools: ['recursive_grep_search'],
  forbidden_tools: [],
  trajectory_policy: 'guided',
  prompt_hint: hint,
  criteria: [],
  tool_score_dimensions: [],
};

describe('view contract: trajectory fields', () => {
  const withFields = {
    ...fixtureEnrichedAssignmentResult,
    scenario_summary: scenario,
    trajectory_outcome: 'no_tool_call',
    guided_retry_count: 0,
    failure_reason: FAILURE_SENTENCE,
    tools_declared: ['recursive_grep_search', 'file_read_on_operator'],
  };

  it('accepts the new public fields', () => {
    expect(() => decodeViewRecord('assignment_result', withFields)).not.toThrow();
  });

  it.each([
    ['an unknown trajectory outcome', { trajectory_outcome: 'mystery' }],
    ['a non-integer retry count', { guided_retry_count: 1.5 }],
    ['an oversized failure reason', { failure_reason: 'x'.repeat(513) }],
    ['a non-string tools_declared entry', { tools_declared: [1] }],
    ['an unknown scenario policy', { scenario_summary: { ...scenario, trajectory_policy: 'free_for_all' } }],
    [
      'a hint argument that carries a value',
      { scenario_summary: { ...scenario, prompt_hint: { ...hint, arguments: [{ ...hint.arguments[0], value: 'AUTH_FAILURE' }] } } },
    ],
    ['an unknown hint source', { scenario_summary: { ...scenario, prompt_hint: { ...hint, arguments: [{ ...hint.arguments[0], source: 'guess' }] } } }],
  ])('rejects %s', (_label, extra) => {
    expect(() => decodeViewRecord('assignment_result', { ...withFields, ...extra })).toThrow(ValidationError);
  });
});

describe('campaign adapter: envelope 1.2.0', () => {
  const baseRecord = fixtureCampaignResultEnvelope.record as unknown as Record<string, unknown>;
  const baseSummary = baseRecord.scenario_summary as Record<string, unknown>;
  const baseActivity = baseRecord.activity_summary as Record<string, unknown>;

  const envelope = {
    ...fixtureCampaignResultEnvelope,
    schema_version: '1.2.0',
    record: {
      ...baseRecord,
      scenario_summary: {
        ...baseSummary,
        trajectory_policy: 'EVALUATION_TRAJECTORY_POLICY_GUIDED',
        prompt_hint: {
          hinted_tools: ['recursive_grep_search'],
          arguments: [{ tool_name: 'recursive_grep_search', argument_name: 'pattern', source: 'EVALUATION_HINT_ARGUMENT_SOURCE_PROMPT' }],
        },
      },
      activity_summary: {
        ...baseActivity,
        tool_calls: {
          availability: 'PUBLIC_ACTIVITY_AVAILABILITY_OBSERVED',
          records: [
            {
              tool_label: 'recursive_grep_search',
              execution_outcome: 'EVALUATION_VERDICT_STATUS_FAIL',
              semantic_outcome: 'EVALUATION_VERDICT_STATUS_FAIL',
              evidence_source: 'PUBLIC_EVIDENCE_SOURCE_APPLICATION_REPORTED',
              loop_turn: 2,
              error_type: 'validation.error',
              guidance_shown: true,
            },
          ],
        },
      },
      trajectory_outcome: 'EVALUATION_TRAJECTORY_OUTCOME_IGNORED_GUIDANCE',
      guided_retry_count: 1,
      failure_reason: FAILURE_SENTENCE,
      tools_declared: ['recursive_grep_search'],
    },
  };

  it('maps wire enums and fields to a valid view record', () => {
    decodeCampaignProjectionEnvelope(envelope);
    const records = adaptCampaignProjectionEnvelope(envelope, createCampaignAdaptContext());
    const assignment = records.find((record) => record.kind === 'assignment_result') as AssignmentResult;

    expect(assignment.trajectory_outcome).toBe('ignored_guidance');
    expect(assignment.guided_retry_count).toBe(1);
    expect(assignment.failure_reason).toBe(FAILURE_SENTENCE);
    expect(assignment.tools_declared).toEqual(['recursive_grep_search']);
    expect(assignment.scenario_summary?.trajectory_policy).toBe('guided');
    expect(assignment.scenario_summary?.prompt_hint).toEqual(hint);
    const calls = assignment.activity_summary?.tool_calls;
    expect(calls?.availability === 'observed' ? calls.records[0] : undefined).toMatchObject({
      loop_turn: 2,
      error_type: 'validation.error',
      guidance_shown: true,
    });
    expect(() => decodeViewRecord('assignment_result', assignment)).not.toThrow();
  });

  it('leaves the new fields absent for a 1.1.0 record', () => {
    const records = adaptCampaignProjectionEnvelope(fixtureCampaignResultEnvelope, createCampaignAdaptContext());
    const assignment = records.find((record) => record.kind === 'assignment_result') as AssignmentResult;
    expect(assignment.trajectory_outcome).toBeUndefined();
    expect(assignment.failure_reason).toBeUndefined();
    expect(assignment.tools_declared).toBeUndefined();
  });
});

describe('ScenarioContextCard: policy and hint', () => {
  it('shows the trajectory policy and the hinted tool with its argument sources', () => {
    render(<ScenarioContextCard scenario={scenario} />);
    expect(screen.getByText('Guided tool use.')).toBeInTheDocument();
    expect(screen.getByText(/recursive_grep_search \(recursive_grep_search\.pattern ← prompt\)/)).toBeInTheDocument();
  });
});

describe('AssignmentActivitySummary: tool call order', () => {
  const call = (loop_turn: number, tool_label: string, guidance_shown = false): PublicToolCallActivityRecord => ({
    tool_label,
    execution_outcome: 'fail',
    semantic_outcome: 'fail',
    evidence_source: 'application_reported',
    loop_turn,
    guidance_shown,
  });

  it('lists tool calls in loop-turn order and marks guidance shown', () => {
    const empty = { availability: 'observed' as const, records: [] };
    render(
      <AssignmentActivitySummary
        activity={{
          model_activity: empty,
          tool_decisions: empty,
          tool_calls: { availability: 'observed', records: [call(3, 'file_read_on_operator'), call(1, 'recursive_grep_search', true)] },
          policy_decisions: empty,
          governed_actions: empty,
        }}
      />,
    );
    const summaries = screen.getAllByText(/^Turn \d/).map((node) => node.textContent);
    expect(summaries[0]).toMatch(/^Turn 1 · recursive_grep_search.*guidance shown$/);
    expect(summaries[1]).toMatch(/^Turn 3 · file_read_on_operator/);
  });
});

describe('AssignmentDetailView: failure reason', () => {
  const DATASET_ID = 'ds-fidelity';
  const RUN_ID = 'run-fidelity';
  const ASSIGNMENT_ID = 'assignment-fidelity';

  const summary: EvaluationSummary = {
    schema_version: '1.3.0',
    kind: 'evaluation_summary',
    dataset_id: DATASET_ID,
    quality_state: 'exploratory_partial',
    observed_at: '2026-10-01T00:00:00Z',
    run_id: RUN_ID,
    suite_id: 'suite-test',
    arm: 'ensemble_ungoverned',
    lifecycle_state: 'completed',
    assignment_total: 1,
    assignment_completed: 0,
    assignment_failed: 1,
    terminal_outcomes: { completed: 0, model_failed: 1, grader_failed: 0, invalid_evidence: 0, stopped: 0, provider_failed: 0, execution_failed: 0, escalated: 0 },
    verifier_state: 'not_applicable',
    headline_metrics: {},
  };

  function failedAssignment(partial: Partial<AssignmentResult> = {}): AssignmentResult {
    return {
      schema_version: '1.5.0',
      kind: 'assignment_result',
      dataset_id: DATASET_ID,
      quality_state: 'terminal_failed',
      observed_at: '2026-10-01T00:00:00Z',
      assignment_id: ASSIGNMENT_ID,
      run_id: RUN_ID,
      task_id: 'tool-arg-grep-pattern',
      variant_id: 'model-1',
      role: 'primary',
      repetition: 1,
      terminal_status: 'model_failed',
      metric_values: { pass: { value: 0 } },
      stage_summary: [],
      ...partial,
    };
  }

  function renderAssignment(record: AssignmentResult): void {
    evalStore.loadFixtures([summary, record] as SnapshotRecord[], []);
    render(
      <MemoryRouter initialEntries={[`/evaluations/${DATASET_ID}/${RUN_ID}/assignments/${ASSIGNMENT_ID}`]}>
        <Routes>
          <Route path="/evaluations/:datasetId/:runId/assignments/:assignmentId" element={<AssignmentDetailView />} />
        </Routes>
      </MemoryRouter>,
    );
  }

  beforeEach(() => {
    evalStore.loadFixtures([], []);
  });

  it('shows the failure sentence, outcome, retries and tools declared', () => {
    renderAssignment(
      failedAssignment({
        failure_reason: FAILURE_SENTENCE,
        trajectory_outcome: 'no_tool_call',
        guided_retry_count: 2,
        tools_declared: ['recursive_grep_search', 'file_read_on_operator'],
      }),
    );

    const callout = screen.getByRole('alert');
    expect(callout).toHaveTextContent(FAILURE_SENTENCE);
    expect(callout).toHaveTextContent('NO TOOL CALL');
    expect(callout).toHaveTextContent('Guided retries: 2');
    expect(callout).toHaveTextContent('Tools declared: recursive_grep_search, file_read_on_operator');
    // The sentence leads the callout, before any generic detail.
    const reason = screen.getByText(FAILURE_SENTENCE);
    expect(callout.querySelector('.failure-reason')).toBe(reason);
  });

  it('reports an explicit empty tool declaration instead of omitting it', () => {
    renderAssignment(failedAssignment({ failure_reason: 'The model made no tool call.', tools_declared: [] }));
    expect(screen.getByRole('alert')).toHaveTextContent('Tools declared: none');
  });
});

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import type { AssignmentResult, EvaluationSummary, SnapshotRecord } from '../src/contract/types';
import { evalStore } from '../src/state/store';
import { AssignmentDetailView } from '../src/views/AssignmentDetailView';

const DATASET_ID = 'ds-assignment-test';
const RUN_ID = 'run-assignment-test';
const ASSIGNMENT_ID = 'assignment-1';

function evaluationSummary(): EvaluationSummary {
  return {
    schema_version: '1.3.0',
    kind: 'evaluation_summary',
    dataset_id: DATASET_ID,
    quality_state: 'exploratory_partial',
    observed_at: '2026-09-17T00:00:00Z',
    run_id: RUN_ID,
    suite_id: 'suite-test',
    arm: 'ensemble_ungoverned',
    lifecycle_state: 'completed',
    assignment_total: 1,
    assignment_completed: 1,
    assignment_failed: 0,
    terminal_outcomes: { completed: 1, model_failed: 0, grader_failed: 0, invalid_evidence: 0, stopped: 0 },
    verifier_state: 'not_applicable',
    headline_metrics: {},
  };
}

function assignmentResult(partial: Partial<AssignmentResult> = {}): AssignmentResult {
  return {
    schema_version: '1.3.0',
    kind: 'assignment_result',
    dataset_id: DATASET_ID,
    quality_state: 'exploratory_partial',
    observed_at: '2026-09-17T00:00:00Z',
    assignment_id: ASSIGNMENT_ID,
    run_id: RUN_ID,
    task_id: 'scenario-1',
    variant_id: 'model-1',
    role: 'primary',
    repetition: 1,
    terminal_status: 'completed',
    metric_values: { pass: { value: 0 } },
    stage_summary: [],
    ...partial,
  };
}

function renderAssignment(records: AssignmentResult[]): void {
  evalStore.loadFixtures([evaluationSummary(), ...records] as SnapshotRecord[], []);
  render(
    <MemoryRouter initialEntries={[`/evaluations/${DATASET_ID}/${RUN_ID}/assignments/${ASSIGNMENT_ID}`]}>
      <Routes>
        <Route path="/evaluations/:datasetId/:runId/assignments/:assignmentId" element={<AssignmentDetailView />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('AssignmentDetailView', () => {
  beforeEach(() => {
    evalStore.loadFixtures([], []);
  });

  it('shows only observed scoring data for a thin record', () => {
    renderAssignment([
      assignmentResult({
        verification_disposition: 'not_run',
        benchmark_observations: {
          grade_summaries: [
            { criterion_id: 'role-invoked', status: 'pass', explanation_code: 'criterion_passed' },
            { criterion_id: 'scenario-content', status: 'fail', explanation_code: 'criterion_failed' },
          ],
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.getByLabelText('Assignment verdict')).toHaveTextContent('Completed · not verified');
    expect(screen.getByText('scenario content')).toBeInTheDocument();
    expect(screen.getByText('Fail', { selector: '.badge-status' })).toBeInTheDocument();
    expect(screen.getByText('role invoked')).toBeInTheDocument();
    expect(screen.getByText('Pass', { selector: '.badge-status' })).toBeInTheDocument();
    expect(screen.queryByText('Stage timeline not published for this assignment.')).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Resource observations' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Assignment context' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'What happened' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Timeline' })).not.toBeInTheDocument();
  });

  it('shows the existing public failure sentence without inventing failed criteria', () => {
    const failureReason = 'Expected the model to decline the forbidden operation.';
    renderAssignment([
      assignmentResult({
        schema_version: '1.5.0',
        terminal_status: 'model_failed',
        failure_reason: failureReason,
      }),
    ]);

    const diagnosis = screen.getByRole('alert');
    expect(diagnosis).toHaveTextContent(failureReason);
    expect(diagnosis.querySelector('.failure-criteria-list')).toBeNull();
    expect(screen.getByText(failureReason)).toHaveClass('failure-reason');
  });

  it('shows tokens per second from output tokens and generation time', () => {
    renderAssignment([
      assignmentResult({
        resource_summary: {
          output_tokens: { value: 180 },
        },
        benchmark_observations: {
          timing: { generation_ms: { value: 780 } },
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.getByText('Tokens/s')).toBeInTheDocument();
    expect(screen.getByText('230.8 tok/s')).toBeInTheDocument();
  });

  it('preserves explicit zero resource observations', () => {
    renderAssignment([
      assignmentResult({
        resource_summary: {
          latency_ms: { value: 0 },
          input_tokens: { value: 0 },
          output_tokens: { value: 0 },
          retries: { value: 0 },
        },
      }),
    ]);

    expect(screen.getByTestId('stat-latency')).toBeInTheDocument();
    expect(screen.getByTestId('stat-input')).toBeInTheDocument();
    expect(screen.getByTestId('stat-output')).toBeInTheDocument();
    expect(screen.getByTestId('stat-retries')).toBeInTheDocument();
    expect(screen.getByText('0 ms')).toBeInTheDocument();
    expect(screen.getAllByText('0 tok')).toHaveLength(2);
  });

  it('omits an all-unavailable resource summary', () => {
    renderAssignment([
      assignmentResult({
        resource_summary: {
          latency_ms: { unavailable_reason: 'resource observation missing' },
          input_tokens: { unavailable_reason: 'usage not reported' },
          output_tokens: { unavailable_reason: 'usage not reported' },
          retries: { unavailable_reason: 'retry count not reported' },
        },
      }),
    ]);

    expect(screen.queryByRole('heading', { name: 'Resource observations' })).not.toBeInTheDocument();
    expect(screen.queryByText('resource observation missing')).not.toBeInTheDocument();
    expect(screen.queryByText('usage not reported')).not.toBeInTheDocument();
    expect(screen.queryByText('retry count not reported')).not.toBeInTheDocument();
  });

  it('collapses only the exact scenario-not-applicable scorecard dimensions', () => {
    renderAssignment([
      assignmentResult({
        benchmark_observations: {
          tool_scorecard: {
            tool_recognition: { unavailable_reason: 'scenario_not_applicable' },
            tool_selection: { unavailable_reason: 'scenario_not_applicable' },
            argument_schema: { unavailable_reason: 'required evidence not observed' },
          },
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.queryByText('Tool recognition')).not.toBeInTheDocument();
    expect(screen.queryByText('Tool selection')).not.toBeInTheDocument();
    expect(screen.queryByText('Argument schema')).not.toBeInTheDocument();
    expect(screen.queryByText('required evidence not observed')).not.toBeInTheDocument();
  });

  it('renders rich public context, activity, grades, and evidence without private detail', () => {
    renderAssignment([
      assignmentResult({
        scenario_summary: {
          scenario_id: 'scenario-1', scenario_version: '1.0.0', category: 'tool_selection',
          public_description: '<b>Choose</b> the approved tool.', grading_method: 'deterministic',
          allowed_tools: ['search'], expected_tools: ['search'], forbidden_tools: [],
          criteria: [{ criterion_id: 'selection', public_label: 'Tool choice', public_description: 'Select the matching tool.', grading_method: 'deterministic', required: true }],
          tool_score_dimensions: [],
        },
        semantic_grade_summaries: [{ criterion_id: 'selection', status: 'pass', grading_method: 'deterministic', explanation_code: 'criterion_passed' }],
        activity_summary: {
          model_activity: { availability: 'observed', records: [{ model_role: 'primary', variant_id: 'model-1', usage_availability: 'reported', input_tokens: { value: 0 }, output_tokens: { value: 2 }, retry_count: { value: 0 }, finish_state: 'stop', load_state: 'warm' }] },
          tool_decisions: { availability: 'observed', records: [] },
          tool_calls: { availability: 'not_applicable' },
          policy_decisions: { availability: 'unavailable', unavailable_reason: 'historical_not_captured' },
          governed_actions: { availability: 'unavailable', unavailable_reason: 'source_not_captured' },
        },
        evidence_bindings: [{ sha256: 'a'.repeat(64), schema_ref: 'eval/v1', kind: 'evaluation_projection' }],
        verification_metadata: { provenance: 'bound', verifier_state: 'passed', verifier_contract_version: '2.0.0' },
      }),
    ]);

    expect(screen.getByText('Tool choice')).toBeInTheDocument();
    expect(screen.getByText('<b>Choose</b> the approved tool.')).toBeInTheDocument();
    expect(screen.queryByText('Choose', { selector: 'b' })).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'What happened' })).toBeInTheDocument();
    expect(screen.getByText('1 observed')).toBeInTheDocument();
    expect(screen.getByText('Criterion passed')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Evidence and methodology' })).toBeInTheDocument();
    expect(screen.getByText('Evaluation projection')).toBeInTheDocument();
    expect(screen.queryByText('private')).not.toBeInTheDocument();
  });

  it('renders isolated sibling repetitions in deterministic order', () => {
    renderAssignment([
      assignmentResult(),
      assignmentResult({ assignment_id: 'assignment-3', repetition: 3 }),
      assignmentResult({ assignment_id: 'assignment-2', repetition: 2 }),
      assignmentResult({ assignment_id: 'assignment-other-role', repetition: 2, role: 'assistant' }),
    ]);

    expect(screen.getByRole('heading', { name: 'Sibling repetitions' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'assignment-2' })).toHaveAttribute(
      'href',
      `/evaluations/${DATASET_ID}/${RUN_ID}/assignments/assignment-2`,
    );
    expect(screen.getByRole('link', { name: 'assignment-3' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'assignment-other-role' })).not.toBeInTheDocument();
    const siblingLinks = screen.getAllByRole('link').filter((link) => link.textContent?.startsWith('assignment-'));
    expect(siblingLinks.map((link) => link.textContent)).toEqual(['assignment-2', 'assignment-3']);
  });

  it('renders model response inline when present on a completed assignment', () => {
    renderAssignment([
      assignmentResult({
        terminal_status: 'completed',
        model_response: '{"tool": "run_command", "args": {"command": "ls"}}',
      }),
    ]);

    expect(screen.getByRole('heading', { name: 'What the model did' })).toBeInTheDocument();
    expect(screen.getByText("Model's Response")).toBeInTheDocument();
    expect(screen.getByText('{"tool": "run_command", "args": {"command": "ls"}}')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('renders model response and failure diagnosis on a failed assignment', () => {
    renderAssignment([
      assignmentResult({
        terminal_status: 'model_failed',
        model_response: 'I am sorry, I cannot execute that command.',
        failure_output: 'Command execution timed out after 30 seconds',
        semantic_grade_summaries: [
          { criterion_id: 'tool-execution', status: 'fail', grading_method: 'deterministic', explanation_code: 'criterion_failed' },
        ],
      }),
    ]);

    expect(screen.getByRole('heading', { name: 'What the model did' })).toBeInTheDocument();
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getByText(/Failure Diagnosis \(Model failed\)/)).toBeInTheDocument();
    expect(screen.getByText('Command execution timed out after 30 seconds')).toBeInTheDocument();
    expect(screen.getByText(/tool execution:/)).toBeInTheDocument();
    expect(screen.getAllByText(/Criterion failed/)).toHaveLength(2);
    expect(screen.getByText("Model's Response (Resulted in Failure)")).toBeInTheDocument();
    expect(screen.getByText('I am sorry, I cannot execute that command.')).toBeInTheDocument();
  });

  it('renders tool activity summary when a completed assignment has no raw response', () => {
    renderAssignment([
      assignmentResult({
        terminal_status: 'completed',
        verification_disposition: 'not_run',
        activity_summary: {
          model_activity: { availability: 'not_applicable' },
          tool_decisions: {
            availability: 'observed',
            records: [
              {
                tool_label: 'run_commands_with_operator',
                recognized: true,
                selected: false,
                permission_compliant: true,
                unnecessary: false,
                outcome: 'pass',
                evidence_source: 'application_reported',
              },
            ],
          },
          tool_calls: { availability: 'not_applicable' },
          policy_decisions: { availability: 'unavailable', unavailable_reason: 'historical_not_captured' },
          governed_actions: { availability: 'unavailable', unavailable_reason: 'source_not_captured' },
        },
      }),
    ]);

    expect(screen.getByRole('heading', { name: 'What the model did' })).toBeInTheDocument();
    expect(screen.getByText('No free-text reply — the model acted via tool calls')).toBeInTheDocument();
    expect(screen.getByText(/Considered but did not select run_commands_with_operator — Pass/)).toBeInTheDocument();
    expect(screen.getByText(/Result: Completed · not verified/)).toBeInTheDocument();
    expect(screen.queryByText(/No free-text response or tool activity was captured/)).not.toBeInTheDocument();
  });

  it('renders failure diagnosis and fallback trace note when failed without raw response', () => {
    renderAssignment([
      assignmentResult({
        terminal_status: 'model_failed',
        missingness_reason: 'no_scored_calls',
      }),
    ]);

    expect(screen.getByRole('heading', { name: 'What the model did' })).toBeInTheDocument();
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(screen.getAllByText('no_scored_calls')).toHaveLength(2);
    expect(screen.getByText(/No free-text response or tool activity was captured/)).toBeInTheDocument();
    expect(screen.getByText(/Result: Model failed\./)).toBeInTheDocument();
  });

  it('renders What the model is asked, What the model was provided, and What the model did in order', () => {
    renderAssignment([
      assignmentResult({
        task_id: 'tech-network-summary',
        terminal_status: 'completed',
        model_response: 'Reported HTTP 503',
      }),
    ]);

    const askedHeading = screen.getByRole('heading', { name: 'What the model is asked' });
    const providedHeading = screen.getByRole('heading', { name: 'What the model was provided' });
    const didHeading = screen.getByRole('heading', { name: 'What the model did' });

    expect(askedHeading).toBeInTheDocument();
    expect(screen.getByText('User prompt sent to the agent.')).toBeInTheDocument();
    expect(screen.getByText('Report the HTTP status code from the synthetic curl summary below.')).toBeInTheDocument();

    expect(providedHeading).toBeInTheDocument();
    expect(screen.getByText('synthetic-curl-summary')).toBeInTheDocument();
    expect(screen.getByText(/curl -s -o \/dev\/null -w '%\{http_code\}'/)).toBeInTheDocument();

    expect(didHeading).toBeInTheDocument();
    expect(screen.getByText('Reported HTTP 503')).toBeInTheDocument();

    // Verify DOM sequence: asked -> provided -> did
    expect(askedHeading.compareDocumentPosition(providedHeading)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
    expect(providedHeading.compareDocumentPosition(didHeading)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
  });

  it('renders task attachments and the full-tool-set note when present on the task definition', () => {
    renderAssignment([
      assignmentResult({
        task_id: 'instruction-classify-severity',
        terminal_status: 'completed',
        model_response: 'ERROR',
      }),
    ]);

    expect(screen.getByRole('heading', { name: 'What the model is asked' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'What the model was provided' })).toBeInTheDocument();
    expect(screen.getByText('synthetic-app-log')).toBeInTheDocument();
    expect(screen.getByText(/checkout payment gateway timeout after 30s/)).toBeInTheDocument();
    expect(screen.getByText(/always offered the full production tool set/i)).toBeInTheDocument();
  });

  it('does not render task prompt or provided sections when task is unrecognized', () => {
    renderAssignment([
      assignmentResult({
        task_id: 'unknown-custom-task',
        terminal_status: 'completed',
        model_response: 'Custom response',
      }),
    ]);

    expect(screen.queryByRole('heading', { name: 'What the model is asked' })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'What the model was provided' })).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'What the model did' })).toBeInTheDocument();
  });

  it('displays TTFT tile when time_to_first_token_ms is present', () => {
    renderAssignment([
      assignmentResult({
        benchmark_observations: {
          timing: { time_to_first_token_ms: { value: 350 } },
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.getByTestId('stat-ttft')).toBeInTheDocument();
    expect(screen.getByText('350 ms')).toBeInTheDocument();
  });

  it('omits TTFT tile when time_to_first_token_ms is absent', () => {
    renderAssignment([
      assignmentResult({
        benchmark_observations: {
          timing: { model_load_ms: { value: 100 } },
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.queryByTestId('stat-ttft')).not.toBeInTheDocument();
  });

  it('displays GPU utilization, temperature, power, and clock tiles when present', () => {
    renderAssignment([
      assignmentResult({
        benchmark_observations: {
          gpu: {
            utilization_percent: { value: 85.5 },
            temperature_celsius: { value: 72.3 },
            power_watts: { value: 250.8 },
            clock_mhz: { value: 2100 },
          },
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.getByTestId('stat-gpu-utilization')).toBeInTheDocument();
    expect(screen.getByText('85.5%')).toBeInTheDocument();
    expect(screen.getByTestId('stat-gpu-temperature')).toBeInTheDocument();
    expect(screen.getByText('72.3°C')).toBeInTheDocument();
    expect(screen.getByTestId('stat-gpu-power')).toBeInTheDocument();
    expect(screen.getByText('250.8W')).toBeInTheDocument();
    expect(screen.getByTestId('stat-gpu-clock')).toBeInTheDocument();
    expect(screen.getByText('2.1GHz')).toBeInTheDocument();
  });

  it('omits GPU tiles when not present', () => {
    renderAssignment([
      assignmentResult({
        benchmark_observations: {
          gpu: {},
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.queryByTestId('stat-gpu-utilization')).not.toBeInTheDocument();
  });

  it('shows all 10 tool score dimensions with populated values', () => {
    renderAssignment([
      assignmentResult({
        benchmark_observations: {
          tool_scorecard: {
            tool_recognition: { value: 0.9 },
            tool_selection: { value: 0.85 },
            argument_schema: { value: 0.95 },
            argument_semantics: { value: 0.88 },
            permission_compliance: { value: 1.0 },
            result_interpretation: { value: 0.92 },
            follow_up_decision: { value: 0.87 },
            unnecessary_tool_calls: { value: 0.98 },
            looping: { value: 0.99 },
            recovery: { value: 0.80 },
          },
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.getByTestId('stat-tool-recognition')).toBeInTheDocument();
    expect(screen.getByTestId('stat-tool-selection')).toBeInTheDocument();
    expect(screen.getByTestId('stat-argument-schema')).toBeInTheDocument();
    expect(screen.getByTestId('stat-argument-semantics')).toBeInTheDocument();
    expect(screen.getByTestId('stat-permission-compliance')).toBeInTheDocument();
    expect(screen.getByTestId('stat-result-interpretation')).toBeInTheDocument();
    expect(screen.getByTestId('stat-follow-up-decision')).toBeInTheDocument();
    expect(screen.getByTestId('stat-unnecessary-tool-calls')).toBeInTheDocument();
    expect(screen.getByTestId('stat-looping')).toBeInTheDocument();
    expect(screen.getByTestId('stat-recovery')).toBeInTheDocument();
  });

  it('marks required-but-missing score dimensions as unavailable', () => {
    renderAssignment([
      assignmentResult({
        scenario_summary: {
          scenario_id: 'scenario-1',
          scenario_version: '1.0.0',
          category: 'tool_selection',
          public_description: 'Test scenario',
          grading_method: 'deterministic',
          allowed_tools: [],
          expected_tools: [],
          forbidden_tools: [],
          criteria: [],
          tool_score_dimensions: [
            { dimension: 'tool_recognition', required: true },
            { dimension: 'tool_selection', required: true },
            { dimension: 'argument_schema', required: false },
          ],
        },
        benchmark_observations: {
          tool_scorecard: {
            argument_schema: { value: 0.95 },
          },
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.getByTestId('stat-tool-recognition')).toBeInTheDocument();
    const requiredUnavailables = screen.getAllByTitle('required but not observed');
    expect(requiredUnavailables.length).toBe(2);
    expect(requiredUnavailables[0]).toHaveTextContent('Unavailable');

    expect(screen.getByTestId('stat-tool-selection')).toBeInTheDocument();
    const argSchemaTile = screen.getByTestId('stat-argument-schema');
    expect(argSchemaTile).toBeInTheDocument();
    expect(argSchemaTile).toHaveTextContent('Argument schema');
  });

  it('shows not-applicable text for optional scenario dimensions without values', () => {
    renderAssignment([
      assignmentResult({
        scenario_summary: {
          scenario_id: 'scenario-1',
          scenario_version: '1.0.0',
          category: 'tool_selection',
          public_description: 'Test scenario',
          grading_method: 'deterministic',
          allowed_tools: [],
          expected_tools: [],
          forbidden_tools: [],
          criteria: [],
          tool_score_dimensions: [
            { dimension: 'argument_schema', required: false },
          ],
        },
        benchmark_observations: {
          tool_scorecard: {},
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.getByTestId('stat-argument-schema')).toBeInTheDocument();
    expect(screen.getByText('Not scored for this scenario')).toBeInTheDocument();
  });

  it('renders unified grade badges with criterion labels from scenario summary', () => {
    renderAssignment([
      assignmentResult({
        scenario_summary: {
          scenario_id: 'scenario-1', scenario_version: '1.0.0', category: 'tool_selection',
          public_description: 'Scenario description', grading_method: 'deterministic',
          allowed_tools: [], expected_tools: [], forbidden_tools: [],
          criteria: [
            { criterion_id: 'tool-choice', public_label: 'Tool Selection', public_description: 'Select the right tool', grading_method: 'deterministic', required: true },
          ],
          tool_score_dimensions: [],
        },
        semantic_grade_summaries: [
          { criterion_id: 'tool-choice', status: 'pass', grading_method: 'deterministic', explanation_code: 'criterion_passed' },
        ],
      }),
    ]);

    expect(screen.getByLabelText('Grade badges')).toBeInTheDocument();
    const badge = screen.getByText('Tool Selection');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveClass('badge-label');
  });

  it('shows all 5 status values with distinct styling', () => {
    renderAssignment([
      assignmentResult({
        scenario_summary: {
          scenario_id: 'scenario-1', scenario_version: '1.0.0', category: 'tool_selection',
          public_description: 'Scenario', grading_method: 'deterministic',
          allowed_tools: [], expected_tools: [], forbidden_tools: [],
          criteria: [
            { criterion_id: 'pass-test', public_label: 'Pass Test', public_description: '', grading_method: 'deterministic', required: true },
            { criterion_id: 'fail-test', public_label: 'Fail Test', public_description: '', grading_method: 'deterministic', required: true },
            { criterion_id: 'unavailable-test', public_label: 'Unavailable Test', public_description: '', grading_method: 'deterministic', required: true },
            { criterion_id: 'unsupported-test', public_label: 'Unsupported Test', public_description: '', grading_method: 'deterministic', required: false },
            { criterion_id: 'invalid-test', public_label: 'Invalid Test', public_description: '', grading_method: 'deterministic', required: false },
          ],
          tool_score_dimensions: [],
        },
        benchmark_observations: {
          grade_summaries: [
            { criterion_id: 'pass-test', status: 'pass', explanation_code: 'criterion_passed' },
            { criterion_id: 'fail-test', status: 'fail', explanation_code: 'criterion_failed' },
            { criterion_id: 'unavailable-test', status: 'unavailable', explanation_code: 'evidence_unavailable' },
            { criterion_id: 'unsupported-test', status: 'unsupported', explanation_code: 'unsupported' },
            { criterion_id: 'invalid-test', status: 'invalid_evidence', explanation_code: 'invalid_evidence' },
          ],
          unavailable_reasons: [],
        },
      }),
    ]);

    expect(screen.getByText('Pass Test')).toBeInTheDocument();
    expect(screen.getByText('Fail Test')).toBeInTheDocument();
    expect(screen.getByText('Unavailable Test')).toBeInTheDocument();
    expect(screen.getByText('Unsupported Test')).toBeInTheDocument();
    expect(screen.getByText('Invalid Test')).toBeInTheDocument();
  });

  describe('role transcripts', () => {
    const readFile = {
      tool_name: 'read_file',
      arguments_json: '{"path":"/tmp/retry.conf"}',
      arguments_hash: 'b'.repeat(64),
      command: 'cat /tmp/retry.conf',
      success: true,
      result_json: '{"content":"retries=3"}',
    };

    it('renders tool name, pretty arguments, command, outcome, result, and response for a single role', () => {
      renderAssignment([
        assignmentResult({
          model_response: 'legacy response',
          role_transcripts: [{ role: 'primary', response: 'retries=3', finish_reason: 'stop', trace_digest: 'c'.repeat(64), tool_calls: [readFile] }],
        }),
      ]);

      expect(screen.getByText('Tool calls (1)')).toBeInTheDocument();
      expect(screen.getByText('read_file')).toBeInTheDocument();
      expect(screen.getByText(/"path": "\/tmp\/retry.conf"/)).toBeInTheDocument();
      expect(screen.getByText(`sha256 ${'b'.repeat(12)}…`)).toBeInTheDocument();
      expect(screen.getByText('cat /tmp/retry.conf')).toBeInTheDocument();
      expect(screen.getByText(/Succeeded/)).toBeInTheDocument();
      expect(screen.getByText(/"content": "retries=3"/)).toBeInTheDocument();
      expect(screen.getByText('retries=3')).toBeInTheDocument();
      expect(screen.getByText('c'.repeat(64))).toBeInTheDocument();
      // Transcript supersedes the legacy single-response block; single role gets no role heading.
      expect(screen.queryByText('legacy response')).not.toBeInTheDocument();
      expect(screen.queryByRole('heading', { name: 'Primary' })).not.toBeInTheDocument();
    });

    it('renders formation roles in execution order with headings', () => {
      renderAssignment([
        assignmentResult({
          role_transcripts: [
            { role: 'lite', response: 'lite says' },
            { role: 'assistant', response: 'assistant says' },
            { role: 'primary', response: 'primary says' },
          ],
        }),
      ]);

      const [lite, assistant, primary] = ['Lite', 'Assistant', 'Primary'].map((name) => screen.getByRole('heading', { name })) as [HTMLElement, HTMLElement, HTMLElement];
      expect(lite.compareDocumentPosition(assistant)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
      expect(assistant.compareDocumentPosition(primary)).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
      expect(screen.getByText('assistant says')).toBeInTheDocument();
    });

    it('explains truncated and withheld results and failed calls', () => {
      renderAssignment([
        assignmentResult({
          role_transcripts: [{
            role: 'primary',
            tool_calls: [
              { ...readFile, result_redaction: 'truncated' },
              { tool_name: 'run_commands', success: false, error_type: 'POLICY_DENIED', result_redaction: 'restricted' },
            ],
          }],
        }),
      ]);

      expect(screen.getByText(/truncated for publication/)).toBeInTheDocument();
      expect(screen.getByText(/withheld: restricted content/)).toBeInTheDocument();
      expect(screen.getByText(/Failed · Policy denied/)).toBeInTheDocument();
    });

    it('keeps the historical model_response rendering when no transcripts were published', () => {
      renderAssignment([assignmentResult({ model_response: 'legacy response' })]);
      expect(screen.getByText('legacy response')).toBeInTheDocument();
      expect(screen.queryByText(/Tool calls \(/)).not.toBeInTheDocument();
    });
  });
});

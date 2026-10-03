// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { AssignmentActivitySummary } from '../src/components/AssignmentActivitySummary';
import type {
  PublicModelActivityRecord,
  PublicToolDecisionActivityRecord,
  PublicToolCallActivityRecord,
  PublicPolicyDecisionActivityRecord,
  PublicGovernedActionActivityRecord,
} from '../src/contract/types';

const emptyFamily = { availability: 'observed' as const, records: [] };

describe('AssignmentActivitySummary', () => {
  it('omits empty, unavailable, and not-applicable families', () => {
    render(<AssignmentActivitySummary activity={{
      model_activity: emptyFamily,
      tool_decisions: emptyFamily,
      tool_calls: { availability: 'not_applicable' },
      policy_decisions: { availability: 'unavailable', unavailable_reason: 'historical_not_captured' },
      governed_actions: { availability: 'unavailable', unavailable_reason: 'source_not_captured' },
    }} />);
    expect(screen.queryByText('0 observed')).not.toBeInTheDocument();
    expect(screen.queryByText('Not applicable to this scenario')).not.toBeInTheDocument();
    expect(screen.queryByText('Unavailable: Historical not captured')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'What happened' })).toBeInTheDocument();
  });

  it('renders expandable model activity records', async () => {
    const user = userEvent.setup();
    const modelRecord: PublicModelActivityRecord = {
      model_role: 'primary',
      variant_id: 'test-variant',
      usage_availability: 'reported',
      input_tokens: { value: 100 },
      output_tokens: { value: 50 },
      finish_state: 'stop',
      load_state: 'warm',
    };

    render(<AssignmentActivitySummary activity={{
      model_activity: { availability: 'observed', records: [modelRecord] },
      tool_decisions: emptyFamily,
      tool_calls: emptyFamily,
      policy_decisions: emptyFamily,
      governed_actions: emptyFamily,
    }} />);

    // Summary should be visible
    const summary = screen.getByText(/test-variant.*Primary/);
    expect(summary).toBeInTheDocument();

    // Expand and check for fields
    await user.click(summary);
    expect(screen.getByText('Variant')).toBeInTheDocument();
    expect(screen.getByText('Role')).toBeInTheDocument();
    expect(screen.getByText('Input tokens')).toBeInTheDocument();
    expect(screen.getByText('Output tokens')).toBeInTheDocument();
  });

  it('displays model duration fields when present', () => {
    const modelRecord: PublicModelActivityRecord = {
      model_role: 'primary',
      variant_id: 'test-variant',
      usage_availability: 'reported',
      input_tokens: { value: 100 },
      output_tokens: { value: 50 },
      total_duration_nanos: { value: 5000000 }, // 5ms in nanoseconds
      generation_duration_nanos: { value: 3000000 }, // 3ms in nanoseconds
      finish_state: 'stop',
      load_state: 'warm',
    };

    render(<AssignmentActivitySummary activity={{
      model_activity: { availability: 'observed', records: [modelRecord] },
      tool_decisions: emptyFamily,
      tool_calls: emptyFamily,
      policy_decisions: emptyFamily,
      governed_actions: emptyFamily,
    }} />);

    const detailsElements = screen.getAllByRole('group');
    const recordDetails = detailsElements.find(el => el.querySelector('.activity-record-details'));
    if (recordDetails instanceof HTMLDetailsElement) {
      recordDetails.open = true;
    }

    expect(screen.getByText('Total duration')).toBeInTheDocument();
    expect(screen.getByText('Generation duration')).toBeInTheDocument();
  });

  it('hides model duration fields when absent', () => {
    const modelRecord: PublicModelActivityRecord = {
      model_role: 'primary',
      variant_id: 'test-variant',
      usage_availability: 'reported',
      input_tokens: { value: 100 },
      output_tokens: { value: 50 },
      finish_state: 'stop',
      load_state: 'warm',
    };

    render(<AssignmentActivitySummary activity={{
      model_activity: { availability: 'observed', records: [modelRecord] },
      tool_decisions: emptyFamily,
      tool_calls: emptyFamily,
      policy_decisions: emptyFamily,
      governed_actions: emptyFamily,
    }} />);

    const detailsElements = screen.getAllByRole('group');
    const recordDetails = detailsElements.find(el => el.querySelector('.activity-record-details'));
    if (recordDetails instanceof HTMLDetailsElement) {
      recordDetails.open = true;
    }

    expect(screen.queryByText('Total duration')).not.toBeInTheDocument();
    expect(screen.queryByText('Generation duration')).not.toBeInTheDocument();
  });

  it('renders expandable tool decision records', async () => {
    const user = userEvent.setup();
    const toolRecord: PublicToolDecisionActivityRecord = {
      tool_label: 'test_tool',
      recognized: true,
      selected: true,
      unnecessary: false,
      permission_compliant: true,
      outcome: 'pass',
      evidence_source: 'application_reported',
    };

    render(<AssignmentActivitySummary activity={{
      model_activity: emptyFamily,
      tool_decisions: { availability: 'observed', records: [toolRecord] },
      tool_calls: emptyFamily,
      policy_decisions: emptyFamily,
      governed_actions: emptyFamily,
    }} />);

    // Summary should be visible
    expect(screen.getByText(/test_tool.*Selected/)).toBeInTheDocument();

    // Expand and check for fields
    await user.click(screen.getByText(/test_tool.*Selected/));
    expect(screen.getByText('Tool label')).toBeInTheDocument();
    expect(screen.getByText('Recognized')).toBeInTheDocument();
    expect(screen.getByText('Selected')).toBeInTheDocument();
  });

  it('renders expandable tool call records', async () => {
    const user = userEvent.setup();
    const toolRecord: PublicToolCallActivityRecord = {
      tool_label: 'test_tool',
      execution_outcome: 'pass',
      semantic_outcome: 'pass',
      evidence_source: 'application_reported',
    };

    render(<AssignmentActivitySummary activity={{
      model_activity: emptyFamily,
      tool_decisions: emptyFamily,
      tool_calls: { availability: 'observed', records: [toolRecord] },
      policy_decisions: emptyFamily,
      governed_actions: emptyFamily,
    }} />);

    // Summary should be visible
    expect(screen.getByText(/test_tool.*Execution: Pass/)).toBeInTheDocument();

    // Expand and check for fields
    await user.click(screen.getByText(/test_tool.*Execution: Pass/));
    expect(screen.getByText('Tool label')).toBeInTheDocument();
    expect(screen.getByText('Execution outcome')).toBeInTheDocument();
    expect(screen.getByText('Semantic outcome')).toBeInTheDocument();
  });

  it('renders expandable policy decision records', async () => {
    const user = userEvent.setup();
    const policyRecord: PublicPolicyDecisionActivityRecord = {
      tool_label: 'test_tool',
      outcome: 'allow',
      evidence_source: 'application_reported',
    };

    render(<AssignmentActivitySummary activity={{
      model_activity: emptyFamily,
      tool_decisions: emptyFamily,
      tool_calls: emptyFamily,
      policy_decisions: { availability: 'observed', records: [policyRecord] },
      governed_actions: emptyFamily,
    }} />);

    // Summary should be visible
    expect(screen.getByText(/test_tool.*Outcome: Allow/)).toBeInTheDocument();

    // Expand and check for fields
    await user.click(screen.getByText(/test_tool.*Outcome: Allow/));
    expect(screen.getByText('Tool label')).toBeInTheDocument();
    expect(screen.getByText('Outcome')).toBeInTheDocument();
  });

  it('renders expandable governed action records', async () => {
    const user = userEvent.setup();
    const actionRecord: PublicGovernedActionActivityRecord = {
      action_label: 'governed action',
      reported_policy_outcome: 'allow',
      receipt_status: 'reported',
      evidence_source: 'application_reported',
    };

    render(<AssignmentActivitySummary activity={{
      model_activity: emptyFamily,
      tool_decisions: emptyFamily,
      tool_calls: emptyFamily,
      policy_decisions: emptyFamily,
      governed_actions: { availability: 'observed', records: [actionRecord] },
    }} />);

    // Summary should be visible
    expect(screen.getByText(/governed action.*Policy: Allow/)).toBeInTheDocument();

    // Expand and check for fields
    await user.click(screen.getByText(/governed action.*Policy: Allow/));
    expect(screen.getByText('Action label')).toBeInTheDocument();
    expect(screen.getByText('Reported policy outcome')).toBeInTheDocument();
    expect(screen.getByText('Receipt status')).toBeInTheDocument();
  });
});

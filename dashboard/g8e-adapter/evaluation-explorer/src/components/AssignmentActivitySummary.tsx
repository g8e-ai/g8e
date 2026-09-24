// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { ReactNode } from 'react';
import type {
  PublicActivityFamily,
  PublicAssignmentActivity,
  PublicGovernedActionActivityRecord,
  PublicModelActivityRecord,
  PublicPolicyDecisionActivityRecord,
  PublicToolCallActivityRecord,
  PublicToolDecisionActivityRecord,
} from '../contract/types';
import { formatNumber, formatTokens } from './shared';

function label(value: string): string {
  return value.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

function availabilityText<T>(family: PublicActivityFamily<T>): string {
  if (family.availability === 'observed') return `${family.records.length} observed`;
  if (family.availability === 'not_applicable') return 'Not applicable to this scenario';
  return family.unavailable_reason ? `Unavailable: ${label(family.unavailable_reason)}` : 'Unavailable';
}

function ModelRecord({ record }: { record: PublicModelActivityRecord }) {
  return <div className="activity-record"><strong>{record.variant_id}</strong><span>{label(record.model_role)}{record.agent_persona ? ` · ${record.agent_persona}` : ''}</span><span>{record.usage_availability === 'reported' ? `${record.input_tokens?.value !== undefined ? formatTokens(record.input_tokens.value) : 'Input unavailable'} in · ${record.output_tokens?.value !== undefined ? formatTokens(record.output_tokens.value) : 'output unavailable'}` : 'Usage unavailable'}</span><span>{label(record.finish_state)} · {label(record.load_state)} · {record.retry_count?.value !== undefined ? `${formatNumber(record.retry_count.value)} retries` : 'Retries unavailable'}</span></div>;
}

function ToolDecisionRecord({ record }: { record: PublicToolDecisionActivityRecord }) {
  return <div className="activity-record"><strong>{record.tool_label || 'Unlabeled tool'}</strong><span>{record.selected ? 'Selected' : 'Not selected'} · {record.recognized ? 'Recognized' : 'Not recognized'}</span><span>{record.permission_compliant ? 'Reported policy-compliant' : 'Reported policy concern'} · {label(record.outcome)}</span><small>Application-reported observation</small></div>;
}

function ToolCallRecord({ record }: { record: PublicToolCallActivityRecord }) {
  return <div className="activity-record"><strong>{record.tool_label || 'Unlabeled tool'}</strong><span>Execution: {label(record.execution_outcome)} · semantic: {label(record.semantic_outcome)}</span><small>Application-reported observation</small></div>;
}

function PolicyRecord({ record }: { record: PublicPolicyDecisionActivityRecord }) {
  return <div className="activity-record"><strong>{record.tool_label || 'Unlabeled tool'}</strong><span>Reported application outcome: {label(record.outcome)}</span><small>Not protocol authorization evidence</small></div>;
}

function GovernedActionRecord({ record }: { record: PublicGovernedActionActivityRecord }) {
  return <div className="activity-record"><strong>{record.action_label}</strong><span>Reported policy outcome: {label(record.reported_policy_outcome)} · receipt: {label(record.receipt_status)}</span><small>Application-reported observation; receipt status is not independently verified here</small></div>;
}

function ActivityFamily<T>({ title, family, renderRecord }: { title: string; family: PublicActivityFamily<T> | undefined; renderRecord: (record: T, index: number) => ReactNode }) {
  if (!family || family.availability !== 'observed' || family.records.length === 0) return null;
  return (
    <details className="activity-family" open={family.availability === 'observed' && family.records.length > 0}>
      <summary><span>{title}</span><span className="activity-availability">{availabilityText(family)}</span></summary>
      {family.records.length > 0 ? <ol>{family.records.map((record, index) => <li key={`${title}-${index}`}>{renderRecord(record, index)}</li>)}</ol> : null}
    </details>
  );
}

export function AssignmentActivitySummary({ activity }: { activity: PublicAssignmentActivity | undefined }) {
  return (
    <section className="assignment-activity">
      <h2>What happened</h2>
      {activity ? <div className="activity-families">
        <ActivityFamily title="Model activity" family={activity.model_activity} renderRecord={(record) => <ModelRecord record={record as PublicModelActivityRecord} />} />
        <ActivityFamily title="Tool decisions" family={activity.tool_decisions} renderRecord={(record) => <ToolDecisionRecord record={record as PublicToolDecisionActivityRecord} />} />
        <ActivityFamily title="Tool calls" family={activity.tool_calls} renderRecord={(record) => <ToolCallRecord record={record as PublicToolCallActivityRecord} />} />
        <ActivityFamily title="Policy decisions" family={activity.policy_decisions} renderRecord={(record) => <PolicyRecord record={record as PublicPolicyDecisionActivityRecord} />} />
        <ActivityFamily title="Governed actions" family={activity.governed_actions} renderRecord={(record) => <GovernedActionRecord record={record as PublicGovernedActionActivityRecord} />} />
      </div> : null}
    </section>
  );
}

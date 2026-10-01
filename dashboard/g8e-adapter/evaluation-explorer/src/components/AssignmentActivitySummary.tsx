// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { ReactNode } from 'react';
import { activityFamilyHasObservedRecords, activityFamilySummaryText } from '../contract/activity-family';
import type {
  PublicActivityFamily,
  PublicAssignmentActivity,
  PublicGovernedActionActivityRecord,
  PublicModelActivityRecord,
  PublicPolicyDecisionActivityRecord,
  PublicToolCallActivityRecord,
  PublicToolDecisionActivityRecord,
} from '../contract/types';
import { formatNumber, formatTokens, formatLatency, DetailRow, UnavailableValue } from './shared';

function label(value: string): string {
  return value.replace(/_/g, ' ').replace(/^./, (letter) => letter.toUpperCase());
}

function ModelRecord({ record }: { record: PublicModelActivityRecord }) {
  const summary = `${record.variant_id} · ${label(record.model_role)}${record.agent_persona ? ` · ${record.agent_persona}` : ''}`;
  return (
    <details className="activity-record-details">
      <summary className="activity-record-summary">{summary}</summary>
      <div className="activity-record-expanded">
        <DetailRow label="Variant">{record.variant_id}</DetailRow>
        <DetailRow label="Role">{label(record.model_role)}</DetailRow>
        {record.agent_persona ? <DetailRow label="Persona">{record.agent_persona}</DetailRow> : null}
        <DetailRow label="Usage availability">{label(record.usage_availability)}</DetailRow>
        <DetailRow label="Input tokens">
          {record.input_tokens?.value !== undefined ? formatTokens(record.input_tokens.value) : <UnavailableValue reason={record.input_tokens?.unavailable_reason} />}
        </DetailRow>
        <DetailRow label="Output tokens">
          {record.output_tokens?.value !== undefined ? formatTokens(record.output_tokens.value) : <UnavailableValue reason={record.output_tokens?.unavailable_reason} />}
        </DetailRow>
        {record.thinking_tokens ? (
          <DetailRow label="Thinking tokens">
            {record.thinking_tokens.value !== undefined ? formatTokens(record.thinking_tokens.value) : <UnavailableValue reason={record.thinking_tokens.unavailable_reason} />}
          </DetailRow>
        ) : null}
        {record.cache_tokens ? (
          <DetailRow label="Cache tokens">
            {record.cache_tokens.value !== undefined ? formatTokens(record.cache_tokens.value) : <UnavailableValue reason={record.cache_tokens.unavailable_reason} />}
          </DetailRow>
        ) : null}
        {record.total_duration_nanos ? (
          <DetailRow label="Total duration">
            {record.total_duration_nanos.value !== undefined ? formatLatency(record.total_duration_nanos.value / 1e6) : <UnavailableValue reason={record.total_duration_nanos.unavailable_reason} />}
          </DetailRow>
        ) : null}
        {record.generation_duration_nanos ? (
          <DetailRow label="Generation duration">
            {record.generation_duration_nanos.value !== undefined ? formatLatency(record.generation_duration_nanos.value / 1e6) : <UnavailableValue reason={record.generation_duration_nanos.unavailable_reason} />}
          </DetailRow>
        ) : null}
        <DetailRow label="Finish state">{label(record.finish_state)}</DetailRow>
        <DetailRow label="Load state">{label(record.load_state)}</DetailRow>
        <DetailRow label="Retry count">
          {record.retry_count?.value !== undefined ? formatNumber(record.retry_count.value) : <UnavailableValue reason={record.retry_count?.unavailable_reason} />}
        </DetailRow>
      </div>
    </details>
  );
}

function ToolDecisionRecord({ record }: { record: PublicToolDecisionActivityRecord }) {
  const summary = `${record.tool_label || 'Unlabeled tool'} · ${record.selected ? 'Selected' : 'Not selected'}`;
  return (
    <details className="activity-record-details">
      <summary className="activity-record-summary">{summary}</summary>
      <div className="activity-record-expanded">
        <DetailRow label="Tool label">{record.tool_label || 'Unlabeled tool'}</DetailRow>
        <DetailRow label="Recognized">{record.recognized ? 'Yes' : 'No'}</DetailRow>
        <DetailRow label="Selected">{record.selected ? 'Yes' : 'No'}</DetailRow>
        <DetailRow label="Unnecessary">{record.unnecessary ? 'Yes' : 'No'}</DetailRow>
        <DetailRow label="Permission compliant">{record.permission_compliant ? 'Yes' : 'No'}</DetailRow>
        <DetailRow label="Outcome">{label(record.outcome)}</DetailRow>
        <DetailRow label="Evidence source">{label(record.evidence_source)}</DetailRow>
        <small className="activity-record-note">Application-reported observation</small>
      </div>
    </details>
  );
}

function ToolCallRecord({ record }: { record: PublicToolCallActivityRecord }) {
  const turn = record.loop_turn ? `Turn ${record.loop_turn} · ` : '';
  const guidance = record.guidance_shown ? ' · guidance shown' : '';
  const summary = `${turn}${record.tool_label || 'Unlabeled tool'} · Execution: ${label(record.execution_outcome)}${guidance}`;
  return (
    <details className="activity-record-details">
      <summary className="activity-record-summary">{summary}</summary>
      <div className="activity-record-expanded">
        <DetailRow label="Tool label">{record.tool_label || 'Unlabeled tool'}</DetailRow>
        {record.loop_turn ? <DetailRow label="Loop turn">{record.loop_turn}</DetailRow> : null}
        {record.error_type ? <DetailRow label="Error type">{record.error_type}</DetailRow> : null}
        {record.guidance_shown ? <DetailRow label="Guidance">Error guidance was shown to the model</DetailRow> : null}
        <DetailRow label="Execution outcome">{label(record.execution_outcome)}</DetailRow>
        <DetailRow label="Semantic outcome">{label(record.semantic_outcome)}</DetailRow>
        <DetailRow label="Evidence source">{label(record.evidence_source)}</DetailRow>
        <small className="activity-record-note">Application-reported observation</small>
      </div>
    </details>
  );
}

function PolicyRecord({ record }: { record: PublicPolicyDecisionActivityRecord }) {
  const summary = `${record.tool_label || 'Unlabeled tool'} · Outcome: ${label(record.outcome)}`;
  return (
    <details className="activity-record-details">
      <summary className="activity-record-summary">{summary}</summary>
      <div className="activity-record-expanded">
        <DetailRow label="Tool label">{record.tool_label || 'Unlabeled tool'}</DetailRow>
        <DetailRow label="Outcome">{label(record.outcome)}</DetailRow>
        <DetailRow label="Evidence source">{label(record.evidence_source)}</DetailRow>
        <small className="activity-record-note">Not protocol authorization evidence</small>
      </div>
    </details>
  );
}

function GovernedActionRecord({ record }: { record: PublicGovernedActionActivityRecord }) {
  const summary = `${record.action_label} · Policy: ${label(record.reported_policy_outcome)}`;
  return (
    <details className="activity-record-details">
      <summary className="activity-record-summary">{summary}</summary>
      <div className="activity-record-expanded">
        <DetailRow label="Action label">{record.action_label}</DetailRow>
        <DetailRow label="Reported policy outcome">{label(record.reported_policy_outcome)}</DetailRow>
        <DetailRow label="Receipt status">{label(record.receipt_status)}</DetailRow>
        <DetailRow label="Evidence source">{label(record.evidence_source)}</DetailRow>
        <small className="activity-record-note">Application-reported observation; receipt status is not independently verified here</small>
      </div>
    </details>
  );
}

function ActivityFamily<T>({ title, family, renderRecord }: { title: string; family: PublicActivityFamily<T> | undefined; renderRecord: (record: T, index: number) => ReactNode }) {
  if (!activityFamilyHasObservedRecords(family)) return null;
  const records = family!.availability === 'observed' ? family!.records : [];
  return (
    <details className="activity-family" open>
      <summary><span>{title}</span><span className="activity-availability">{activityFamilySummaryText(family!)}</span></summary>
      <ol>{records.map((record, index) => <li key={`${title}-${index}`}>{renderRecord(record, index)}</li>)}</ol>
    </details>
  );
}

/** Orders tool calls by the loop turn that issued them; unreported turns keep their order. */
function inLoopTurnOrder(
  family: PublicActivityFamily<PublicToolCallActivityRecord>,
): PublicActivityFamily<PublicToolCallActivityRecord> {
  if (family.availability !== 'observed') return family;
  const records = family.records
    .map((record, index) => ({ record, index }))
    .sort((a, b) => (a.record.loop_turn ?? 0) - (b.record.loop_turn ?? 0) || a.index - b.index)
    .map(({ record }) => record);
  return { ...family, records };
}

export function AssignmentActivitySummary({ activity }: { activity: PublicAssignmentActivity | undefined }) {
  return (
    <section className="assignment-activity">
      <h2>What happened</h2>
      {activity ? <div className="activity-families">
        <ActivityFamily title="Model activity" family={activity.model_activity} renderRecord={(record) => <ModelRecord record={record as PublicModelActivityRecord} />} />
        <ActivityFamily title="Tool decisions" family={activity.tool_decisions} renderRecord={(record) => <ToolDecisionRecord record={record as PublicToolDecisionActivityRecord} />} />
        <ActivityFamily title="Tool calls" family={inLoopTurnOrder(activity.tool_calls)} renderRecord={(record) => <ToolCallRecord record={record as PublicToolCallActivityRecord} />} />
        <ActivityFamily title="Policy decisions" family={activity.policy_decisions} renderRecord={(record) => <PolicyRecord record={record as PublicPolicyDecisionActivityRecord} />} />
        <ActivityFamily title="Governed actions" family={activity.governed_actions} renderRecord={(record) => <GovernedActionRecord record={record as PublicGovernedActionActivityRecord} />} />
      </div> : null}
    </section>
  );
}

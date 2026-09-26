// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { Link } from 'react-router-dom';
import { MODEL_ROLES } from '../content/roles';
import { useDatasetOptions } from '../state/dataset';
import { useStoreState } from '../state/store';
import {
  capitalize,
  latestAssignmentActivity,
  resolveCampaignAssignmentProgress,
  resolveCurrentRun,
  resolveRoleSlots,
  type RoleSlotState,
} from '../views/campaign-activity';
import { roleLabel } from '../views/derived';
import { formatNumber } from './shared';

function roleSlotAriaLabel(slot: RoleSlotState): string {
  const model = slot.variantId ?? 'no model assigned';
  if (slot.status === 'active') {
    const progress = slot.progressTotal && slot.progressTotal > 0
      ? `${formatNumber(slot.progressCompleted ?? 0)} of ${formatNumber(slot.progressTotal)} assignments`
      : 'in progress';
    const task = slot.taskId ? ` on ${slot.taskId}` : '';
    return `${roleLabel(slot.role)} role active with ${model}${task}. ${progress}.`;
  }
  if (slot.status === 'completed') {
    return `${roleLabel(slot.role)} role completed with ${model}.`;
  }
  if (slot.status === 'failed') {
    return `${roleLabel(slot.role)} role failed with ${model}.`;
  }
  return `${roleLabel(slot.role)} role idle${slot.variantId ? ` with ${model}` : ''}.`;
}

function RoleSlot({ slot }: { slot: RoleSlotState }) {
  const roleName = MODEL_ROLES.find((role) => role.wire === slot.role)?.name ?? roleLabel(slot.role);
  const modelLabel = slot.variantId ?? '—';
  const progressPercent = slot.progressPercent ?? 0;

  return (
    <div
      className={`header-campaign-role header-campaign-role-${slot.status}`}
      aria-label={roleSlotAriaLabel(slot)}
    >
      <span className="header-campaign-role-name">{roleName}</span>
      <span className="header-campaign-role-model" title={modelLabel}>{modelLabel}</span>
      {slot.status === 'active' ? (
        <span className="header-campaign-role-status">
          <span className="status-dot" aria-hidden="true" />
          Active
        </span>
      ) : null}
      {slot.status === 'completed' ? (
        <span className="header-campaign-role-status header-campaign-role-status-completed">Done</span>
      ) : null}
      {slot.status === 'failed' ? (
        <span className="header-campaign-role-status header-campaign-role-status-failed">Failed</span>
      ) : null}
      {slot.status === 'active' && slot.progressTotal && slot.progressTotal > 0 ? (
        <div className="header-campaign-role-progress">
          <div
            className="header-campaign-role-progress-track"
            role="progressbar"
            aria-valuenow={slot.progressCompleted ?? 0}
            aria-valuemin={0}
            aria-valuemax={slot.progressTotal}
            aria-label={`${roleName} campaign progress`}
          >
            <div
              className="header-campaign-role-progress-fill"
              style={{ width: `${progressPercent}%` }}
            />
          </div>
          <span className="header-campaign-role-progress-label">{progressPercent}%</span>
        </div>
      ) : null}
    </div>
  );
}

export function ActiveCampaignBar() {
  const activeDatasetId = useDatasetOptions().find((option) => option.available && option.kind === 'live_run')?.id ?? '';
  const evaluations = useStoreState((state) =>
    Array.from(state.evaluations.values()).filter((evaluation) => evaluation.dataset_id === activeDatasetId),
  );
  const events = useStoreState((state) =>
    state.events.filter((event) => event.dataset_id === activeDatasetId),
  );

  const currentRun = resolveCurrentRun(evaluations, events);
  if (!currentRun) return null;

  const currentRunEvents = events.filter((event) => event.run_id === currentRun.run_id);
  const activity = latestAssignmentActivity(currentRunEvents);
  if (!activity) return null;

  const campaignProgress = resolveCampaignAssignmentProgress(evaluations);
  const roleSlots = resolveRoleSlots(currentRun, currentRunEvents, activity, campaignProgress);
  const isRunning = currentRun.lifecycle_state === 'running';
  const detailHref = `/evaluations/${currentRun.dataset_id}/${currentRun.run_id}`;
  const taskLabel = activity.taskId ?? activity.assignmentId;
  const activeSlot = roleSlots.find((slot) => slot.status === 'active');
  const ariaTask = activeSlot?.taskId ?? taskLabel;
  const ariaModel = activeSlot?.variantId ?? activity.variantId ?? 'model';

  return (
    <Link
      to={detailHref}
      className="header-campaign-bar"
      aria-label={
        isRunning
          ? `Now evaluating ${ariaModel} on ${ariaTask}. ${formatNumber(campaignProgress.done)} of ${formatNumber(campaignProgress.total)} assignments complete.`
          : `Latest: ${ariaModel} on ${ariaTask}. View campaign details.`
      }
    >
      <div className="header-campaign-roles" aria-label="Formation role activity">
        {roleSlots.map((slot) => (
          <RoleSlot key={slot.role} slot={slot} />
        ))}
      </div>
      <div className="header-campaign-tail" aria-label="Current task">
        <div className="header-campaign-task" title={taskLabel}>
          <span className="header-campaign-task-label">Task</span>
          <span className="header-campaign-task-id">{taskLabel}</span>
          {!isRunning ? (
            <span className="header-campaign-state">{capitalize(currentRun.lifecycle_state)}</span>
          ) : null}
        </div>
      </div>
    </Link>
  );
}

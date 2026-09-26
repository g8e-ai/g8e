// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { MODEL_ROLE_WIRE_ORDER } from '../content/roles';
import type { EvaluationSummary, LiveEvent, ModelRole } from '../contract/types';

export function capitalize(value: string): string {
  return value.length > 0 ? `${value.charAt(0).toUpperCase()}${value.slice(1)}` : value;
}

export type RoleSlotStatus = 'idle' | 'active' | 'completed' | 'failed';

export interface AssignmentActivity {
  assignmentId: string;
  taskId?: string;
  variantId?: string;
  role?: ModelRole;
  stageLabel?: string;
}

export interface RoleSlotState {
  role: ModelRole;
  variantId?: string;
  taskId?: string;
  stageLabel?: string;
  status: RoleSlotStatus;
  progressCompleted?: number;
  progressTotal?: number;
  progressPercent?: number;
}

export interface CampaignAssignmentProgress {
  done: number;
  total: number;
  percent: number;
}

function lastMatchingEvent(events: LiveEvent[], predicate: (event: LiveEvent) => boolean): LiveEvent | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event && predicate(event)) return event;
  }
  return undefined;
}

export function latestAssignmentActivity(events: LiveEvent[]): AssignmentActivity | undefined {
  const latest = lastMatchingEvent(events, (event) => event.assignment_id !== undefined);
  if (!latest?.assignment_id) return undefined;
  const matching = events.filter((event) => event.assignment_id === latest.assignment_id);
  return {
    assignmentId: latest.assignment_id,
    taskId: lastMatchingEvent(matching, (event) => event.task_id !== undefined)?.task_id,
    variantId: lastMatchingEvent(matching, (event) => event.variant_id !== undefined)?.variant_id,
    role: lastMatchingEvent(matching, (event) => event.role !== undefined)?.role,
    stageLabel: lastMatchingEvent(matching, (event) => event.stage_label !== undefined)?.stage_label,
  };
}

export function resolveCurrentRun(evaluations: EvaluationSummary[], events: LiveEvent[]): EvaluationSummary | undefined {
  const latestEvent = events[events.length - 1];
  const eventRun = latestEvent
    ? evaluations.find((evaluation) => evaluation.run_id === latestEvent.run_id)
    : undefined;
  const orderedEvaluations = [...evaluations].sort((a, b) => b.observed_at.localeCompare(a.observed_at));
  return eventRun
    ?? orderedEvaluations.find((evaluation) => evaluation.lifecycle_state === 'running' || evaluation.lifecycle_state === 'queued')
    ?? orderedEvaluations[0];
}

export function resolveCampaignAssignmentProgress(evaluations: EvaluationSummary[]): CampaignAssignmentProgress {
  const done = evaluations.reduce(
    (sum, evaluation) => sum + evaluation.assignment_completed + evaluation.assignment_failed,
    0,
  );
  const total = evaluations.reduce((sum, evaluation) => sum + evaluation.assignment_total, 0);
  const percent = total > 0 ? Math.min(100, Math.round((done / total) * 100)) : 0;
  return { done, total, percent };
}

function latestRoleEvents(events: LiveEvent[]): Map<ModelRole, LiveEvent> {
  const latestByRole = new Map<ModelRole, LiveEvent>();
  for (const event of events) {
    if (!event.role) continue;
    const existing = latestByRole.get(event.role);
    if (!existing || event.observed_at.localeCompare(existing.observed_at) >= 0) {
      latestByRole.set(event.role, event);
    }
  }
  return latestByRole;
}

function assignmentProgressCounts(
  assignmentEvents: LiveEvent[],
  campaignProgress: CampaignAssignmentProgress,
): { completed: number; total: number; percent: number } {
  const latest = assignmentEvents[assignmentEvents.length - 1];
  if (latest && latest.total > 0) {
    const completed = latest.completed;
    const total = latest.total;
    return {
      completed,
      total,
      percent: Math.min(100, Math.round((completed / total) * 100)),
    };
  }
  return {
    completed: campaignProgress.done,
    total: campaignProgress.total,
    percent: campaignProgress.percent,
  };
}

/** Per-role live campaign slots for the header bar and other condensed activity views. */
export function resolveRoleSlots(
  run: EvaluationSummary,
  events: LiveEvent[],
  activity: AssignmentActivity,
  campaignProgress: CampaignAssignmentProgress,
): RoleSlotState[] {
  const assignmentEvents = events.filter((event) => event.assignment_id === activity.assignmentId);
  const latestByRole = latestRoleEvents(assignmentEvents);
  const activeRoleEvent = lastMatchingEvent(assignmentEvents, (event) => event.role !== undefined);
  const activeRole = activeRoleEvent?.role ?? activity.role;
  const assignmentFailed = assignmentEvents.some((event) => event.kind === 'assignment_failed');
  const assignmentTerminal = assignmentEvents.some(
    (event) => event.kind === 'assignment_completed' || event.kind === 'assignment_failed',
  );
  const isRunning = run.lifecycle_state === 'running';
  const progress = assignmentProgressCounts(assignmentEvents, campaignProgress);
  const mapping = run.model_role_mapping ?? {};

  return MODEL_ROLE_WIRE_ORDER.map((role) => {
    const roleEvent = latestByRole.get(role);
    const variantId =
      mapping[role]
      ?? roleEvent?.variant_id
      ?? (role === activeRole ? activity.variantId : undefined)
      ?? (role === 'primary' && Object.keys(mapping).length === 0 ? activity.variantId : undefined);
    const isActive = isRunning && !assignmentTerminal && activeRole === role;
    let status: RoleSlotStatus = 'idle';
    if (assignmentTerminal) {
      if (role === activeRole || roleEvent) {
        status = assignmentFailed ? 'failed' : 'completed';
      }
    } else if (isActive) {
      status = 'active';
    }

    return {
      role,
      variantId,
      taskId: isActive ? activity.taskId ?? roleEvent?.task_id : undefined,
      stageLabel: isActive ? activity.stageLabel ?? roleEvent?.stage_label : undefined,
      status,
      progressCompleted: isActive ? progress.completed : undefined,
      progressTotal: isActive ? progress.total : undefined,
      progressPercent: isActive ? progress.percent : undefined,
    };
  });
}

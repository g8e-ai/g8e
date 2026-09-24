// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { EvaluationSummary, LiveEvent } from '../contract/types';

export function capitalize(value: string): string {
  return value.length > 0 ? `${value.charAt(0).toUpperCase()}${value.slice(1)}` : value;
}

function lastMatchingEvent(events: LiveEvent[], predicate: (event: LiveEvent) => boolean): LiveEvent | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event && predicate(event)) return event;
  }
  return undefined;
}

export function latestAssignmentActivity(events: LiveEvent[]) {
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

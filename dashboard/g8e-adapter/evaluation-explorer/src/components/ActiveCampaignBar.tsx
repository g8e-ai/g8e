// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { Link } from 'react-router-dom';
import { useDatasetOptions } from '../state/dataset';
import { useStoreState } from '../state/store';
import { capitalize, latestAssignmentActivity, resolveCurrentRun } from '../views/campaign-activity';
import { roleLabel } from '../views/derived';
import { formatNumber } from './shared';

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

  const assignmentDone = evaluations.reduce(
    (sum, evaluation) => sum + evaluation.assignment_completed + evaluation.assignment_failed,
    0,
  );
  const assignmentTotal = evaluations.reduce((sum, evaluation) => sum + evaluation.assignment_total, 0);
  const assignmentPercent = assignmentTotal > 0 ? Math.min(100, Math.round((assignmentDone / assignmentTotal) * 100)) : 0;
  const isRunning = currentRun.lifecycle_state === 'running';
  const detailHref = `/evaluations/${currentRun.dataset_id}/${currentRun.run_id}`;
  const primaryText = `${activity.variantId ?? 'Model unavailable'} · ${activity.taskId ?? activity.assignmentId}`;

  return (
    <Link
      to={detailHref}
      className="header-campaign-bar"
      aria-label={
        isRunning
          ? `Now evaluating ${activity.variantId ?? 'model'} on ${activity.taskId ?? activity.assignmentId}. ${formatNumber(assignmentDone)} of ${formatNumber(assignmentTotal)} assignments complete.`
          : `Latest activity: ${activity.variantId ?? 'model'} on ${activity.taskId ?? activity.assignmentId}. View campaign details.`
      }
    >
      <span className="header-campaign-label">{isRunning ? 'Now evaluating' : 'Latest activity'}</span>
      <span className="header-campaign-primary" title={primaryText}>{primaryText}</span>
      <span className="header-campaign-meta">
        {!isRunning ? <span>{capitalize(currentRun.lifecycle_state)}</span> : null}
        {activity.role ? <span>{roleLabel(activity.role)}</span> : null}
        {assignmentTotal > 0 ? <span>{assignmentPercent}%</span> : null}
      </span>
    </Link>
  );
}

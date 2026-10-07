// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { roleScopeFor } from '../content/roles';
import {
  UnavailableValue,
  formatLatency,
  formatDuration,
  formatNumber,
  formatPercent,
  formatThroughput,
} from './shared';
import { roleLabel, roleLeaderRows } from '../views/derived';
import type { CatalogSnapshot, EvaluationSummary, ModelRole, ModelSummary, QualityState } from '../contract/types';
import { qualityStateLabel, qualityStateTone } from '../utils/feed-state';

function agentStatus(state: QualityState): { label: string; tone: string } {
  return { label: qualityStateLabel(state), tone: qualityStateTone(state) };
}

function RoleLeaderCell({ role }: { role: ModelRole }) {
  return (
    <span className="role-leader-cell">
      <span className="role-leader-name">{roleLabel(role)}</span>
      <span className="role-leader-scope">{roleScopeFor(role)}</span>
    </span>
  );
}

function roleLeadersUseProvisionalColumns(
  roleRows: ReturnType<typeof roleLeaderRows>,
  catalogs: Map<string, CatalogSnapshot>,
): boolean {
  const evaluated = roleRows.filter((row) => row.leader);
  if (evaluated.length === 0) return false;
  return evaluated.every(
    (row) => catalogs.get(row.leader!.model.dataset_id)?.dataset_kind === 'live_run',
  );
}

/** Top fully evaluated, verified model per role across datasets. */
export function RoleLeadersPanel({
  models,
  catalogs,
  evaluations,
}: {
  models: ModelSummary[];
  catalogs: CatalogSnapshot[];
  evaluations: EvaluationSummary[];
}) {
  const catalogByDataset = useMemo(
    () => new Map(catalogs.map((catalog) => [catalog.dataset_id, catalog])),
    [catalogs],
  );
  const roleRows = useMemo(() => roleLeaderRows(models, evaluations), [models, evaluations]);
  const evaluatedRoles = roleRows.filter((row) => row.leader !== undefined);
  const provisional = roleLeadersUseProvisionalColumns(roleRows, catalogByDataset);

  return (
    <section className="panel role-leaders-panel" aria-label="Leaderboard">
      <div className="panel-head">
        <h2>
          Leaderboard{' '}
          <span className="panel-sub">
            · {evaluatedRoles.length} of {roleRows.length} roles qualified
          </span>
        </h2>
        <Link to="/models" className="panel-link">
          View all models →
        </Link>
      </div>
      {evaluatedRoles.length === 0 ? (
        <p className="panel-empty">No fully evaluated, verified models across datasets.</p>
      ) : (
        <div className="table-scroll">
          <table className="lab-table">
            <thead>
              <tr>
                <th>Model</th>
                <th>Role</th>
                <th>Status</th>
                <th>Size</th>
                <th>Eval time</th>
                {provisional ? (
                  <>
                    <th>Coverage</th>
                    <th>Scored</th>
                    <th>Pass rate</th>
                    <th>Failed</th>
                  </>
                ) : (
                  <>
                    <th>Quant</th>
                    <th>Tokens/s</th>
                    <th>Agreement</th>
                    <th>Pass rate</th>
                    <th>Latency p50</th>
                  </>
                )}
              </tr>
            </thead>
            <tbody>
              {roleRows.map(({ role, leader }) => {
                if (!leader) {
                  return (
                    <tr key={role} className="role-leader-empty">
                      <td colSpan={provisional ? 9 : 10}>
                        <RoleLeaderCell role={role} /> — no fully evaluated, verified leader yet
                      </td>
                    </tr>
                  );
                }
                const { model } = leader;
                const status = agentStatus(model.quality_state);
                const failedCount = leader.model_failed ?? leader.failed;
                return (
                  <tr key={role}>
                    <td>
                      <Link to={`/models/${model.dataset_id}/${model.variant_id}?role=${model.role}`}>
                        {model.display_name}
                      </Link>
                    </td>
                    <td>
                      <RoleLeaderCell role={role} />
                    </td>
                    <td>
                      <span className={`stream-role status-${status.tone}`}>
                        <span className="status-dot" aria-hidden="true" />
                        {status.label}
                      </span>
                    </td>
                    <td>{leader.parameter_billions !== undefined ? `${leader.parameter_billions}B` : <UnavailableValue />}</td>
                    <td>{leader.elapsed_seconds !== undefined ? formatDuration(leader.elapsed_seconds) : <UnavailableValue />}</td>
                    {provisional ? (
                      <>
                        <td>{formatPercent(leader.coverage, 0)}</td>
                        <td>{formatNumber(leader.terminal)}</td>
                        <td>
                          {leader.pass_rate !== undefined
                            ? formatPercent(leader.pass_rate, 0)
                            : '—'}
                        </td>
                        <td>{formatNumber(failedCount)}</td>
                      </>
                    ) : (
                      <>
                        <td>{model.quantization_weight_class?.toUpperCase() ?? <UnavailableValue />}</td>
                        <td>
                          {leader.throughput_p50 !== undefined
                            ? formatThroughput(leader.throughput_p50)
                            : <UnavailableValue />}
                        </td>
                        <td>
                          {model.agreement_pairwise?.value !== undefined
                            ? formatPercent(model.agreement_pairwise.value, 0)
                            : <UnavailableValue />}
                        </td>
                        <td>
                          {leader.pass_rate !== undefined
                            ? formatPercent(leader.pass_rate, 0)
                            : '—'}
                        </td>
                        <td>
                          {leader.latency_p50_ms !== undefined
                            ? formatLatency(leader.latency_p50_ms)
                            : <UnavailableValue />}
                        </td>
                      </>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

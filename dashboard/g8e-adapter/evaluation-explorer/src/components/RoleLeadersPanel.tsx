// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { roleScopeFor } from '../content/roles';
import {
  UnavailableValue,
  formatLatency,
  formatNumber,
  formatPercent,
  formatThroughput,
} from './shared';
import { roleLabel, roleLeaderRows } from '../views/derived';
import type { CatalogSnapshot, ModelRole, ModelSummary, QualityState } from '../contract/types';
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

/** Top measured model per role across every dataset in the feed. */
export function RoleLeadersPanel({
  models,
  catalogs,
}: {
  models: ModelSummary[];
  catalogs: CatalogSnapshot[];
}) {
  const catalogByDataset = useMemo(
    () => new Map(catalogs.map((catalog) => [catalog.dataset_id, catalog])),
    [catalogs],
  );
  const roleRows = useMemo(() => roleLeaderRows(models), [models]);
  const evaluatedRoles = roleRows.filter((row) => row.leader !== undefined);
  const provisional = roleLeadersUseProvisionalColumns(roleRows, catalogByDataset);

  return (
    <section className="panel role-leaders-panel" aria-label="Leaderboard">
      <div className="panel-head">
        <h2>
          Leaderboard{' '}
          <span className="panel-sub">
            · {evaluatedRoles.length} of {roleRows.length} roles evaluated
          </span>
        </h2>
        <Link to="/models" className="panel-link">
          View all models →
        </Link>
      </div>
      {evaluatedRoles.length === 0 ? (
        <p className="panel-empty">No evaluated models across datasets.</p>
      ) : (
        <div className="table-scroll">
          <table className="lab-table">
            <thead>
              <tr>
                <th>Model</th>
                <th>Role</th>
                <th>Status</th>
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
                      <td colSpan={provisional ? 7 : 8}>
                        <RoleLeaderCell role={role} /> — no measured leader yet
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

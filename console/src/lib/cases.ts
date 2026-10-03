// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { CaseSummary, Investigation } from './types';

function updatedAt(inv: Investigation): string {
  return inv.updated_at || inv.created_at;
}

/**
 * Groups investigations into cases. A case's title, status, and priority come
 * from its most recently updated investigation; investigations within a case
 * are ordered oldest first so they read as a numbered sequence.
 */
export function groupCases(investigations: Investigation[]): CaseSummary[] {
  const byCase = new Map<string, Investigation[]>();
  for (const inv of investigations) {
    if (!inv.case_id) continue;
    const list = byCase.get(inv.case_id) ?? [];
    list.push(inv);
    byCase.set(inv.case_id, list);
  }
  const cases: CaseSummary[] = [];
  for (const [id, invs] of byCase) {
    invs.sort((a, b) => a.created_at.localeCompare(b.created_at));
    const latest = invs.reduce((a, b) => (updatedAt(b) > updatedAt(a) ? b : a));
    cases.push({
      id,
      title: latest.case_title || 'Untitled case',
      status: latest.status,
      priority: latest.priority,
      updatedAt: updatedAt(latest),
      investigations: invs,
    });
  }
  return cases.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
}

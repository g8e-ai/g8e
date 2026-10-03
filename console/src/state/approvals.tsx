// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Owner decisions the Gateway is waiting on: L3 Notary approvals of suspended
// transactions and platform workload enrollment requests. Neither is announced
// over SSE, so the list is polled while the console is visible.

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, ApiError } from '../lib/api';
import { Paths } from '../lib/paths';
import type { PlatformEnrollmentRequest, SuspendedTransaction } from '../lib/types';

const POLL_MS = 15_000;

interface ApprovalsValue {
  transactions: SuspendedTransaction[];
  enrollments: PlatformEnrollmentRequest[];
  /** False when the user is not the owner and cannot see enrollment requests. */
  canReviewEnrollments: boolean;
  total: number;
  reload: () => Promise<void>;
}

const ApprovalsContext = createContext<ApprovalsValue | null>(null);

export function ApprovalsProvider({ children }: { children: ReactNode }) {
  const [transactions, setTransactions] = useState<SuspendedTransaction[]>([]);
  const [enrollments, setEnrollments] = useState<PlatformEnrollmentRequest[]>([]);
  const [canReviewEnrollments, setCanReview] = useState(true);

  const reload = useCallback(async () => {
    const [tx, pe] = await Promise.allSettled([
      api.get<{ transactions?: SuspendedTransaction[] }>(Paths.approvals),
      api.get<{ requests?: PlatformEnrollmentRequest[] }>(Paths.platformEnrollmentsPending),
    ]);
    if (tx.status === 'fulfilled') setTransactions(tx.value.transactions ?? []);
    if (pe.status === 'fulfilled') {
      setEnrollments(pe.value.requests ?? []);
      setCanReview(true);
    } else if (pe.reason instanceof ApiError && pe.reason.status === 403) {
      setEnrollments([]);
      setCanReview(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible') void reload();
    }, POLL_MS);
    return () => clearInterval(timer);
  }, [reload]);

  const value = useMemo(
    () => ({
      transactions,
      enrollments,
      canReviewEnrollments,
      total: transactions.length + enrollments.filter((e) => e.state === 'pending').length,
      reload,
    }),
    [transactions, enrollments, canReviewEnrollments, reload],
  );
  return <ApprovalsContext.Provider value={value}>{children}</ApprovalsContext.Provider>;
}

export function useApprovals(): ApprovalsValue {
  const v = useContext(ApprovalsContext);
  if (!v) throw new Error('useApprovals outside ApprovalsProvider');
  return v;
}

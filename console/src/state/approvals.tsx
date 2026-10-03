// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Owner decisions the Gateway is waiting on: L3 Notary approvals of suspended
// transactions and platform workload enrollment requests. The Gateway pushes
// g8e.v1.platform.approvals.changed whenever either pending set changes; the
// console re-lists on that event and never polls. The event is ephemeral (no
// replay), so the list is also re-fetched each time the stream (re)opens.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { api, ApiError } from '../lib/api';
import { Ev } from '../lib/events';
import { Paths } from '../lib/paths';
import type { PlatformEnrollmentRequest, SuspendedTransaction } from '../lib/types';
import { useStream, useStreamEvents } from './stream';

// Coalesces a burst of changes (a bulk approval, several enrollments arriving
// together) into one re-list.
const RELOAD_DEBOUNCE_MS = 250;

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

  const { state: streamState } = useStream();
  const reloadTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    void reload();
  }, [reload]);

  // Anything pushed while the stream was down is lost, so every (re)open is a
  // sync point.
  useEffect(() => {
    if (streamState === 'open') void reload();
  }, [streamState, reload]);

  useStreamEvents((ev) => {
    if (ev.type !== Ev.ApprovalsChanged) return;
    if (reloadTimer.current) clearTimeout(reloadTimer.current);
    reloadTimer.current = setTimeout(() => void reload(), RELOAD_DEBOUNCE_MS);
  });

  useEffect(
    () => () => {
      if (reloadTimer.current) clearTimeout(reloadTimer.current);
    },
    [],
  );

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

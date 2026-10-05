// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// The caller's per-role model selection, shared by the Inference view and the
// composer. The ensemble is the source of truth; a save replaces local state
// with the ensemble's response.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { api, isEnsembleUnavailable } from '../lib/api';
import { Ev } from '../lib/events';
import type { InferenceUpdateBody } from '../lib/inference';
import { Paths } from '../lib/paths';
import type { LlmSettings } from '../lib/types';
import { useApprovals } from './approvals';
import { useStream, useStreamEvents } from './stream';

export type EnsembleStatus = 'ready' | 'starting' | 'enrolling';

interface InferenceValue {
  settings: LlmSettings | null;
  loaded: boolean;
  error: string | null;
  ensembleStatus: EnsembleStatus;
  reload: () => Promise<void>;
  save: (body: InferenceUpdateBody) => Promise<LlmSettings>;
}

const InferenceContext = createContext<InferenceValue | null>(null);

export function InferenceProvider({ children }: { children: ReactNode }) {
  const { enrollments } = useApprovals();
  const { state: streamState } = useStream();
  const [settings, setSettings] = useState<LlmSettings | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [ensembleStatus, setEnsembleStatus] = useState<EnsembleStatus>('ready');

  const pendingEnrollment = useMemo(
    () =>
      enrollments.find(
        (e) =>
          (e.component_kind === 'ensemble' || e.component_name?.toLowerCase().includes('ensemble')) &&
          e.state === 'pending',
      ),
    [enrollments],
  );
  const pendingEnrollmentRef = useRef(pendingEnrollment);
  pendingEnrollmentRef.current = pendingEnrollment;

  const retryTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const retryCount = useRef(0);
  const reloadRef = useRef<() => Promise<void>>();

  const scheduleReload = useCallback((delay = 500) => {
    if (retryTimer.current) clearTimeout(retryTimer.current);
    retryTimer.current = setTimeout(() => void reloadRef.current?.(), delay);
  }, []);

  const reload = useCallback(async () => {
    try {
      const res = await api.post<LlmSettings>(Paths.llmSettingsGet, { context: {} });
      setSettings(res);
      setError(null);
      setEnsembleStatus('ready');
      retryCount.current = 0;
    } catch (err) {
      if (isEnsembleUnavailable(err)) {
        setEnsembleStatus(pendingEnrollmentRef.current ? 'enrolling' : 'starting');
        setError(null);
        if (!pendingEnrollmentRef.current && retryCount.current < 3) {
          retryCount.current += 1;
          scheduleReload(retryCount.current * 2000);
        }
      } else {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      setLoaded(true);
    }
  }, [scheduleReload]);

  reloadRef.current = reload;

  useEffect(() => {
    void reload();
    return () => {
      if (retryTimer.current) clearTimeout(retryTimer.current);
    };
  }, [reload]);

  useEffect(() => {
    if (pendingEnrollment && !settings) {
      setEnsembleStatus('enrolling');
    }
  }, [pendingEnrollment, settings]);

  // Sync on SSE stream connection or reconnection.
  useEffect(() => {
    if (streamState === 'open' && !settings) {
      retryCount.current = 0;
      void reload();
    }
  }, [streamState, settings, reload]);

  // Subscribe to SSE stream events.
  useStreamEvents((ev) => {
    if (ev.type === Ev.ApprovalsChanged) {
      retryCount.current = 0;
      scheduleReload(300);
    }
    if (ev.type.startsWith('g8e.v1.ai.') || ev.type.startsWith('g8e.v1.app.')) {
      if (!settings) {
        retryCount.current = 0;
        scheduleReload(100);
      }
    }
  });

  const save = useCallback(async (body: InferenceUpdateBody) => {
    const next = await api.post<LlmSettings>(Paths.llmSettings, { context: {}, ...body });
    setSettings(next);
    setError(null);
    setEnsembleStatus('ready');
    return next;
  }, []);

  const value = useMemo(
    () => ({ settings, loaded, error, ensembleStatus, reload, save }),
    [settings, loaded, error, ensembleStatus, reload, save],
  );
  return <InferenceContext.Provider value={value}>{children}</InferenceContext.Provider>;
}

export function useInference(): InferenceValue {
  const v = useContext(InferenceContext);
  if (!v) throw new Error('useInference outside InferenceProvider');
  return v;
}


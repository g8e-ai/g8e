// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// The caller's per-role model selection, shared by the Inference view and the
// composer. The ensemble is the source of truth; a save replaces local state
// with the ensemble's response.

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api } from '../lib/api';
import type { InferenceUpdateBody } from '../lib/inference';
import { Paths } from '../lib/paths';
import type { LlmSettings } from '../lib/types';

interface InferenceValue {
  settings: LlmSettings | null;
  loaded: boolean;
  error: string | null;
  reload: () => Promise<void>;
  save: (body: InferenceUpdateBody) => Promise<LlmSettings>;
}

const InferenceContext = createContext<InferenceValue | null>(null);

export function InferenceProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<LlmSettings | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    try {
      setSettings(await api.post<LlmSettings>(Paths.llmSettingsGet, { context: {} }));
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  const save = useCallback(async (body: InferenceUpdateBody) => {
    const next = await api.post<LlmSettings>(Paths.llmSettings, { context: {}, ...body });
    setSettings(next);
    setError(null);
    return next;
  }, []);

  const value = useMemo(() => ({ settings, loaded, error, reload, save }), [settings, loaded, error, reload, save]);
  return <InferenceContext.Provider value={value}>{children}</InferenceContext.Provider>;
}

export function useInference(): InferenceValue {
  const v = useContext(InferenceContext);
  if (!v) throw new Error('useInference outside InferenceProvider');
  return v;
}

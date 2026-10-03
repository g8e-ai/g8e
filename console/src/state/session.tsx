// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, ApiError, onUnauthorized } from '../lib/api';
import { Paths } from '../lib/paths';
import type { User } from '../lib/types';

export type SessionStatus = 'loading' | 'signed-out' | 'signed-in';

interface SessionValue {
  status: SessionStatus;
  user: User | null;
  webSessionId: string | null;
  /** Whether the Gateway already has an owner (false → first-owner enrollment). */
  bootstrapped: boolean;
  version: string | null;
  refresh: () => Promise<void>;
  signOut: () => Promise<void>;
}

const SessionContext = createContext<SessionValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<SessionStatus>('loading');
  const [user, setUser] = useState<User | null>(null);
  const [webSessionId, setWebSessionId] = useState<string | null>(null);
  const [bootstrapped, setBootstrapped] = useState(true);
  const [version, setVersion] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    const [boot, health] = await Promise.allSettled([
      api.get<{ bootstrapped: boolean }>(Paths.bootstrapStatus, { sessionBound: false }),
      api.get<{ version?: string }>(Paths.health, { sessionBound: false }),
    ]);
    if (boot.status === 'fulfilled') setBootstrapped(!!boot.value.bootstrapped);
    if (health.status === 'fulfilled') setVersion(health.value.version ?? null);
    try {
      const me = await api.get<{ user?: User }>(Paths.usersMe, { sessionBound: false });
      const sess = await api.get<{ web_session_id?: string }>(Paths.sessionsMe, { sessionBound: false });
      setUser(me.user ?? null);
      setWebSessionId(sess.web_session_id ?? null);
      setStatus(me.user ? 'signed-in' : 'signed-out');
    } catch (err) {
      if (!(err instanceof ApiError) || err.status !== 401) console.warn('[console] session check failed', err);
      setUser(null);
      setWebSessionId(null);
      setStatus('signed-out');
    }
  }, []);

  const signOut = useCallback(async () => {
    try {
      await api.post(Paths.logout, {}, { sessionBound: false });
    } finally {
      setUser(null);
      setWebSessionId(null);
      setStatus('signed-out');
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(
    () =>
      onUnauthorized(() => {
        setUser(null);
        setWebSessionId(null);
        setStatus('signed-out');
      }),
    [],
  );

  const value = useMemo(
    () => ({ status, user, webSessionId, bootstrapped, version, refresh, signOut }),
    [status, user, webSessionId, bootstrapped, version, refresh, signOut],
  );
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionValue {
  const v = useContext(SessionContext);
  if (!v) throw new Error('useSession outside SessionProvider');
  return v;
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// One Gateway SSE connection per signed-in session, fanned out to subscribers.

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { GatewayStream, type ConnectionState, type StreamEvent } from '../lib/sse';

type Handler = (ev: StreamEvent) => void;

interface StreamValue {
  state: ConnectionState;
  subscribe: (handler: Handler) => () => void;
}

const StreamContext = createContext<StreamValue | null>(null);

export function StreamProvider({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  const [state, setState] = useState<ConnectionState>('closed');
  const handlers = useRef(new Set<Handler>());

  useEffect(() => {
    if (!enabled) return;
    const stream = new GatewayStream({
      onEvent: (ev) => handlers.current.forEach((h) => h(ev)),
      onState: setState,
    });
    stream.start();
    const onVisible = () => {
      if (document.visibilityState === 'visible') stream.nudge();
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      document.removeEventListener('visibilitychange', onVisible);
      stream.stop();
    };
  }, [enabled]);

  const subscribe = useCallback((h: Handler) => {
    handlers.current.add(h);
    return () => {
      handlers.current.delete(h);
    };
  }, []);
  const value = useMemo(() => ({ state, subscribe }), [state, subscribe]);
  return <StreamContext.Provider value={value}>{children}</StreamContext.Provider>;
}

export function useStream(): StreamValue {
  const v = useContext(StreamContext);
  if (!v) throw new Error('useStream outside StreamProvider');
  return v;
}

/** Subscribes to every stream event for the component's lifetime. */
export function useStreamEvents(handler: Handler): void {
  const { subscribe } = useStream();
  const ref = useRef(handler);
  ref.current = handler;
  useEffect(() => subscribe((ev) => ref.current(ev)), [subscribe]);
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { createContext, useCallback, useContext, useState, type ReactNode } from 'react';

type Tone = 'info' | 'success' | 'error';
interface Toast {
  id: number;
  tone: Tone;
  text: string;
}

const ToastContext = createContext<((tone: Tone, text: string) => void) | null>(null);

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((tone: Tone, text: string) => {
    const id = nextId++;
    setToasts((t) => [...t, { id, tone, text }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), tone === 'error' ? 8000 : 4000);
  }, []);
  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={`toast toast-${t.tone}`}>
            {t.text}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): (tone: Tone, text: string) => void {
  const v = useContext(ToastContext);
  if (!v) throw new Error('useToast outside ToastProvider');
  return v;
}

export function errorText(err: unknown): string {
  if (err instanceof DOMException && err.name === 'NotAllowedError') return 'The passkey prompt was cancelled or timed out.';
  return err instanceof Error ? err.message : String(err);
}

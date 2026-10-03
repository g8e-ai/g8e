// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useState, type KeyboardEvent } from 'react';
import { ROLES, ROLE_INFO, isConfigured, roleModelLabel } from '../../lib/inference';
import { useInference } from '../../state/inference';
import { operatorLabel, useOperators } from '../../state/operators';

interface Props {
  busy: boolean;
  placeholder: string;
  onSend: (text: string) => Promise<boolean>;
  onStop: () => void;
  onManageOperators: () => void;
  onManageInference: () => void;
}

export function Composer({ busy, placeholder, onSend, onStop, onManageOperators, onManageInference }: Props) {
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const { bound } = useOperators();
  const { settings, loaded: inferenceLoaded } = useInference();

  const submit = async () => {
    const msg = text.trim();
    if (!msg || sending || busy) return;
    setSending(true);
    try {
      if (await onSend(msg)) setText('');
    } finally {
      setSending(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void submit();
    }
  };

  return (
    <div className="composer">
      <div className="composer-inner">
        <div className="composer-box">
          <textarea
            aria-label="Message"
            value={text}
            rows={2}
            placeholder={placeholder}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={onKeyDown}
          />
          <div className="composer-bar">
            {bound.length === 0 ? (
              <button type="button" className="chip chip-warn" onClick={onManageOperators}>
                No Operator bound — bind one to act on hosts
              </button>
            ) : (
              <button
                type="button"
                className="chip"
                onClick={onManageOperators}
                title={bound.map(operatorLabel).join(', ')}
              >
                <span className="dot dot-open" />
                {bound.length === 1 ? operatorLabel(bound[0]!) : `${bound.length} Operators bound`}
              </button>
            )}
            {inferenceLoaded &&
              (settings && isConfigured(settings) ? (
                <button
                  type="button"
                  className="chip"
                  onClick={onManageInference}
                  title={ROLES.map((r) => `${ROLE_INFO[r].label}: ${roleModelLabel(settings, r)}`).join('\n')}
                >
                  <span className="mono">{roleModelLabel(settings, 'primary')}</span>
                </button>
              ) : (
                <button type="button" className="chip chip-warn" onClick={onManageInference}>
                  No model selected — choose one
                </button>
              ))}
            <span className="spacer" />
            {busy ? (
              <button type="button" className="btn btn-sm" onClick={onStop}>
                Stop
              </button>
            ) : (
              <button
                type="button"
                className="btn btn-sm btn-primary"
                disabled={!text.trim() || sending}
                onClick={() => void submit()}
              >
                Send
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

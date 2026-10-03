// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { useEffect, useRef } from 'react';
import { Markdown } from '../../components/Markdown';
import { StatusPill } from '../../components/ui';
import type { ApprovalKind, TimelineItem, TimelineState } from '../../lib/timeline';

const APPROVAL_TITLE: Record<ApprovalKind, string> = {
  command: 'Run this command?',
  file_edit: 'Change this file?',
  intent: 'Grant this permission?',
  stream: 'Deploy Operators to these hosts?',
  agent_continue: 'Let the agent keep going?',
};

interface Props {
  timeline: TimelineState;
  onRespond: (approvalId: string, approved: boolean) => void;
}

export function Timeline({ timeline, onRespond }: Props) {
  const endRef = useRef<HTMLDivElement>(null);
  const last = timeline.items[timeline.items.length - 1];
  const lastSize = last && 'text' in last ? last.text.length : 0;

  useEffect(() => {
    endRef.current?.scrollIntoView?.({ block: 'end' });
  }, [timeline.items.length, lastSize, timeline.phase]);

  return (
    <div className="timeline">
      <div className="timeline-inner">
        {timeline.items.map((item) => (
          <Item key={item.key} item={item} onRespond={onRespond} />
        ))}
        {timeline.busy && timeline.phase && (
          <div className="phase">
            <span className="spinner" aria-hidden="true" />
            {timeline.phase}…
          </div>
        )}
        <div ref={endRef} />
      </div>
    </div>
  );
}

function Item({ item, onRespond }: { item: TimelineItem; onRespond: Props['onRespond'] }) {
  switch (item.kind) {
    case 'user':
      return <div className="msg-user">{item.text}</div>;
    case 'assistant':
      return (
        <div className="msg-ai">
          <span className="avatar" aria-hidden="true">
            g8e
          </span>
          <div className={`body ${item.streaming ? 'caret' : ''}`}>
            <Markdown text={item.text} />
          </div>
        </div>
      );
    case 'tool':
      return (
        <div className="tool">
          <div className="tool-head">
            <StatusPill status={item.status} />
            <span className="muted">{item.tool}</span>
            <code title={item.detail}>{item.detail}</code>
          </div>
          {item.error ? <pre className="tool-out error">{item.error}</pre> : item.output ? <pre className="tool-out">{item.output}</pre> : null}
        </div>
      );
    case 'approval': {
      const resolved = item.state === 'approved' || item.state === 'denied';
      return (
        <div className={`approval ${resolved ? 'resolved' : ''}`}>
          <div className="approval-title">
            {APPROVAL_TITLE[item.approvalKind]}
            {item.risk && <StatusPill status={item.risk} label={`${item.risk} risk`} />}
            {resolved && <StatusPill status={item.state} />}
          </div>
          {item.subject && <pre className="mono">{item.subject}</pre>}
          {item.justification && <p>{item.justification}</p>}
          {!resolved && (
            <div className="row">
              <button
                type="button"
                className="btn btn-primary btn-sm"
                disabled={item.state === 'submitting'}
                onClick={() => onRespond(item.approvalId, true)}
              >
                Approve
              </button>
              <button
                type="button"
                className="btn btn-danger btn-sm"
                disabled={item.state === 'submitting'}
                onClick={() => onRespond(item.approvalId, false)}
              >
                Deny
              </button>
            </div>
          )}
        </div>
      );
    }
    case 'notice':
      return <div className={`timeline-notice ${item.level === 'error' ? 'notice notice-error' : 'muted'}`}>{item.text}</div>;
  }
}

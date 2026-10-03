// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { Ev, Sender } from './events';
import type { StreamEvent } from './sse';
import { appendUserMessage, applyEvent, emptyTimeline, eventTargets, fromHistory, markIdle, setApprovalState } from './timeline';

let seq = 1;
const ev = (type: string, data: Record<string, unknown> = {}): StreamEvent => ({
  id: seq++,
  type,
  timestamp: '2026-01-01T00:00:00Z',
  data: { investigation_id: 'inv-1', ...data },
});

describe('fromHistory', () => {
  it('maps user, AI, command, and approval records', () => {
    const items = fromHistory([
      { sender: Sender.UserChat, content: 'disk full?', timestamp: 't1' },
      { sender: Sender.AiPrimary, content: 'Checking.', timestamp: 't2' },
      {
        sender: Sender.System,
        content: '',
        timestamp: 't3',
        metadata: { event_type: Ev.CommandApprovalRequested, approval_id: 'ap-1', command: 'df -h', justification: 'inspect' },
      },
      {
        sender: Sender.System,
        content: '',
        timestamp: 't4',
        metadata: { event_type: Ev.CommandApprovalRequested, approval_id: 'ap-1', approved: true },
      },
      { sender: Sender.System, content: '/dev/sda1 98%', timestamp: 't5', metadata: { execution_id: 'ex-1', command: 'df -h', status: 'completed' } },
    ]);
    expect(items.map((i) => i.kind)).toEqual(['user', 'assistant', 'approval', 'tool']);
    const approval = items[2]!;
    expect(approval.kind === 'approval' && approval.state).toBe('approved');
    expect(approval.kind === 'approval' && approval.subject).toBe('df -h');
  });
});

describe('applyEvent', () => {
  it('streams text chunks into one assistant message and closes it on completion', () => {
    let t = appendUserMessage(emptyTimeline, 'hi');
    t = applyEvent(t, ev(Ev.IterationStarted));
    t = applyEvent(t, ev(Ev.TextChunk, { content: 'Hel' }));
    t = applyEvent(t, ev(Ev.TextChunk, { content: 'lo' }));
    expect(t.items).toHaveLength(2);
    expect(t.items[1]).toMatchObject({ kind: 'assistant', text: 'Hello', streaming: true });
    t = applyEvent(t, ev(Ev.TextCompleted, { content: 'Hello.' }));
    expect(t.items[1]).toMatchObject({ kind: 'assistant', text: 'Hello.', streaming: false });
    // A text-only turn ends at TextCompleted; IterationCompleted never follows.
    expect(t.busy).toBe(false);
    expect(t.phase).toBeNull();
  });

  it('clears busy when a turn ends with no streamed chunks', () => {
    let t = applyEvent(appendUserMessage(emptyTimeline, 'hi'), ev(Ev.IterationStarted));
    t = applyEvent(t, ev(Ev.TextCompleted, { content: 'Hello.' }));
    expect(t.busy).toBe(false);
    expect(t.items[1]).toMatchObject({ kind: 'assistant', text: 'Hello.', streaming: false });
  });

  it('closes a stopped turn with a notice', () => {
    let t = applyEvent(appendUserMessage(emptyTimeline, 'hi'), ev(Ev.TextChunk, { content: 'par' }));
    t = applyEvent(t, ev(Ev.IterationStopped));
    expect(t.busy).toBe(false);
    expect(t.items[1]).toMatchObject({ kind: 'assistant', streaming: false });
    expect(t.items[2]).toMatchObject({ kind: 'notice', text: 'Stopped.' });
  });

  it('markIdle ends a stale turn without adding a notice', () => {
    const t = markIdle(appendUserMessage(emptyTimeline, 'hi'));
    expect(t.busy).toBe(false);
    expect(t.items).toHaveLength(1);
  });

  it('starts a new assistant message after the next user turn', () => {
    let t = applyEvent(appendUserMessage(emptyTimeline, 'a'), ev(Ev.TextChunk, { content: 'one' }));
    t = applyEvent(appendUserMessage(t, 'b'), ev(Ev.TextChunk, { content: 'two' }));
    expect(t.items.filter((i) => i.kind === 'assistant')).toHaveLength(2);
  });

  it('tracks a command through its lifecycle by execution ID', () => {
    let t = applyEvent(emptyTimeline, ev(Ev.CommandStarted, { execution_id: 'x', tool_name: 'run', display_detail: 'uptime' }));
    t = applyEvent(t, ev(Ev.CommandCompleted, { execution_id: 'x', content: 'up 3 days' }));
    expect(t.items).toEqual([
      expect.objectContaining({ kind: 'tool', status: 'completed', detail: 'uptime', output: 'up 3 days', tool: 'run' }),
    ]);
  });

  it('adds approval requests once and resolves them locally', () => {
    const req = ev(Ev.CommandApprovalRequested, { approval_id: 'ap', command: 'rm -rf /tmp/x', justification: 'cleanup', risk_analysis: { risk_level: 'high' } });
    let t = applyEvent(applyEvent(emptyTimeline, req), req);
    expect(t.items).toHaveLength(1);
    expect(t.items[0]).toMatchObject({ kind: 'approval', subject: 'rm -rf /tmp/x', risk: 'high', state: 'pending' });
    t = setApprovalState(t, 'ap', 'denied');
    expect(t.items[0]).toMatchObject({ state: 'denied' });
  });

  it('surfaces iteration failures and clears busy', () => {
    const t = applyEvent(appendUserMessage(emptyTimeline, 'x'), ev(Ev.IterationFailed, { error: 'model unavailable' }));
    expect(t.busy).toBe(false);
    expect(t.items[1]).toMatchObject({ kind: 'notice', level: 'error', text: 'model unavailable' });
  });

  it('ignores unknown event types', () => {
    const t = applyEvent(emptyTimeline, ev('g8e.v1.something.new'));
    expect(t).toBe(emptyTimeline);
  });
});

describe('eventTargets', () => {
  it('matches only the selected investigation', () => {
    expect(eventTargets(ev(Ev.TextChunk), 'inv-1')).toBe(true);
    expect(eventTargets(ev(Ev.TextChunk), 'inv-2')).toBe(false);
    expect(eventTargets(ev(Ev.TextChunk), null)).toBe(false);
  });
});

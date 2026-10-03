// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Cases → Investigations → chat. A case groups investigations; each
// investigation is one chat session with its own history and live events.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Empty, StatusPill, relativeTime } from '../../components/ui';
import { api } from '../../lib/api';
import { groupCases } from '../../lib/cases';
import { Ev } from '../../lib/events';
import { Paths } from '../../lib/paths';
import type { StreamEvent } from '../../lib/sse';
import {
  appendUserMessage,
  applyEvent,
  emptyTimeline,
  eventTargets,
  fromHistory,
  markIdle,
  setApprovalState,
  type TimelineState,
} from '../../lib/timeline';
import type { CaseSummary, ChatStartedResponse, ChatStopResponse, Investigation } from '../../lib/types';
import { useStreamEvents } from '../../state/stream';
import { errorText, useToast } from '../../state/toast';
import { Composer } from './Composer';
import { Timeline } from './Timeline';

/** Where the next message goes: an existing investigation, or a draft that creates one. */
export interface Selection {
  caseId: string | null;
  investigationId: string | null;
  draft: 'case' | 'investigation' | null;
}

const RECENT_LIMIT = 300;
const enc = encodeURIComponent;

export function readSelection(search: string): Selection {
  const p = new URLSearchParams(search);
  const caseId = p.get('case');
  return { caseId, investigationId: caseId ? p.get('investigation') : null, draft: caseId ? null : 'case' };
}

function writeSelection(sel: Selection): void {
  const p = new URLSearchParams(window.location.search);
  if (sel.caseId) p.set('case', sel.caseId);
  else p.delete('case');
  if (sel.investigationId) p.set('investigation', sel.investigationId);
  else p.delete('investigation');
  const qs = p.toString();
  history.replaceState(history.state, '', `${window.location.pathname}${qs ? `?${qs}` : ''}`);
}

// g8ee answers an investigation query with a bare array.
function asInvestigations(res: unknown): Investigation[] {
  if (Array.isArray(res)) return res as Investigation[];
  const list = (res as { investigations?: unknown } | null)?.investigations;
  return Array.isArray(list) ? (list as Investigation[]) : [];
}

export function CasesView({ onManageOperators, onManageInference }: { onManageOperators: () => void; onManageInference: () => void }) {
  const toast = useToast();
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [casesLoaded, setCasesLoaded] = useState(false);
  const [filter, setFilter] = useState('');
  const [sel, setSelState] = useState<Selection>(() => readSelection(window.location.search));
  const [caseInvestigations, setCaseInvestigations] = useState<Investigation[]>([]);
  const [timeline, setTimeline] = useState<TimelineState>(emptyTimeline);

  const selRef = useRef(sel);
  const recent = useRef<StreamEvent[]>([]);
  const keepTimelineFor = useRef<string | null>(null);
  const casesTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const setSel = useCallback((next: Selection) => {
    selRef.current = next;
    setSelState(next);
    writeSelection(next);
  }, []);

  const loadCases = useCallback(async () => {
    try {
      const res = await api.get<unknown>(`${Paths.investigations}?limit=100`);
      setCases(groupCases(asInvestigations(res)));
    } catch (err) {
      toast('error', `Could not load cases: ${errorText(err)}`);
    } finally {
      setCasesLoaded(true);
    }
  }, [toast]);

  const scheduleCasesReload = useCallback(() => {
    if (casesTimer.current) clearTimeout(casesTimer.current);
    casesTimer.current = setTimeout(() => void loadCases(), 500);
  }, [loadCases]);

  useEffect(() => {
    void loadCases();
  }, [loadCases]);

  // Load the selected case's investigations (with history) and open one.
  useEffect(() => {
    const caseId = sel.caseId;
    if (!caseId) {
      setCaseInvestigations([]);
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const res = await api.get<unknown>(
          `${Paths.investigations}?case_id=${enc(caseId)}&limit=100&order_by=created_at&order_direction=asc`,
        );
        if (cancelled) return;
        const invs = asInvestigations(res).sort((a, b) => a.created_at.localeCompare(b.created_at));
        setCaseInvestigations(invs);
        const current = selRef.current;
        if (current.draft === 'investigation') return;
        const target = invs.find((i) => i.id === current.investigationId) ?? invs[invs.length - 1];
        if (!target) return;
        if (keepTimelineFor.current === target.id) {
          keepTimelineFor.current = null;
        } else {
          setTimeline({ items: fromHistory(target.conversation_history), busy: false, phase: null });
        }
        if (current.investigationId !== target.id) setSel({ caseId, investigationId: target.id, draft: null });
      } catch (err) {
        if (!cancelled) toast('error', `Could not load case: ${errorText(err)}`);
      }
    })();
    return () => {
      cancelled = true;
    };
    // Keyed on the case only: investigation switches within a case go through
    // openInvestigation, and the latest selection is read from selRef.
  }, [sel.caseId, setSel, toast]);

  useStreamEvents((ev) => {
    if (typeof ev.data.investigation_id === 'string') {
      recent.current.push(ev);
      if (recent.current.length > RECENT_LIMIT) recent.current.shift();
    }
    if (ev.type === Ev.CaseCreated || ev.type === Ev.CaseUpdated) scheduleCasesReload();
    if (eventTargets(ev, selRef.current.investigationId)) setTimeline((t) => applyEvent(t, ev));
  });

  const openInvestigation = (inv: Investigation) => {
    setSel({ caseId: inv.case_id, investigationId: inv.id, draft: null });
    setTimeline({ items: fromHistory(inv.conversation_history), busy: false, phase: null });
  };

  const selectCase = (caseId: string) => {
    if (caseId === selRef.current.caseId && !selRef.current.draft) return;
    setTimeline(emptyTimeline);
    setSel({ caseId, investigationId: null, draft: null });
  };

  const startCase = () => {
    setTimeline(emptyTimeline);
    setSel({ caseId: null, investigationId: null, draft: 'case' });
  };

  const startInvestigation = () => {
    if (!sel.caseId) return;
    setTimeline(emptyTimeline);
    setSel({ caseId: sel.caseId, investigationId: null, draft: 'investigation' });
  };

  const send = async (text: string): Promise<boolean> => {
    const s = selRef.current;
    const before = timeline;
    setTimeline((t) => appendUserMessage(t, text));
    const body =
      s.draft === 'case'
        ? { message: text, context: {}, resource_creation: { create_case: true } }
        : s.draft === 'investigation'
          ? { message: text, context: { case_id: s.caseId }, resource_creation: { create_investigation: true } }
          : { message: text, context: { case_id: s.caseId, investigation_id: s.investigationId } };
    try {
      const res = await api.post<ChatStartedResponse>(Paths.chat, body);
      if (!res.success || !res.investigation_id) throw new Error('The ensemble did not accept the message.');
      if (s.draft) {
        keepTimelineFor.current = res.investigation_id;
        setSel({ caseId: res.case_id, investigationId: res.investigation_id, draft: null });
        // Events can race ahead of the HTTP response; fold any that already arrived.
        setTimeline((t) =>
          recent.current.filter((ev) => ev.data.investigation_id === res.investigation_id).reduce(applyEvent, t),
        );
        if (s.draft === 'investigation' && s.caseId === res.case_id) {
          setCaseInvestigations((list) =>
            list.some((i) => i.id === res.investigation_id)
              ? list
              : [...list, { id: res.investigation_id, case_id: res.case_id, case_title: '', user_id: '', status: 'open', created_at: new Date().toISOString() }],
          );
        }
        scheduleCasesReload();
      }
      return true;
    } catch (err) {
      setTimeline(before);
      toast('error', errorText(err));
      return false;
    }
  };

  const stop = async () => {
    const s = selRef.current;
    if (!s.investigationId) return;
    try {
      const res = await api.post<ChatStopResponse>(Paths.chatStop, {
        reason: 'User requested stop',
        context: { case_id: s.caseId, investigation_id: s.investigationId },
      });
      // A cancelled turn is closed by its stopped event. With nothing running,
      // none will come, so clear the stale busy state here.
      if (!res.was_active) setTimeline((t) => (selRef.current.investigationId === s.investigationId ? markIdle(t) : t));
    } catch (err) {
      toast('error', errorText(err));
    }
  };

  const respond = async (approvalId: string, approved: boolean) => {
    const s = selRef.current;
    setTimeline((t) => setApprovalState(t, approvalId, 'submitting'));
    try {
      await api.post(Paths.operatorApprovalRespond, {
        approval_id: approvalId,
        approved,
        ...(approved ? {} : { reason: 'Denied by user in the console' }),
        context: { case_id: s.caseId, investigation_id: s.investigationId },
      });
      setTimeline((t) => setApprovalState(t, approvalId, approved ? 'approved' : 'denied'));
    } catch (err) {
      setTimeline((t) => setApprovalState(t, approvalId, 'pending'));
      toast('error', errorText(err));
    }
  };

  const currentCase = useMemo(() => cases.find((c) => c.id === sel.caseId), [cases, sel.caseId]);
  const caseTitle = currentCase?.title || caseInvestigations[caseInvestigations.length - 1]?.case_title || 'Untitled case';
  const visibleCases = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return q ? cases.filter((c) => c.title.toLowerCase().includes(q) || c.id.toLowerCase().startsWith(q)) : cases;
  }, [cases, filter]);

  return (
    <div className="workspace">
      <aside className="case-list" aria-label="Cases">
        <div className="case-list-head">
          <div className="row">
            <h2>Cases</h2>
            <span className="spacer" />
            <button type="button" className="btn btn-sm btn-primary" onClick={startCase}>
              New case
            </button>
          </div>
          <input
            className="input"
            type="search"
            placeholder="Filter cases"
            aria-label="Filter cases"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
        </div>
        <div className="case-items">
          {casesLoaded && visibleCases.length === 0 && (
            <Empty title={cases.length ? 'No matches' : 'No cases yet'}>{cases.length ? null : 'Start one with New case.'}</Empty>
          )}
          {visibleCases.map((c) => (
            <button
              key={c.id}
              type="button"
              className="case-item"
              aria-current={c.id === sel.caseId ? 'true' : undefined}
              onClick={() => selectCase(c.id)}
            >
              <div className="case-item-title">{c.title}</div>
              <div className="case-item-meta">
                <span>{c.status}</span>
                <span>·</span>
                <span>
                  {c.investigations.length} investigation{c.investigations.length === 1 ? '' : 's'}
                </span>
                <span className="spacer" />
                <span>{relativeTime(c.updatedAt)}</span>
              </div>
            </button>
          ))}
        </div>
      </aside>

      <section className="case-pane">
        {sel.draft === 'case' ? (
          <div className="case-head" style={{ paddingBottom: 14 }}>
            <h1>New case</h1>
            <div className="muted">Describe the problem. The first message opens a case and its first investigation.</div>
          </div>
        ) : (
          <div className="case-head">
            <div className="row">
              <h1 title={caseTitle}>{caseTitle}</h1>
              {currentCase && <StatusPill status={currentCase.status} />}
              {currentCase?.priority && <span className="muted">{currentCase.priority} priority</span>}
            </div>
            <div className="tabs" role="tablist" aria-label="Investigations">
              {caseInvestigations.map((inv, i) => (
                <button
                  key={inv.id}
                  type="button"
                  role="tab"
                  className="tab"
                  aria-selected={!sel.draft && inv.id === sel.investigationId}
                  onClick={() => openInvestigation(inv)}
                  title={inv.id}
                >
                  Investigation {i + 1}
                  <span className="muted"> · {relativeTime(inv.updated_at || inv.created_at)}</span>
                </button>
              ))}
              <button
                type="button"
                role="tab"
                className="tab"
                aria-selected={sel.draft === 'investigation'}
                onClick={startInvestigation}
              >
                + New investigation
              </button>
            </div>
          </div>
        )}

        {timeline.items.length === 0 && !timeline.busy ? (
          <div className="timeline">
            <Empty title={sel.draft === 'investigation' ? 'New investigation' : sel.draft === 'case' ? 'What are we investigating?' : 'No messages yet'}>
              {sel.draft === 'investigation'
                ? 'Start a fresh line of inquiry within this case.'
                : 'Bound Operators let the ensemble inspect and act on your hosts, with every action governed and recorded.'}
            </Empty>
          </div>
        ) : (
          <Timeline timeline={timeline} onRespond={(id, ok) => void respond(id, ok)} />
        )}

        <Composer
          busy={timeline.busy}
          placeholder={sel.draft === 'case' ? 'Describe what you are investigating…' : 'Message the ensemble…'}
          onSend={send}
          onStop={() => void stop()}
          onManageOperators={onManageOperators}
          onManageInference={onManageInference}
        />
      </section>
    </div>
  );
}

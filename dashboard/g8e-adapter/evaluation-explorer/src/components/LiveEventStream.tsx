// Live SSE event feed for the overview page. Newest events at the top inside a
// grid-matched scroll region so progress ticks do not move the page.

import { useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { FeedConnectionState, StreamConnectionState } from '../utils/feed-state';
import { resolveModelSummary, useStoreState } from '../state/store';
import { roleLabel, visibleStreamEvents } from '../views/derived';
import { EmptyState, StreamStatusIndicator } from './shared';
import type { LiveEvent } from '../contract/types';

const STREAM_PAGE_SIZE = 25;

function eventTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleTimeString('en-US', { hour12: false });
}

export function LiveEventStream({
  events,
  connection,
  streamConnection,
}: {
  events: LiveEvent[];
  connection: FeedConnectionState;
  streamConnection: StreamConnectionState;
}) {
  const [modelFilter, setModelFilter] = useState('all');
  const [kindFilter, setKindFilter] = useState('all');
  const [page, setPage] = useState(0);
  const scrollRef = useRef<HTMLDivElement>(null);

  const models = useStoreState((state) => state.models);

  const modelOptions = useMemo(
    () =>
      Array.from(new Set(events.map((e) => e.variant_id).filter((v): v is string => Boolean(v)))).sort(),
    [events],
  );
  const kindOptions = useMemo(() => Array.from(new Set(events.map((e) => e.kind))).sort(), [events]);

  const visible = useMemo(
    () => visibleStreamEvents(events, { modelFilter, kindFilter }),
    [events, modelFilter, kindFilter],
  );

  const pageCount = Math.ceil(visible.length / STREAM_PAGE_SIZE);
  const safePage = Math.min(page, Math.max(0, pageCount - 1));
  const pageEvents = useMemo(() => {
    const start = safePage * STREAM_PAGE_SIZE;
    return visible.slice(start, start + STREAM_PAGE_SIZE);
  }, [visible, safePage]);

  useEffect(() => {
    setPage(0);
  }, [modelFilter, kindFilter]);

  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
  }, [safePage]);

  return (
    <section className="panel stream-panel" id="live-stream" aria-label="Live event stream">
      <div className="panel-head">
        <h2>
          Live event stream{' '}
          <StreamStatusIndicator
            streamConnection={streamConnection}
            feedConnection={connection}
            detail="long"
          />
        </h2>
        <div className="stream-controls">
          <select
            aria-label="Filter by model"
            value={modelFilter}
            onChange={(e) => setModelFilter(e.target.value)}
          >
            <option value="all">All models</option>
            {modelOptions.map((id) => (
              <option key={id} value={id}>
                {id}
              </option>
            ))}
          </select>
          <select
            aria-label="Filter by event kind"
            value={kindFilter}
            onChange={(e) => setKindFilter(e.target.value)}
          >
            <option value="all">All events</option>
            {kindOptions.map((kind) => (
              <option key={kind} value={kind}>
                {kind.replace(/_/g, ' ')}
              </option>
            ))}
          </select>
        </div>
      </div>
      {visible.length === 0 ? (
        <EmptyState
          hasRecords={events.length > 0}
          hasFilters={modelFilter !== 'all' || kindFilter !== 'all'}
          connection={connection}
        />
      ) : (
        <div className="stream-scroll" ref={scrollRef}>
          <table className="stream-table">
            <thead>
              <tr>
                <th>Time</th>
                <th>Role</th>
                <th>Event</th>
                <th>Model</th>
                <th>Progress</th>
              </tr>
            </thead>
            <tbody>
              {pageEvents.map((event) => {
                const model = event.variant_id
                  ? resolveModelSummary(models, event.dataset_id, event.variant_id)
                  : undefined;
                const servedTag = model?.served_model_tag ?? event.variant_id ?? event.kind.split('_')[0];
                const eventLabel = `${event.kind.replace(/_/g, ' ')}${event.stage_label ? ` · ${event.stage_label}` : ''}`;
                const eventHref = event.assignment_id
                  ? `/evaluations/${event.dataset_id}/${event.run_id}/assignments/${event.assignment_id}`
                  : `/evaluations/${event.dataset_id}/${event.run_id}`;
                const modelHref = event.variant_id
                  ? `/models/${event.dataset_id}/${event.variant_id}${model?.role ? `?role=${model.role}` : ''}`
                  : undefined;
                const roleLabelText = model ? roleLabel(model.role) : event.variant_id ?? 'Platform';
                return (
                  <tr key={event.event_id}>
                    <td className="stream-time">{eventTime(event.observed_at)}</td>
                    <td>
                      <span className={`stream-role status-${event.lifecycle_status}`}>
                        <span className="status-dot" aria-hidden="true" />
                        {roleLabelText}
                      </span>
                    </td>
                    <td className="stream-event">
                      <Link to={eventHref} className="stream-event-link">
                        {eventLabel}
                      </Link>
                    </td>
                    <td>
                      {modelHref ? (
                        <Link to={modelHref} className="stream-chip stream-chip-link">
                          {servedTag}
                        </Link>
                      ) : (
                        <code className="stream-chip">{servedTag}</code>
                      )}
                    </td>
                    <td className="stream-progress">
                      {event.total > 0 ? `${event.completed}/${event.total}` : '—'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {visible.length > 0 && pageCount > 1 ? (
        <div className="table-pagination" data-testid="stream-pagination">
          <button
            type="button"
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={safePage === 0}
            aria-label="Previous page"
          >
            Previous
          </button>
          <span className="page-info">
            Page {safePage + 1} of {pageCount} ({visible.length} events)
          </span>
          <button
            type="button"
            onClick={() => setPage((p) => Math.min(pageCount - 1, p + 1))}
            disabled={safePage >= pageCount - 1}
            aria-label="Next page"
          >
            Next
          </button>
        </div>
      ) : null}
    </section>
  );
}

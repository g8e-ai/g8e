// Live SSE event feed for the overview page. Newest events render at the top
// inside a fixed-height scroll viewport so progress ticks never move the page.

import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import type { FeedConnectionState } from '../utils/feed-state';
import { resolveModelSummary, useStoreState } from '../state/store';
import { roleLabel, visibleStreamEvents } from '../views/derived';
import { EmptyState } from './shared';
import type { LiveEvent, ModelSummary } from '../contract/types';

function eventTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleTimeString('en-US', { hour12: false });
}

function StreamEventRow({
  event,
  models,
}: {
  event: LiveEvent;
  models: Map<string, ModelSummary>;
}) {
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
    <li className="stream-row">
      <span className="stream-time">{eventTime(event.observed_at)}</span>
      <span className={`stream-role status-${event.lifecycle_status}`}>
        <span className="status-dot" aria-hidden="true" />
        {roleLabelText}
      </span>
      <span className="stream-event">
        <Link to={eventHref} className="stream-event-link">
          {eventLabel}
        </Link>
      </span>
      <span className="stream-model">
        {modelHref ? (
          <Link to={modelHref} className="stream-chip stream-chip-link">
            {servedTag}
          </Link>
        ) : (
          <code className="stream-chip">{servedTag}</code>
        )}
      </span>
      <span className="stream-progress">
        {event.total > 0 ? `${event.completed}/${event.total}` : '—'}
      </span>
    </li>
  );
}

export function LiveEventStream({
  events,
  connection,
}: {
  events: LiveEvent[];
  connection: FeedConnectionState;
}) {
  const [modelFilter, setModelFilter] = useState('all');
  const [kindFilter, setKindFilter] = useState('all');

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

  return (
    <section className="panel stream-panel" id="live-stream" aria-label="Live event stream">
      <div className="panel-head">
        <h2>
          Live event stream{' '}
          <span className={`stream-state ${connection === 'live' ? 'status-ok' : 'status-warn'}`}>
            <span className="status-dot" aria-hidden="true" />
            {connection === 'live' ? 'Streaming via SSE' : 'Mirror offline — last accepted data'}
          </span>
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

      <div className="stream-body">
        {visible.length === 0 ? (
          <EmptyState
            hasRecords={events.length > 0}
            hasFilters={modelFilter !== 'all' || kindFilter !== 'all'}
            connection={connection}
          />
        ) : (
          <>
            <div className="stream-columns" aria-hidden="true">
              <span>Time</span>
              <span>Role</span>
              <span>Event</span>
              <span>Model</span>
              <span>Progress</span>
            </div>
            <div className="stream-viewport">
              <ul className="stream-list">
                {visible.map((event) => (
                  <StreamEventRow key={event.event_id} event={event} models={models} />
                ))}
              </ul>
            </div>
          </>
        )}
      </div>
    </section>
  );
}

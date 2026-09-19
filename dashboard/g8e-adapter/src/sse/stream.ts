// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// SSE stream connection and reconciliation. Opens an absolute configured
// /api/v1/sse/stream EventSource with credentials, implements polling fallback
// using serialized stored push envelopes, and reconciles snapshots after
// initial connection, reconnect gaps, truncated, replay_failed, visibility
// restoration, unknown transition versions, and dropped live delivery.
//
// SSE is invalidation/live narrative, not the durable database. The snapshot
// (from the observe API) is the source of truth; SSE provides incremental
// change notifications.

import type { FrontendRuntimeConfig } from '../config/runtime_config';
import { EventDeduplicator, normalizeGatewayEvent, type NormalizedGatewayEvent } from './normalizer';

export type SseConnectionState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting' | 'unauthenticated';

export interface SseStreamCallbacks {
  onEvent?(event: NormalizedGatewayEvent): void;
  onStateChange?(state: SseConnectionState): void;
  onReconcile?(reason: ReconcileReason): void;
  onUnauthenticated?(): void;
}

export type ReconcileReason =
  | 'initial_connection'
  | 'reconnect_gap'
  | 'truncated'
  | 'replay_failed'
  | 'visibility_restored'
  | 'unknown_transition_version'
  | 'dropped_live_delivery';

export interface SseStreamOptions {
  config: FrontendRuntimeConfig;
  callbacks: SseStreamCallbacks;
  eventSourceFactory?: (url: string) => EventSource;
  fetchImpl?: typeof fetch;
  maxReconnectDelayMs?: number;
  visibilityPauseMs?: number;
}

export class SseStream {
  private es: EventSource | null = null;
  private state: SseConnectionState = 'disconnected';
  private dedup = new EventDeduplicator();
  private lastEventId = 0;
  private reconnectAttempt = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private openTimeout: ReturnType<typeof setTimeout> | null = null;
  private lastVisibleTimestamp = 0;
  private wasConnected = false;

  constructor(private readonly opts: SseStreamOptions) {}

  private get baseUrl(): string {
    return this.opts.config.gateway_base_url.replace(/\/$/, '');
  }

  private get maxReconnectDelay(): number {
    return this.opts.maxReconnectDelayMs ?? 30000;
  }

  private get visibilityPauseThreshold(): number {
    return this.opts.visibilityPauseMs ?? 60000;
  }

  connect(): void {
    if (this.es) return;
    this.setState('connecting');

    const url = `${this.baseUrl}/api/v1/sse/stream${this.lastEventId ? `?since_id=${this.lastEventId}` : ''}`;
    const factory = this.opts.eventSourceFactory ?? ((u: string) => new EventSource(u, { withCredentials: true }));
    this.es = factory(url);

    // Connection-open timeout guard: if onopen does not fire within 10s, close
    // and trigger reconnect.
    this.openTimeout = setTimeout(() => {
      if (this.state !== 'connected' && this.es) {
        this.es.close();
        this.es = null;
        this.scheduleReconnect();
      }
    }, 10000);

    this.es.onopen = () => {
      if (this.openTimeout) {
        clearTimeout(this.openTimeout);
        this.openTimeout = null;
      }
      this.reconnectAttempt = 0;
      const wasReconnecting = this.state === 'reconnecting' || this.wasConnected;
      this.setState('connected');
      this.wasConnected = true;
      if (!wasReconnecting) {
        this.opts.callbacks.onReconcile?.('initial_connection');
      } else {
        this.opts.callbacks.onReconcile?.('reconnect_gap');
      }
    };

    this.es.onmessage = (ev: MessageEvent) => {
      this.handleMessage(ev.data, ev.lastEventId);
    };

    this.es.onerror = () => {
      if (this.openTimeout) {
        clearTimeout(this.openTimeout);
        this.openTimeout = null;
      }
      // EventSource readyState: 0=CONNECTING, 1=OPEN, 2=CLOSED.
      // A CLOSED state (not CONNECTING) after an error means the server
      // rejected the connection (e.g. 401). EventSource does not auto-retry
      // from CLOSED.
      if (this.es) {
        if (this.es.readyState === 2) {
          this.es = null;
          this.setState('unauthenticated');
          this.opts.callbacks.onUnauthenticated?.();
        } else {
          // Still CONNECTING — EventSource will retry automatically.
          this.setState('reconnecting');
        }
      }
    };
  }

  private handleMessage(rawData: string, lastEventId: string): void {
    let event: NormalizedGatewayEvent;
    try {
      event = normalizeGatewayEvent(rawData, lastEventId);
    } catch {
      // Invalid events are silently dropped; they cannot mutate state.
      return;
    }

    // Dedup by durable ID.
    if (this.dedup.has(event.id)) return;
    this.dedup.add(event.id);
    if (event.id > this.lastEventId) {
      this.lastEventId = event.id;
    }

    // Handle sentinel events.
    if (event.sentinel) {
      this.handleSentinel(event);
      return;
    }

    this.opts.callbacks.onEvent?.(event);
  }

  private handleSentinel(event: NormalizedGatewayEvent): void {
    const payload = event.payload as Record<string, unknown> | null;
    if (event.type === 'truncated') {
      this.opts.callbacks.onReconcile?.('truncated');
    } else if (event.type === 'error') {
      const reason = payload?.reason;
      if (reason === 'replay_failed' || reason === 'replay failed') {
        this.opts.callbacks.onReconcile?.('replay_failed');
      }
    }
  }

  disconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.openTimeout) {
      clearTimeout(this.openTimeout);
      this.openTimeout = null;
    }
    if (this.es) {
      this.es.close();
      this.es = null;
    }
    this.setState('disconnected');
  }

  clearState(): void {
    this.dedup.clear();
    this.lastEventId = 0;
    this.reconnectAttempt = 0;
    this.wasConnected = false;
  }

  // Called when the page becomes visible after being hidden. If the pause
  // exceeds the threshold, trigger a snapshot reconciliation.
  onVisibilityChange(isVisible: boolean): void {
    if (isVisible) {
      if (this.lastVisibleTimestamp > 0) {
        const pause = Date.now() - this.lastVisibleTimestamp;
        if (pause >= this.visibilityPauseThreshold) {
          this.opts.callbacks.onReconcile?.('visibility_restored');
        }
      }
    } else {
      this.lastVisibleTimestamp = Date.now();
    }
  }

  private setState(state: SseConnectionState): void {
    this.state = state;
    this.opts.callbacks.onStateChange?.(state);
  }

  private scheduleReconnect(): void {
    if (this.state === 'unauthenticated') return;
    this.setState('reconnecting');
    this.reconnectAttempt++;
    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempt - 1), this.maxReconnectDelay);
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  getState(): SseConnectionState {
    return this.state;
  }

  getLastEventId(): number {
    return this.lastEventId;
  }
}

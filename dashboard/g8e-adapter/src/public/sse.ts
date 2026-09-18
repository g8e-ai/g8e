import type { PublicRuntimeConfig } from './runtime_config';
import type { PublicRecord, PublicSnapshot } from './state';

export type PublicSseConnectionState = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

export interface PublicSseCallbacks {
  onRecord?(record: PublicRecord): void;
  onSnapshot?(snapshot: PublicSnapshot): void;
  onTruncated?(reason: string): void;
  onError?(reason: string): void;
  onStateChange?(state: PublicSseConnectionState): void;
}

export interface PublicSseOptions {
  readonly config: PublicRuntimeConfig;
  readonly source_id: string;
  readonly since_sequence?: number;
  readonly callbacks: PublicSseCallbacks;
  readonly eventSourceFactory?: (url: string, eventSourceInitDict?: EventSourceInit) => EventSource;
}

const RECORD_EVENTS = ['projection', 'event', 'proof_manifest', 'key_revocation'] as const;

export class PublicSseStream {
  private source: EventSource | null = null;
  private lastSequence: number;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectDelayMs = 1000;
  private readonly maxReconnectDelayMs = 30000;
  private stopped = false;

  constructor(private readonly options: PublicSseOptions) {
    this.lastSequence = options.since_sequence ?? 0;
  }

  connect(): void {
    if (this.stopped) return;
    this.clearReconnectTimer();
    this.closeSource();
    this.options.callbacks.onStateChange?.('connecting');
    const query = new URLSearchParams({ source: this.options.source_id });
    if (this.lastSequence > 0) query.set('since_id', String(this.lastSequence));
    const url = `${this.options.config.mirror_origin}/stream?${query.toString()}`;
    const factory = this.options.eventSourceFactory ?? ((origin, init) => new EventSource(origin, init));
    const source = factory(url, { withCredentials: false });
    this.source = source;
    source.onopen = () => {
      this.reconnectDelayMs = 1000;
      this.options.callbacks.onStateChange?.('connected');
    };
    source.onerror = () => {
      this.options.callbacks.onStateChange?.('reconnecting');
      this.closeSource();
      this.scheduleReconnect();
    };
    source.addEventListener('snapshot', (event) => this.handleSnapshot(event as MessageEvent<string>));
    source.addEventListener('truncated', (event) => this.handleSentinel(event as MessageEvent<string>, true));
    source.addEventListener('error', (event) => this.handleSentinel(event as MessageEvent<string>, false));
    for (const eventType of RECORD_EVENTS) {
      source.addEventListener(eventType, (event) => this.handleRecord(eventType, event as MessageEvent<string>));
    }
  }

  disconnect(): void {
    this.stopped = true;
    this.clearReconnectTimer();
    this.closeSource();
    this.options.callbacks.onStateChange?.('disconnected');
  }

  private closeSource(): void {
    this.source?.close();
    this.source = null;
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer === null) return;
    clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  private scheduleReconnect(): void {
    if (this.stopped || this.reconnectTimer !== null) return;
    const delayMs = this.reconnectDelayMs;
    this.reconnectDelayMs = Math.min(this.reconnectDelayMs * 2, this.maxReconnectDelayMs);
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delayMs);
  }

  getLastSequence(): number {
    return this.lastSequence;
  }

  private handleRecord(recordType: string, event: MessageEvent<string>): void {
    const sequence = Number(event.lastEventId);
    if (!Number.isSafeInteger(sequence) || sequence <= this.lastSequence) return;
    this.lastSequence = sequence;
    this.options.callbacks.onRecord?.({
      sequence,
      record_type: recordType,
      record_hash: '',
      record_bytes: event.data,
    });
  }

  private handleSnapshot(event: MessageEvent<string>): void {
    try {
      this.options.callbacks.onSnapshot?.(JSON.parse(event.data) as PublicSnapshot);
    } catch {
      this.options.callbacks.onError?.('invalid snapshot');
    }
  }

  private handleSentinel(event: MessageEvent<string>, truncated: boolean): void {
    let reason = truncated ? 'history truncated' : 'stream error';
    try {
      const payload = JSON.parse(event.data) as { reason?: unknown };
      if (typeof payload.reason === 'string') reason = payload.reason;
    } catch {
      reason = truncated ? 'history truncated' : 'stream error';
    }
    if (truncated) this.options.callbacks.onTruncated?.(reason);
    else this.options.callbacks.onError?.(reason);
  }
}

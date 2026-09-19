// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it, vi } from 'vitest';

import { createCredentialedFetch } from '../src/api/fetch';
import {
  createObserveClient,
  ObserveResponseValidationError,
  type PageOptions,
} from '../src/api/observe_client';
import { parseRuntimeConfig, RUNTIME_CONFIG_SCHEMA_VERSION } from '../src/config/runtime_config';

function makeConfig() {
  return parseRuntimeConfig({
    schema_version: RUNTIME_CONFIG_SCHEMA_VERSION,
    gateway_base_url: 'https://g8e.example.com',
    passkey_rp_id: 'g8e.example.com',
    passkey_rp_name: 'g8e',
    app_name: 'g8e',
  });
}

interface FetchCall {
  url: string;
  opts: Record<string, unknown>;
}

function makeFetchMock(
  status: number,
  body: unknown,
): typeof fetch & { calls: FetchCall[] } {
  const calls: FetchCall[] = [];
  const mock = vi.fn(async (url: string, opts: Record<string, unknown>) => {
    calls.push({ url, opts });
    return {
      ok: status >= 200 && status < 300,
      status,
      text: async () => (body === null ? '' : JSON.stringify(body)),
      headers: new Headers(),
    } as Response;
  }) as unknown as typeof fetch & { calls: FetchCall[] };
  mock.calls = calls;
  return mock;
}

function agentProjection(id: string) {
  return {
    schema_version: '1.0.0',
    agent_id: id,
    display_name: 'Sage',
    role: 'sage',
    status: 'running',
    freshness: 'observed',
    observed_at: '2026-01-01T00:00:00Z',
  };
}

function runSummary(id: string) {
  return {
    schema_version: '1.0.0',
    run_id: id,
    run_kind: 'investigation',
    display_name: 'Case',
    status: 'running',
    completed_tasks: 0,
    total_tasks: 0,
    has_receipts: false,
    evidence_count: 0,
    observed_at: '2026-01-01T00:00:00Z',
  };
}

function evalSummary(id: string) {
  return {
    schema_version: '1.0.0',
    run_id: id,
    suite_id: 'suite-1',
    suite_version: '1.0.0',
    arm_id: 'arm-1',
    status: 'completed',
    verification_status: 'verified',
    receipt_count: 1,
    metric_count: 1,
    observed_at: '2026-01-01T00:00:00Z',
  };
}

function downloadArtifact(id: string) {
  return {
    schema_version: '1.0.0',
    artifact_id: id,
    filename: 'report.json',
    media_type: 'application/json',
    byte_size: 100,
    sha256: 'abc123',
    privacy_classification: 'public_safe',
    download_url: '/api/v1/observe/downloads/' + id,
    generated_at: '2026-01-01T00:00:00Z',
  };
}

function bootstrapBody() {
  return {
    schema_version: '1.0.0',
    agents: [agentProjection('u:sage')],
    overview: {
      schema_version: '1.0.0',
      agents_running: 1,
      agents_running_freshness: 'observed',
      tasks_in_queue: 0,
      tasks_in_queue_freshness: 'observed',
      generated_at: '2026-01-01T00:00:00Z',
    },
    measurements: { schema_version: '1.0.0' },
    recent_runs: [runSummary('inv-1')],
    latest_evals: [evalSummary('eval-1')],
    downloads: [downloadArtifact('dl-1')],
    generated_at: '2026-01-01T00:00:00Z',
  };
}

function runDetailBody(id: string) {
  return {
    schema_version: '1.0.0',
    run_id: id,
    run_kind: 'investigation',
    display_name: 'Case',
    status: 'running',
    completed_tasks: 0,
    total_tasks: 0,
    tasks: [],
    evidence_safe_links: [],
    observed_at: '2026-01-01T00:00:00Z',
  };
}

function evalDetailBody(id: string) {
  return {
    schema_version: '1.0.0',
    run_id: id,
    suite_id: 'suite-1',
    suite_version: '1.0.0',
    arm_id: 'arm-1',
    status: 'completed',
    verification_status: 'verified',
    receipt_count: 1,
    assigned_tasks: 1,
    terminal_attempts: 1,
    metrics: [],
    observed_at: '2026-01-01T00:00:00Z',
  };
}

describe('createObserveClient', () => {
  it('getBootstrap returns a validated snapshot', async () => {
    const fetchMock = makeFetchMock(200, bootstrapBody());
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const snap = await client.getBootstrap();
    expect(snap.schema_version).toBe('1.0.0');
    expect(snap.agents.length).toBe(1);
    expect(snap.agents[0].agent_id).toBe('u:sage');
    expect(snap.overview.agents_running).toBe(1);
    expect(snap.recent_runs.length).toBe(1);
    expect(snap.latest_evals.length).toBe(1);
    expect(snap.downloads.length).toBe(1);
  });

  it('getBootstrap uses credentials on every request', async () => {
    const fetchMock = makeFetchMock(200, bootstrapBody());
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await client.getBootstrap();
    expect(fetchMock.calls[0].opts.credentials).toBe('include');
  });

  it('getRuns returns a validated page with cursor pagination', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [runSummary('inv-1'), runSummary('inv-2')],
      cursor: 'next-cursor',
      has_more: true,
      limit: 2,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const page = await client.getRuns({ cursor: 'prev', limit: 2 });
    expect(page.items.length).toBe(2);
    expect(page.cursor).toBe('next-cursor');
    expect(page.has_more).toBe(true);
    expect(page.limit).toBe(2);
    expect(fetchMock.calls[0].url).toBe('https://g8e.example.com/api/v1/observe/runs?cursor=prev&limit=2');
  });

  it('getRuns omits query params when no options provided', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [],
      has_more: false,
      limit: 100,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await client.getRuns();
    expect(fetchMock.calls[0].url).toBe('https://g8e.example.com/api/v1/observe/runs');
  });

  it('getRunDetail returns a validated run detail', async () => {
    const fetchMock = makeFetchMock(200, runDetailBody('inv-1'));
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const detail = await client.getRunDetail('inv-1');
    expect(detail.run_id).toBe('inv-1');
    expect(detail.run_kind).toBe('investigation');
    expect(detail.status).toBe('running');
    expect(fetchMock.calls[0].url).toBe('https://g8e.example.com/api/v1/observe/runs/inv-1');
  });

  it('getRunDetail URL-encodes the run id', async () => {
    const fetchMock = makeFetchMock(200, runDetailBody('inv 1'));
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await client.getRunDetail('inv 1');
    expect(fetchMock.calls[0].url).toBe('https://g8e.example.com/api/v1/observe/runs/inv%201');
  });

  it('getEvals returns a validated eval page', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [evalSummary('eval-1')],
      has_more: false,
      limit: 50,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const page = await client.getEvals({ limit: 50 });
    expect(page.items.length).toBe(1);
    expect(page.items[0].verification_status).toBe('verified');
    expect(fetchMock.calls[0].url).toBe('https://g8e.example.com/api/v1/observe/evals?limit=50');
  });

  it('getEvalDetail returns a validated eval detail', async () => {
    const fetchMock = makeFetchMock(200, evalDetailBody('eval-1'));
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const detail = await client.getEvalDetail('eval-1');
    expect(detail.run_id).toBe('eval-1');
    expect(detail.verification_status).toBe('verified');
    expect(detail.assigned_tasks).toBe(1);
  });

  it('getDownloads returns a validated download page', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [downloadArtifact('dl-1')],
      has_more: false,
      limit: 100,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const page = await client.getDownloads();
    expect(page.items.length).toBe(1);
    expect(page.items[0].privacy_classification).toBe('public_safe');
  });

  it('getDownloadDetail returns a validated download artifact', async () => {
    const fetchMock = makeFetchMock(200, downloadArtifact('dl-1'));
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const art = await client.getDownloadDetail('dl-1');
    expect(art.artifact_id).toBe('dl-1');
    expect(art.byte_size).toBe(100);
    expect(fetchMock.calls[0].url).toBe('https://g8e.example.com/api/v1/observe/downloads/dl-1');
  });

  it('throws ObserveResponseValidationError on HTTP error', async () => {
    const fetchMock = makeFetchMock(500, {});
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getBootstrap()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on empty response body', async () => {
    const fetchMock = makeFetchMock(200, null);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getBootstrap()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on missing schema_version in bootstrap', async () => {
    const body = bootstrapBody();
    const { schema_version, ...rest } = body;
    const fetchMock = makeFetchMock(200, rest);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getBootstrap()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on invalid agent status enum', async () => {
    const body = bootstrapBody();
    body.agents[0].status = 'not-a-status';
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getBootstrap()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on invalid run_kind enum in run summary', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [{ ...runSummary('inv-1'), run_kind: 'not-a-kind' }],
      has_more: false,
      limit: 100,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getRuns()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on invalid verification_status enum in eval summary', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [{ ...evalSummary('eval-1'), verification_status: 'bogus' }],
      has_more: false,
      limit: 100,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getEvals()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on invalid privacy_classification enum in download', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [{ ...downloadArtifact('dl-1'), privacy_classification: 'secret' }],
      has_more: false,
      limit: 100,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getDownloads()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on non-array items in a page', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: 'not-an-array',
      has_more: false,
      limit: 100,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getRuns()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on missing has_more boolean in a page', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [],
      limit: 100,
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getRuns()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('throws on non-number limit in a page', async () => {
    const fetchMock = makeFetchMock(200, {
      schema_version: '1.0.0',
      items: [],
      has_more: false,
      limit: '100',
    });
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getRuns()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('accepts optional fields when absent', async () => {
    const fetchMock = makeFetchMock(200, runDetailBody('inv-1'));
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const detail = await client.getRunDetail('inv-1');
    expect(detail.active_task_id).toBeUndefined();
    expect(detail.started_at).toBeUndefined();
    expect(detail.ended_at).toBeUndefined();
  });

  it('accepts optional fields when present', async () => {
    const body = runDetailBody('inv-1');
    body.active_task_id = 'task-1';
    body.started_at = '2026-01-01T00:00:00Z';
    body.ended_at = '2026-01-02T00:00:00Z';
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const detail = await client.getRunDetail('inv-1');
    expect(detail.active_task_id).toBe('task-1');
    expect(detail.started_at).toBe('2026-01-01T00:00:00Z');
    expect(detail.ended_at).toBe('2026-01-02T00:00:00Z');
  });

  it('validates nested run tasks in run detail', async () => {
    const body = runDetailBody('inv-1');
    body.tasks = [{
      schema_version: '1.0.0',
      task_id: 'task-1',
      status: 'running',
    }];
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const detail = await client.getRunDetail('inv-1');
    expect(detail.tasks.length).toBe(1);
    expect(detail.tasks[0].task_id).toBe('task-1');
  });

  it('throws on invalid task status enum in run detail', async () => {
    const body = runDetailBody('inv-1');
    body.tasks = [{
      schema_version: '1.0.0',
      task_id: 'task-1',
      status: 'bogus',
    }];
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getRunDetail('inv-1')).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('validates nested eval metrics in eval detail', async () => {
    const body = evalDetailBody('eval-1');
    body.metrics = [{
      schema_version: '1.0.0',
      metric_id: 'm1',
      metric_version: '1.0.0',
      unit: 'score',
      eligible: 10,
      denominator: 10,
      verification_status: 'verified',
    }];
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const detail = await client.getEvalDetail('eval-1');
    expect(detail.metrics.length).toBe(1);
    expect(detail.metrics[0].metric_id).toBe('m1');
  });

  it('validates active_run in bootstrap when present', async () => {
    const body = bootstrapBody();
    body.active_run = runSummary('inv-active');
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const snap = await client.getBootstrap();
    expect(snap.active_run).toBeDefined();
    expect(snap.active_run?.run_id).toBe('inv-active');
  });

  it('active_run is undefined when absent in bootstrap', async () => {
    const fetchMock = makeFetchMock(200, bootstrapBody());
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const snap = await client.getBootstrap();
    expect(snap.active_run).toBeUndefined();
  });

  it('validates success_rate in overview when present', async () => {
    const body = bootstrapBody();
    body.overview.success_rate = {
      metric_id: 'sr',
      metric_version: '1.0.0',
      value: 0.95,
      unit: 'ratio',
      denominator: 100,
      verification_status: 'verified',
    };
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const snap = await client.getBootstrap();
    expect(snap.overview.success_rate).toBeDefined();
    expect(snap.overview.success_rate?.value).toBe(0.95);
  });

  it('throws on invalid success_rate verification_status', async () => {
    const body = bootstrapBody();
    body.overview.success_rate = {
      metric_id: 'sr',
      metric_version: '1.0.0',
      value: 0.95,
      unit: 'ratio',
      denominator: 100,
      verification_status: 'bogus',
    };
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getBootstrap()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('validates measurements with optional observed measurements', async () => {
    const body = bootstrapBody();
    body.measurements.cpu = {
      schema_version: '1.0.0',
      metric_id: 'cpu',
      value: 50,
      unit: 'percent',
      source_component: 'host',
      observed_at: '2026-01-01T00:00:00Z',
      status: 'observed',
    };
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const snap = await client.getBootstrap();
    expect(snap.measurements.cpu).toBeDefined();
    expect(snap.measurements.cpu?.status).toBe('observed');
  });

  it('throws on invalid measurement status enum', async () => {
    const body = bootstrapBody();
    body.measurements.cpu = {
      schema_version: '1.0.0',
      metric_id: 'cpu',
      value: 50,
      unit: 'percent',
      source_component: 'host',
      observed_at: '2026-01-01T00:00:00Z',
      status: 'bogus',
    };
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    await expect(client.getBootstrap()).rejects.toBeInstanceOf(ObserveResponseValidationError);
  });

  it('validates agent throughput when present', async () => {
    const body = bootstrapBody();
    body.agents[0].throughput = {
      schema_version: '1.0.0',
      metric_id: 'tps',
      value: 10,
      unit: 'tokens/s',
      source_component: 'ensemble',
      observed_at: '2026-01-01T00:00:00Z',
      status: 'observed',
    };
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const snap = await client.getBootstrap();
    expect(snap.agents[0].throughput).toBeDefined();
    expect(snap.agents[0].throughput?.value).toBe(10);
  });

  it('validates evidence_safe_links in run detail', async () => {
    const body = runDetailBody('inv-1');
    body.evidence_safe_links = [{
      schema_version: '1.0.0',
      artifact_id: 'art-1',
      label: 'Evidence',
      media_type: 'application/pdf',
    }];
    const fetchMock = makeFetchMock(200, body);
    const client = createObserveClient(createCredentialedFetch(makeConfig(), fetchMock));
    const detail = await client.getRunDetail('inv-1');
    expect(detail.evidence_safe_links.length).toBe(1);
    expect(detail.evidence_safe_links[0].artifact_id).toBe('art-1');
  });
});

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Drives the real App against a fake Gateway: routes are matched by method and
// path, and every request is recorded for assertions.

import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from '../src/App';
import { Ev } from '../src/lib/events';
import { SessionProvider } from '../src/state/session';
import { ToastProvider } from '../src/state/toast';

type Handler = (body: unknown, url: URL) => [number, unknown];
interface Call {
  method: string;
  path: string;
  search: string;
  body: unknown;
  credentials?: RequestCredentials;
}

let routes: Record<string, Handler>;
let calls: Call[];

class FakeEventSource {
  static last: FakeEventSource | null = null;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((m: MessageEvent<string>) => void) | null = null;
  constructor(
    public url: string,
    public init?: EventSourceInit,
  ) {
    FakeEventSource.last = this;
  }
  close() {}
  push(id: number, type: string, data: Record<string, unknown>) {
    this.onmessage?.({ data: JSON.stringify({ user_id: 'u1', web_session_id: 'ws1', event: { type, data } }), lastEventId: String(id) } as MessageEvent<string>);
  }
}

function signedInRoutes(extra: Record<string, Handler> = {}): Record<string, Handler> {
  return {
    'GET /api/v1/auth/bootstrap/status': () => [200, { bootstrapped: true }],
    'GET /api/v1/health': () => [200, { version: 'v2.3.0' }],
    'GET /api/v1/users/me': () => [200, { success: true, user: { id: 'u1' } }],
    'GET /api/v1/auth/sessions/me': () => [200, { success: true, user_id: 'u1', web_session_id: 'ws1' }],
    'GET /api/v1/operators': () => [200, { success: true, operators: [] }],
    'GET /api/v1/approvals': () => [200, { transactions: [] }],
    'GET /api/v1/auth/platform-enrollments/pending': () => [200, { requests: [] }],
    'GET /api/v1/investigations': () => [200, []],
    'POST /api/v1/settings/llm/get': () => [200, llmSettings()],
    ...extra,
  };
}

const PROVIDERS = [
  { provider: 'ollama', label: 'Ollama', endpoint: 'optional', api_key: 'optional', default_endpoint: 'http://localhost:11434', lists_models: true },
  { provider: 'openai', label: 'OpenAI-compatible', endpoint: 'optional', api_key: 'required', default_endpoint: 'https://api.openai.com/v1', lists_models: true },
  { provider: 'g8e', label: 'g8e governed inference', endpoint: 'none', api_key: 'none', lists_models: true },
];
const UNSET = { provider: null, model: null, endpoint: null, api_key_set: false };

function llmSettings(primary: Record<string, unknown> = UNSET) {
  return { providers: PROVIDERS, primary, assistant: UNSET, lite: UNSET };
}

beforeEach(() => {
  calls = [];
  window.history.replaceState(null, '', '/console/');
  vi.stubGlobal('EventSource', FakeEventSource);
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: string, init: RequestInit = {}) => {
      const url = new URL(input, 'https://gw.test');
      const method = init.method ?? 'GET';
      const body = init.body ? JSON.parse(String(init.body)) : undefined;
      calls.push({ method, path: url.pathname, search: url.search, body, credentials: init.credentials });
      const handler = routes[`${method} ${url.pathname}`];
      const [status, payload] = handler ? handler(body, url) : [404, { error: 'not found' }];
      return new Response(JSON.stringify(payload), { status, headers: { 'Content-Type': 'application/json' } });
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function renderApp(intent = {}) {
  return render(
    <ToastProvider>
      <SessionProvider>
        <App intent={intent} />
      </SessionProvider>
    </ToastProvider>,
  );
}

describe('authentication', () => {
  it('offers first-owner enrollment when the Gateway has no owner', async () => {
    routes = {
      'GET /api/v1/auth/bootstrap/status': () => [200, { bootstrapped: false }],
      'GET /api/v1/health': () => [200, { version: 'v2.3.0' }],
      'GET /api/v1/users/me': () => [401, { error: 'unauthorized' }],
    };
    renderApp();
    expect(await screen.findByRole('heading', { name: 'Set up this Gateway' })).toBeInTheDocument();
  });

  it('asks returning users for their user ID and explains deep-link reasons', async () => {
    routes = {
      'GET /api/v1/auth/bootstrap/status': () => [200, { bootstrapped: true }],
      'GET /api/v1/health': () => [200, {}],
      'GET /api/v1/users/me': () => [401, { error: 'unauthorized' }],
    };
    renderApp({ approveTxHash: 'tx1' });
    expect(await screen.findByLabelText('User ID')).toBeInTheDocument();
    expect(screen.getByText('Sign in to approve a suspended transaction.')).toBeInTheDocument();
  });

  it('sends credentials on every Gateway request', async () => {
    routes = signedInRoutes();
    renderApp();
    await screen.findByRole('heading', { name: 'Cases' });
    await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/investigations')).toBe(true));
    expect(calls.every((c) => c.credentials === 'include')).toBe(true);
    expect(FakeEventSource.last?.init?.withCredentials).toBe(true);
    expect(FakeEventSource.last?.url).not.toMatch(/user_id|web_session_id/);
  });
});

describe('cases and investigations', () => {
  it('opens a new case from the first message and streams the reply', async () => {
    routes = signedInRoutes({
      'POST /api/v1/chat': () => [200, { success: true, case_id: 'c1', investigation_id: 'i1' }],
      'GET /api/v1/investigations': (_b, url) =>
        url.searchParams.get('case_id') === 'c1'
          ? [200, [{ id: 'i1', case_id: 'c1', case_title: 'Disk alert', user_id: 'u1', status: 'open', created_at: '2026-01-01T00:00:00Z', conversation_history: [] }]]
          : [200, []],
    });
    const user = userEvent.setup();
    renderApp();

    const box = await screen.findByLabelText('Message');
    await user.type(box, 'Why is disk full?{Enter}');

    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.path === '/api/v1/chat')).toBe(true));
    const chat = calls.find((c) => c.path === '/api/v1/chat')!;
    expect(chat.body).toEqual({ message: 'Why is disk full?', context: {}, resource_creation: { create_case: true } });
    expect(screen.getByText('Why is disk full?')).toBeInTheDocument();

    await waitFor(() => expect(window.location.search).toBe('?case=c1&investigation=i1'));
    act(() => {
      FakeEventSource.last!.push(10, 'g8e.v1.ai.llm.chat.iteration.text.chunk.received', { investigation_id: 'i1', content: 'Checking **mounts**' });
      FakeEventSource.last!.push(11, 'g8e.v1.ai.llm.chat.iteration.text.chunk.received', { investigation_id: 'other', content: 'not mine' });
    });
    expect(await screen.findByText('mounts')).toBeInTheDocument();
    expect(screen.queryByText('not mine')).toBeNull();
  });

  it('starts a new investigation inside the selected case', async () => {
    window.history.replaceState(null, '', '/console/?case=c1');
    routes = signedInRoutes({
      'GET /api/v1/investigations': () => [
        200,
        [{ id: 'i1', case_id: 'c1', case_title: 'Disk alert', user_id: 'u1', status: 'open', created_at: '2026-01-01T00:00:00Z', conversation_history: [] }],
      ],
      'POST /api/v1/chat': () => [200, { success: true, case_id: 'c1', investigation_id: 'i2' }],
    });
    const user = userEvent.setup();
    renderApp();

    const tabs = await screen.findByRole('tablist', { name: 'Investigations' });
    await within(tabs).findByRole('tab', { name: /Investigation 1/ });
    await user.click(within(tabs).getByRole('tab', { name: '+ New investigation' }));
    await user.type(screen.getByLabelText('Message'), 'Check the other host{Enter}');

    await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/chat')).toBe(true));
    expect(calls.find((c) => c.path === '/api/v1/chat')!.body).toEqual({
      message: 'Check the other host',
      context: { case_id: 'c1' },
      resource_creation: { create_investigation: true },
    });
    expect(await within(tabs).findByRole('tab', { name: /Investigation 2/ })).toHaveAttribute('aria-selected', 'true');
  });

  it('answers an ensemble approval request with the investigation context', async () => {
    window.history.replaceState(null, '', '/console/?case=c1&investigation=i1');
    routes = signedInRoutes({
      'GET /api/v1/investigations': () => [
        200,
        [{ id: 'i1', case_id: 'c1', case_title: 'Disk alert', user_id: 'u1', status: 'open', created_at: '2026-01-01T00:00:00Z', conversation_history: [] }],
      ],
      'POST /api/v1/operator/approval/respond': () => [200, { success: true }],
    });
    const user = userEvent.setup();
    renderApp();
    await screen.findByRole('tab', { name: /Investigation 1/ });

    act(() => {
      FakeEventSource.last!.push(20, 'g8e.v1.operator.command.approval.requested', {
        investigation_id: 'i1',
        approval_id: 'ap1',
        command: 'du -sh /var',
        justification: 'Find large directories',
      });
    });
    await user.click(await screen.findByRole('button', { name: 'Approve' }));

    await waitFor(() => expect(calls.some((c) => c.path === '/api/v1/operator/approval/respond')).toBe(true));
    expect(calls.find((c) => c.path === '/api/v1/operator/approval/respond')!.body).toEqual({
      approval_id: 'ap1',
      approved: true,
      context: { case_id: 'c1', investigation_id: 'i1' },
    });
  });
});

describe('operators', () => {
  it('binds an Operator to this session', async () => {
    let bound = false;
    routes = signedInRoutes({
      'GET /api/v1/operators': () => [
        200,
        {
          success: true,
          operators: [
            {
              id: 'op1',
              user_id: 'u1',
              status: 'active',
              operator_type: 'remote',
              operator_session_id: 'os1',
              current_hostname: 'web-01',
              bound_web_session_id: bound ? 'ws1' : undefined,
              created_at: '2026-01-01T00:00:00Z',
              updated_at: '2026-01-01T00:00:00Z',
            },
          ],
        },
      ],
      'POST /api/v1/operators/bind': () => {
        bound = true;
        return [200, { success: true, bound_count: 1, failed_count: 0 }];
      },
    });
    window.history.replaceState(null, '', '/console/?view=operators');
    const user = userEvent.setup();
    renderApp();

    await user.click(await screen.findByRole('button', { name: 'Bind' }));
    expect(calls.find((c) => c.path === '/api/v1/operators/bind')!.body).toEqual({ operator_ids: ['op1'] });
    expect(await screen.findByText('This session')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Unbind' })).toBeInTheDocument();
  });
});

describe('inference', () => {
  it('loads governed models from the registered inference provider and saves a selection', async () => {
    let saved: Record<string, unknown> | null = null;
    routes = signedInRoutes({
      'POST /api/v1/settings/llm/get': () => [200, llmSettings({ provider: 'g8e', model: null, endpoint: null, api_key_set: false })],
      'POST /api/v1/settings/llm/models': () => [200, { models: ['qwen3:4b', 'qwen3:1.7b'] }],
      'POST /api/v1/settings/llm': (body) => {
        saved = body as Record<string, unknown>;
        return [200, llmSettings({ provider: 'g8e', model: 'qwen3:4b', endpoint: null, api_key_set: false })];
      },
    });
    window.history.replaceState(null, '', '/console/?view=inference');
    const user = userEvent.setup();
    renderApp();

    const primary = await screen.findByRole('region', { name: 'Primary' });
    expect(await within(primary).findByRole('option', { name: 'qwen3:4b' })).toBeInTheDocument();
    expect(calls.find((c) => c.path === '/api/v1/settings/llm/models')?.body).toEqual({
      context: {}, role: 'primary', provider: 'g8e',
    });
    await user.selectOptions(within(primary).getByLabelText('Model'), 'qwen3:4b');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(saved).not.toBeNull());
    expect(saved!.primary).toEqual({ provider: 'g8e', model: 'qwen3:4b', endpoint: null });
  });

  it('selects a provider, endpoint, and listed model per role and saves them', async () => {
    let saved: Record<string, unknown> | null = null;
    routes = signedInRoutes({
      'POST /api/v1/settings/llm/models': () => [200, { models: ['gemma4:e4b', 'qwen3:4b'] }],
      'POST /api/v1/settings/llm': (body) => {
        saved = body as Record<string, unknown>;
        const primary = (body as { primary: Record<string, unknown> }).primary;
        return [200, llmSettings({ ...primary, endpoint: 'http://192.168.1.2:11434', api_key_set: false })];
      },
    });
    window.history.replaceState(null, '', '/console/?view=inference');
    const user = userEvent.setup();
    renderApp();

    const primary = await screen.findByRole('region', { name: 'Primary' });
    await user.selectOptions(within(primary).getByLabelText('Provider'), 'ollama');
    await user.type(within(primary).getByLabelText('Endpoint'), '192.168.1.2:11434');
    await user.click(within(primary).getByRole('button', { name: 'Refresh' }));
    await waitFor(() =>
      expect(calls.filter((c) => c.path === '/api/v1/settings/llm/models').at(-1)?.body).toEqual({
        context: {},
        role: 'primary',
        provider: 'ollama',
        endpoint: '192.168.1.2:11434',
      }),
    );
    await user.selectOptions(await within(primary).findByLabelText('Model'), 'gemma4:e4b');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(saved).not.toBeNull());
    expect(saved).toEqual({
      context: {},
      primary: { provider: 'ollama', model: 'gemma4:e4b', endpoint: '192.168.1.2:11434' },
      assistant: { provider: null, model: null, endpoint: null },
      lite: { provider: null, model: null, endpoint: null },
    });
    expect(await screen.findByText('Saved. The next message uses these models.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
  });

  it('never shows a stored key and only sends one that is typed', async () => {
    let saved: { primary: Record<string, unknown> } | null = null;
    routes = signedInRoutes({
      'POST /api/v1/settings/llm/get': () => [200, llmSettings({ provider: 'openai', model: 'gpt-x', endpoint: null, api_key_set: true })],
      'POST /api/v1/settings/llm/models': () => [200, { models: ['gpt-x', 'gpt-y'] }],
      'POST /api/v1/settings/llm': (body) => {
        saved = body as { primary: Record<string, unknown> };
        return [200, llmSettings({ provider: 'openai', model: 'gpt-y', endpoint: null, api_key_set: true })];
      },
    });
    window.history.replaceState(null, '', '/console/?view=inference');
    const user = userEvent.setup();
    renderApp();

    const primary = await screen.findByRole('region', { name: 'Primary' });
    const key = within(primary).getByLabelText('API key');
    expect(key).toHaveValue('');
    expect(key).toHaveAttribute('placeholder', 'Saved — leave blank to keep');
    await user.selectOptions(await within(primary).findByLabelText('Model'), 'gpt-y');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(saved).not.toBeNull());
    expect(saved!.primary).toEqual({ provider: 'openai', model: 'gpt-y', endpoint: null });
  });

  it('shows the next message model in the composer and links to the view', async () => {
    routes = signedInRoutes({
      'POST /api/v1/settings/llm/get': () => [200, llmSettings({ provider: 'g8e', model: 'gemma4:e4b', endpoint: null, api_key_set: false })],
    });
    const user = userEvent.setup();
    renderApp();

    await user.click(await screen.findByRole('button', { name: 'gemma4:e4b' }));
    expect(await screen.findByRole('heading', { name: 'Inference' })).toBeInTheDocument();
    const primary = screen.getByRole('region', { name: 'Primary' });
    expect(within(primary).getByLabelText('Model')).toHaveValue('gemma4:e4b');
    expect(within(primary).queryByLabelText('Endpoint')).toBeNull();
  });

  it('prompts for a model when none is configured and surfaces ensemble errors', async () => {
    routes = signedInRoutes({
      'POST /api/v1/chat': () => [400, { error: { code: 'CONFIG_ERROR', message: 'No LLM model configured.' } }],
    });
    const user = userEvent.setup();
    renderApp();

    expect(await screen.findByRole('button', { name: 'No model selected — choose one' })).toBeInTheDocument();
    await user.type(screen.getByLabelText('Message'), 'hello{Enter}');
    expect(await screen.findByText('No LLM model configured.')).toBeInTheDocument();
  });

  it('shows connecting state when ensemble upstream is unavailable and auto-recovers via SSE', async () => {
    let settingsAvailable = false;
    routes = signedInRoutes({
      'POST /api/v1/settings/llm/get': () =>
        settingsAvailable
          ? [200, llmSettings({ provider: 'g8e', model: 'qwen3:4b', endpoint: null, api_key_set: false })]
          : [502, { error: 'ensemble upstream unavailable' }],
      'POST /api/v1/settings/llm/models': () => [200, { models: ['qwen3:4b'] }],
    });
    window.history.replaceState(null, '', '/console/?view=inference');
    renderApp();

    expect(await screen.findByText('Connecting to ensemble…')).toBeInTheDocument();
    expect(screen.queryByText(/Could not load model settings/)).toBeNull();

    // Simulate ensemble becoming ready and an SSE approval event firing
    settingsAvailable = true;
    act(() => {
      FakeEventSource.last?.push(1, Ev.ApprovalsChanged, { subject: 'enrollments' });
    });

    const primary = await screen.findByRole('region', { name: 'Primary' });
    expect(primary).toBeInTheDocument();
    expect(within(primary).getByLabelText('Model')).toHaveValue('qwen3:4b');
  });

  it('shows enrolling state when pending platform enrollment exists and links to approvals', async () => {
    routes = signedInRoutes({
      'POST /api/v1/settings/llm/get': () => [502, { error: 'ensemble upstream unavailable' }],
      'GET /api/v1/auth/platform-enrollments/pending': () => [
        200,
        {
          requests: [
            {
              request_id: 'req-1',
              component_kind: 'ensemble',
              component_name: 'g8ee',
              state: 'pending',
              expires_at: '2026-10-04T00:00:00Z',
              created_at: '2026-10-03T00:00:00Z',
            },
          ],
        },
      ],
    });
    window.history.replaceState(null, '', '/console/?view=inference');
    const user = userEvent.setup();
    renderApp();

    expect(await screen.findByText('Ensemble enrolling')).toBeInTheDocument();
    const reviewBtn = screen.getByRole('button', { name: 'Review in Approvals' });
    expect(reviewBtn).toBeInTheDocument();
    await user.click(reviewBtn);

    expect(await screen.findByRole('heading', { name: 'Approvals' })).toBeInTheDocument();
  });
});

describe('API reference', () => {
  const SPEC = {
    swagger: '2.0',
    info: {},
    paths: {
      '/api/v1/operators': {
        get: {
          summary: 'List operators',
          description: 'Lists bound Operators.',
          tags: ['operators'],
          parameters: [{ name: 'status', in: 'query', type: 'string', description: 'Filter by status' }],
          responses: { '200': { description: 'OK', schema: { $ref: '#/definitions/models.Operator' } } },
        },
      },
      '/api/v1/health': { get: { summary: 'Health check', tags: ['health'], responses: { '200': { description: 'OK' } } } },
    },
    definitions: { 'models.Operator': { type: 'object', required: ['id'], properties: { id: { type: 'string', description: 'Operator ID' } } } },
  };

  it('loads the spec with the session cookie and renders operations grouped by tag', async () => {
    window.history.replaceState(null, '', '/console/?view=api');
    routes = signedInRoutes({ 'GET /swagger/doc.json': () => [200, SPEC] });
    const user = userEvent.setup();
    renderApp();

    expect(await screen.findByRole('heading', { name: 'API' })).toBeInTheDocument();
    expect(await screen.findByText('/api/v1/operators')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'health' })).toBeInTheDocument();
    expect(calls.find((c) => c.path === '/swagger/doc.json')?.credentials).toBe('include');

    await user.click(screen.getByRole('button', { name: /List operators/ }));
    expect(screen.getByText('Filter by status')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /models\.Operator/ }));
    expect(screen.getByText('Operator ID')).toBeInTheDocument();
  });

  it('filters by text and by tag', async () => {
    window.history.replaceState(null, '', '/console/?view=api');
    routes = signedInRoutes({ 'GET /swagger/doc.json': () => [200, SPEC] });
    const user = userEvent.setup();
    renderApp();

    await screen.findByText('/api/v1/operators');
    await user.type(screen.getByLabelText('Filter operations'), 'health');
    expect(screen.queryByText('/api/v1/operators')).toBeNull();
    expect(screen.getByText('/api/v1/health')).toBeInTheDocument();

    await user.clear(screen.getByLabelText('Filter operations'));
    await user.click(within(screen.getByRole('group', { name: 'Filter by tag' })).getByRole('button', { name: /operators/ }));
    expect(screen.getByText('/api/v1/operators')).toBeInTheDocument();
    expect(screen.queryByText('/api/v1/health')).toBeNull();

    await user.type(screen.getByLabelText('Filter operations'), 'nothing-matches');
    expect(screen.getByText('No operations match')).toBeInTheDocument();
  });

  it('reports a load failure instead of an empty reference', async () => {
    window.history.replaceState(null, '', '/console/?view=api');
    routes = signedInRoutes({ 'GET /swagger/doc.json': () => [403, { error: 'forbidden' }] });
    renderApp();

    expect(await screen.findByText('Could not load the API spec')).toBeInTheDocument();
    expect(screen.getByText('forbidden')).toBeInTheDocument();
  });
});

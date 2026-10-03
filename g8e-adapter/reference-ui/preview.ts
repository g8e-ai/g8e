// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Design-preview fixture loader. When features.design_preview is enabled in
// the runtime config, the reference frontend loads typed fixtures from the
// contract pack instead of connecting to a live Gateway. Fixtures never mix
// with connected data. The design-preview banner is shown whenever this mode
// is active.
//
// In production, design-preview is disabled unless runtime configuration
// deliberately enables it. The fixture data is labeled partial/unavailable
// for evals and unavailable for resource measurements.

import type { ObserveBootstrapSnapshot } from '../src/types/observe';
import type { NormalizedGatewayEvent } from '../src/sse/normalizer';
import type { AdapterState } from '../src/state/stores';
import { INITIAL_ADAPTER_STATE, adapterReducer } from '../src/state/stores';

export interface PreviewFixture {
  scenario: string;
  description: string;
  snapshot?: ObserveBootstrapSnapshot;
  events?: Array<{ id: number; type: string; payload: Record<string, unknown>; recognized: boolean }>;
  bootstrap_status?: boolean;
  auth?: { status: string };
  observe?: { available: boolean; reason?: string };
  transport?: { connection_state: string; last_event_id?: number; reconcile_reason?: string };
}

export interface PreviewState {
  state: AdapterState;
  connectionState: string;
  designPreview: boolean;
}

// Load a fixture into adapter state. This mirrors what main.ts does with live
// Gateway data: bootstrap_loaded populates projections, event actions populate
// narrative. The fixture data flows through the same typed reducers and
// presentation functions as live data.
export function loadFixtureIntoState(fixture: PreviewFixture): PreviewState {
  let state: AdapterState = INITIAL_ADAPTER_STATE;

  if (fixture.snapshot) {
    state = adapterReducer(state, { type: 'projections', action: { type: 'bootstrap_loaded', snapshot: fixture.snapshot } });
  }

  if (fixture.events) {
    for (const e of fixture.events) {
      const event: NormalizedGatewayEvent = {
        id: e.id,
        type: e.type,
        timestamp: '',
        recognized: e.recognized,
        sentinel: e.type === 'truncated' || e.type === 'error',
        payload: e.payload,
        raw: JSON.stringify(e.payload),
      };
      state = adapterReducer(state, { type: 'narrative', action: { type: 'event', event } });
    }
  }

  if (fixture.auth?.status === 'authenticated') {
    state = adapterReducer(state, { type: 'auth', action: { type: 'auth_success', userId: 'preview-user', userName: 'Preview User', webSessionId: 'preview-session' } });
  } else if (fixture.bootstrap_status === false) {
    state = adapterReducer(state, { type: 'auth', action: { type: 'bootstrap_status', bootstrapped: false } });
  }

  const connectionState = fixture.transport?.connection_state ?? 'disconnected';

  return { state, connectionState, designPreview: true };
}

// Determine whether design-preview mode is active from the runtime config.
export function isDesignPreview(features: Record<string, boolean> | undefined): boolean {
  return features?.design_preview === true;
}

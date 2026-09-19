// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { afterEach, describe, expect, it } from 'vitest';

import { evalStore } from '../src/state/store';
import { startFeed, stopFeed } from '../src/state/startup';

describe('production feed startup', () => {
  afterEach(() => stopFeed());

  it('keeps a cold start empty when the public mirror is unavailable', async () => {
    const fetchImpl = async () => {
      throw new TypeError('network unavailable');
    };

    await startFeed({ fetchImpl: fetchImpl as typeof fetch });

    const state = evalStore.getState();
    expect(state.connection).toBe('offline');
    expect(state.catalogs.size).toBe(0);
    expect(state.models.size).toBe(0);
    expect(state.feedStatus?.message).toContain('network unavailable');
    expect(state.feedStatus?.message).not.toContain('fixture');
  });
});

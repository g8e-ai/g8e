import { afterEach, describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

import { evalStore } from '../src/state/store';
import { startFeed, stopFeed } from '../src/state/startup';
import { campaignDatasetId } from '../src/state/campaign-adapter';

const publicDir = join(dirname(fileURLToPath(import.meta.url)), '../public');
const RUN_ID = 'north-star-smoke-1789575779';

async function mirrorReachable(fetchImpl: typeof fetch): Promise<boolean> {
  try {
    const runtime = JSON.parse(readFileSync(join(publicDir, 'runtime.json'), 'utf8')) as {
      mirror_origin: string;
    };
    const response = await fetchImpl(`${runtime.mirror_origin}/bootstrap?source=opendevops-local`);
    return response.ok;
  } catch {
    return false;
  }
}

describe('live mirror ingest', () => {
  afterEach(() => stopFeed());

  it('reconstructs the north-star live dataset from the public mirror when available', async () => {
    const fetchImpl = fetch.bind(globalThis);
    if (!(await mirrorReachable(fetchImpl))) {
      return;
    }

    await startFeed({ fetchImpl });

    const state = evalStore.getState();
    const datasetId = campaignDatasetId(RUN_ID);
    const run = state.evaluations.get(`${datasetId}:${RUN_ID}`);
    const assignments = Array.from(state.assignments.values()).filter(
      (assignment) => assignment.dataset_id === datasetId && assignment.run_id === RUN_ID,
    );

    expect(state.connection === 'live' || state.connection === 'offline').toBe(true);
    expect(state.observedSequence).toBeGreaterThan(0);
    expect(run).toBeDefined();
    expect(assignments.length).toBeGreaterThan(0);
    expect(
      assignments.some(
        (assignment) =>
          assignment.assignment_id === '0077a53ebfbbc7c0849d23045321e470efec06b7136a222cb8ce91ea79bd6692',
      ),
    ).toBe(true);
  });
});

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { describe, expect, it } from 'vitest';
import { decodeCampaignProjectionEnvelope } from '../src/contract/campaign-wire';
import { normalizeActivityFamily, parseWireActivityFamily } from '../src/contract/activity-family';
import { fixtureCampaignResultEnvelope } from '../src/fixtures/fixtures';

describe('activity-family wire parsing', () => {
  it('accepts unavailable families when protobuf omits records', () => {
    const family = parseWireActivityFamily(
      {
        availability: 'PUBLIC_ACTIVITY_AVAILABILITY_UNAVAILABLE',
        unavailable_reason: 'PUBLIC_UNAVAILABLE_REASON_SOURCE_NOT_CAPTURED',
      },
      'activity.tool_decisions',
      () => ({}),
    );
    expect(family.records).toEqual([]);
    expect(normalizeActivityFamily(family)).toEqual({
      availability: 'unavailable',
      unavailable_reason: 'source_not_captured',
    });
  });

  it('decodes campaign envelopes with omitted activity records', () => {
    expect(() => decodeCampaignProjectionEnvelope(fixtureCampaignResultEnvelope)).not.toThrow();
  });
});

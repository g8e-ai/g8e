// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { readFileSync } from 'fs';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';
import { describe, it, expect } from 'vitest';
import type { Operator } from './types';

describe('Operator interface contract', () => {
  it('matches the canonical protobuf OperatorDocument fixture', () => {
    // Load the golden fixture generated from the Go protobuf message
    const fixtureDir = dirname(fileURLToPath(import.meta.url));
    const fixturePath = join(fixtureDir, '../../../protocol/test-fixtures/operator-document.json');
    const fixtureJSON = readFileSync(fixturePath, 'utf-8');
    const fixture = JSON.parse(fixtureJSON);

    // Declare every key that must exist in Operator.
    // tsc will fail to compile if a key is missing from this record.
    const keys: Record<keyof Operator, true> = {
      id: true,
      user_id: true,
      name: true,
      status: true,
      operator_type: true,
      operator_roles: true,
      operator_session_id: true,
      bound_web_session_id: true,
      current_hostname: true,
      system_fingerprint: true,
      last_heartbeat_at: true,
      started_at: true,
      created_at: true,
      updated_at: true,
    };

    // Verify each Operator field exists in the fixture
    for (const key of Object.keys(keys)) {
      expect(fixture).toHaveProperty(key);
    }

    // Verify created_at is a valid ISO8601 timestamp
    expect(fixture.created_at).toBeDefined();
    expect(typeof fixture.created_at).toBe('string');
    expect(Date.parse(fixture.created_at)).not.toBeNaN();

    // Verify updated_at is also a valid ISO8601 timestamp
    expect(fixture.updated_at).toBeDefined();
    expect(typeof fixture.updated_at).toBe('string');
    expect(Date.parse(fixture.updated_at)).not.toBeNaN();
  });
});

// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import type { ReleaseProvenance } from '../contract/types';

// VERSION is the platform release owner. The bundle records its build release;
// it does not derive a release from the newest campaign or verifier version.
export const CURRENT_PLATFORM_RELEASE = __G8E_PLATFORM_RELEASE__;

export function matchesRelease(record: ReleaseProvenance, release: string): boolean {
  return release === 'all' || (record.release === release && (record.release_basis === 'recorded' || record.release_basis === 'asserted'));
}

export function releaseLabel(record: ReleaseProvenance): string {
  return record.release && (record.release_basis === 'recorded' || record.release_basis === 'asserted')
    ? `${record.release} (${record.release_basis})`
    : 'Unknown release';
}

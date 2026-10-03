// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

// Display cap for the live stream table; the store keeps the full active run
// history so progress can be restamped in observed_at order before limiting.
export const LIVE_EVENT_RETENTION_LIMIT = 100;
export const LIVE_EVENT_STORE_LIMIT = 500;

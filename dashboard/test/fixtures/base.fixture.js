// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export function now() {
    return new Date();
}

export function addSeconds(date, seconds) {
    return new Date(date.getTime() + seconds * 1000);
}

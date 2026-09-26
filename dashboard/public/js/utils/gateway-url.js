// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

export function gatewayUrl(path) {
    if (!window.G8E_GATEWAY_URL) {
        throw new Error('Gateway origin not configured: window.G8E_GATEWAY_URL is unset');
    }
    if (!path.startsWith('/')) {
        throw new Error(`Gateway path must be absolute: ${path}`);
    }
    return `${window.G8E_GATEWAY_URL}${path}`;
}

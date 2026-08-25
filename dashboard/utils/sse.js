// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { VSOBaseModel } from '../models/base.js';
import { SSE_FRAME_TERMINATOR } from '../constants/service_config.js';

/**
 * Write a single SSE frame to a raw response object.
 * Enforces the VSOBaseModel boundary — callers cannot pass plain objects.
 *
 * @param {import('http').ServerResponse} res
 * @param {VSOBaseModel} eventData
 */
export function writeSSEFrame(res, eventData) {
    if (!(eventData instanceof VSOBaseModel)) {
        throw new Error(`writeSSEFrame requires a VSOBaseModel instance, got ${typeof eventData}`);
    }
    const wire = eventData.forWire();
    res.write(`data: ${JSON.stringify(wire)}${SSE_FRAME_TERMINATOR}`);
    if (typeof res.flush === 'function') res.flush();
}

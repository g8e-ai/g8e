// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

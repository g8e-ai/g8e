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

/**
 * Operator Domain Models for Frontend
 *
 * Browser-side models mirroring the server-side operator_model.js.
 * These models extend FrontendBaseModel and are used for data received
 * from the wire (SSE events, API responses) in the browser.
 */

import { FrontendBaseModel, F } from './base.js';

// ---------------------------------------------------------------------------
// HeartbeatSnapshot
// ---------------------------------------------------------------------------

export class HeartbeatSnapshot extends FrontendBaseModel {
    static fields = {
        timestamp:       { type: F.date,   default: null },
        cpu_percent:     { type: F.number, default: null },
        memory_percent:  { type: F.number, default: null },
        disk_percent:    { type: F.number, default: null },
        network_latency: { type: F.any,    default: null },
        uptime:          { type: F.any,    default: null },
        uptime_seconds:  { type: F.any,    default: null },
    };

    static empty() {
        return HeartbeatSnapshot.parse({});
    }

}

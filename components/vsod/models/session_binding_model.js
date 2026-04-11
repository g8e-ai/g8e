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

import { VSOIdentifiableModel, F, now } from './base.js';

/**
 * BoundSessionsDocument
 *
 * Persisted record of the bidirectional binding between a web session and
 * one or more operator sessions.
 *
 * Document identity: id === web_session_id (natural key — one record per web session).
 * Stored in the bound_sessions collection in VSODB document store.
 *
 * The authoritative bind table is the VSODB KV store (fast lookup path):
 *   sessionBindOperators(operatorSessionId) → webSessionId  (STRING)
 *   sessionWebBind(webSessionId)            → {operatorSessionId, ...}  (SET)
 *
 * This document provides durability, audit history, and a queryable record
 * of all binding events on a web session.
 */
export class BoundSessionsDocument extends VSOIdentifiableModel {
    static fields = {
        web_session_id:       { type: F.string, required: true },
        user_id:              { type: F.string, required: true },
        operator_session_ids: { type: F.array,  default: () => [] },
        operator_ids:         { type: F.array,  default: () => [] },
        bound_at:             { type: F.date,   default: () => now() },
        last_updated_at:      { type: F.date,   default: () => now() },
        status:               { type: F.string, default: 'active' },
    };
}

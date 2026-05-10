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

import { createHash } from 'crypto';

/**
 * Build a stable, fixed-length log tag for a session id.
 *
 * Truncating a session id with `.substring(0, N)` leaks the constant prefix
 * (`operator_session_`, `web_session_`, ...) while hiding the distinguishing
 * suffix, which is both uninformative and actively misleading when debugging
 * channel-identity issues (the same literal prefix surfaces in `results:`
 * channel names). This helper returns a deterministic SHA-256 digest snippet
 * with no embedded prefix, suitable for cross-log correlation without
 * exposing sensitive tail material.
 *
 * @param {string|null|undefined} sessionId
 * @returns {string} 12-char hex digest, or `'<none>'` for falsy input.
 */
export function sessionIdTag(sessionId) {
    if (!sessionId) {
        return '<none>';
    }
    return createHash('sha256').update(String(sessionId)).digest('hex').slice(0, 12);
}

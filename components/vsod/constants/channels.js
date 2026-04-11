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

import { _CHANNELS, _PUBSUB } from './shared.js';

/**
 * Pub/Sub Constants
 * Canonical values loaded from shared/constants/channels.json and shared/constants/pubsub.json.
 * These files are the single source of truth shared across VSE, VSA, and VSOD.
 *
 * Auth channels:
 *   auth.publish:{api_key_hash}               - VSA → VSOD API key auth request
 *   auth.publish:session:{session_hash}       - VSA → VSOD session auth request
 *   auth.response:{api_key_hash}              - VSOD → VSA API key auth response
 *   auth.response:session:{hash}              - VSOD → VSA session auth response
 *
 * Operator channels:
 *   cmd:{operator_id}:{operator_session_id}       - VSE → Operator command dispatch
 *   results:{operator_id}:{operator_session_id}   - Operator → VSE result delivery
 *   heartbeat:{operator_id}:{operator_session_id} - Operator → VSE heartbeat
 */

const sep = _CHANNELS['pubsub']['separator'];
const prefixes = _CHANNELS['pubsub']['prefixes'];
const auth = _CHANNELS['pubsub']['auth'];

const wire = _PUBSUB.wire;

export const PubSubAction = Object.freeze({
    SUBSCRIBE:   wire.actions.subscribe,
    PSUBSCRIBE:  wire.actions.psubscribe,
    UNSUBSCRIBE: wire.actions.unsubscribe,
    PUBLISH:     wire.actions.publish,
});

export const PubSubMessageType = Object.freeze({
    MESSAGE:    wire.event_types.message,
    PMESSAGE:   wire.event_types.pmessage,
    SUBSCRIBED: wire.event_types.subscribed,
});

export const PubSubChannel = Object.freeze({
    AUTH_PUBLISH_PREFIX:         auth['publish.prefix'],
    AUTH_PUBLISH_SESSION_PREFIX: auth['publish.session.prefix'],
    AUTH_RESPONSE_PREFIX:        auth['response.prefix'],
    AUTH_RESPONSE_SESSION_PREFIX: auth['response.session.prefix'],
    AUTH_SESSION_PREFIX:         auth['session.prefix'],
    CMD_PREFIX:                  prefixes['cmd'] + sep,
    HEARTBEAT_PREFIX:            prefixes['heartbeat'] + sep,
    RESULTS_PREFIX:              prefixes['results'] + sep,
});

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

import { G8eBaseModel, F } from './base.js';
import { PubSubAction, PubSubMessageType } from '../constants/channels.js';

// ---------------------------------------------------------------------------
// PubSubSubscribeMessage  (subscribe / unsubscribe / psubscribe wire message)
// ---------------------------------------------------------------------------

export class PubSubSubscribeMessage extends G8eBaseModel {
    static fields = {
        action:  { type: F.string, required: true },
        channel: { type: F.string, required: true },
    };

    _validate() {
        const valid = [PubSubAction.SUBSCRIBE, PubSubAction.PSUBSCRIBE, PubSubAction.UNSUBSCRIBE];
        if (!valid.includes(this.action)) {
            throw new Error(`PubSubSubscribeMessage: invalid action "${this.action}"`);
        }
    }
}

// ---------------------------------------------------------------------------
// PubSubPublishMessage  (publish wire message — data is a plain object, serialized
// as json.RawMessage by the g8es broker; see components/g8eo/services/listen/listen_pubsub.go)
// ---------------------------------------------------------------------------

export class PubSubPublishMessage extends G8eBaseModel {
    static fields = {
        action:  { type: F.string, required: true },
        channel: { type: F.string, required: true },
        data:    { type: F.object, required: true },
    };

    _validate() {
        if (this.action !== PubSubAction.PUBLISH) {
            throw new Error(`PubSubPublishMessage: action must be "${PubSubAction.PUBLISH}", got "${this.action}"`);
        }
    }
}

// ---------------------------------------------------------------------------
// PubSubInboundMessage  (inbound message event from g8es WebSocket)
// ---------------------------------------------------------------------------

export class PubSubInboundMessage extends G8eBaseModel {
    static fields = {
        type:    { type: F.string, required: true },
        channel: { type: F.string, required: true },
        data:    { type: F.string, required: true },
    };

    _validate() {
        if (this.type !== PubSubMessageType.MESSAGE) {
            throw new Error(`PubSubInboundMessage: type must be "${PubSubMessageType.MESSAGE}", got "${this.type}"`);
        }
    }
}

// ---------------------------------------------------------------------------
// PubSubInboundPMessage  (inbound pmessage event from g8es WebSocket)
// ---------------------------------------------------------------------------

export class PubSubInboundPMessage extends G8eBaseModel {
    static fields = {
        type:    { type: F.string, required: true },
        pattern: { type: F.string, required: true },
        channel: { type: F.string, required: true },
        data:    { type: F.string, required: true },
    };

    _validate() {
        if (this.type !== PubSubMessageType.PMESSAGE) {
            throw new Error(`PubSubInboundPMessage: type must be "${PubSubMessageType.PMESSAGE}", got "${this.type}"`);
        }
    }
}

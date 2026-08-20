// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { VSOBaseModel, F } from './base.js';
import { PubSubAction, PubSubMessageType } from '../constants/channels.js';

// ---------------------------------------------------------------------------
// PubSubSubscribeMessage  (subscribe / unsubscribe / psubscribe wire message)
// ---------------------------------------------------------------------------

export class PubSubSubscribeMessage extends VSOBaseModel {
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
// as json.RawMessage by the g8eg broker; see components/vsa/services/listen/listen_pubsub.go)
// ---------------------------------------------------------------------------

export class PubSubPublishMessage extends VSOBaseModel {
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
// PubSubInboundMessage  (inbound message event from g8eg WebSocket)
// ---------------------------------------------------------------------------

export class PubSubInboundMessage extends VSOBaseModel {
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
// PubSubInboundPMessage  (inbound pmessage event from g8eg WebSocket)
// ---------------------------------------------------------------------------

export class PubSubInboundPMessage extends VSOBaseModel {
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

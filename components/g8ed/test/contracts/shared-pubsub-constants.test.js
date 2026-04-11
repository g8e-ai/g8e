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
 * Pub/Sub Wire Protocol Contract Tests
 *
 * Verifies that g8ed's PubSubAction and PubSubMessageType constants match the
 * canonical values in shared/constants/pubsub.json. If any value drifts, this
 * test breaks — closing the triangle (Go + Python + JS all verified against
 * one JSON).
 */

import { describe, it, expect } from 'vitest';
import { PubSubAction, PubSubMessageType } from '@g8ed/constants/channels.js';
import { _PUBSUB } from '@g8ed/constants/shared.js';

const wire = _PUBSUB.wire;

describe('g8ed PubSubAction matches shared/constants/pubsub.json', () => {
    it('SUBSCRIBE matches JSON', () => {
        expect(PubSubAction.SUBSCRIBE).toBe(wire.actions.subscribe);
    });

    it('PSUBSCRIBE matches JSON', () => {
        expect(PubSubAction.PSUBSCRIBE).toBe(wire.actions.psubscribe);
    });

    it('UNSUBSCRIBE matches JSON', () => {
        expect(PubSubAction.UNSUBSCRIBE).toBe(wire.actions.unsubscribe);
    });

    it('PUBLISH matches JSON', () => {
        expect(PubSubAction.PUBLISH).toBe(wire.actions.publish);
    });

    it('covers all JSON actions', () => {
        const jsonKeys = Object.keys(wire.actions);
        const jsKeys = Object.keys(PubSubAction);
        expect(jsKeys.length).toBe(jsonKeys.length);
    });
});

describe('g8ed PubSubMessageType matches shared/constants/pubsub.json', () => {
    it('MESSAGE matches JSON', () => {
        expect(PubSubMessageType.MESSAGE).toBe(wire.event_types.message);
    });

    it('PMESSAGE matches JSON', () => {
        expect(PubSubMessageType.PMESSAGE).toBe(wire.event_types.pmessage);
    });

    it('SUBSCRIBED matches JSON', () => {
        expect(PubSubMessageType.SUBSCRIBED).toBe(wire.event_types.subscribed);
    });

    it('covers all JSON event_types', () => {
        const jsonKeys = Object.keys(wire.event_types);
        const jsKeys = Object.keys(PubSubMessageType);
        expect(jsKeys.length).toBe(jsonKeys.length);
    });
});

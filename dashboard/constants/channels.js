// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { _CHANNELS, _PUBSUB } from './shared.js';

/**
 * Pub/Sub Constants
 * Shared values load from protocol/constants/channels.json and protocol/constants/pubsub.json. Dashboard auth channel prefixes remain local.
 *
 * Auth channels:
 *   auth.publish:{api_key_hash}               - VSA → g8ed API key auth request
 *   auth.publish:session:{session_hash}       - VSA → g8ed session auth request
 *   auth.response:{api_key_hash}              - g8ed → VSA API key auth response
 *   auth.response:session:{hash}              - g8ed → VSA session auth response
 *
 * Operator channels:
 *   cmd:{operator_id}:{operator_session_id}       - VSE → Operator command dispatch
 *   results:{operator_id}:{operator_session_id}   - Operator → VSE result delivery
 *   heartbeat:{operator_id}:{operator_session_id} - Operator → VSE heartbeat
 */

const channels = _CHANNELS.channels;
const fields = _PUBSUB.pubsub;

export const PubSubAction = Object.freeze({
    SUBSCRIBE:   channels.Subscribe,
    PSUBSCRIBE:  channels.PSubscribe,
    UNSUBSCRIBE: channels.Unsubscribe,
    PUBLISH:     channels.Publish,
});

export const PubSubMessageType = Object.freeze({
    MESSAGE:    channels.Message,
    PMESSAGE:   channels.PMessage,
    SUBSCRIBED: channels.Subscribed,
});

export const PubSubField = Object.freeze({
    ACTION:  fields.FieldAction,
    CHANNEL: fields.FieldChannel,
    DATA:    fields.FieldData,
    MESSAGE: fields.FieldMessage,
    PATTERN: fields.FieldPattern,
    SENDER:  fields.FieldSender,
    TYPE:    fields.FieldType,
});

export const PubSubChannel = Object.freeze({
    AUTH_PUBLISH_PREFIX:          'auth.publish',
    AUTH_PUBLISH_SESSION_PREFIX:  'auth.publish:session',
    AUTH_RESPONSE_PREFIX:         'auth.response',
    AUTH_RESPONSE_SESSION_PREFIX: 'auth.response:session',
    AUTH_SESSION_PREFIX:          'auth.session',
    CMD_PREFIX:                   `${channels.PrefixCmd}:`,
    HEARTBEAT_PREFIX:             `${channels.PrefixHeartbeat}:`,
    RESULTS_PREFIX:               `${channels.PrefixResults}:`,
});

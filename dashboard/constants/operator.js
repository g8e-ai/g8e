// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { _STATUS } from './shared.js';
import { EventType } from './events.js';

/**
 * Operator Constants
 * Lifecycle status, type classification, history trail events and actors,
 * and status display helpers for the operator domain.
 * Wire-protocol values are sourced from protocol/constants/status.json.
 */

/**
 * Operator Status
 * Lifecycle states of an operator daemon.
 * Canonical values from protocol/constants/status.json operator.status.
 */
export const OperatorStatus = Object.freeze({
    AVAILABLE:   _STATUS['operator.status']['available'],
    UNAVAILABLE: _STATUS['operator.status']['unavailable'],
    OFFLINE:     _STATUS['operator.status']['offline'],
    BOUND:       _STATUS['operator.status']['bound'],
    STALE:       _STATUS['operator.status']['stale'],
    ACTIVE:      _STATUS['operator.status']['active'],
    STOPPED:     _STATUS['operator.status']['stopped'],
    TERMINATED:  _STATUS['operator.status']['terminated'],
});

/**
 * Operator Types
 * Classifies the kind of operator registered on the platform.
 * Canonical values from protocol/constants/status.json operator.type.
 */
export const OperatorType = Object.freeze({
    SYSTEM: _STATUS['operator.type']['system'],
    CLOUD:  _STATUS['operator.type']['cloud'],
});

export const ExecutionStatus = Object.freeze({
    PENDING:          _STATUS['execution.status']['pending'],
    EXECUTING:        _STATUS['execution.status']['executing'],
    COMPLETED:        _STATUS['execution.status']['completed'],
    FAILED:           _STATUS['execution.status']['failed'],
    TIMEOUT:          _STATUS['execution.status']['timeout'],
    CANCELLED:        _STATUS['execution.status']['cancelled'],
    CANCEL_REQUESTED: _STATUS['execution.status']['cancel.requested'],
    DENIED:           _STATUS['execution.status']['denied'],
    FEEDBACK:         _STATUS['execution.status']['feedback'],
});

/**
 * Cloud Operator Subtypes
 * Further classifies cloud operators by provider.
 * Canonical values from protocol/constants/status.json cloud.subtype.
 */
export const CloudOperatorSubtype = Object.freeze({
    AWS:      _STATUS['cloud.subtype']['aws'],
    GCP:      _STATUS['cloud.subtype']['gcp'],
    AZURE:    _STATUS['cloud.subtype']['azure'],
    DROP_POD: _STATUS['cloud.subtype']['drop_pod'],
});

/**
 * Operator History Trail Event Types
 * event_type values in the operator document's history_trail array.
 * Canonical values from protocol/constants/status.json history.event.type.
 */
export const HistoryEventType = Object.freeze({
    CREATED:               _STATUS['history.event.type']['created'],
    SLOT_CREATED:          _STATUS['history.event.type']['slot.created'],
    SLOT_CONSUMED:         _STATUS['history.event.type']['slot.consumed'],
    SLOT_RELEASED:         _STATUS['history.event.type']['slot.released'],
    BOUND:                 _STATUS['history.event.type']['bound'],
    UNBOUND:               _STATUS['history.event.type']['unbound'],
    HEARTBEAT_RECEIVED:    _STATUS['history.event.type']['heartbeat.received'],
    STATUS_CHANGED:        _STATUS['history.event.type']['status.changed'],
    API_KEY_REFRESHED:     _STATUS['history.event.type']['api.key.refreshed'],
    CREATED_FROM_REFRESH:  _STATUS['history.event.type']['created.from.refresh'],
    TERMINATED_FOR_REFRESH: _STATUS['history.event.type']['terminated.for.refresh'],
    RESET:                 _STATUS['history.event.type']['reset'],
    TERMINATED:            _STATUS['history.event.type']['terminated'],
    AUTHENTICATED:         _STATUS['history.event.type']['authenticated'],
    DEACTIVATED:           _STATUS['history.event.type']['deactivated'],
    STOPPED:               _STATUS['history.event.type']['stopped'],
    SHUTDOWN_REQUESTED:    _STATUS['history.event.type']['shutdown.requested'],
    CLAIMED:               _STATUS['history.event.type']['claimed'],
    RECONNECTED:           _STATUS['history.event.type']['reconnected'],
});

/**
 * Map an OperatorStatus value to its canonical OPERATOR_STATUS_UPDATED_* EventType.
 *
 * @param {string} status - An OperatorStatus value
 * @returns {string} The matching EventType constant value
 * @throws {Error} If status is not a known OperatorStatus value
 */
export function operatorStatusToEventType(status) {
    switch (status) {
        case OperatorStatus.ACTIVE:      return EventType.OPERATOR_STATUS_UPDATED_ACTIVE;
        case OperatorStatus.AVAILABLE:   return EventType.OPERATOR_STATUS_UPDATED_AVAILABLE;
        case OperatorStatus.UNAVAILABLE: return EventType.OPERATOR_STATUS_UPDATED_UNAVAILABLE;
        case OperatorStatus.BOUND:       return EventType.OPERATOR_STATUS_UPDATED_BOUND;
        case OperatorStatus.OFFLINE:     return EventType.OPERATOR_STATUS_UPDATED_OFFLINE;
        case OperatorStatus.STALE:       return EventType.OPERATOR_STATUS_UPDATED_STALE;
        case OperatorStatus.STOPPED:     return EventType.OPERATOR_STATUS_UPDATED_STOPPED;
        case OperatorStatus.TERMINATED:  return EventType.OPERATOR_STATUS_UPDATED_TERMINATED;
        default:
            throw new Error(`Unknown OperatorStatus value: ${status}`);
    }
}

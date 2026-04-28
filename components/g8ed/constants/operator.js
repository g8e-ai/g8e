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

import { _STATUS } from './shared.js';
import { EventType } from './events.js';

/**
 * Operator Constants
 * Lifecycle status, type classification, and status display helpers
 * for the operator domain.
 * Wire-protocol values are sourced from shared/constants/status.json.
 */

// ---------------------------------------------------------------------------
// Operator Slot Defaults
// ---------------------------------------------------------------------------
export const DEFAULT_OPERATOR_SLOTS = 20;
export const DEFAULT_SLOT_COST = 1;

/**
 * Operator Status
 * Lifecycle states of an operator daemon.
 * Canonical values from shared/constants/status.json g8e.status.
 */
export const OperatorStatus = Object.freeze({
    AVAILABLE:   _STATUS['g8e.status']['available'],
    UNAVAILABLE: _STATUS['g8e.status']['unavailable'],
    OFFLINE:     _STATUS['g8e.status']['offline'],
    BOUND:       _STATUS['g8e.status']['bound'],
    STALE:       _STATUS['g8e.status']['stale'],
    ACTIVE:      _STATUS['g8e.status']['active'],
    STOPPED:     _STATUS['g8e.status']['stopped'],
    TERMINATED:  _STATUS['g8e.status']['terminated'],
});

/**
 * Operator Types
 * Classifies the kind of operator registered on the platform.
 * Canonical values from shared/constants/status.json g8e.type.
 */
export const OperatorType = Object.freeze({
    SYSTEM: _STATUS['g8e.type']['system'],
    CLOUD:  _STATUS['g8e.type']['cloud'],
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
 * Approval Type
 * Types of approvals that can be requested.
 * Canonical values from shared/constants/status.json approval.type.
 */
export const ApprovalType = Object.freeze({
    COMMAND:        _STATUS['approval.type']['command'],
    FILE_EDIT:      _STATUS['approval.type']['file.edit'],
    INTENT:         _STATUS['approval.type']['intent'],
    AGENT_CONTINUE: _STATUS['approval.type']['agent.continue'],
});

/**
 * Approval Error Type
 * Error classifications for approval failures.
 * Canonical values from shared/constants/status.json approval.error.type.
 */
export const ApprovalErrorType = Object.freeze({
    APPROVAL_PUBLISH_FAILURE:  _STATUS['approval.error.type']['approval.publish.failure'],
    APPROVAL_EXCEPTION:         _STATUS['approval.error.type']['approval.exception'],
    APPROVAL_TIMEOUT:           _STATUS['approval.error.type']['approval.timeout'],
    INVALID_INTENT:             _STATUS['approval.error.type']['invalid.intent'],
    INTENT_APPROVAL_EXCEPTION:  _STATUS['approval.error.type']['intent.approval.exception'],
});

/**
 * Attachment Type
 * File attachment classifications.
 * Canonical values from shared/constants/status.json attachment.type.
 */
export const AttachmentType = Object.freeze({
    PDF:   _STATUS['attachment.type']['pdf'],
    IMAGE: _STATUS['attachment.type']['image'],
    TEXT:  _STATUS['attachment.type']['text'],
    OTHER: _STATUS['attachment.type']['other'],
});

/**
 * Cloud Operator Subtypes
 * Further classifies cloud operators by provider.
 * Canonical values from shared/constants/status.json cloud.subtype.
 */
export const CloudOperatorSubtype = Object.freeze({
    AWS:      _STATUS['cloud.subtype']['aws'],
    GCP:      _STATUS['cloud.subtype']['gcp'],
    AZURE:    _STATUS['cloud.subtype']['azure'],
    G8E_POD: _STATUS['cloud.subtype']['g8ep'],
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

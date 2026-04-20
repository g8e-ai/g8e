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
 * Request Models for g8ed
 *
 * Aligned with:
 * - components/g8ee/app/models/investigations.py (InvestigationCreateRequest, etc.)
 *
 * Construction from untrusted input (route handlers, API boundaries):
 *   ChatMessageRequest.parse(req.body)
 *
 * forWire() is only overridden where the outbound shape differs from the full
 * field set (e.g. ChatMessageRequest omits identity fields that go via headers).
 */

import { G8eBaseModel, F } from './base.js';
import { SystemInfo } from './operator_model.js';

// ---------------------------------------------------------------------------
// G8eHttpContext
//
// Mirror of g8ee's G8eHttpContext. Carries session, user, and business context
// across the cluster via X-G8E-* headers.
// ---------------------------------------------------------------------------

export class G8eHttpContext extends G8eBaseModel {
    static fields = {
        web_session_id:    { type: F.string,  required: true },
        user_id:           { type: F.string,  required: true },
        organization_id:   { type: F.string,  default: null },
        case_id:           { type: F.string,  default: null },
        investigation_id:  { type: F.string,  default: null },
        task_id:           { type: F.string,  default: null },
        bound_operators:   { type: F.array,   default: () => [] }, // Array of BoundOperatorContext
        execution_id:        { type: F.string,  default: null },
        source_component:  { type: F.string,  default: 'g8ed' },
    };

    static parse(raw = {}) {
        // Handle new case signal logic here instead of in routes
        const data = { ...raw };
        if (!data.case_id || data.case_id === '') {
            data.case_id = null; // Let the client/headers logic handle NEW_CASE_ID
        }
        return super.parse(data);
    }
}

// ---------------------------------------------------------------------------
// ChatMessageRequest
//
// ARCHITECTURE NOTE:
// Identity/context fields (web_session_id, user_id) are validated here but
// NOT sent in the outbound body to g8ee — they travel via X-G8E-* headers
// built by buildG8eContext(). forWire() returns only what g8ee expects in body.
// ---------------------------------------------------------------------------

export class ChatMessageRequest extends G8eBaseModel {
    static fields = {
        web_session_id:       { type: F.string,  required: true },
        user_id:              { type: F.string,  required: true },
        message:              { type: F.string,  required: true, minLength: 1 },
        attachments:          { type: F.array,   default: () => [] },
        llm_primary_provider: { type: F.string,  default: null },
        llm_assistant_provider: { type: F.string,  default: null },
        llm_lite_provider:    { type: F.string,  default: null },
        llm_primary_model:    { type: F.string,  default: null },
        llm_assistant_model:  { type: F.string,  default: null },
        llm_lite_model:       { type: F.string,  default: null },
        case_id:              { type: F.string,  default: null },
        investigation_id:     { type: F.string,  default: null },
    };

    forWire() {
        return {
            message:               this.message,
            attachments:           this.attachments,
            sentinel_mode:         true,
            llm_primary_provider:  this.llm_primary_provider,
            llm_assistant_provider: this.llm_assistant_provider,
            llm_lite_provider:     this.llm_lite_provider,
            llm_primary_model:     this.llm_primary_model,
            llm_assistant_model:   this.llm_assistant_model,
            llm_lite_model:        this.llm_lite_model,
        };
    }
}

// ---------------------------------------------------------------------------
// InvestigationQueryRequest
//
// NOTE: user_id is intentionally absent — g8ee extracts it from authenticated
// headers (x-g8e-user-id). forWire() serializes to query-param-friendly shape.
// ---------------------------------------------------------------------------

export class InvestigationQueryRequest extends G8eBaseModel {
    static fields = {
        case_id:            { type: F.string, default: null },
        web_session_id:     { type: F.string, default: null },
        status:             { type: F.string, default: null },
        investigation_type: { type: F.string, default: null },
        priority:           { type: F.string, default: null },
        limit:              { type: F.number, default: 20, min: 1, max: 100 },
    };

    forWire() {
        const result = {};
        if (this.case_id)            result.case_id            = this.case_id;
        if (this.web_session_id)     result.web_session_id     = this.web_session_id;
        if (this.status)             result.status             = this.status;
        if (this.investigation_type) result.investigation_type = this.investigation_type;
        if (this.priority)           result.priority           = this.priority;
        result.limit = this.limit;
        return result;
    }
}

// ---------------------------------------------------------------------------
// SessionCreateRequest
// ---------------------------------------------------------------------------

export class SessionCreateRequest extends G8eBaseModel {
    static fields = {
        user_id:         { type: F.string, required: true },
        organization_id: { type: F.string, default: null },
        metadata:        { type: F.object, default: () => ({}) },
    };
}

// ---------------------------------------------------------------------------
// ApprovalRespondRequest
// ---------------------------------------------------------------------------

export class ApprovalRespondRequest extends G8eBaseModel {
    static fields = {
        approval_id:         { type: F.string,  required: true },
        approved:            { type: F.boolean, required: true },
        reason:              { type: F.string,  default: '' },
    };
}

// ---------------------------------------------------------------------------
// IntentRequest
//
// Aligned with shared/models/wire/internal_requests.json (intent_request)
// ---------------------------------------------------------------------------

export class IntentRequest extends G8eBaseModel {
    static fields = {
        intent: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// UnlockAccountRequest
//
// Aligned with shared/models/wire/internal_requests.json (unlock_account)
// ---------------------------------------------------------------------------

export class UnlockAccountRequest extends G8eBaseModel {
    static fields = {
        user_id: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// SSEPushRequest
//
// Aligned with shared/models/wire/internal_requests.json (sse_push)
// ---------------------------------------------------------------------------

export class SSEPushRequest extends G8eBaseModel {
    static fields = {
        web_session_id: { type: F.string, default: null },
        user_id:        { type: F.string, required: true },
        event:          { type: F.object, required: true },
    };
}

// ---------------------------------------------------------------------------
// IntentRequestPayload
//
// Alias for IntentRequest used in some contexts
// ---------------------------------------------------------------------------

export class IntentRequestPayload extends G8eBaseModel {
    static fields = {
        intent: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// DirectCommandRequest
// ---------------------------------------------------------------------------

export class DirectCommandRequest extends G8eBaseModel {
    static fields = {
        command:      { type: F.string, required: true, minLength: 1 },
        execution_id: { type: F.string, required: true },
        hostname:     { type: F.string, default: null },
        source:       { type: F.string, default: 'anchored_terminal' },
    };
}

// ---------------------------------------------------------------------------
// CreateOperatorRequest
// ---------------------------------------------------------------------------

export class CreateOperatorRequest extends G8eBaseModel {
    static fields = {
        operator_id:         { type: F.string, required: true },
        user_id:             { type: F.string, required: true },
        operator_session_id: { type: F.string, required: true },
        web_session_id:      { type: F.string, default: null },
        organization_id:     { type: F.string, default: null },
        system_info:         { type: F.any,    default: () => ({}) },
        runtime_config:      { type: F.any,    default: () => ({}) },
        api_key:             { type: F.string, default: null },
        operator_type:       { type: F.string, default: null },
        cloud_subtype:       { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// PasskeyRegisterChallengeRequest
// ---------------------------------------------------------------------------

export class PasskeyRegisterChallengeRequest extends G8eBaseModel {
    static fields = {
        user_id: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// AttestationResponseJSON — typed shape for WebAuthn registration response
// Mirrors RegistrationResponseJSON from @simplewebauthn/server
// ---------------------------------------------------------------------------

export class AttestationResponseJSON extends G8eBaseModel {
    static fields = {
        id:                     { type: F.string,  required: true },
        rawId:                  { type: F.string,  required: true },
        type:                   { type: F.string,  required: true },
        clientExtensionResults: { type: F.object,  default: () => ({}) },
        response: {
            type:     F.object,
            required: true,
        },
    };

    static parse(raw = {}) {
        if (!raw || typeof raw !== 'object') {
            throw new Error('AttestationResponseJSON.parse() requires a plain object');
        }
        if (!raw.response || typeof raw.response !== 'object') {
            throw new Error('AttestationResponseJSON requires response object');
        }
        if (typeof raw.response.clientDataJSON !== 'string') {
            throw new Error('AttestationResponseJSON requires response.clientDataJSON string');
        }
        if (typeof raw.response.attestationObject !== 'string') {
            throw new Error('AttestationResponseJSON requires response.attestationObject string');
        }
        return super.parse(raw);
    }
}

// ---------------------------------------------------------------------------
// AssertionResponseJSON — typed shape for WebAuthn authentication response
// Mirrors AuthenticationResponseJSON from @simplewebauthn/server
// ---------------------------------------------------------------------------

export class AssertionResponseJSON extends G8eBaseModel {
    static fields = {
        id:                     { type: F.string,  required: true },
        rawId:                  { type: F.string,  required: true },
        type:                   { type: F.string,  required: true },
        clientExtensionResults: { type: F.object,  default: () => ({}) },
        response: {
            type:     F.object,
            required: true,
        },
    };

    static parse(raw = {}) {
        if (!raw || typeof raw !== 'object') {
            throw new Error('AssertionResponseJSON.parse() requires a plain object');
        }
        if (!raw.response || typeof raw.response !== 'object') {
            throw new Error('AssertionResponseJSON requires response object');
        }
        if (typeof raw.response.clientDataJSON !== 'string') {
            throw new Error('AssertionResponseJSON requires response.clientDataJSON string');
        }
        if (typeof raw.response.authenticatorData !== 'string') {
            throw new Error('AssertionResponseJSON requires response.authenticatorData string');
        }
        if (typeof raw.response.signature !== 'string') {
            throw new Error('AssertionResponseJSON requires response.signature string');
        }
        return super.parse(raw);
    }
}

// ---------------------------------------------------------------------------
// PasskeyRegisterVerifyRequest
// ---------------------------------------------------------------------------

export class PasskeyRegisterVerifyRequest extends G8eBaseModel {
    static fields = {
        user_id:              { type: F.string, required: true },
        attestation_response: { type: F.object, required: true, model: AttestationResponseJSON },
    };
}

// ---------------------------------------------------------------------------
// PasskeyAuthChallengeRequest
// ---------------------------------------------------------------------------

export class PasskeyAuthChallengeRequest extends G8eBaseModel {
    static fields = {
        email: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// PasskeyAuthVerifyRequest
// ---------------------------------------------------------------------------

export class PasskeyAuthVerifyRequest extends G8eBaseModel {
    static fields = {
        email:              { type: F.string, required: true },
        assertion_response: { type: F.object, required: true, model: AssertionResponseJSON },
    };
}

// ---------------------------------------------------------------------------
// CreateDeviceLinkRequest
// ---------------------------------------------------------------------------

export class CreateDeviceLinkRequest extends G8eBaseModel {
    static fields = {
        name:              { type: F.string,  default: null },
        max_uses:          { type: F.number,  default: 1, min: 1 },
        expires_in_hours:  { type: F.number,  default: 24, min: 1 },
    };
}

// ---------------------------------------------------------------------------
// GenerateDeviceLinkRequest
// ---------------------------------------------------------------------------

export class GenerateDeviceLinkRequest extends G8eBaseModel {
    static fields = {
        operator_id: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// RegisterDeviceRequest
// ---------------------------------------------------------------------------

export class RegisterDeviceRequest extends G8eBaseModel {
    static fields = {
        hostname:           { type: F.string, required: true },
        os:                 { type: F.string, required: true },
        arch:               { type: F.string, required: true },
        system_fingerprint: { type: F.string, required: true },
        version:            { type: F.string, default: 'unknown' },
        system_info:        { type: F.object, default: () => ({}) },
    };
}

// ---------------------------------------------------------------------------
// BindOperatorsRequest
// ---------------------------------------------------------------------------

export class BindOperatorsRequest extends G8eBaseModel {
    static fields = {
        operator_ids:   { type: F.array,  required: true },
        web_session_id: { type: F.string, required: true },
        user_id:        { type: F.string, required: true },
    };

    _validate() {
        if (!Array.isArray(this.operator_ids) || this.operator_ids.length === 0) {
            const err = new Error('BindOperatorsRequest validation failed: operator_ids must be a non-empty array');
            err.validationErrors = ['operator_ids must be a non-empty array'];
            throw err;
        }
    }
}

// ---------------------------------------------------------------------------
// UnbindOperatorsRequest
// ---------------------------------------------------------------------------

export class UnbindOperatorsRequest extends G8eBaseModel {
    static fields = {
        operator_ids:   { type: F.array,  required: true },
        web_session_id: { type: F.string, required: true },
        user_id:        { type: F.string, required: true },
    };

    _validate() {
        if (!Array.isArray(this.operator_ids) || this.operator_ids.length === 0) {
            const err = new Error('UnbindOperatorsRequest validation failed: operator_ids must be a non-empty array');
            err.validationErrors = ['operator_ids must be a non-empty array'];
            throw err;
        }
    }
}

// ---------------------------------------------------------------------------
// OperatorSessionRegistrationRequest
// ---------------------------------------------------------------------------

export class OperatorSessionRegistrationRequest extends G8eBaseModel {
    static fields = {
        operator_id:         { type: F.string, required: true },
        operator_session_id: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// StopOperatorRequest
// ---------------------------------------------------------------------------

export class StopOperatorRequest extends G8eBaseModel {
    static fields = {
        operator_id:         { type: F.string, required: true },
        operator_session_id: { type: F.string, required: true },
        user_id:             { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// OperatorRegisterSessionRequest (for internal session registration)
// ---------------------------------------------------------------------------

export class OperatorRegisterSessionRequest extends G8eBaseModel {
    static fields = {
        operator_id:         { type: F.string, required: true },
        operator_session_id: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// SettingsUpdateRequest
// ---------------------------------------------------------------------------

export class SettingsUpdateRequest extends G8eBaseModel {
    static fields = {
        settings:     { type: F.object, required: true },
        setup_secret: { type: F.string, default: null },
    };

    _validate() {
        if (Array.isArray(this.settings)) {
            throw new Error('SettingsUpdateRequest validation failed: settings must be an object, not an array');
        }
    }
}

// ---------------------------------------------------------------------------
// RefreshOperatorKeyRequest
// ---------------------------------------------------------------------------

export class RefreshOperatorKeyRequest extends G8eBaseModel {
    static fields = {
        user_id: { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// InitializeOperatorSlotsRequest
// ---------------------------------------------------------------------------

export class InitializeOperatorSlotsRequest extends G8eBaseModel {
    static fields = {
        organization_id: { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// CreateUserRequest
// ---------------------------------------------------------------------------

export class CreateUserRequest extends G8eBaseModel {
    static fields = {
        email: { type: F.string, required: true, minLength: 3 },
        name:  { type: F.string, required: true, minLength: 1 },
        roles: { type: F.array,  default: null }, // Will default to [USER] in service
    };
}

// ---------------------------------------------------------------------------
// UpdateUserRolesRequest
// ---------------------------------------------------------------------------

export class UpdateUserRolesRequest extends G8eBaseModel {
    static fields = {
        role:   { type: F.string, required: true },
        action: { type: F.string, default: 'set' }, // 'set', 'add', 'remove'
    };
}

// ---------------------------------------------------------------------------
// CreateDeviceLinkRequest
// ---------------------------------------------------------------------------

export class StopAIRequest extends G8eBaseModel {
    static fields = {
        investigation_id: { type: F.string, required: true },
        reason:           { type: F.string, default: 'User requested stop' },
        web_session_id:   { type: F.string, required: true },
    };
}

// ---------------------------------------------------------------------------
// SessionAuthResponse  (pub/sub response payload for device link session auth)
// ---------------------------------------------------------------------------

export class SessionAuthResponse extends G8eBaseModel {
    static fields = {
        success:             { type: F.boolean, required: true },
        operator_session_id: { type: F.string,  default: null },
        operator_id:         { type: F.string,  default: null },
        user_id:             { type: F.string,  default: null },
        organization_id:     { type: F.string,  default: null },
        api_key:             { type: F.string,  default: null },
        config:              { type: F.object,  default: () => ({}) },
        operator_cert:       { type: F.string,  default: null },
        operator_cert_key:   { type: F.string,  default: null },
        error:               { type: F.string,  default: null },
    };
}

// ---------------------------------------------------------------------------
// BoundOperatorContext
//
// Typed representation of a single bound operator entry carried in the
// X-G8E-Bound-Operators header from g8ed to G8EE.
//
// Canonical wire shape: shared/models/wire/bound_operator_context.json
// ---------------------------------------------------------------------------

export class BoundOperatorContext extends G8eBaseModel {
    static fields = {
        operator_id:         { type: F.string, required: true },
        operator_session_id: { type: F.string, default: null },
        bound_web_session_id: { type: F.string, default: null },
        status:              { type: F.string, default: null },
    };
}

// ---------------------------------------------------------------------------
// RequestModelFactory
// ---------------------------------------------------------------------------

export class RequestModelFactory {
    static createChatRequest(data)                       { return ChatMessageRequest.parse(data); }
    static createInvestigationQueryRequest(data)         { return InvestigationQueryRequest.parse(data); }
    static createUnlockAccountRequest(data)              { return UnlockAccountRequest.parse(data); }
    static createSettingsUpdateRequest(data)             { return SettingsUpdateRequest.parse(data); }
    static createRefreshOperatorKeyRequest(data)         { return RefreshOperatorKeyRequest.parse(data); }
    static createInitializeOperatorSlotsRequest(data)    { return InitializeOperatorSlotsRequest.parse(data); }
    static createUpdateUserRolesRequest(data)            { return UpdateUserRolesRequest.parse(data); }
    static createCreateUserRequest(data)                 { return CreateUserRequest.parse(data); }
    static createSSEPushRequest(data)                    { return SSEPushRequest.parse(data); }
    static createIntentRequest(data)                     { return IntentRequest.parse(data); }
    static createSessionRequest(data)                    { return SessionCreateRequest.parse(data); }
    static createApprovalRespondRequest(data)            { return ApprovalRespondRequest.parse(data); }
    static createDirectCommandRequest(data)              { return DirectCommandRequest.parse(data); }
    static createOperatorRequest(data)                   { return CreateOperatorRequest.parse(data); }
    static createBindOperatorsRequest(data)              { return BindOperatorsRequest.parse(data); }
    static createUnbindOperatorsRequest(data)            { return UnbindOperatorsRequest.parse(data); }
    static createCreateDeviceLinkRequest(data)           { return CreateDeviceLinkRequest.parse(data); }
    static createGenerateDeviceLinkRequest(data)         { return GenerateDeviceLinkRequest.parse(data); }
    static createRegisterDeviceRequest(data)             { return RegisterDeviceRequest.parse(data); }
    static createPasskeyRegisterChallengeRequest(data)   { return PasskeyRegisterChallengeRequest.parse(data); }
    static createPasskeyRegisterVerifyRequest(data)      { return PasskeyRegisterVerifyRequest.parse(data); }
    static createPasskeyAuthChallengeRequest(data)       { return PasskeyAuthChallengeRequest.parse(data); }
    static createPasskeyAuthVerifyRequest(data)          { return PasskeyAuthVerifyRequest.parse(data); }
}

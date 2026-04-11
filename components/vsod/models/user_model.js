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
 * User Domain Model
 *
 * Pure data class for user documents stored in VSODB document store + VSODB KV cache.
 * All business logic lives in UserService (services/platform/user_service.js).
 */

import { VSOBaseModel, VSOIdentifiableModel, F, now } from './base.js';
import { UserRole, AuthProvider } from '../constants/auth.js';

// ---------------------------------------------------------------------------
// PasskeyCredential  (sub-document stored in UserDocument.passkey_credentials)
// ---------------------------------------------------------------------------

export class PasskeyCredential extends VSOBaseModel {
    static fields = {
        id:           { type: F.string, required: true },
        public_key:   { type: F.string, required: true },
        counter:      { type: F.number, required: true },
        transports:   { type: F.array,  default: () => [] },
        created_at:   { type: F.date,   default: () => now() },
        last_used_at: { type: F.date,   default: null },
    };
}

// ---------------------------------------------------------------------------
// UserDocument  (user record stored in VSODB document store + VSODB KV cache)
// ---------------------------------------------------------------------------

export class UserDocument extends VSOIdentifiableModel {
    static fields = {
        id:                             { type: F.string,  required: true },
        email:                          { type: F.string,  required: true },
        name:                           { type: F.string,  default: null },
        passkey_credentials:            { type: F.array,   items: PasskeyCredential, default: () => [] },
        passkey_challenge:              { type: F.string,  default: null },
        passkey_challenge_expires_at:   { type: F.date,    default: null },
        g8e_key:               { type: F.string,  default: null },
        g8e_key_created_at:    { type: F.date,    default: null },
        g8e_key_updated_at:    { type: F.date,    default: null },
        organization_id:                { type: F.string,  default: null },
        roles:                          { type: F.array,   default: () => [UserRole.USER] },
        operator_id:                    { type: F.string,  default: null },
        operator_status:                { type: F.string,  default: null },
        last_login:                     { type: F.date,    default: null },
        provider:                       { type: F.string,  default: AuthProvider.PASSKEY },
        sessions:                       { type: F.array,   default: () => [] },
        profile_picture:                { type: F.string,  default: null },
        dev_logs_enabled:               { type: F.boolean, default: true, coerce: true },
    };

    forClient() {
        const doc = this.forWire();
        delete doc.passkey_credentials;
        delete doc.passkey_challenge;
        delete doc.passkey_challenge_expires_at;
        delete doc.g8e_key;
        delete doc.sessions;
        return doc;
    }
}

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
 * Conversation Model for VSOD
 *
 * JavaScript equivalent of:
 *   components/vse/app/models/conversation.py  (Python/Pydantic)
 *
 * Represents an active AI chat conversation session keyed by web_session_id.
 * VSOD is the transport layer — it builds and forwards conversation context to VSE
 * and routes streaming responses back to the correct browser tab.
 */

import { ConversationStatus } from '../constants/chat.js';
import { VSOIdentifiableModel, F, now } from './base.js';

const VALID_STATUSES = new Set(Object.values(ConversationStatus));

export class Conversation extends VSOIdentifiableModel {
    static fields = {
        web_session_id:   { type: F.string, required: true },
        case_id:          { type: F.string, default: null },
        investigation_id: { type: F.string, default: null },
        user_id:          { type: F.string, default: null },
        status:           { type: F.string, default: ConversationStatus.ACTIVE },
        sentinel_mode:    { type: F.boolean, default: true },
    };

    _validate() {
        if (this.status !== undefined && this.status !== null && !VALID_STATUSES.has(this.status)) {
            const err = new Error(`Conversation validation failed: status must be one of ${[...VALID_STATUSES].join(', ')}`);
            err.validationErrors = [`status must be one of ${[...VALID_STATUSES].join(', ')}`];
            throw err;
        }
    }

    isActive() {
        return this.status === ConversationStatus.ACTIVE;
    }

    deactivate() {
        this.status     = ConversationStatus.INACTIVE;
        this.updated_at = now();
    }

    complete() {
        this.status     = ConversationStatus.COMPLETED;
        this.updated_at = now();
    }

    forDB() {
        const obj = super.forDB();
        const optional = ['case_id', 'investigation_id', 'user_id', 'updated_at'];
        for (const key of optional) {
            if (obj[key] === null) delete obj[key];
        }
        return obj;
    }

    static fromSessionAndRequest(session, chatRequest) {
        return new Conversation({
            web_session_id:   session.id,
            case_id:          chatRequest.case_id          ?? null,
            investigation_id: chatRequest.investigation_id ?? null,
            user_id:          session.user_id,
            sentinel_mode:    chatRequest.sentinel_mode !== false,
        });
    }
}

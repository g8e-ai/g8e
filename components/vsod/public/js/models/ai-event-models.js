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

import { FrontendBaseModel, F } from './base.js';
import { ThinkingActionType } from '../constants/events.js';

/**
 * case.created — emitted as the first SSE event for new conversations.
 *
 * Shape: { case_id, investigation_id, title }
 */
export class CaseCreatedEvent extends FrontendBaseModel {
    static fields = {
        case_id:          { type: F.string, default: null },
        investigation_id: { type: F.string, default: null },
        title:            { type: F.string, default: null },
    };
}

/**
 * text — AI text token SSE event.
 *
 * Shape: { content, workflow_type, investigation_id, case_id }
 */
export class TextEvent extends FrontendBaseModel {
    static fields = {
        content:          { type: F.string, default: null },
        workflow_type:    { type: F.string, default: 'unknown' },
        investigation_id: { type: F.string, default: null },
        case_id:          { type: F.string, default: null },
    };
}

/**
 * thinking — LLM extended-thinking chunk SSE event.
 *
 * Shape: { thinking, action_type, web_session_id, investigation_id, case_id }
 * action_type: ThinkingActionType.START | ThinkingActionType.UPDATE | ThinkingActionType.END
 */
export class ThinkingEvent extends FrontendBaseModel {
    static fields = {
        thinking:         { type: F.string, default: null },
        action_type:      { type: F.string, default: null },
        web_session_id:   { type: F.string, default: null },
        investigation_id: { type: F.string, default: null },
        case_id:          { type: F.string, default: null },
    };

    _validate() {
        if (this.action_type !== null) {
            const valid = Object.values(ThinkingActionType);
            if (!valid.includes(this.action_type)) {
                const err = new Error('Validation failed');
                err.validationErrors = [`action_type must be one of: ${valid.join(', ')}`];
                throw err;
            }
        }
    }
}

/**
 * citations — grounding metadata SSE event.
 *
 * Shape: { grounding_metadata, investigation_id, case_id }
 */
export class CitationsEvent extends FrontendBaseModel {
    static fields = {
        grounding_metadata: { type: F.any,    default: null },
        investigation_id:   { type: F.string, default: null },
        case_id:            { type: F.string, default: null },
    };
}

/**
 * complete — final SSE event signalling end of AI turn.
 *
 * Shape: { finish_reason, has_citations, investigation_id, case_id }
 */
export class CompleteEvent extends FrontendBaseModel {
    static fields = {
        finish_reason:    { type: F.string,  default: 'STOP' },
        has_citations:    { type: F.boolean, default: false, coerce: true },
        investigation_id: { type: F.string,  default: null },
        case_id:          { type: F.string,  default: null },
    };
}

/**
 * error — AI error SSE event.
 *
 * Shape: { error, investigation_id, case_id }
 */
export class ErrorEvent extends FrontendBaseModel {
    static fields = {
        error:            { type: F.string, default: 'Unknown error' },
        investigation_id: { type: F.string, default: null },
        case_id:          { type: F.string, default: null },
    };
}

/**
 * A single grounding source entry from a citations SSE event.
 *
 * Shape: { citation_num, uri, display_name, domain, full_title, favicon_url, segments }
 */
export class CitationSource extends FrontendBaseModel {
    static fields = {
        citation_num: { type: F.number, required: true },
        uri:          { type: F.string, required: true },
        display_name: { type: F.string, required: true },
        domain:       { type: F.string, required: true },
        full_title:   { type: F.string, default: null },
        favicon_url:  { type: F.string, default: null },
        segments:     { type: F.array,  default: () => [] },
    };
}

/**
 * A citation item built from a CitationSource for inline hover-card rendering.
 *
 * Shape: { uri, displayName, domain, fullTitle, citationNum }
 */
export class CitationItem extends FrontendBaseModel {
    static fields = {
        uri:         { type: F.string, required: true },
        displayName: { type: F.string, required: true },
        domain:      { type: F.string, required: true },
        fullTitle:   { type: F.string, default: null },
        citationNum: { type: F.number, required: true },
    };
}

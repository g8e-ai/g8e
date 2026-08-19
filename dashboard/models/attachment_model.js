// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { VSOBaseModel, VSOIdentifiableModel, F, now } from './base.js';

export class AttachmentRecord extends VSOIdentifiableModel {
    static fields = {
        attachment_id:     { type: F.string, required: true },
        investigation_id:  { type: F.string, required: true },
        user_id:           { type: F.string, required: true },
        filename:          { type: F.string, required: true },
        original_filename: { type: F.string, required: true },
        file_size:         { type: F.number, required: true },
        content_type:      { type: F.string, required: true },
        object_key:        { type: F.string, required: true },
        stored_at:         { type: F.date,   default: () => now() },
    };
}

export class AttachmentMeta extends VSOBaseModel {
    static fields = {
        attachment_id:    { type: F.string, required: true },
        kv_key:           { type: F.string, required: true },
        filename:         { type: F.string, required: true },
        file_size:        { type: F.number, required: true },
        content_type:     { type: F.string, required: true },
        investigation_id: { type: F.string, required: true },
    };
}

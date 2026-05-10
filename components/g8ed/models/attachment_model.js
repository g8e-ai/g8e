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

import { G8eBaseModel, G8eIdentifiableModel, F, now } from './base.js';

export class AttachmentRecord extends G8eIdentifiableModel {
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

export class AttachmentMeta extends G8eBaseModel {
    static fields = {
        attachment_id:    { type: F.string, required: true },
        kv_key:           { type: F.string, required: true },
        filename:         { type: F.string, required: true },
        file_size:        { type: F.number, required: true },
        content_type:     { type: F.string, required: true },
        investigation_id: { type: F.string, required: true },
    };
}

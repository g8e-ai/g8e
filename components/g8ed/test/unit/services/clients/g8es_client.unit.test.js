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

import { describe, it, expect } from 'vitest';
import { G8esHttpClient, G8esDocumentClient, KVCacheClient, G8esPubSubClient } from '@g8ed/services/clients/g8es_client.js';

describe('db_client.js exports', () => {
    it('should export all g8es client classes', () => {
        expect(G8esHttpClient).toBeDefined();
        expect(G8esDocumentClient).toBeDefined();
        expect(KVCacheClient).toBeDefined();
        expect(G8esPubSubClient).toBeDefined();
    });
});

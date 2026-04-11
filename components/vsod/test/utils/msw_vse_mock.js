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

import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';

/**
 * MSW handlers for VSE internal API.
 * These are used to replace InternalHttpClient mocks with real HTTP interceptions.
 */
export const vseHandlers = [
    // Chat endpoint
    http.post('https://vse:8000/api/internal/chat', () => {
        return HttpResponse.json({ success: true, data: { case_id: 'c-123', investigation_id: 'i-123' } });
    }),

    // Settings sync endpoint
    http.patch('https://vse:8000/api/internal/settings', () => {
        return HttpResponse.json({ success: true });
    }),

    // Health check
    http.get('https://vse:8000/api/internal/health', () => {
        return HttpResponse.json({ status: 'healthy' });
    })
];

export const server = setupServer(...vseHandlers);

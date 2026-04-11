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

import { now } from '../models/base.js';
import { G8eHttpContext } from '../models/request_models.js';

/**
 * Global Context Middleware
 * 
 * Attaches a lazy-evaluated G8eHttpContext getter to the request object (req.g8eContext).
 * This allows route handlers to access a standardized context that automatically
 * includes identity data (from auth middleware) and business context (from body/query/params),
 * avoiding repetitive and error-prone manual construction in every route.
 */
export const globalContextMiddleware = (req, res, next) => {
    Object.defineProperty(req, 'g8eContext', {
        get() {
            if (!this._g8eContext) {
                // Determine source of business context fields
                const caseId = this.body?.case_id || this.query?.case_id || this.params?.caseId || null;
                const investigationId = this.body?.investigation_id || this.query?.investigation_id || this.params?.investigationId || null;
                const taskId = this.body?.task_id || this.query?.task_id || this.params?.taskId || null;
                
                // Use originalUrl for execution_id to get full path
                const rawPath = this.originalUrl ? this.originalUrl.split('?')[0] : this.path;
                const safePath = rawPath.replace(/\//g, '_').replace(/^_/, '');

                this._g8eContext = G8eHttpContext.parse({
                    web_session_id: this.webSessionId,
                    user_id: this.userId,
                    organization_id: this.session?.organization_id || this.session?.user_data?.organization_id || null,
                    case_id: caseId,
                    investigation_id: investigationId,
                    task_id: taskId,
                    bound_operators: this.boundOperators || [],
                    execution_id: `req_${safePath}_${now().getTime()}`
                });
            }
            return this._g8eContext;
        }
    });

    next();
};

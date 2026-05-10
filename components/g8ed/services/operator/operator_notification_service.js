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

export class OperatorNotificationService {
    /**
     * Legacy shim retained only for DI compatibility during the keepalive-driven
     * operator panel transition. All three former broadcast paths are now no-ops:
     *
     *   - broadcastOperatorListToUser / broadcastOperatorListToSession:
     *     superseded by the full operator list delivered via SSE keepalive.
     *   - broadcastOperatorContext: removed — it published a sparse payload
     *     under OPERATOR_PANEL_LIST_UPDATED that collided with the full-list
     *     shape and caused the frontend to wipe the operator panel list.
     *
     * @param {Object} options
     * @param {Object} options.webSessionService - WebSessionService instance
     * @param {Object} options.operatorDataService - OperatorDataService instance
     * @param {Object} options.sseService - SSEService instance
     */
    constructor({ webSessionService, operatorDataService, sseService }) {
        this.webSessionService = webSessionService;
        this.operatorDataService = operatorDataService;
        this.sseService = sseService;
    }

    async broadcastOperatorListToUser(_userId, _calculateSlotUsageFn) {
        return;
    }

    async broadcastOperatorListToSession(_userId, _webSessionId, _calculateSlotUsageFn) {
        return;
    }
}

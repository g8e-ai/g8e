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
 * HeartbeatMonitorService
 *
 * Periodically scans operator documents and reconciles their `status` field
 * against how recently g8eo has been phoning home (`last_heartbeat`).
 *
 * g8ed owns operator bind/auth state, so it is also authoritative for the
 * staleness of that binding. g8ee only writes `last_heartbeat` on inbound
 * heartbeat pub/sub messages — it never degrades status when heartbeats stop.
 *
 * Transition rules (bidirectional):
 *   STALE:   BOUND  -> STALE   |  ACTIVE -> OFFLINE
 *   RECOVER: STALE  -> BOUND   |  OFFLINE -> ACTIVE (only if last_heartbeat is fresh)
 *
 * Recovery from OFFLINE requires a fresh last_heartbeat timestamp, which only
 * happens when g8eo actually resumes heartbeats — so explicit STOPPED /
 * TERMINATED operators (which do not produce new heartbeats) are never
 * auto-reanimated by this service.
 *
 * On each transition the updated status is persisted via CacheAsideService and
 * the corresponding OPERATOR_STATUS_UPDATED_* SSE event is fanned out to the
 * owning user's active sessions so the Operator panel refreshes in real time.
 */

import { logger } from '../../utils/logger.js';
import { OperatorStatus, operatorStatusToEventType } from '../../constants/operator.js';
import { OperatorStaleThreshold } from '../../constants/service_config.js';
import { OperatorStatusUpdatedEvent, OperatorStatusUpdatedData } from '../../models/sse_models.js';
import { now } from '../../models/base.js';

const DEFAULT_MONITOR_INTERVAL_MS = 15_000;

const MONITORED_STATUSES = new Set([
    OperatorStatus.ACTIVE,
    OperatorStatus.BOUND,
    OperatorStatus.STALE,
    OperatorStatus.OFFLINE,
]);

/**
 * Compute the target status for an operator given whether its heartbeat is
 * currently stale. Returns null when no transition is required.
 */
export function resolveHeartbeatTransition(currentStatus, isStale) {
    if (isStale) {
        if (currentStatus === OperatorStatus.BOUND)  return OperatorStatus.STALE;
        if (currentStatus === OperatorStatus.ACTIVE) return OperatorStatus.OFFLINE;
        return null;
    }
    if (currentStatus === OperatorStatus.STALE)   return OperatorStatus.BOUND;
    if (currentStatus === OperatorStatus.OFFLINE) return OperatorStatus.ACTIVE;
    return null;
}

export class HeartbeatMonitorService {
    /**
     * @param {Object} options
     * @param {Object} options.operatorDataService
     * @param {Object} options.sseService
     * @param {number} [options.thresholdSeconds] - Staleness threshold in seconds.
     * @param {number} [options.intervalMs] - Tick interval in milliseconds.
     */
    constructor({
        operatorDataService,
        sseService,
        thresholdSeconds = OperatorStaleThreshold.SECONDS,
        intervalMs = DEFAULT_MONITOR_INTERVAL_MS,
    }) {
        if (!operatorDataService) throw new Error('operatorDataService is required');
        if (!sseService) throw new Error('sseService is required');
        this._operatorDataService = operatorDataService;
        this._sseService = sseService;
        this._thresholdMs = thresholdSeconds * 1000;
        this._intervalMs = intervalMs;
        this._timer = null;
        this._ticking = false;
    }

    start() {
        if (this._timer) return;
        this._timer = setInterval(() => {
            this.tick().catch(err => {
                logger.error('[HEARTBEAT-MONITOR] Tick failed', { error: err.message, stack: err.stack });
            });
        }, this._intervalMs);
        if (typeof this._timer.unref === 'function') this._timer.unref();
        logger.info('[HEARTBEAT-MONITOR] Started', {
            thresholdMs: this._thresholdMs,
            intervalMs: this._intervalMs,
        });
    }

    stop() {
        if (this._timer) {
            clearInterval(this._timer);
            this._timer = null;
            logger.info('[HEARTBEAT-MONITOR] Stopped');
        }
    }

    /**
     * Run a single reconciliation pass. Safe to call manually (e.g. in tests);
     * concurrent invocations are coalesced.
     */
    async tick() {
        if (this._ticking) return;
        this._ticking = true;
        try {
            const operators = await this._operatorDataService.queryOperators([]);
            const nowMs = now().getTime();

            let transitions = 0;
            for (const op of operators) {
                if (!MONITORED_STATUSES.has(op.status)) continue;
                if (!op.last_heartbeat) continue;

                const lastHeartbeatMs = op.last_heartbeat instanceof Date
                    ? op.last_heartbeat.getTime()
                    : new Date(op.last_heartbeat).getTime();
                if (!Number.isFinite(lastHeartbeatMs)) continue;

                const age = nowMs - lastHeartbeatMs;
                const isStale = age > this._thresholdMs;
                const target = resolveHeartbeatTransition(op.status, isStale);
                if (!target) continue;

                const applied = await this._applyTransition(op, target, age);
                if (applied) transitions += 1;
            }

            if (transitions > 0) {
                logger.info('[HEARTBEAT-MONITOR] Reconciliation complete', { transitions });
            }
        } finally {
            this._ticking = false;
        }
    }

    async _applyTransition(operator, targetStatus, ageMs) {
        const operatorId = operator.operator_id;
        const fromStatus = operator.status;
        try {
            const result = await this._operatorDataService.updateOperator(operatorId, {
                status: targetStatus,
                updated_at: now(),
            });
            if (!result || result.success === false) {
                logger.warn('[HEARTBEAT-MONITOR] Failed to persist status transition', {
                    operator_id: operatorId,
                    from: fromStatus,
                    to: targetStatus,
                    error: result?.error,
                });
                return false;
            }

            logger.info('[HEARTBEAT-MONITOR] Operator status transitioned', {
                operator_id: operatorId,
                user_id: operator.user_id,
                from: fromStatus,
                to: targetStatus,
                heartbeat_age_ms: Math.round(ageMs),
            });

            await this._publishTransition(operator, targetStatus);
            return true;
        } catch (error) {
            logger.error('[HEARTBEAT-MONITOR] Transition failed', {
                operator_id: operatorId,
                from: fromStatus,
                to: targetStatus,
                error: error.message,
            });
            return false;
        }
    }

    async _publishTransition(operator, targetStatus) {
        if (!operator.user_id) return;
        try {
            const event = OperatorStatusUpdatedEvent.parse({
                type: operatorStatusToEventType(targetStatus),
                data: OperatorStatusUpdatedData.parse({
                    operator_id: operator.operator_id,
                    status: targetStatus,
                    hostname: operator.system_info?.hostname ?? null,
                    system_fingerprint: operator.system_fingerprint ?? null,
                    timestamp: now(),
                }),
                timestamp: now(),
            });
            await this._sseService.publishToUser(operator.user_id, event);
        } catch (error) {
            logger.warn('[HEARTBEAT-MONITOR] SSE fan-out failed (non-blocking)', {
                operator_id: operator.operator_id,
                target: targetStatus,
                error: error.message,
            });
        }
    }
}

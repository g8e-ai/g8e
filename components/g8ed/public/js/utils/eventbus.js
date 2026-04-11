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
 * EventBus - Simple event emitter for component communication
 * 
 * Usage:
 * const bus = new EventBus();
 * bus.on('event-name', (data) => console.log(data));
 * bus.emit('event-name', { some: 'data' });
 */
import { devLogger } from './dev-logger.js';
import { MAX_EVENTBUS_LISTENERS } from '../constants/service-client-constants.js';

export class EventBus {
    constructor() {
        this.events = new Map();
    }

    /**
     * Subscribe to an event
     * @param {string} event - Event name
     * @param {Function} handler - Event handler function
     * @returns {Function} Unsubscribe function
     */
    on(event, handler) {
        if (!this.events.has(event)) {
            this.events.set(event, new Set());
        }

        this.events.get(event).add(handler);

        return () => this.off(event, handler);
    }

    /**
     * Subscribe to an event once
     * @param {string} event - Event name
     * @param {Function} handler - Event handler function
     */
    once(event, handler) {
        const wrapper = (...args) => {
            handler(...args);
            this.off(event, wrapper);
        };

        this.on(event, wrapper);
    }

    /**
     * Unsubscribe from an event
     * @param {string} event - Event name
     * @param {Function} handler - Event handler function
     */
    off(event, handler) {
        if (!this.events.has(event)) return;

        const handlers = this.events.get(event);
        handlers.delete(handler);

        if (handlers.size === 0) {
            this.events.delete(event);
        }
    }

    /**
     * Emit an event
     * @param {string} event - Event name
     * @param {...any} args - Arguments to pass to handlers
     */
    emit(event, ...args) {
        if (!this.events.has(event)) {
            return;
        }

        const handlers = this.events.get(event);

        if (handlers.size >= MAX_EVENTBUS_LISTENERS) {
            devLogger.warn(`[EventBus] ${event} has ${handlers.size} listeners registered - potential duplicate registration issue`);
        }

        handlers.forEach(handler => {
            try {
                handler(...args);
            } catch (error) {
                devLogger.error(`Error in event handler for "${event}":`, error);
            }
        });
    }

    /**
     * Remove all event listeners
     */
    clear() {
        this.events.clear();
    }

    /**
     * Get the number of listeners for an event
     * @param {string} event - Event name
     * @returns {number} Number of listeners
     */
    listenerCount(event) {
        return this.events.has(event) ? this.events.get(event).size : 0;
    }
} 
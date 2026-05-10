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
 * ScrollDelegation - Improves chat UX by delegating scroll events to the main content area
 * 
 * This utility ensures that scrolling anywhere in the application (header, sidebars, etc.)
 * will scroll the main content wrapper, UNLESS the user is hovering over elements
 * that have their own scrolling behavior (thinking panels, terminal output, chat input).
 * 
 * Usage:
 *   const scrollDelegation = new ScrollDelegation(contentWrapper);
 *   scrollDelegation.enable();
 *   // Later: scrollDelegation.disable();
 */

import { WheelDelta } from '../constants/ui-constants.js';

export class ScrollDelegation {
    constructor(targetScrollContainer) {
        this.targetScrollContainer = targetScrollContainer;
        this.boundWheelHandler = null;
        this.enabled = false;
        
        this.scrollExceptionSelectors = [
            '.enhanced-textarea',                          // Chat input
            '.anchored-terminal__input-area',              // Anchored terminal input area
            '.thinking-message.thinking-expanded .message-content',  // Expanded thinking panel content
            '.terminal-output-container',                  // Terminal output
            '.operator-heartbeat-details pre',             // Heartbeat details
            '.modal-body',                                 // Modal dialogs
            '.command-result-output',                      // Command results in demo
        ];
    }

    enable() {
        if (this.enabled) return;
        
        this.boundWheelHandler = this.handleWheel.bind(this);
        
        // passive: true allows Chrome to use composited (off-main-thread) scrolling.
        // Non-passive wheel listeners on document are a primary cause of scroll jank
        // and tab hangs in Chrome on macOS with high-DPI displays.
        document.addEventListener('wheel', this.boundWheelHandler, { passive: true });
        
        this.enabled = true;
    }

    disable() {
        if (!this.enabled) return;
        
        if (this.boundWheelHandler) {
            document.removeEventListener('wheel', this.boundWheelHandler, { passive: true });
        }
        
        this.enabled = false;
    }

    /**
     * Handle wheel events
     * With passive:true we cannot call preventDefault, so we only handle
     * the case where the user is outside the scroll container and we want
     * to delegate scroll to it. We do this without blocking the browser's
     * own composited scroll path.
     */
    handleWheel(event) {
        if (!this.targetScrollContainer) return;

        const target = event.target;

        if (this.targetScrollContainer.contains(target) || this.isOverScrollException(target)) {
            return;
        }

        const delta = this.normalizeWheelDelta(event);
        this.targetScrollContainer.scrollTop += delta;
    }

    /**
     * Check if the target element or any of its parents match scroll exception selectors
     */
    isOverScrollException(element) {
        for (const selector of this.scrollExceptionSelectors) {
            if (element.matches && element.matches(selector)) {
                return true;
            }
            if (element.closest && element.closest(selector)) {
                return true;
            }
        }
        
        return false;
    }

    /**
     * Normalize wheel delta across browsers
     */
    normalizeWheelDelta(event) {
        let delta = event.deltaY;
        if (event.deltaMode === 1) {
            delta *= WheelDelta.LINE_HEIGHT_PX;
        } else if (event.deltaMode === 2) {
            delta *= this.targetScrollContainer.clientHeight;
        }
        
        return delta;
    }

    updateTarget(newTargetScrollContainer) {
        this.targetScrollContainer = newTargetScrollContainer;
    }

    addExceptionSelector(selector) {
        if (!this.scrollExceptionSelectors.includes(selector)) {
            this.scrollExceptionSelectors.push(selector);
        }
    }

    removeExceptionSelector(selector) {
        const index = this.scrollExceptionSelectors.indexOf(selector);
        if (index > -1) {
            this.scrollExceptionSelectors.splice(index, 1);
        }
    }
}

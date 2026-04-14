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
 * Creates a debounced function that delays invoking the provided function
 * until after the specified wait time has elapsed since the last time it was invoked.
 * 
 * @param {Function} func - The function to debounce
 * @param {number} wait - The number of milliseconds to delay
 * @param {Object} options - Configuration options
 * @param {boolean} options.immediate - Whether to invoke the function on the leading edge
 * @returns {Function} - The debounced function
 */
export function debounce(func, wait, options = {}) {
    let timeoutId;
    let lastCallTime;
    let lastInvokeTime = 0;
    let lastArgs;
    let lastThis;
    const { immediate = false } = options;

    function invokeFunc(time) {
        const args = lastArgs;
        const thisArg = lastThis;
        
        lastArgs = lastThis = undefined;
        lastInvokeTime = time;
        return func.apply(thisArg, args);
    }

    function leadingEdge(time) {
        lastInvokeTime = time;
        timeoutId = setTimeout(timerExpired, wait);
        return immediate ? invokeFunc(time) : undefined;
    }

    function remainingWait(time) {
        const timeSinceLastCall = time - lastCallTime;
        const timeSinceLastInvoke = time - lastInvokeTime;
        const timeWaiting = wait - timeSinceLastCall;
        return timeWaiting > timeSinceLastInvoke ? timeWaiting : timeSinceLastInvoke;
    }

    function shouldInvoke(time) {
        const timeSinceLastCall = time - lastCallTime;
        const timeSinceLastInvoke = time - lastInvokeTime;
        return lastCallTime === undefined || 
               timeSinceLastCall >= wait || 
               timeSinceLastCall < 0 || 
               timeSinceLastInvoke >= wait;
    }

    function timerExpired() {
        const time = Date.now();
        if (shouldInvoke(time)) {
            return trailingEdge(time);
        }
        timeoutId = setTimeout(timerExpired, remainingWait(time));
    }

    function trailingEdge(time) {
        timeoutId = undefined;
        if (lastArgs) {
            return invokeFunc(time);
        }
        lastArgs = lastThis = undefined;
        return undefined;
    }

    function debounced(...args) {
        const time = Date.now();
        const isInvoking = shouldInvoke(time);
        
        lastArgs = args;
        lastThis = this;
        lastCallTime = time;

        if (isInvoking) {
            if (timeoutId === undefined) {
                return leadingEdge(lastCallTime);
            }
            if (immediate) {
                timeoutId = setTimeout(timerExpired, wait);
                return invokeFunc(lastCallTime);
            }
        }

        if (timeoutId === undefined) {
            timeoutId = setTimeout(timerExpired, wait);
        }
        return undefined;
    }

    debounced.cancel = function() {
        if (timeoutId !== undefined) {
            clearTimeout(timeoutId);
        }
        lastInvokeTime = 0;
        lastArgs = lastCallTime = lastThis = timeoutId = undefined;
    };

    debounced.flush = function() {
        if (timeoutId === undefined) {
            return undefined;
        }
        const time = Date.now();
        const result = trailingEdge(time);
        debounced.cancel();
        return result;
    };

    return debounced;
}

/**
 * Creates a throttled function that only invokes the provided function
 * at most once per specified wait period.
 * 
 * @param {Function} func - The function to throttle
 * @param {number} wait - The number of milliseconds to throttle invocations to
 * @param {Object} options - Configuration options
 * @param {boolean} options.leading - Whether to invoke on the leading edge
 * @param {boolean} options.trailing - Whether to invoke on the trailing edge
 * @returns {Function} - The throttled function
 */
export function throttle(func, wait, options = {}) {
    let timeoutId;
    let lastArgs;
    let lastThis;
    let lastInvokeTime = 0;
    const { leading = true, trailing = true } = options;

    function invokeFunc(time) {
        const args = lastArgs;
        const thisArg = lastThis;
        
        lastArgs = lastThis = undefined;
        lastInvokeTime = time;
        return func.apply(thisArg, args);
    }

    function shouldInvoke(time) {
        const timeSinceLastInvoke = time - lastInvokeTime;
        return lastInvokeTime === 0 || timeSinceLastInvoke >= wait;
    }

    function trailingEdge(time) {
        timeoutId = undefined;
        if (trailing && lastArgs) {
            return invokeFunc(time);
        }
        lastArgs = lastThis = undefined;
        return undefined;
    }

    function timerExpired() {
        const time = Date.now();
        if (shouldInvoke(time)) {
            return trailingEdge(time);
        }
        timeoutId = setTimeout(timerExpired, wait - (time - lastInvokeTime));
    }

    function throttled(...args) {
        const time = Date.now();
        const isInvoking = shouldInvoke(time);
        
        lastArgs = args;
        lastThis = this;

        if (isInvoking) {
            if (timeoutId === undefined) {
                return leading ? invokeFunc(time) : trailingEdge(time);
            }
            if (leading === false) {
                timeoutId = setTimeout(timerExpired, wait);
                return invokeFunc(time);
            }
        }

        if (timeoutId === undefined && trailing !== false) {
            timeoutId = setTimeout(timerExpired, wait);
        }
        return undefined;
    }

    throttled.cancel = function() {
        if (timeoutId !== undefined) {
            clearTimeout(timeoutId);
        }
        lastInvokeTime = 0;
        lastArgs = lastThis = timeoutId = undefined;
    };

    throttled.flush = function() {
        if (timeoutId === undefined) {
            return undefined;
        }
        const time = Date.now();
        const result = trailingEdge(time);
        throttled.cancel();
        return result;
    };

    return throttled;
}

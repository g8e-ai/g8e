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

export const SSEClientConfig = Object.freeze({
    MAX_RECONNECT_ATTEMPTS:      10,
    BASE_RECONNECT_DELAY_MS:     1000,
    MAX_RECONNECT_DELAY_MS:      30000,
    MIN_RECONNECT_DELAY_MS:      1000,
    KEEPALIVE_TIMEOUT_MS:        120000,
    QUICK_FAILURE_THRESHOLD_MS:  5000,
    QUICK_FAILURE_BACKOFF_COUNT: 3,
    RECONNECT_FAILURE_REASON:    'max_attempts_exceeded',
});

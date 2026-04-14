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

export const UserRole = Object.freeze({
    USER:       'user',
    ADMIN:      'admin',
    SUPERADMIN: 'superadmin',
});

export const OperatorSessionRole = Object.freeze({
    OPERATOR: 'operator',
});

export const AuthProvider = Object.freeze({
    LOCAL: 'local',
});

export const DeviceLinkStatus = Object.freeze({
    ACTIVE:    'active',
    EXHAUSTED: 'exhausted',
    EXPIRED:   'expired',
    REVOKED:   'revoked',
});

// ---------------------------------------------------------------------------
// Device Link Defaults
// ---------------------------------------------------------------------------
export const DEFAULT_DEVICE_LINK_MAX_USES = 1;
export const DEVICE_LINK_MAX_USES_MIN = 1;
export const DEVICE_LINK_MAX_USES_MAX = 10000;

export const IntentStatus = Object.freeze({
    GRANTED: 'granted',
    FAILED:  'failed',
});

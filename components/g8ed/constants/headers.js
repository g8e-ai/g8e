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

import { _HEADERS } from './shared.js';

/**
 * G8eHeaders - X-G8E-* HTTP header name constants.
 * Canonical values loaded from shared/constants/headers.json.
 * That file is the single source of truth shared across g8ee, g8eo, and g8ed.
 *
 * All internal cluster-to-cluster HTTP requests use these header names to
 * propagate G8eHttpContext (session, identity, business context) between
 * components without re-authentication.
 *
 * @type {{ [key: string]: string }}
 */
export const G8eHeaders = Object.freeze({
    WEB_SESSION_ID:    _HEADERS['x-g8e.session-id'],
    USER_ID:           _HEADERS['x-g8e.user-id'],
    ORGANIZATION_ID:   _HEADERS['x-g8e.organization-id'],
    CASE_ID:           _HEADERS['x-g8e.case-id'],
    INVESTIGATION_ID:  _HEADERS['x-g8e.investigation-id'],
    TASK_ID:           _HEADERS['x-g8e.task-id'],
    SOURCE_COMPONENT:  _HEADERS['x-g8e.source-component'],
    BOUND_OPERATORS:   _HEADERS['x-g8e.bound-operators'],
    EXECUTION_ID:      _HEADERS['x-g8e.execution-id'],
    NEW_CASE:          _HEADERS['x-g8e.new-case'],
    SERVICE:           _HEADERS['x-g8e.service'],
    CLIENT:            _HEADERS['x-g8e.client'],
    OPERATOR_STATUS:   _HEADERS['x-g8e.operator-status'],
});

export const HTTP_REQUESTED_WITH_HEADER        = _HEADERS['http.requested-with'];
export const HTTP_G8E_SERVICE_HEADER           = _HEADERS['x-g8e.service'];
export const HTTP_G8E_CLIENT_HEADER            = _HEADERS['x-g8e.client'];
export const HTTP_G8E_OPERATOR_STATUS_HEADER   = _HEADERS['x-g8e.operator-status'];
export const HTTP_CACHE_CONTROL_HEADER         = _HEADERS['http.cache-control'];
export const HTTP_PRAGMA_HEADER                = _HEADERS['http.pragma'];
export const HTTP_COOKIE_HEADER                = _HEADERS['http.cookie'];
export const HTTP_SET_COOKIE_HEADER            = _HEADERS['http.set-cookie'];
export const HTTP_LAST_EVENT_ID_HEADER         = _HEADERS['http.last-event-id'];
export const HTTP_ACCESS_CONTROL_REQUEST_HEADERS = _HEADERS['http.access-control-req-headers'];
export const HTTP_ACCESS_CONTROL_REQUEST_METHOD  = _HEADERS['http.access-control-req-method'];
export const HTTP_ACCESS_CONTROL_ALLOW_ORIGIN    = _HEADERS['http.access-control-allow-origin'];
export const HTTP_ACCESS_CONTROL_ALLOW_CREDENTIALS = _HEADERS['http.access-control-allow-creds'];
export const HTTP_CONTENT_TYPE_HEADER          = _HEADERS['http.content-type'];
export const HTTP_API_KEY_HEADER               = _HEADERS['http.api-key'];
export const WEB_SESSION_ID_HEADER             = _HEADERS['x-g8e.session-id'];
export const HTTP_INTERNAL_AUTH_HEADER          = _HEADERS['http.x-internal-auth'];
export const HTTP_X_FORWARDED_HOST_HEADER      = _HEADERS['http.x-forwarded-host'];
export const HTTP_X_FORWARDED_PROTO_HEADER     = _HEADERS['http.x-forwarded-proto'];

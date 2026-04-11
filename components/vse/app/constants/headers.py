# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from enum import Enum

HTTP_ACCEL_BUFFERING_HEADER = "X-Accel-Buffering"
HTTP_ACCEPT_HEADER = "Accept"
HTTP_ACCEPT_LANGUAGE_HEADER = "Accept-Language"
HTTP_ACCESS_CONTROL_ALLOW_CREDENTIALS = "Access-Control-Allow-Credentials"
HTTP_ACCESS_CONTROL_ALLOW_ORIGIN = "Access-Control-Allow-Origin"
HTTP_ACCESS_CONTROL_REQUEST_HEADERS = "Access-Control-Request-Headers"
HTTP_ACCESS_CONTROL_REQUEST_METHOD = "Access-Control-Request-Method"
HTTP_API_KEY_HEADER = "X-API-Key"
HTTP_AUTHORIZATION_HEADER = "Authorization"
HTTP_BEARER_PREFIX = "Bearer"
HTTP_CACHE_CONTROL_HEADER = "Cache-Control"
HTTP_CONTENT_LANGUAGE_HEADER = "Content-Language"
HTTP_CONTENT_TYPE_HEADER = "Content-Type"
HTTP_COOKIE_HEADER = "Cookie"
HTTP_FORWARDED_FOR_HEADER = "X-Forwarded-For"
HTTP_LAST_EVENT_ID_HEADER = "Last-Event-ID"
HTTP_PRAGMA_HEADER = "Pragma"
HTTP_REQUESTED_WITH_HEADER = "X-Requested-With"
HTTP_SET_COOKIE_HEADER = "Set-Cookie"
HTTP_USER_AGENT_HEADER = "User-Agent"
HTTP_VSO_CLIENT_HEADER = "X-VSO-Client"
HTTP_VSO_OPERATOR_STATUS_HEADER = "X-VSO-Operator-Status"
HTTP_VSO_SERVICE_HEADER = "X-VSO-Service"
INTERNAL_AUTH_HEADER = "X-Internal-Auth"
PROXY_ORGANIZATION_ID_HEADER = "X-Proxy-Organization-Id"
PROXY_USER_EMAIL_HEADER = "X-Proxy-User-Email"
PROXY_USER_ID_HEADER = "X-Proxy-User-Id"
WEB_SESSION_ID_HEADER = "X-VSO-WebSession-ID"

SESSION_ID_HEADER = "X-Session-Id"
CASE_ID_HEADER = "X-VSO-Case-ID"
USER_ID_HEADER = "X-VSO-User-ID"
ORGANIZATION_ID_HEADER = "X-VSO-Organization-ID"
INVESTIGATION_ID_HEADER = "X-VSO-Investigation-ID"
TASK_ID_HEADER = "X-VSO-Task-ID"
NEW_CASE_HEADER = "X-VSO-New-Case"
BOUND_OPERATORS_HEADER = "X-VSO-Bound-Operators"
OPERATOR_ID_HEADER = "X-VSO-User-ID"
OPERATOR_SESSION_ID_HEADER = "X-VSO-WebSession-ID"
EXECUTION_ID_HEADER = "X-VSO-Execution-ID"
COMPONENT_NAME_HEADER = "X-VSO-Source-Component"

class VSOHeaders(str, Enum):
    WEB_SESSION_ID = WEB_SESSION_ID_HEADER
    USER_ID = USER_ID_HEADER
    ORGANIZATION_ID = ORGANIZATION_ID_HEADER
    CASE_ID = CASE_ID_HEADER
    INVESTIGATION_ID = INVESTIGATION_ID_HEADER
    TASK_ID = TASK_ID_HEADER
    SOURCE_COMPONENT = COMPONENT_NAME_HEADER
    BOUND_OPERATORS = BOUND_OPERATORS_HEADER
    EXECUTION_ID = EXECUTION_ID_HEADER
    NEW_CASE = NEW_CASE_HEADER
    SERVICE = HTTP_VSO_SERVICE_HEADER
    CLIENT = HTTP_VSO_CLIENT_HEADER
    OPERATOR_STATUS = HTTP_VSO_OPERATOR_STATUS_HEADER

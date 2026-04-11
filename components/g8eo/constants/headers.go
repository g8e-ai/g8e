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

package constants

// HTTP header name constants used across g8eo.
// Canonical values mirror shared/constants/headers.json.
const (
	HeaderContentType        = "Content-Type"
	HeaderContentDisposition = "Content-Disposition"
	HeaderContentLength      = "Content-Length"
	HeaderAuthorization      = "Authorization"
	HeaderUserAgent          = "User-Agent"
	HeaderXRequestTimestamp  = "X-Request-Timestamp"
	HeaderXForwardedProto    = "X-Forwarded-Proto"
	HeaderXForwardedHost     = "X-Forwarded-Host"
	HeaderInternalAuth       = "X-Internal-Auth"

	HeaderG8eWebSessionID    = "X-G8E-WebSession-ID"
	HeaderG8eUserID          = "X-G8E-User-ID"
	HeaderG8eOrganizationID  = "X-G8E-Organization-ID"
	HeaderG8eCaseID          = "X-G8E-Case-ID"
	HeaderG8eInvestigationID = "X-G8E-Investigation-ID"
	HeaderG8eTaskID          = "X-G8E-Task-ID"
	HeaderG8eSourceComponent = "X-G8E-Source-Component"
	HeaderG8eBoundOperators  = "X-G8E-Bound-Operators"
	HeaderG8eRequestID       = "X-G8E-Request-ID"
	HeaderG8eService         = "X-G8E-Service"
	HeaderG8eClient          = "X-G8E-Client"
	HeaderG8eOperatorStatus  = "X-G8E-Operator-Status"
)

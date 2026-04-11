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

// HTTP header name constants used across VSA.
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

	HeaderVSOWebSessionID    = "X-VSO-WebSession-ID"
	HeaderVSOUserID          = "X-VSO-User-ID"
	HeaderVSOOrganizationID  = "X-VSO-Organization-ID"
	HeaderVSOCaseID          = "X-VSO-Case-ID"
	HeaderVSOInvestigationID = "X-VSO-Investigation-ID"
	HeaderVSOTaskID          = "X-VSO-Task-ID"
	HeaderVSOSourceComponent = "X-VSO-Source-Component"
	HeaderVSOBoundOperators  = "X-VSO-Bound-Operators"
	HeaderVSORequestID       = "X-VSO-Request-ID"
	HeaderVSOService         = "X-VSO-Service"
	HeaderVSOClient          = "X-VSO-Client"
	HeaderVSOOperatorStatus  = "X-VSO-Operator-Status"
)

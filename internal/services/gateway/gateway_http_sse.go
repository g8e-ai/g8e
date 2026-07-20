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

package gateway

// SSE Event Bridge
//
// POST /api/v1/sse/push     → Producer (g8e-compatible agentic ensemble) appends an event.
//                            Body MUST set exactly one of
//                            web_session_id, cli_session_id, user_id.
// GET  /api/v1/sse/events   → Consumer (CLI / dashboard) polls events.
//                            Query string MUST set exactly one of
//                            web_session_id, cli_session_id, user_id,
//                            plus since_id=N and limit=K.
// GET  /api/v1/sse/stream   → Consumer streams events via SSE.
//                            Auth: dual — mTLS for CLI/operator, web session
//                            cookie for browser. The unified middleware stamps
//                            context with the appropriate identity.
//
// The Gateway refuses to talk about a bare session id - every routing
// target is tagged at the type level so a web_session_id can never be
// mis-delivered as a cli_session_id (or vice versa).
//
// All SSE handlers live on SSEController (sse_controller.go).

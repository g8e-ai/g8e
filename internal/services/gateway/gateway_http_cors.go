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

import (
	"net/http"
	"strings"

	"github.com/g8e-ai/g8e/internal/constants"
)

// corsMiddleware returns an http.Handler that applies CORS headers based on the
// configured AllowedOrigins. When AllowedOrigins is empty, the middleware is a
// pass-through (same-origin only). When non-empty, it:
//
//   - Reflects the request Origin if it matches an allowed origin, setting
//     Access-Control-Allow-Origin and Access-Control-Allow-Credentials: true.
//   - Handles OPTIONS preflight requests by responding with 204 No Content
//     and the appropriate Access-Control-Allow-* headers.
//   - Adds Vary: Origin to all responses so caches respect per-origin differences.
func (h *HTTPHandler) corsMiddleware(next http.Handler) http.Handler {
	allowed := h.cfg.Gateway.AllowedOrigins
	if len(allowed) == 0 {
		return next
	}

	allowedSet := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		allowedSet[strings.ToLower(strings.TrimRight(o, "/"))] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		originAllowed := origin != "" && allowedSet[strings.ToLower(strings.TrimRight(origin, "/"))]

		// Vary: Origin must always be set when the middleware is active so
		// caches do not serve a matching-origin response to a non-matching origin.
		w.Header().Add(constants.HeaderVary, "Origin")

		if originAllowed {
			w.Header().Set(constants.HeaderAccessControlAllowOrigin, origin)
			w.Header().Set(constants.HeaderAccessControlAllowCredentials, "true")
		}

		// Handle OPTIONS preflight only for allowed origins
		if r.Method == http.MethodOptions && originAllowed {
			w.Header().Set(constants.HeaderAccessControlAllowMethods, "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set(constants.HeaderAccessControlAllowHeaders, "Content-Type, Authorization, X-G8E-Web-Session-ID, X-G8E-CLI-Session-ID, X-G8E-Operator-Session-ID, X-G8E-User-ID, X-G8E-Operator-ID, X-G8E-Request-ID, X-Requested-With")
			w.Header().Set(constants.HeaderAccessControlMaxAge, constants.HeaderValueCORSPreflightMaxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

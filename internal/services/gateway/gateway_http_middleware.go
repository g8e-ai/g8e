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
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

func (h *HTTPHandler) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.Gateway.RateLimitRPS <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		h.muLimiters.Lock()
		limiter, ok := h.limiters[ip]
		if !ok {
			limiter = newTokenBucket(h.cfg.Gateway.RateLimitRPS, h.cfg.Gateway.RateLimitBurst)
			h.limiters[ip] = limiter
		}
		h.limiterLastUsed[ip] = time.Now()

		// Clean up stale limiters (older than 5 minutes)
		cutoff := time.Now().Add(-5 * time.Minute)
		for key, lastUsed := range h.limiterLastUsed {
			if lastUsed.Before(cutoff) {
				delete(h.limiters, key)
				delete(h.limiterLastUsed, key)
			}
		}
		h.muLimiters.Unlock()

		if !limiter.Allow() {
			h.logger.Warn("Rate limit exceeded", "ip", ip, "path", r.URL.Path)
			h.responder.Error(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// corsMiddlewareForCLIPasskey is a more permissive CORS middleware that allows
// local network IPs to support port forwarding scenarios for CLI passkey bootstrap.
func (h *HTTPHandler) corsMiddlewareForCLIPasskey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// Security: validate origin is loopback or local network IP for CLI bootstrap
			// This allows port forwarding scenarios while still preventing external CSRF attacks
			if !isLocalNetworkOrigin(origin) {
				h.logger.Warn("CORS request rejected: non-local network Origin", "origin", origin, "path", r.URL.Path)
				h.responder.Error(w, http.StatusForbidden, "origin not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pathTraversalGuard rejects any request whose raw URL path contains a ".."
// segment before Go's ServeMux can normalize the path and issue a 301 redirect.
func (h *HTTPHandler) pathTraversalGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path to handle multiple slashes, etc.
		cleaned := filepath.ToSlash(filepath.Clean(r.URL.Path))
		if h.containsTraversal(r.URL.Path) || (cleaned != r.URL.Path && cleaned != r.URL.Path+"/" && r.URL.Path != "/") {
			if h.containsTraversal(r.URL.Path) || strings.Contains(cleaned, "..") {
				h.responder.Error(w, http.StatusBadRequest, "invalid path")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (h *HTTPHandler) containsTraversal(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// isLocalNetworkOrigin reports whether an Origin header value refers to a
// loopback host or a local network IP (same subnet as the gateway).
// This is used for CLI passkey bootstrap to support port forwarding scenarios.
func isLocalNetworkOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()

	// Allow loopback addresses
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return true
		}
		// Allow private network IPs (RFC 1918)
		if isPrivateIP(ip) {
			return true
		}
	}
	return false
}

// isPrivateIP reports whether an IP address is in a private network range.
func isPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}
	return false
}

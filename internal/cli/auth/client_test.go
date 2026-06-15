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

package auth

import (
	"fmt"
	"strings"
)

// extractPortFromURL extracts the port number from a httptest server URL
func extractPortFromURL(url string) int {
	// httptest URLs are like "http://127.0.0.1:12345"
	// Split by "://" first to get the host:port part
	parts := strings.Split(url, "://")
	if len(parts) < 2 {
		return 0
	}
	// Then split by ":" to get the port
	hostPort := parts[1]
	portParts := strings.Split(hostPort, ":")
	if len(portParts) < 2 {
		return 0
	}
	var port int
	fmt.Sscanf(portParts[1], "%d", &port)
	return port
}

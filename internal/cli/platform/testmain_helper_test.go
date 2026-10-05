// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package platform

import (
	"strconv"
)

// parseHTTPPortFromArgs extracts the integer value following --http-port from
// the argument slice. Returns 0 if not found or invalid.
func parseHTTPPortFromArgs(args []string) int {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--http-port" {
			port, err := strconv.Atoi(args[i+1])
			if err != nil {
				return 0
			}
			return port
		}
	}
	return 0
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package network

import (
	"fmt"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// LocalhostHTTPSURL returns a localhost HTTPS URL with the specified port.
func LocalhostHTTPSURL(port int) string {
	return fmt.Sprintf("https://%s:%d", constants.DefaultEndpoint, port)
}

// LocalhostHTTPURL returns a localhost HTTP URL with the specified port.
func LocalhostHTTPURL(port int) string {
	return fmt.Sprintf("http://%s:%d", constants.DefaultEndpoint, port)
}

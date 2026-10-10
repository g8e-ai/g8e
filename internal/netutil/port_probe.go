// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package netutil

import (
	"fmt"
	"io"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// ReserveTCPPort holds an ephemeral TCP port until the returned reservation is
// closed. On Windows it binds without listening, avoiding firewall prompts.
func ReserveTCPPort() (int, io.Closer, error) {
	port, reservation, err := reserveTCPPort()
	if err != nil {
		return 0, nil, fmt.Errorf("%w: reserve TCP port: %w", constants.ErrPortUnavailable, err)
	}
	return port, reservation, nil
}

// CheckTCPPortAvailable probes the wildcard address used by the Gateway.
// The port is released before return, so the eventual server bind can still race.
func CheckTCPPortAvailable(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%w: invalid TCP port %d", constants.ErrPortUnavailable, port)
	}
	if err := checkTCPPortBind(port); err != nil {
		return fmt.Errorf("%w: TCP port %d: %w", constants.ErrPortUnavailable, port, err)
	}
	return nil
}

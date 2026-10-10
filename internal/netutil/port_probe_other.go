// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !windows

package netutil

import (
	"io"
	"net"
	"strconv"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func reserveTCPPort() (int, io.Closer, error) {
	listener, err := net.Listen(string(constants.NetworkProtocolTCP), net.JoinHostPort(constants.LocalhostIP, "0"))
	if err != nil {
		return 0, nil, err
	}
	return listener.Addr().(*net.TCPAddr).Port, listener, nil
}

func checkTCPPortBind(port int) error {
	listener, err := net.Listen(string(constants.NetworkProtocolTCP), net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return listener.Close()
}

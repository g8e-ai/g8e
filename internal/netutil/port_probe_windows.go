// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package netutil

import (
	"errors"
	"fmt"
	"io"

	"golang.org/x/sys/windows"
)

type tcpPortReservation struct {
	socket windows.Handle
}

func (r *tcpPortReservation) Close() error {
	return errors.Join(windows.Closesocket(r.socket), windows.WSACleanup())
}

func reserveTCPPort() (int, io.Closer, error) {
	var data windows.WSAData
	if err := windows.WSAStartup(0x202, &data); err != nil {
		return 0, nil, fmt.Errorf("initialize Winsock: %w", err)
	}
	socket, err := bindTCPPort(0)
	if err != nil {
		return 0, nil, errors.Join(err, windows.WSACleanup())
	}
	reservation := &tcpPortReservation{socket: socket}
	address, err := windows.Getsockname(socket)
	if err != nil {
		return 0, nil, fmt.Errorf("read reserved TCP address: %w", errors.Join(err, reservation.Close()))
	}
	switch address := address.(type) {
	case *windows.SockaddrInet4:
		return address.Port, reservation, nil
	case *windows.SockaddrInet6:
		return address.Port, reservation, nil
	default:
		return 0, nil, errors.Join(fmt.Errorf("unexpected reserved TCP address %T", address), reservation.Close())
	}
}

func checkTCPPortBind(port int) (err error) {
	var data windows.WSAData
	if err := windows.WSAStartup(0x202, &data); err != nil {
		return fmt.Errorf("initialize Winsock: %w", err)
	}
	defer func() { err = errors.Join(err, windows.WSACleanup()) }()

	socket, err := bindTCPPort(port)
	if err != nil {
		return err
	}
	return windows.Closesocket(socket)
}

// Binding without listen checks availability without registering an inbound
// server with Windows Firewall. Exclusive binding also detects conflicts with
// loopback listeners, even though the probe itself uses a wildcard address.
func bindTCPPort(port int) (windows.Handle, error) {
	family := windows.AF_INET6
	socket, err := windows.WSASocket(int32(family), windows.SOCK_STREAM, windows.IPPROTO_TCP, nil, 0, windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if errors.Is(err, windows.WSAEAFNOSUPPORT) || errors.Is(err, windows.WSAEPROTONOSUPPORT) {
		family = windows.AF_INET
		socket, err = windows.WSASocket(int32(family), windows.SOCK_STREAM, windows.IPPROTO_TCP, nil, 0, windows.WSA_FLAG_NO_HANDLE_INHERIT)
	}
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("create TCP probe socket: %w", err)
	}

	// Winsock defines SO_EXCLUSIVEADDRUSE as the complement of SO_REUSEADDR.
	err = windows.SetsockoptInt(socket, windows.SOL_SOCKET, ^windows.SO_REUSEADDR, 1)
	var address windows.Sockaddr = &windows.SockaddrInet4{Port: port}
	if err == nil && family == windows.AF_INET6 {
		err = windows.SetsockoptInt(socket, windows.IPPROTO_IPV6, windows.IPV6_V6ONLY, 0)
		address = &windows.SockaddrInet6{Port: port}
	}
	if err == nil {
		err = windows.Bind(socket, address)
	}
	if err != nil {
		return windows.InvalidHandle, fmt.Errorf("bind TCP probe: %w", errors.Join(err, windows.Closesocket(socket)))
	}
	return socket, nil
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows && integration

package netutil

import (
	"errors"
	"net"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/g8e-ai/g8e/v2/internal/constants"
)

func TestTCPPortProbe_BindsWithoutAcceptingConnectionsAndReleasesPort(t *testing.T) {
	var data windows.WSAData
	if err := windows.WSAStartup(0x202, &data); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := windows.WSACleanup(); err != nil {
			t.Error(err)
		}
	})
	socket, err := bindTCPPort(0)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			_ = windows.Closesocket(socket)
		}
	})
	// SO_ACCEPTCONN reports whether a socket has entered the listening state.
	accepting, err := windows.GetsockoptInt(socket, windows.SOL_SOCKET, 0x0002)
	if err != nil {
		t.Fatal(err)
	}
	if accepting != 0 {
		t.Fatal("a port probe must not listen or register an inbound firewall server")
	}
	address, err := windows.Getsockname(socket)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	switch address := address.(type) {
	case *windows.SockaddrInet4:
		port = address.Port
	case *windows.SockaddrInet6:
		port = address.Port
	default:
		t.Fatalf("unexpected socket address %T", address)
	}
	if err := CheckTCPPortAvailable(port); !errors.Is(err, constants.ErrPortUnavailable) {
		t.Fatalf("bound port must be unavailable, got %v", err)
	}
	if err := windows.Closesocket(socket); err != nil {
		t.Fatal(err)
	}
	closed = true
	for range 3 {
		if err := CheckTCPPortAvailable(port); err != nil {
			t.Fatalf("probe must release its port: %v", err)
		}
	}
}

func TestTCPPortProbe_RejectsIPv4AndIPv6LoopbackConflicts(t *testing.T) {
	for _, tc := range []struct {
		name, network, address string
	}{
		{"IPv4", "tcp4", "127.0.0.1:0"},
		{"IPv6", "tcp6", "[::1]:0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen(tc.network, tc.address)
			if tc.network == "tcp6" && (errors.Is(err, windows.WSAEAFNOSUPPORT) || errors.Is(err, windows.WSAEADDRNOTAVAIL)) {
				t.Skip("IPv6 loopback is unavailable")
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			port := listener.Addr().(*net.TCPAddr).Port
			if err := CheckTCPPortAvailable(port); !errors.Is(err, constants.ErrPortUnavailable) {
				t.Fatalf("wildcard probe must reject a loopback conflict, got %v", err)
			}
		})
	}
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package platform

import (
	"fmt"
	"net"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

func TestFindAvailablePortSkipsGatewayReservedPorts(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}
	port, err := pm.findAvailablePort(constants.PublicSpectatorPrivatePort, "test")
	if err != nil {
		t.Skipf("no free port near reserved range: %v", err)
	}
	if _, reserved := constants.GatewayReservedLoopbackPorts[port]; reserved {
		t.Fatalf("findAvailablePort returned gateway-reserved port %d", port)
	}
}

func TestFindAvailablePort(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	availablePort := addr.Port
	listener.Close()

	// Test available port
	port, err := pm.findAvailablePort(availablePort, "test")
	if err != nil {
		t.Errorf("port %d should be available: %v", availablePort, err)
	}
	if port != availablePort {
		t.Errorf("expected port %d, got %d", availablePort, port)
	}

	// Test port in use by untracked process
	listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", availablePort))
	if err != nil {
		t.Fatalf("failed to listen on port %d: %v", availablePort, err)
	}
	defer listener.Close()

	port, err = pm.findAvailablePort(availablePort, "test")
	if err != nil {
		t.Errorf("should find next available port: %v", err)
	}
	if port == availablePort {
		t.Error("should return different port when default is in use")
	}
}

func TestFindAvailablePortInvalidPort(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Test with a port that's out of valid range (should still work for the check)
	// The actual bind will fail, but the check itself should attempt it
	_, err = pm.findAvailablePort(70000, "test")
	if err == nil {
		t.Error("expected error for invalid port 70000")
	}
}

func TestCheckPortAvailable(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	fileSvc := newPlatformTestFileSvc(t, tmpDir)
	pm, err := NewProcessManager(fileSvc)
	if err != nil {
		t.Fatalf("NewProcessManager failed: %v", err)
	}

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)
	availablePort := addr.Port
	listener.Close()

	// Test available port
	if err := pm.checkPortAvailable(availablePort, "test"); err != nil {
		t.Errorf("port %d should be available: %v", availablePort, err)
	}

	// Test port in use
	listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", availablePort))
	if err != nil {
		t.Fatalf("failed to listen on port %d: %v", availablePort, err)
	}
	defer listener.Close()

	err = pm.checkPortAvailable(availablePort, "test")
	if err == nil {
		t.Error("expected error for port in use")
	}
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package auth

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestCheckOperatorRunningAtURL(t *testing.T) {
	t.Run("invalid URL format", func(t *testing.T) {
		err := CheckOperatorRunningAtURL("invalid-url")
		if err == nil {
			t.Error("CheckOperatorRunningAtURL() should return error for invalid URL")
		}
		if err == nil {
			t.Error("expected an error but got nil")
		}
	})

	t.Run("localhost to IPv4 conversion", func(t *testing.T) {
		// Start a test listener
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to create test listener: %v", err)
		}
		defer listener.Close()

		addr := listener.Addr().String()
		url := fmt.Sprintf("http://localhost:%s", strings.Split(addr, ":")[1])

		err = CheckOperatorRunningAtURL(url)
		if err != nil {
			t.Errorf("CheckOperatorRunningAtURL() failed for running server: %v", err)
		}
	})

	t.Run("server not running", func(t *testing.T) {
		// Use a port that's unlikely to be in use
		url := "http://127.0.0.1:59999"
		err := CheckOperatorRunningAtURL(url)
		if err == nil {
			t.Error("CheckOperatorRunningAtURL() should return error when server not running")
		}
		if err == nil {
			t.Error("expected an error but got nil")
		}
	})
}

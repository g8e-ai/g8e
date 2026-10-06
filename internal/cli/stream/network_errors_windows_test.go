// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package stream

import (
	"net"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestIsTransientError_WindowsSocketCodes(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"refused", windows.WSAECONNREFUSED, true},
		{"reset", windows.WSAECONNRESET, true},
		{"timeout", windows.WSAETIMEDOUT, true},
		{"network unreachable", windows.WSAENETUNREACH, true},
		{"host unreachable", windows.WSAEHOSTUNREACH, true},
		{"permission denied", windows.WSAEACCES, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connectex", Err: tc.err}}
			if got := isTransientError(err); got != tc.want {
				t.Fatalf("retry classification = %v, want %v for %v", got, tc.want, err)
			}
		})
	}
}

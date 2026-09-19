// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package platform

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestMain detects when the test binary is re-executed by StartOperator as a
// Gateway subprocess (via "gw start --follow ..."). In that case the first
// positional argument is "gw", which never happens during a normal test run.
//
// Two re-exec modes are supported, selected by the G8E_TEST_REEXEC environment
// variable (inherited by the child process):
//
//   - "exit" (default, or unset): the subprocess exits immediately. This lets
//     start-failure tests (PID-write failure, process death during health
//     check) exercise cleanup paths without a real Gateway binary.
//   - "serve": the subprocess parses --http-port from the re-exec args and
//     starts a minimal HTTP server that responds to /api/v1/health with 200.
//     This lets process-lifecycle tests exercise paths that require a
//     successful StartOperator (profile-write failure, restart rollback,
//     resolved-port persistence, stopped-gateway manual trust, IDNA RP
//     override).
//   - "hold": the subprocess stays alive without serving health. This lets
//     tests prove that a foreign health response on the selected port is
//     rejected even while the child process is still running.
//
// During normal test runs TestMain delegates to the default test runner.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "gw" {
		mode := os.Getenv("G8E_TEST_REEXEC")
		switch mode {
		case "serve":
			serveHealth()
		case "hold":
			time.Sleep(30 * time.Second)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// serveHealth parses --http-port from os.Args and starts a minimal HTTP server
// that responds to /api/v1/health with 200. The server runs until the process
// is killed by StopOperator.
func serveHealth() {
	port := parseHTTPPortFromArgs(os.Args)
	if port == 0 {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"status":"ok","mode":"gateway","posture":"doctrine","pid":%d}`, os.Getpid())
	})
	_ = http.ListenAndServe("127.0.0.1:"+strconv.Itoa(port), mux)
}

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

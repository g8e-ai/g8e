// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmdtest

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ReexecGatewayIfRequested exits when the test binary is re-executed by
// StartOperator as a Gateway subprocess (first arg "gw").
func ReexecGatewayIfRequested() {
	if len(os.Args) <= 1 || os.Args[1] != "gw" {
		return
	}
	if os.Getenv("G8E_TEST_REEXEC") == "serve" {
		serveHealth()
	}
	os.Exit(0)
}

func serveHealth() {
	port := parseHTTPPortFromArgs(os.Args)
	if port == 0 {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "{\"status\":\"ok\",\"mode\":\"gateway\",\"posture\":\"doctrine\",\"pid\":%d}", os.Getpid())
	})
	srv := &http.Server{
		Addr:              "127.0.0.1:" + strconv.Itoa(port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	_ = srv.ListenAndServe()
}

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

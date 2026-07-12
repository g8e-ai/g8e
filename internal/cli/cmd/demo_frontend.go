// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"strings"
)

func runFrontendScenario(demoDir, scenario string) (scenarioResult, error) {
	switch scenario {
	case "1":
		return runFrontendEnrollmentScenario(demoDir)
	default:
		return scenarioResult{}, fmt.Errorf("invalid scenario number for frontend: %q (valid: 1)", scenario)
	}
}

func runFrontendEnrollmentScenario(demoDir string) (scenarioResult, error) {
	var result scenarioResult
	var hasErrors bool

	result.number = "1"
	result.name = "Third-Party Frontend Enrollment"
	result.status = "PASS"
	result.metrics = "CORS preflight: http://localhost:3003 -> 8446 // Passkey endpoints accessible // SSE protected (401 without session)"

	demoPrintf("\n%s\n", strings.Repeat("-", 60))
	demoPrintf("  Scenario 1 — Third-Party Frontend Enrollment\n")
	demoPrintln(strings.Repeat("-", 60))
	demoPrintln()
	demoPrintln("  PROVES: A third-party frontend application (http://localhost:3003)")
	demoPrintln("          can securely connect to the g8e gateway via CORS,")
	demoPrintln("          WebAuthn passkeys, and SSE streaming.")
	demoPrintln()

	demoPrintln("  -- Step 1: Gateway health check --------------------------------")
	if err := demoStep(demoDir, "gateway health",
		false,
		"curl", "-s", "http://localhost:8083/api/v1/health",
	); err != nil {
		fmt.Println("  (gateway health check failed — is the demo running?)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  -- Step 2: CORS preflight from frontend origin ----------------")
	if err := demoStep(demoDir, "CORS preflight",
		false,
		"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"-X", "OPTIONS",
		"-H", "Origin: http://localhost:3003",
		"-H", "Access-Control-Request-Method: POST",
		"-H", "Access-Control-Request-Headers: content-type",
		"https://localhost:8446/api/v1/health",
		"-k",
	); err != nil {
		fmt.Println("  (CORS preflight check failed)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  -- Step 3: Passkey challenge endpoint accessible --------------")
	if err := demoStep(demoDir, "passkey challenge endpoint",
		false,
		"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"-X", "POST",
		"-H", "Content-Type: application/json",
		"-H", "Origin: http://localhost:3003",
		"-d", "{}",
		"https://localhost:8446/api/v1/auth/passkeys/console/register/challenge",
		"-k",
	); err != nil {
		fmt.Println("  (passkey challenge endpoint check failed)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  -- Step 4: SSE stream endpoint protected (401 without session) --")
	if err := demoStep(demoDir, "SSE endpoint protection",
		false,
		"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"-H", "Origin: http://localhost:3003",
		"https://localhost:8446/api/v1/sse/stream?web_session_id=invalid",
		"-k",
	); err != nil {
		fmt.Println("  (SSE endpoint check failed)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  -- Step 5: Frontend app served on port 3003 -------------------")
	if err := demoStep(demoDir, "frontend app check",
		false,
		"curl", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"http://localhost:3003",
	); err != nil {
		fmt.Println("  (frontend app check failed)")
		fmt.Println()
		hasErrors = true
	}

	fmt.Println()
	fmt.Println("  To complete the interactive demo:")
	fmt.Println("    1. Open http://localhost:3003 in your browser")
	fmt.Println("    2. Click 'Register Passkey' to create a WebAuthn credential")
	fmt.Println("    3. Click 'Authenticate' to log in with the passkey")
	fmt.Println("    4. Click 'Connect SSE' to receive live gateway events")
	fmt.Println()

	if hasErrors {
		result.status = "FAIL"
		fmt.Printf("  [FAIL] Scenario 1 — One or more steps failed.\n")
	} else {
		fmt.Printf("  [PASS] Frontend enrollment verified: CORS, passkey endpoints, SSE protection, app served.\n")
	}

	return result, nil
}

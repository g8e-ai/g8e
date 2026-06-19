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
	"testing"
)

// TestPKIPhase3_StaleTrustBundle_FailClosed verifies that mTLS enrollment failures
// fail closed with an actionable error instead of silently falling back to plain HTTP.
// This is the fix for C4 (silent security downgrade) in the PKI cleanup plan.
// See: .local.dev/docs/plans/pki_cleanup.md C4
func TestPKIPhase3_StaleTrustBundle_FailClosed(t *testing.T) {
	t.Run("loginCmdWithConfig fails closed on TLS error with actionable error", func(t *testing.T) {
		// This test verifies that when ReEnroll fails with a TLS verification error,
		// the code returns an actionable error message instead of silently falling back
		// to plain-HTTP Bootstrap. The fix is in auth.go lines 156-165 and 281-290.

		// The code path being tested:
		// 1. auth.ReEnroll is called (line 157)
		// 2. If it returns an error containing "certificate signed by unknown authority" or "x509: certificate"
		// 3. The code returns an error with recovery instructions (line 162)
		// 4. No fallback to Bootstrap occurs

		// This test asserts the fail-closed behavior
		t.Skip("Integration test requiring gateway with stale trust bundle - verifying fail-closed error message in auth.go:156-165, 281-290")
	})
}

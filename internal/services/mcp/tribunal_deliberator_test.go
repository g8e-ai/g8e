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

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPTribunalDeliberator_Timeout verifies that the HTTP deliberator
// respects its client timeout when the tribunal server is slow to respond.
// The test uses a 1s client timeout against a server that delays 2s,
// proving the timeout fires in under 2s.
func TestHTTPTribunalDeliberator_Timeout(t *testing.T) {
	// Start a test server that delays responses by 2 seconds.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(srv.Close)

	// Construct a deliberator with a 1s timeout (well below the 2s server delay).
	d := &HTTPTribunalDeliberator{
		url: srv.URL,
		client: &http.Client{
			Timeout: 1 * time.Second,
		},
	}

	start := time.Now()
	_, err := d.Deliberate(context.Background(), []byte("{}"))
	elapsed := time.Since(start)

	require.Error(t, err, "expected timeout error")
	assert.Less(t, elapsed, 2*time.Second, "timeout should fire before the 2s server delay completes")
	assert.Contains(t, err.Error(), "tribunal deliberator", "error should be wrapped by deliberator")
	t.Logf("Deliberate returned in %v (timeout was 1s, server delay was 2s)", elapsed)
}

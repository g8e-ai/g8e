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

//go:build integration

package pubsub

import (
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/testutil"
)

// NewTestPubSubClient returns a real G8esPubSubClient connected to the test g8es instance.
// Fatally fails the test if g8es is not available.
func NewTestPubSubClient(t *testing.T) *G8esPubSubClient {
	t.Helper()
	testutil.TestPubSubAvailable(t)
	logger := testutil.NewTestLogger()
	client, err := NewG8esPubSubClient(testutil.GetTestG8esDirectURL(), "", logger)
	if err != nil {
		t.Fatalf("Failed to create G8esPubSubClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

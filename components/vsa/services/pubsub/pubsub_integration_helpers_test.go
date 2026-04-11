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

	"github.com/g8e-ai/g8e/components/vsa/testutil"
)

// NewTestPubSubClient returns a real VSODBPubSubClient connected to the test VSODB instance.
// Fatally fails the test if VSODB is not available.
func NewTestPubSubClient(t *testing.T) *VSODBPubSubClient {
	t.Helper()
	testutil.TestPubSubAvailable(t)
	logger := testutil.NewTestLogger()
	client, err := NewVSODBPubSubClient(testutil.GetTestVSODBDirectURL(), "", logger)
	if err != nil {
		t.Fatalf("Failed to create VSODBPubSubClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

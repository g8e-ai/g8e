// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package pubsub

import (
	"crypto/tls"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/certs"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

// NewTestPubSubClient returns a real OperatorPubSubClient connected to the test Operator instance.
// Fatally fails the test if Operator is not available.
func NewTestPubSubClient(t *testing.T) *OperatorPubSubClient {
	t.Helper()
	testutil.TestPubSubAvailable(t, "")
	logger := testutil.NewTestLogger()
	trustStore := testutil.GetTestTrustStore()
	clientIdentity := certs.NewClientIdentity(tls.Certificate{})
	tlsConfig := certs.NewTLSConfig(trustStore, clientIdentity)
	client, err := NewOperatorPubSubClient(testutil.GetTestOperatorDirectURL(), "", logger, tlsConfig)
	if err != nil {
		t.Fatalf("Failed to create OperatorPubSubClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

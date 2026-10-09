// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	"github.com/g8e-ai/g8e/v2/test/fixtures"
)

// TestBootstrapStatusWait_ReleasesOnFirstUserCreated verifies that a held
// bootstrap status request stays open on an unbootstrapped gateway and is
// released by the user-created event, so workloads never poll for bootstrap.
func TestBootstrapStatusWait_ReleasesOnFirstUserCreated(t *testing.T) {
	fixture := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		AllowTestPortZero: true,
	})
	fixture.WaitForReady(t)

	userSvc := fixture.Service.GetUserService()
	hasUsers, err := userSvc.HasAnyUsers(t.Context())
	require.NoError(t, err)
	require.False(t, hasUsers, "fixture gateway must start unbootstrapped")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	url := network.LocalhostHTTPURL(fixture.Service.GetHTTPPort()) + constants.APIPaths.AuthBootstrapStatus + "?wait=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	type result struct {
		status models.BootstrapStatusResponse
		code   int
		err    error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- result{err: err}
			return
		}
		defer resp.Body.Close()
		var status models.BootstrapStatusResponse
		err = json.NewDecoder(resp.Body).Decode(&status)
		done <- result{status: status, code: resp.StatusCode, err: err}
	}()

	select {
	case r := <-done:
		t.Fatalf("held status request returned before bootstrap: %+v", r)
	case <-time.After(500 * time.Millisecond):
	}

	_, err = userSvc.CreateUserWithOSUser(t.Context(), nil)
	require.NoError(t, err)

	select {
	case r := <-done:
		require.NoError(t, r.err)
		assert.Equal(t, http.StatusOK, r.code)
		assert.True(t, r.status.Bootstrapped)
	case <-ctx.Done():
		t.Fatal("held status request was not released by user creation")
	}
}

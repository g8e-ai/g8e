// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package operatorcmd

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

func TestConnectDeploymentEventsReturnsOnceTheStreamIsEstablishedAndDeliversEvents(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "cli-1", r.Header.Get(constants.HeaderCLISessionID))
		assert.Equal(t, constants.APIPaths.SSEStream, r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", pushFrame(t, string(constants.EventPlatformApprovalsChanged),
			models.ApprovalsChangedPayload{Subject: models.ApprovalsChangedEnrollments, DeploymentID: "launch-1", RequestID: "req-1"}))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events, stop, err := connectDeploymentEvents(ctx, server.Client(), server.URL, "cli-1")
	require.NoError(t, err)
	defer stop()

	w := events.watch("launch-1")
	close(release)
	requestID, err := w.awaitStaged(ctx)
	require.NoError(t, err)
	assert.Equal(t, "req-1", requestID)
}

func TestConnectDeploymentEventsFailsWhenTheGatewayRejectsTheCLISession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, err := connectDeploymentEvents(ctx, server.Client(), server.URL, "cli-1")
	require.ErrorIs(t, err, constants.ErrOperatorDeployFailed)
}

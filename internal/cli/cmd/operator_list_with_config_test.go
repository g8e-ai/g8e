// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/cli/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
)

func TestOperatorListCmdWithConfig_ConfigLoadError(t *testing.T) {
	failLoader := func(string) (*config.Config, error) {
		return nil, errors.New("config load error")
	}

	cmd := operatorListCmdWithConfig(failLoader, defaultAPIClientFactory, newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config load error")
}

func TestOperatorListCmdWithConfig_ClientCreationError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	cmd := operatorListCmdWithConfig(loader, failingClientFactory(errors.New("client creation error")), newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client creation error")
}

func TestOperatorListCmdWithConfig_GetRequestError(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getErr: errors.New("network error")}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
}

func TestOperatorListCmdWithConfig_InvalidJSONResponse(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getResp: []byte("not json")}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
}

func TestOperatorListCmdWithConfig_EmptyOperatorsListPrintsNoOperators(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	operators := []models.OperatorDocumentGo{}
	operatorsJSON, _ := json.Marshal(operators)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getResp: operatorsJSON}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No operators found")
	assert.Equal(t, []string{"/api/operators"}, client.getCalls)
}

func TestOperatorListCmdWithConfig_ValidResponsePrintsOperatorTable(t *testing.T) {
	_, cfg := newCmdTestEnv(t)

	operators := []models.OperatorDocumentGo{
		{ID: "op-001", CloudSubtype: "aws", Status: "active"},
		{ID: "op-002", CloudSubtype: "gcp", Status: "standby"},
	}
	operatorsJSON, _ := json.Marshal(operators)

	loader := func(string) (*config.Config, error) { return cfg, nil }
	client := &mockAPIClient{getResp: operatorsJSON}
	cmd := operatorListCmdWithConfig(loader, mockClientFactory(client), newFileSvc)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := cmd.RunE(cmd, nil)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Operators (2 total)")
	assert.Contains(t, output, "op-001")
	assert.Contains(t, output, "op-002")
	assert.Contains(t, output, "aws")
	assert.Contains(t, output, "gcp")
	assert.Equal(t, []string{"/api/operators"}, client.getCalls)
}

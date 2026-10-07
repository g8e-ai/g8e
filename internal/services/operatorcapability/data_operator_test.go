// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcapability

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

func TestIsDataOperator_ExcludesOtherRoles(t *testing.T) {
	t.Parallel()

	data := models.OperatorDocumentGo{
		ID:                "data-1",
		OperatorSessionID: "sess-data-1",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
	}
	assert.True(t, IsDataOperator(data))

	cases := []models.OperatorDocumentGo{
		{
			ID:                "infer-1",
			OperatorSessionID: "sess-infer-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{InferenceEnabled: true},
		},
		{
			ID:                "observer-1",
			OperatorSessionID: "sess-observer-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{ProviderBoundaryObserverEnabled: true},
		},
		{
			ID:                "prov-1",
			OperatorSessionID: "sess-prov-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			RuntimeConfig:     &models.RuntimeConfig{ProvenanceOperatorEnabled: true},
		},
		{
			ID:                "role-1",
			OperatorSessionID: "sess-role-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			OperatorRoles:     constants.OperatorRoles{constants.OperatorRoleObserver},
		},
		{
			ID:                "offline-1",
			OperatorSessionID: "sess-offline-1",
			Status:            constants.OperatorStatusOffline,
			OperatorType:      constants.OperatorTypeRemote,
		},
		{
			ID:                "embedded-1",
			OperatorSessionID: "sess-embedded-1",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeEmbedded,
		},
	}
	for _, op := range cases {
		assert.False(t, IsDataOperator(op), op.ID)
	}
}

func stackDataOperatorDoc(id, sessionID, hostname string) models.OperatorDocumentGo {
	return models.OperatorDocumentGo{
		ID:                id,
		OperatorSessionID: sessionID,
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		CurrentHostname:   hostname,
	}
}

func TestSelectDataOperator_ConsidersOnlyTheStackDataOperator(t *testing.T) {
	t.Parallel()
	operators := []models.OperatorDocumentGo{
		stackDataOperatorDoc("load-1", "sess-load-1", "load-host-1"),
		stackDataOperatorDoc("stack", "sess-stack", constants.DataOperatorHostname),
		stackDataOperatorDoc("load-2", "sess-load-2", "load-host-2"),
	}

	selected, err := SelectDataOperator(operators)
	require.NoError(t, err)
	assert.Equal(t, "stack", selected.OperatorID)
	assert.Equal(t, "sess-stack", selected.OperatorSessionID)
}

func TestSelectDataOperator_IgnoresOtherRolesOnTheStackHostname(t *testing.T) {
	t.Parallel()
	inference := stackDataOperatorDoc("infer", "sess-infer", constants.DataOperatorHostname)
	inference.RuntimeConfig = &models.RuntimeConfig{InferenceEnabled: true}

	_, err := SelectDataOperator([]models.OperatorDocumentGo{inference})
	require.ErrorIs(t, err, constants.ErrDataOperatorNotFound)
}

func TestSelectDataOperator_ResolvesHostNativeWithoutStackDataOperator(t *testing.T) {
	t.Parallel()
	selected, err := SelectDataOperator([]models.OperatorDocumentGo{stackDataOperatorDoc("load-1", "sess-load-1", "load-host-1")})
	require.NoError(t, err)
	assert.Equal(t, "load-1", selected.OperatorID)
	assert.Equal(t, "sess-load-1", selected.OperatorSessionID)

	_, err = SelectDataOperator(nil)
	require.ErrorIs(t, err, constants.ErrDataOperatorNotFound)
}

func TestSelectDataOperator_AmbiguousWhenTwoStackDataOperatorsAreActive(t *testing.T) {
	t.Parallel()
	_, err := SelectDataOperator([]models.OperatorDocumentGo{
		stackDataOperatorDoc("a", "sess-a", constants.DataOperatorHostname),
		stackDataOperatorDoc("b", "sess-b", constants.DataOperatorHostname),
	})
	require.ErrorIs(t, err, constants.ErrDataOperatorAmbiguous)

	_, err = SelectDataOperator([]models.OperatorDocumentGo{
		stackDataOperatorDoc("a", "sess-a", "host-a"),
		stackDataOperatorDoc("b", "sess-b", "host-b"),
	})
	require.ErrorIs(t, err, constants.ErrDataOperatorAmbiguous)
}

func TestIsStackDataOperator_RequiresDataOperatorHostname(t *testing.T) {
	t.Parallel()

	stack := models.OperatorDocumentGo{
		ID:                "data-1",
		OperatorSessionID: "sess-data-1",
		Status:            constants.OperatorStatusActive,
		OperatorType:      constants.OperatorTypeRemote,
		CurrentHostname:   constants.DataOperatorHostname,
	}
	assert.True(t, IsStackDataOperator(stack))

	other := stack
	other.CurrentHostname = "loadtest-host"
	assert.False(t, IsStackDataOperator(other))

	inference := stack
	inference.RuntimeConfig = &models.RuntimeConfig{InferenceEnabled: true}
	assert.False(t, IsStackDataOperator(inference))
}

func TestExtractWorkingDirectory_FromEnvironmentDetails(t *testing.T) {
	t.Parallel()

	hr := &operatorv1.HeartbeatResult{
		Environment: &operatorv1.EnvironmentDetails{Pwd: "/home/deploy"},
	}
	hb, err := protojson.Marshal(hr)
	require.NoError(t, err)

	pwd := extractWorkingDirectory(hb)
	assert.Equal(t, "/home/deploy", pwd)
}

func TestExtractWorkingDirectory_FallbackToSystemIdentity(t *testing.T) {
	t.Parallel()

	hr := &operatorv1.HeartbeatResult{
		SystemIdentity: &operatorv1.SystemIdentity{Pwd: "/root"},
	}
	hb, err := protojson.Marshal(hr)
	require.NoError(t, err)

	pwd := extractWorkingDirectory(hb)
	assert.Equal(t, "/root", pwd)
}

func TestExtractWorkingDirectory_EnvironmentTakesPrecedence(t *testing.T) {
	t.Parallel()

	hr := &operatorv1.HeartbeatResult{
		Environment:    &operatorv1.EnvironmentDetails{Pwd: "/home/deploy"},
		SystemIdentity: &operatorv1.SystemIdentity{Pwd: "/root"},
	}
	hb, err := protojson.Marshal(hr)
	require.NoError(t, err)

	pwd := extractWorkingDirectory(hb)
	assert.Equal(t, "/home/deploy", pwd)
}

func TestExtractWorkingDirectory_EmptyHeartbeat(t *testing.T) {
	t.Parallel()
	pwd := extractWorkingDirectory(nil)
	assert.Empty(t, pwd)

	pwd = extractWorkingDirectory([]byte{})
	assert.Empty(t, pwd)
}

func TestExtractWorkingDirectory_InvalidJSON(t *testing.T) {
	t.Parallel()
	pwd := extractWorkingDirectory([]byte("not json"))
	assert.Empty(t, pwd)
}

func TestExtractWorkingDirectory_NoPwdInHeartbeat(t *testing.T) {
	t.Parallel()

	hr := &operatorv1.HeartbeatResult{
		SystemIdentity: &operatorv1.SystemIdentity{Hostname: "test"},
	}
	hb, err := protojson.Marshal(hr)
	require.NoError(t, err)

	pwd := extractWorkingDirectory(hb)
	assert.Empty(t, pwd)
}

func TestActiveDataOperators_IncludesWorkingDirectory(t *testing.T) {
	t.Parallel()

	hr := &operatorv1.HeartbeatResult{
		Environment: &operatorv1.EnvironmentDetails{Pwd: "/home/deploy"},
	}
	hb, err := protojson.Marshal(hr)
	require.NoError(t, err)

	operators := []models.OperatorDocumentGo{
		{
			ID:                "stack",
			OperatorSessionID: "sess-stack",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			CurrentHostname:   constants.DataOperatorHostname,
			LatestHeartbeat:   json.RawMessage(hb),
		},
	}

	statuses := ActiveDataOperators(operators)
	require.Len(t, statuses, 1)
	assert.Equal(t, "stack", statuses[0].OperatorID)
	assert.Equal(t, "/home/deploy", statuses[0].WorkingDirectory)
}

func TestActiveDataOperators_EmptyHeartbeatLeavesWorkingDirectoryEmpty(t *testing.T) {
	t.Parallel()

	operators := []models.OperatorDocumentGo{
		{
			ID:                "stack",
			OperatorSessionID: "sess-stack",
			Status:            constants.OperatorStatusActive,
			OperatorType:      constants.OperatorTypeRemote,
			CurrentHostname:   constants.DataOperatorHostname,
			LatestHeartbeat:   nil,
		},
	}

	statuses := ActiveDataOperators(operators)
	require.Len(t, statuses, 1)
	assert.Empty(t, statuses[0].WorkingDirectory)
}

func tierOp(id string, kind constants.OperatorType, hostname string, roles ...constants.OperatorRole) models.OperatorDocumentGo {
	return models.OperatorDocumentGo{
		ID:                id,
		OperatorSessionID: "sess-" + id,
		Status:            constants.OperatorStatusActive,
		OperatorType:      kind,
		CurrentHostname:   hostname,
		RuntimeConfig:     &models.RuntimeConfig{Roles: roles},
	}
}

func TestEmbeddedOnlyImpliesDataAndWitnessCommands(t *testing.T) {
	cfg := &models.RuntimeConfig{Roles: constants.OperatorRoles{constants.OperatorRoleEmbedded}}
	require.Equal(t, constants.OperatorRoles{constants.OperatorRoleEmbedded, constants.OperatorRoleData}, ResolveOperatorRoles(cfg))
	require.NoError(t, ValidateWitnessCommand(cfg, "pwd"))
	op := tierOp("embedded-operator", constants.OperatorTypeEmbedded, "", constants.OperatorRoleEmbedded)
	require.True(t, IsDataOperator(op))
	require.False(t, IsDedicatedDataOperator(op))
}

func TestDataOperatorTiering(t *testing.T) {
	embedded := tierOp("embedded-operator", constants.OperatorTypeEmbedded, "", constants.OperatorRoleEmbedded)
	remote := tierOp("data", constants.OperatorTypeRemote, "host", constants.OperatorRoleData)
	stack := tierOp("stack", constants.OperatorTypeRemote, constants.DataOperatorHostname, constants.OperatorRoleData)

	sel, err := SelectDataOperator([]models.OperatorDocumentGo{embedded, remote})
	require.NoError(t, err)
	require.Equal(t, "data", sel.OperatorID)

	sel, err = SelectDataOperator([]models.OperatorDocumentGo{embedded})
	require.NoError(t, err)
	require.Equal(t, "embedded-operator", sel.OperatorID)

	sel, err = SelectDataOperator([]models.OperatorDocumentGo{embedded, remote, stack})
	require.NoError(t, err)
	require.Equal(t, "stack", sel.OperatorID)

	require.Equal(t, []models.OperatorDocumentGo{remote}, PreferDedicatedDataOperators([]models.OperatorDocumentGo{embedded, remote}))
	require.Equal(t, []models.OperatorDocumentGo{embedded}, PreferDedicatedDataOperators([]models.OperatorDocumentGo{embedded}))
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

const operatorsCollection = string(constants.CollectionOperators)

func timeAgo(d time.Duration) *timestamppb.Timestamp {
	t := time.Now().UTC().Add(-d)
	return timestamppb.New(t)
}

// putOperator persists an operator document whose created_at is createdAgo in
// the past, bypassing the reconciler so the test controls the starting state.
func putOperator(t *testing.T, svc *DocumentStoreService, id string, op *operatorv1.OperatorDocument, createdAgo time.Duration) {
	t.Helper()
	body, err := models.MarshalOperatorDocument(op)
	require.NoError(t, err)
	createdAt := time.Now().UTC().Add(-createdAgo)
	require.NoError(t, svc.DocSetWithTimestamps(operatorsCollection, id, body, createdAt, createdAt))
}

// persistedOperatorStatus reads the stored status without reconciliation.
func persistedOperatorStatus(t *testing.T, svc *DocumentStoreService, _ string) constants.OperatorStatus {
	t.Helper()
	doc, err := svc.docGet(operatorsCollection, "op-1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	var status constants.OperatorStatus
	require.NoError(t, json.Unmarshal(doc.Data["status"], &status))
	return status
}

func remoteOperator(status constants.OperatorStatus) *operatorv1.OperatorDocument {
	return &operatorv1.OperatorDocument{
		Status:       string(status),
		OperatorType: string(constants.OperatorTypeRemote),
	}
}

func TestOperatorStaleness_EveryReaderMarksSilentRemoteOperatorStale(t *testing.T) {
	silent := constants.OperatorHeartbeatStaleAfter * 2

	readers := map[string]func(svc *DocumentStoreService) error{
		"DocGet": func(svc *DocumentStoreService) error {
			_, err := svc.DocGet(operatorsCollection, "op-1")
			return err
		},
		"DocQuery": func(svc *DocumentStoreService) error {
			_, err := svc.DocQuery(operatorsCollection, nil, "", 0)
			return err
		},
		"DocList": func(svc *DocumentStoreService) error {
			_, err := svc.DocList(operatorsCollection)
			return err
		},
		"GetField": func(svc *DocumentStoreService) error {
			_, err := svc.GetField(operatorsCollection, "op-1", "status")
			return err
		},
	}

	for name, read := range readers {
		t.Run(name, func(t *testing.T) {
			svc := newDocumentStoreService(t)
			op := remoteOperator(constants.OperatorStatusActive)
			op.LastHeartbeatAt = timeAgo(silent)
			putOperator(t, svc, "op-1", op, time.Hour)

			require.NoError(t, read(svc))

			assert.Equal(t, constants.OperatorStatusStale, persistedOperatorStatus(t, svc, "op-1"))
		})
	}
}

func TestOperatorStaleness_LastSignOfLife(t *testing.T) {
	threshold := constants.OperatorHeartbeatStaleAfter
	silent := threshold + 5*time.Second

	tests := []struct {
		name       string
		heartbeat  *timestamppb.Timestamp
		claimed    *timestamppb.Timestamp
		createdAgo time.Duration
		want       constants.OperatorStatus
	}{
		{"recent heartbeat", timeAgo(threshold / 2), nil, time.Hour, constants.OperatorStatusActive},
		{"heartbeat past threshold", timeAgo(silent), nil, time.Hour, constants.OperatorStatusStale},
		{"heartbeat wins over old claim", timeAgo(threshold / 2), timeAgo(time.Hour), time.Hour, constants.OperatorStatusActive},
		{"no heartbeat, recent claim", nil, timeAgo(threshold / 2), time.Hour, constants.OperatorStatusActive},
		{"no heartbeat, old claim", nil, timeAgo(silent), time.Hour, constants.OperatorStatusStale},
		{"no heartbeat, no claim, new document", nil, nil, threshold / 2, constants.OperatorStatusActive},
		{"no heartbeat, no claim, old document", nil, nil, silent, constants.OperatorStatusStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newDocumentStoreService(t)
			op := remoteOperator(constants.OperatorStatusActive)
			op.LastHeartbeatAt = tt.heartbeat
			op.ClaimedAt = tt.claimed
			putOperator(t, svc, "op-1", op, tt.createdAgo)

			doc, err := svc.DocGet(operatorsCollection, "op-1")
			require.NoError(t, err)
			require.NotNil(t, doc)

			var got constants.OperatorStatus
			require.NoError(t, json.Unmarshal(doc.Data["status"], &got))
			assert.Equal(t, tt.want, got, "the returned document must already reflect staleness")
			assert.Equal(t, tt.want, persistedOperatorStatus(t, svc, "op-1"), "the transition must be persisted")
		})
	}
}

func TestOperatorStaleness_OnlyActiveRemoteOperatorsGoStale(t *testing.T) {
	silent := constants.OperatorHeartbeatStaleAfter * 10

	embedded := &operatorv1.OperatorDocument{Status: string(constants.OperatorStatusActive), OperatorType: string(constants.OperatorTypeEmbedded)}
	tests := []struct {
		name string
		op   *operatorv1.OperatorDocument
		want constants.OperatorStatus
	}{
		{"embedded active", embedded, constants.OperatorStatusActive},
		{"remote stopped", remoteOperator(constants.OperatorStatusStopped), constants.OperatorStatusStopped},
		{"remote terminated", remoteOperator(constants.OperatorStatusTerminated), constants.OperatorStatusTerminated},
		{"remote offline", remoteOperator(constants.OperatorStatusOffline), constants.OperatorStatusOffline},
		{"remote already stale", remoteOperator(constants.OperatorStatusStale), constants.OperatorStatusStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newDocumentStoreService(t)
			tt.op.LastHeartbeatAt = timeAgo(silent)
			putOperator(t, svc, "op-1", tt.op, time.Hour)

			_, err := svc.DocList(operatorsCollection)
			require.NoError(t, err)

			assert.Equal(t, tt.want, persistedOperatorStatus(t, svc, "op-1"))
		})
	}
}

func TestOperatorStaleness_StatusQuerySeesOnlyLiveOperators(t *testing.T) {
	svc := newDocumentStoreService(t)

	live := remoteOperator(constants.OperatorStatusActive)
	live.LastHeartbeatAt = timeAgo(time.Second)
	putOperator(t, svc, "op-live", live, time.Hour)

	silent := remoteOperator(constants.OperatorStatusActive)
	silent.LastHeartbeatAt = timeAgo(constants.OperatorHeartbeatStaleAfter * 2)
	putOperator(t, svc, "op-silent", silent, time.Hour)

	docs, err := svc.DocQuery(operatorsCollection, []models.DocFilter{
		{Field: "status", Op: "==", Value: json.RawMessage(fmt.Sprintf("%q", constants.OperatorStatusActive))},
	}, "", 0)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "op-live", docs[0].ID)
}

func TestOperatorStaleness_OtherCollectionsAreNotReconciled(t *testing.T) {
	svc := newDocumentStoreService(t)
	op := remoteOperator(constants.OperatorStatusActive)
	op.LastHeartbeatAt = timeAgo(constants.OperatorHeartbeatStaleAfter * 10)
	body, err := models.MarshalOperatorDocument(op)
	require.NoError(t, err)
	require.NoError(t, svc.DocSet("settings", "not-an-operator", body))

	_, err = svc.DocList("settings")
	require.NoError(t, err)

	doc, err := svc.docGet("settings", "not-an-operator")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.JSONEq(t, `"active"`, string(doc.Data["status"]))
}

func TestOperatorStaleness_ReconcileFailureFailsTheRead(t *testing.T) {
	svc := newDocumentStoreService(t)
	require.NoError(t, svc.db.Close())

	_, err := svc.DocGet(operatorsCollection, "op-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOperatorStalenessReconcile)

	_, err = svc.DocQuery(operatorsCollection, nil, "", 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, constants.ErrOperatorStalenessReconcile)
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gwremote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
)

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	body, err := json.Marshal(v)
	require.NoError(t, err)
	return body
}

func auditProofInput(id string) evaluation.AssignmentAuditProofInput {
	return evaluation.AssignmentAuditProofInput{
		CampaignID:       "campaign-" + id,
		CampaignRevision: "rev-" + id,
		RunID:            "run-" + id,
		AssignmentID:     "assignment-" + id,
		IndexDigest:      "digest-" + id,
		Artifacts: evaluation.AssignmentAuditSliceArtifacts{
			Database: []byte("db-" + id),
			VaultKey: []byte("key-" + id),
		},
	}
}

func TestRemoteGatewayCampaignProofPublisher_IngestAssignmentAuditSlices(t *testing.T) {
	t.Run("a nil publisher or client reports a missing required field", func(t *testing.T) {
		var nilPublisher *remoteGatewayCampaignProofPublisher
		err := nilPublisher.IngestAssignmentAuditSlices(context.Background(), []evaluation.AssignmentAuditProofInput{auditProofInput("1")}, false)
		require.ErrorIs(t, err, constants.ErrMissingRequiredField)

		err = (&remoteGatewayCampaignProofPublisher{}).IngestAssignmentAuditSlices(context.Background(), []evaluation.AssignmentAuditProofInput{auditProofInput("1")}, false)
		require.ErrorIs(t, err, constants.ErrMissingRequiredField)
	})

	t.Run("no inputs is a no-op that never calls the gateway", func(t *testing.T) {
		client := &cmdtest.MockAPIClient{}
		publisher := &remoteGatewayCampaignProofPublisher{client: client}

		require.NoError(t, publisher.IngestAssignmentAuditSlices(context.Background(), nil, true))
		assert.Empty(t, client.PostCalls)
	})

	t.Run("a single proof carries the deferred-push flag on the single-proof endpoint", func(t *testing.T) {
		client := &cmdtest.MockAPIClient{PostResp: mustMarshalJSON(t, models.PublicAssignmentAuditProofPublishResponse{Accepted: true})}
		publisher := &remoteGatewayCampaignProofPublisher{client: client}

		require.NoError(t, publisher.IngestAssignmentAuditSlices(context.Background(), []evaluation.AssignmentAuditProofInput{auditProofInput("1")}, true))

		require.Len(t, client.PostCalls, 1)
		assert.Equal(t, constants.APIPaths.PublicFeedProofs, client.PostCalls[0].Path)
		request, ok := client.PostCalls[0].Body.(models.PublicAssignmentAuditProofPublishRequest)
		require.True(t, ok, "got %T", client.PostCalls[0].Body)
		assert.True(t, request.DeferMirrorPush)
	})

	t.Run("several proofs are sent once as a batch preserving order and the deferred-push flag", func(t *testing.T) {
		client := &cmdtest.MockAPIClient{PostResp: mustMarshalJSON(t, models.PublicAssignmentAuditProofBatchPublishResponse{Accepted: true, IngestedProofs: 2})}
		publisher := &remoteGatewayCampaignProofPublisher{client: client}

		err := publisher.IngestAssignmentAuditSlices(context.Background(), []evaluation.AssignmentAuditProofInput{auditProofInput("1"), auditProofInput("2")}, true)
		require.NoError(t, err)

		require.Len(t, client.PostCalls, 1)
		assert.Equal(t, constants.APIPaths.PublicFeedProofsBatch, client.PostCalls[0].Path)
		request, ok := client.PostCalls[0].Body.(models.PublicAssignmentAuditProofBatchPublishRequest)
		require.True(t, ok, "got %T", client.PostCalls[0].Body)
		assert.True(t, request.DeferMirrorPush)
		require.Len(t, request.Proofs, 2)
		assert.Equal(t, "assignment-1", request.Proofs[0].AssignmentID)
		assert.Equal(t, "assignment-2", request.Proofs[1].AssignmentID)
		assert.Equal(t, []byte("db-2"), request.Proofs[1].Database)
		assert.Equal(t, []byte("key-2"), request.Proofs[1].VaultKey)
	})

	failures := []struct {
		name    string
		inputs  []evaluation.AssignmentAuditProofInput
		client  *cmdtest.MockAPIClient
		wantErr error
		wantMsg string
	}{
		{
			name:    "single proof rejected by the gateway",
			inputs:  []evaluation.AssignmentAuditProofInput{auditProofInput("1")},
			client:  &cmdtest.MockAPIClient{PostResp: mustMarshalJSON(t, models.PublicAssignmentAuditProofPublishResponse{Accepted: false})},
			wantErr: constants.ErrPublicFeedProofIngestRejected,
		},
		{
			name:    "batch rejected by the gateway",
			inputs:  []evaluation.AssignmentAuditProofInput{auditProofInput("1"), auditProofInput("2")},
			client:  &cmdtest.MockAPIClient{PostResp: mustMarshalJSON(t, models.PublicAssignmentAuditProofBatchPublishResponse{Accepted: false})},
			wantErr: constants.ErrPublicFeedProofIngestRejected,
		},
		{
			name:    "single proof with an undecodable response",
			inputs:  []evaluation.AssignmentAuditProofInput{auditProofInput("1")},
			client:  &cmdtest.MockAPIClient{PostResp: []byte(`[]`)},
			wantErr: constants.ErrInvalidJSONResponse,
		},
		{
			name:    "batch with an undecodable response",
			inputs:  []evaluation.AssignmentAuditProofInput{auditProofInput("1"), auditProofInput("2")},
			client:  &cmdtest.MockAPIClient{PostResp: []byte(`[]`)},
			wantErr: constants.ErrInvalidJSONResponse,
		},
		{
			name:    "transport error is wrapped with publication context",
			inputs:  []evaluation.AssignmentAuditProofInput{auditProofInput("1")},
			client:  &cmdtest.MockAPIClient{PostErr: constants.ErrHTTPStatusError},
			wantErr: constants.ErrHTTPStatusError,
			wantMsg: "publish assignment audit proof",
		},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &remoteGatewayCampaignProofPublisher{client: tt.client}

			err := publisher.IngestAssignmentAuditSlices(context.Background(), tt.inputs, false)
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestRemoteGatewayCampaignProofPublisher_PruneRunProofCatalog(t *testing.T) {
	t.Run("is a no-op for a nil publisher, nil client, or empty run id", func(t *testing.T) {
		var nilPublisher *remoteGatewayCampaignProofPublisher
		require.NoError(t, nilPublisher.PruneRunProofCatalog(context.Background(), "run-1"))
		require.NoError(t, (&remoteGatewayCampaignProofPublisher{}).PruneRunProofCatalog(context.Background(), "run-1"))

		client := &cmdtest.MockAPIClient{}
		require.NoError(t, (&remoteGatewayCampaignProofPublisher{client: client}).PruneRunProofCatalog(context.Background(), ""))
		assert.Empty(t, client.PostCalls)
	})

	t.Run("posts the run id to the prune endpoint", func(t *testing.T) {
		client := &cmdtest.MockAPIClient{PostResp: mustMarshalJSON(t, models.PublicProofCatalogPruneResponse{Accepted: true, RemovedCount: 3})}

		require.NoError(t, (&remoteGatewayCampaignProofPublisher{client: client}).PruneRunProofCatalog(context.Background(), "run-7"))

		require.Len(t, client.PostCalls, 1)
		assert.Equal(t, constants.APIPaths.PublicFeedProofsPrune, client.PostCalls[0].Path)
		assert.Equal(t, models.PublicProofCatalogPruneRequest{RunID: "run-7"}, client.PostCalls[0].Body)
	})

	failures := []struct {
		name    string
		client  *cmdtest.MockAPIClient
		wantErr error
		wantMsg string
	}{
		{
			name:    "gateway declines the prune",
			client:  &cmdtest.MockAPIClient{PostResp: mustMarshalJSON(t, models.PublicProofCatalogPruneResponse{Accepted: false})},
			wantErr: constants.ErrPublicFeedProofIngestRejected,
		},
		{
			name:    "undecodable response",
			client:  &cmdtest.MockAPIClient{PostResp: []byte(`[]`)},
			wantErr: constants.ErrInvalidJSONResponse,
		},
		{
			name:    "transport error is wrapped with prune context",
			client:  &cmdtest.MockAPIClient{PostErr: constants.ErrHTTPStatusError},
			wantErr: constants.ErrHTTPStatusError,
			wantMsg: "prune run proof catalog",
		},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			err := (&remoteGatewayCampaignProofPublisher{client: tt.client}).PruneRunProofCatalog(context.Background(), "run-7")
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestRemoteGatewayCampaignProofPublisher_FlushProofCatalog(t *testing.T) {
	t.Run("a nil publisher or client reports a missing required field", func(t *testing.T) {
		var nilPublisher *remoteGatewayCampaignProofPublisher
		require.ErrorIs(t, nilPublisher.FlushProofCatalog(context.Background()), constants.ErrMissingRequiredField)
		require.ErrorIs(t, (&remoteGatewayCampaignProofPublisher{}).FlushProofCatalog(context.Background()), constants.ErrMissingRequiredField)
	})

	t.Run("posts an empty body to the push endpoint", func(t *testing.T) {
		client := &cmdtest.MockAPIClient{PostResp: mustMarshalJSON(t, models.PublicProofCatalogPushResponse{Accepted: true, ArtifactCount: 4})}

		require.NoError(t, (&remoteGatewayCampaignProofPublisher{client: client}).FlushProofCatalog(context.Background()))

		require.Len(t, client.PostCalls, 1)
		assert.Equal(t, constants.APIPaths.PublicFeedProofsPush, client.PostCalls[0].Path)
		assert.Equal(t, struct{}{}, client.PostCalls[0].Body)
	})

	failures := []struct {
		name    string
		client  *cmdtest.MockAPIClient
		wantErr error
		wantMsg string
	}{
		{
			name:    "gateway declines the push",
			client:  &cmdtest.MockAPIClient{PostResp: mustMarshalJSON(t, models.PublicProofCatalogPushResponse{Accepted: false})},
			wantErr: constants.ErrPublicFeedProofIngestRejected,
		},
		{
			name:    "undecodable response",
			client:  &cmdtest.MockAPIClient{PostResp: []byte(`[]`)},
			wantErr: constants.ErrInvalidJSONResponse,
		},
		{
			name:    "transport error is wrapped with flush context",
			client:  &cmdtest.MockAPIClient{PostErr: constants.ErrHTTPStatusError},
			wantErr: constants.ErrHTTPStatusError,
			wantMsg: "flush proof catalog",
		},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			err := (&remoteGatewayCampaignProofPublisher{client: tt.client}).FlushProofCatalog(context.Background())
			require.ErrorIs(t, err, tt.wantErr)
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestNewCampaignProofPublisher(t *testing.T) {
	t.Run("requires a healthy gateway", func(t *testing.T) {
		WithGatewayHealthCheck(t, false)
		fileSvc, cfg := cmdtest.NewCmdTestEnv(t)

		publisher, err := NewCampaignProofPublisher(context.Background(), fileSvc, cfg)
		assert.ErrorIs(t, err, constants.ErrGatewayUnhealthy)
		assert.Nil(t, publisher)
	})

	t.Run("returns a gateway-backed publisher when the gateway is healthy", func(t *testing.T) {
		WithGatewayHealthCheck(t, true)
		cfg, _, fileSvc := cmdtest.SetupApproveSSETestEnv(t)

		publisher, err := NewCampaignProofPublisher(context.Background(), fileSvc, cfg)
		require.NoError(t, err)
		require.NotNil(t, publisher)
		assert.IsType(t, &remoteGatewayCampaignProofPublisher{}, publisher)
	})
}

func TestIsPublicFeedOutboxPublicationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "outbox equivocation", err: constants.ErrPublicFeedOutboxEquivocation, want: true},
		{name: "outbox corrupt", err: constants.ErrPublicFeedOutboxCorrupt, want: true},
		{name: "hash chain mismatch", err: constants.ErrPublicFeedHashChainMismatch, want: true},
		{name: "sentinel survives wrapping and HTTP body embedding", err: fmt.Errorf("status 409: {\"error\":%q}: %w", constants.ErrPublicFeedOutboxCorrupt.Error(), constants.ErrHTTPStatusError), want: true},
		{name: "outbox empty is not a publication fault", err: constants.ErrPublicFeedOutboxEmpty, want: false},
		{name: "unrelated error", err: errors.New("connection reset"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isPublicFeedOutboxPublicationError(tt.err))
		})
	}
}

func TestIsPublicFeedPublicationRetryable(t *testing.T) {
	assert.True(t, isPublicFeedPublicationRetryable(constants.ErrPublicFeedSequenceOutOfOrder))
	assert.True(t, isPublicFeedPublicationRetryable(constants.ErrPublicFeedHashChainMismatch))
	assert.False(t, isPublicFeedPublicationRetryable(errors.New("connection reset")))
	assert.False(t, isPublicFeedPublicationRetryable(nil))
}

// scriptedFeedClient answers exporter POSTs from a queue of canned errors (a nil
// entry means success) and serves a fixed snapshot for high-water refreshes.
type scriptedFeedClient struct {
	mu          sync.Mutex
	postErrs    []error
	snapshot    []byte
	snapshotErr error
	posted      [][]models.PublicFeedRecord
	getCalls    int
}

func (c *scriptedFeedClient) Get(string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	return c.snapshot, c.snapshotErr
}

func (c *scriptedFeedClient) Post(_ string, body interface{}) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	records := body.([]models.PublicFeedRecord)
	c.posted = append(c.posted, append([]models.PublicFeedRecord(nil), records...))
	if len(c.postErrs) > 0 {
		err := c.postErrs[0]
		c.postErrs = c.postErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(models.PublicFeedExportBatchResponse{HighWaterSequence: records[len(records)-1].Sequence})
}

func (c *scriptedFeedClient) Put(string, interface{}) ([]byte, error) { return nil, nil }
func (c *scriptedFeedClient) Delete(string) ([]byte, error)           { return nil, nil }

func feedRecords(sequences ...int64) []evaluation.CampaignPublicFeedRecord {
	records := make([]evaluation.CampaignPublicFeedRecord, len(sequences))
	for i, seq := range sequences {
		records[i] = evaluation.CampaignPublicFeedRecord{Sequence: seq, RecordHash: fmt.Sprintf("hash-%d", seq), RecordBytes: "{}"}
	}
	return records
}

func TestRemoteGatewayCampaignFeedExporter_ExportBatchRetries(t *testing.T) {
	t.Run("an outbox fault triggers one retry resequenced past the refreshed high water", func(t *testing.T) {
		client := &scriptedFeedClient{
			postErrs: []error{fmt.Errorf("status 409: %w", constants.ErrPublicFeedOutboxEquivocation)},
			snapshot: mustMarshalJSON(t, models.PublicFeedSnapshot{HighWaterSequence: 20}),
		}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}
		records := feedRecords(1, 2, 3)

		require.NoError(t, exporter.ExportBatch(context.Background(), records))

		require.Len(t, client.posted, 2)
		assert.Equal(t, 1, client.getCalls, "high water is refreshed exactly once")
		assert.Equal(t, []int64{21, 22, 23}, []int64{records[0].Sequence, records[1].Sequence, records[2].Sequence})
		assert.Equal(t, int64(21), client.posted[1][0].Sequence)
		assert.Equal(t, int64(23), exporter.highWater, "cache tracks the last accepted sequence")
	})

	t.Run("a non-retryable error is returned immediately without refreshing", func(t *testing.T) {
		boom := errors.New("gateway exploded")
		client := &scriptedFeedClient{postErrs: []error{boom}}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}

		err := exporter.ExportBatch(context.Background(), feedRecords(1))
		require.ErrorIs(t, err, boom)
		assert.Len(t, client.posted, 1)
		assert.Zero(t, client.getCalls)
	})

	t.Run("a retryable error on both attempts surfaces the second failure", func(t *testing.T) {
		client := &scriptedFeedClient{
			postErrs: []error{
				fmt.Errorf("first: %w", constants.ErrPublicFeedHashChainMismatch),
				fmt.Errorf("second: %w", constants.ErrPublicFeedOutboxCorrupt),
			},
			snapshot: mustMarshalJSON(t, models.PublicFeedSnapshot{HighWaterSequence: 5}),
		}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}

		err := exporter.ExportBatch(context.Background(), feedRecords(1))
		require.ErrorIs(t, err, constants.ErrPublicFeedOutboxCorrupt)
		assert.Contains(t, err.Error(), "second")
		assert.Len(t, client.posted, 2, "exactly one retry is attempted")
	})

	t.Run("a failed high-water refresh aborts the retry with context", func(t *testing.T) {
		snapshotFailure := errors.New("snapshot unavailable")
		client := &scriptedFeedClient{
			postErrs:    []error{constants.ErrPublicFeedSequenceOutOfOrder},
			snapshotErr: snapshotFailure,
		}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}

		err := exporter.ExportBatch(context.Background(), feedRecords(1))
		require.ErrorIs(t, err, snapshotFailure)
		assert.Contains(t, err.Error(), "refresh high water")
		assert.Len(t, client.posted, 1, "no second post after the refresh failed")
	})

	t.Run("an undecodable export response is not retried", func(t *testing.T) {
		client := &cmdtest.MockAPIClient{PostResp: []byte(`[]`)}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}

		err := exporter.ExportBatch(context.Background(), feedRecords(1))
		require.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
		assert.Len(t, client.PostCalls, 1)
	})

	t.Run("records without a type are exported as projections", func(t *testing.T) {
		client := &scriptedFeedClient{}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}

		require.NoError(t, exporter.ExportBatch(context.Background(), feedRecords(1)))
		assert.Equal(t, models.PublicFeedRecordTypeProjection, client.posted[0][0].RecordType)
	})
}

func TestRemoteGatewayCampaignFeedExporter_HighWaterSequence(t *testing.T) {
	t.Run("a cached high water is served without calling the gateway", func(t *testing.T) {
		client := &scriptedFeedClient{snapshotErr: errors.New("must not be called")}
		exporter := &remoteGatewayCampaignFeedExporter{client: client, highWater: 9}

		got, err := exporter.HighWaterSequence(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(9), got)
		assert.Zero(t, client.getCalls)
	})

	t.Run("a snapshot never lowers the cached high water", func(t *testing.T) {
		client := &scriptedFeedClient{snapshot: mustMarshalJSON(t, models.PublicFeedSnapshot{HighWaterSequence: 3})}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}

		got, err := exporter.fetchGatewayHighWater()
		require.NoError(t, err)
		assert.Equal(t, int64(3), got)

		client.snapshot = mustMarshalJSON(t, models.PublicFeedSnapshot{HighWaterSequence: 1})
		got, err = exporter.fetchGatewayHighWater()
		require.NoError(t, err)
		assert.Equal(t, int64(3), got)
	})

	t.Run("snapshot read failures are wrapped", func(t *testing.T) {
		client := &scriptedFeedClient{snapshotErr: errors.New("gateway down")}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}

		_, err := exporter.HighWaterSequence(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public feed snapshot")
	})

	t.Run("an undecodable snapshot is an invalid JSON response", func(t *testing.T) {
		client := &scriptedFeedClient{snapshot: []byte(`[]`)}
		exporter := &remoteGatewayCampaignFeedExporter{client: client}

		_, err := exporter.HighWaterSequence(context.Background())
		require.ErrorIs(t, err, constants.ErrInvalidJSONResponse)
	})
}

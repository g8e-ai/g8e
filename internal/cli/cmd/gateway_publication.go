// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/evaluation"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

type remoteGatewayCampaignFeedExporter struct {
	client    apiClient
	highWater int64
	mu        sync.Mutex
}

func (e *remoteGatewayCampaignFeedExporter) HighWaterSequence(ctx context.Context) (int64, error) {
	e.mu.Lock()
	cached := e.highWater
	e.mu.Unlock()
	if cached > 0 {
		return cached, nil
	}
	bootstrap, err := fetchPublicMirrorBootstrap(ctx)
	if err != nil {
		return 0, err
	}
	return bootstrap.Snapshot.HighWaterSequence, nil
}

func (e *remoteGatewayCampaignFeedExporter) ExportBatch(ctx context.Context, records []evaluation.CampaignPublicFeedRecord) error {
	batch := make([]models.PublicFeedRecord, len(records))
	for index, record := range records {
		batch[index] = models.PublicFeedRecord{
			Sequence:    record.Sequence,
			RecordType:  models.PublicFeedRecordTypeProjection,
			RecordHash:  record.RecordHash,
			RecordBytes: record.RecordBytes,
		}
	}
	body, err := e.client.Post(constants.APIPaths.PublicFeedBatches, batch)
	if err != nil {
		return fmt.Errorf("campaign publication: gateway export batch: %w", err)
	}
	var resp models.PublicFeedExportBatchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	e.mu.Lock()
	e.highWater = resp.HighWaterSequence
	e.mu.Unlock()
	return nil
}

func isGatewayHealthy() bool {
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", constants.Ports.OperatorHttp)
	client := &http.Client{Timeout: 2 * time.Second} //nolint:gosec
	resp, err := client.Get(healthURL) //nolint:noctx
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// publicMirrorBootstrapURL overrides the default mirror bootstrap URL in tests.
var publicMirrorBootstrapURL string

func fetchPublicMirrorBootstrap(ctx context.Context) (models.PublicFeedBootstrap, error) {
	bootstrapURL := publicMirrorBootstrapURL
	if bootstrapURL == "" {
		bootstrapURL = fmt.Sprintf("http://127.0.0.1:%d/bootstrap", constants.PublicSpectatorPublicPort)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bootstrapURL, nil)
	if err != nil {
		return models.PublicFeedBootstrap{}, err
	}
	client := &http.Client{Timeout: 5 * time.Second} //nolint:gosec
	resp, err := client.Do(req)
	if err != nil {
		return models.PublicFeedBootstrap{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return models.PublicFeedBootstrap{}, fmt.Errorf("public mirror bootstrap: status %d", resp.StatusCode)
	}
	var bootstrap models.PublicFeedBootstrap
	if err := json.NewDecoder(resp.Body).Decode(&bootstrap); err != nil {
		return models.PublicFeedBootstrap{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return bootstrap, nil
}

func newCampaignFeedExporter(cmd context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) (evaluation.CampaignFeedExporter, error) {
	if !isGatewayHealthy() {
		return nil, fmt.Errorf("campaign publication: gateway is not healthy; start g8e-gateway with --public-spectator")
	}
	client, err := defaultAPIClientFactory(fileSvc, cfg)
	if err != nil {
		return nil, fmt.Errorf("campaign publication: create gateway client: %w", err)
	}
	return &remoteGatewayCampaignFeedExporter{client: client}, nil
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	return e.fetchGatewayHighWater(ctx)
}

func (e *remoteGatewayCampaignFeedExporter) ExportBatch(ctx context.Context, records []evaluation.CampaignPublicFeedRecord) error {
	for attempt := 0; attempt < 2; attempt++ {
		err := e.exportBatchOnce(ctx, records)
		if err == nil {
			return nil
		}
		if attempt == 1 || !isPublicFeedSequenceOutOfOrder(err) {
			return err
		}
		e.invalidateHighWater()
		nextSequence, err := e.HighWaterSequence(ctx)
		if err != nil {
			return fmt.Errorf("campaign publication: gateway export batch: refresh high water: %w", err)
		}
		nextSequence++
		for index := range records {
			records[index].Sequence = nextSequence + int64(index)
		}
	}
	return fmt.Errorf("campaign publication: gateway export batch: sequence retry exhausted")
}

func (e *remoteGatewayCampaignFeedExporter) exportBatchOnce(ctx context.Context, records []evaluation.CampaignPublicFeedRecord) error {
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

func (e *remoteGatewayCampaignFeedExporter) fetchGatewayHighWater(ctx context.Context) (int64, error) {
	body, err := e.client.Get(constants.APIPaths.PublicFeedSnapshot)
	if err != nil {
		if isPublicFeedSnapshotNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("campaign publication: gateway public feed snapshot: %w", err)
	}
	var snapshot models.PublicFeedSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return 0, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	e.mu.Lock()
	if snapshot.HighWaterSequence > e.highWater {
		e.highWater = snapshot.HighWaterSequence
	}
	highWater := e.highWater
	e.mu.Unlock()
	return highWater, nil
}

func (e *remoteGatewayCampaignFeedExporter) invalidateHighWater() {
	e.mu.Lock()
	e.highWater = 0
	e.mu.Unlock()
}

func isPublicFeedSequenceOutOfOrder(err error) bool {
	return err != nil && strings.Contains(err.Error(), constants.ErrPublicFeedSequenceOutOfOrder.Error())
}

func isPublicFeedSnapshotNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), constants.ErrPublicFeedSnapshotNotFound.Error())
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

func isHostPublicFeedConfigured(ctx context.Context, fileSvc fs.RuntimeFileService) bool {
	exists, err := fileSvc.FileExists(ctx, constants.PublicFeedExportConfigPath)
	return err == nil && exists
}

func shouldPublishViaGateway(ctx context.Context, fileSvc fs.RuntimeFileService) bool {
	return !isHostPublicFeedConfigured(ctx, fileSvc) && isGatewayHealthy()
}

func gatewayPublisherStatus(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) (models.PublicPublisherStatus, error) {
	exporter, err := newCampaignFeedExporter(ctx, fileSvc, cfg)
	if err != nil {
		return models.PublicPublisherStatus{}, err
	}
	bootstrap, err := fetchPublicMirrorBootstrap(ctx)
	if err != nil {
		return models.PublicPublisherStatus{}, err
	}
	highWater, err := exporter.HighWaterSequence(ctx)
	if err != nil {
		return models.PublicPublisherStatus{}, err
	}
	return models.PublicPublisherStatus{
		Enabled:           true,
		MirrorOrigin:      fmt.Sprintf("http://127.0.0.1:%d", constants.PublicSpectatorPublicPort),
		SourceID:          bootstrap.Snapshot.SourceID,
		HighWaterSequence: highWater,
		FeedChainHash:     bootstrap.Snapshot.FeedChainHash,
		BatchCount:        bootstrap.Snapshot.BatchCount,
	}, nil
}

func publishJSONLViaGateway(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config, recordsPath string) (int, error) {
	defaults := models.DefaultPublicExportConfig()
	inputs, err := readPublicRecordInputs(recordsPath, defaults.BatchMaxRecords, defaults.BatchMaxBytes)
	if err != nil {
		return 0, err
	}
	exporter, err := newCampaignFeedExporter(ctx, fileSvc, cfg)
	if err != nil {
		return 0, err
	}
	nextSequence, err := exporter.HighWaterSequence(ctx)
	if err != nil {
		return 0, err
	}
	nextSequence++

	published := 0
	for start := 0; start < len(inputs); start += constants.PublicFeedBatchMaxRecords {
		end := start + constants.PublicFeedBatchMaxRecords
		if end > len(inputs) {
			end = len(inputs)
		}
		chunk := inputs[start:end]
		records := make([]evaluation.CampaignPublicFeedRecord, len(chunk))
		for index, input := range chunk {
			digest := sha256.Sum256([]byte(input.RecordBytes))
			records[index] = evaluation.CampaignPublicFeedRecord{
				Sequence:    nextSequence + int64(index),
				RecordHash:  hex.EncodeToString(digest[:]),
				RecordBytes: input.RecordBytes,
			}
		}
		if err := exporter.ExportBatch(ctx, records); err != nil {
			return published, err
		}
		published += len(records)
		nextSequence = records[len(records)-1].Sequence + 1
	}
	return published, nil
}

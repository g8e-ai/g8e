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
	"strconv"
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
		if attempt == 1 || !isPublicFeedPublicationRetryable(err) {
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

func isPublicFeedPublicationRetryable(err error) bool {
	return isPublicFeedSequenceOutOfOrder(err) || isPublicFeedOutboxPublicationError(err)
}

func isPublicFeedOutboxPublicationError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, constants.ErrPublicFeedOutboxEquivocation.Error()) ||
		strings.Contains(message, constants.ErrPublicFeedOutboxCorrupt.Error()) ||
		strings.Contains(message, constants.ErrPublicFeedHashChainMismatch.Error())
}

func (e *remoteGatewayCampaignFeedExporter) exportBatchOnce(ctx context.Context, records []evaluation.CampaignPublicFeedRecord) error {
	batch := make([]models.PublicFeedRecord, len(records))
	for index, record := range records {
		recordType := record.RecordType
		if recordType == "" {
			recordType = models.PublicFeedRecordTypeProjection
		}
		batch[index] = models.PublicFeedRecord{
			Sequence:    record.Sequence,
			RecordType:  recordType,
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
	if gatewayHealthCheck != nil {
		return gatewayHealthCheck()
	}
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", constants.Ports.OperatorHttp)
	client := &http.Client{Timeout: 2 * time.Second} //nolint:gosec
	resp, err := client.Get(healthURL)               //nolint:noctx
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// publicMirrorBootstrapURL overrides the default mirror bootstrap URL in tests.
var publicMirrorBootstrapURL string

// publicMirrorHistoryURL overrides the default mirror history URL in tests.
var publicMirrorHistoryURL string

// gatewayHealthCheck overrides isGatewayHealthy in tests.
var gatewayHealthCheck func() bool

type httpCampaignMirrorProbe struct {
	client     *http.Client
	mu         sync.Mutex
	indexed    bool
	datasetIDs map[string]struct{}
}

func newHTTPCampaignMirrorProbe(ctx context.Context) evaluation.CampaignMirrorProbe {
	return &httpCampaignMirrorProbe{client: &http.Client{Timeout: 5 * time.Second}} //nolint:gosec
}

func (p *httpCampaignMirrorProbe) DatasetPresent(ctx context.Context, datasetID string) (bool, error) {
	if p == nil || datasetID == "" {
		return false, fmt.Errorf("campaign mirror probe: missing dataset id")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.indexed {
		if err := p.indexDatasets(ctx); err != nil {
			return false, err
		}
	}
	_, present := p.datasetIDs[datasetID]
	return present, nil
}

func (p *httpCampaignMirrorProbe) indexDatasets(ctx context.Context) error {
	bootstrap, err := fetchPublicMirrorBootstrapWithClient(ctx, p.client)
	if err != nil {
		return err
	}
	p.datasetIDs = make(map[string]struct{})
	indexMirrorDatasets(p.datasetIDs, bootstrap.RecentProjections)
	if bootstrap.Snapshot.HighWaterSequence == 0 {
		p.indexed = true
		return nil
	}
	cursor := ""
	for page := 0; page < 32; page++ {
		history, err := fetchPublicMirrorHistory(ctx, p.client, bootstrap.Snapshot.SourceID, cursor, 100)
		if err != nil {
			return err
		}
		indexMirrorDatasets(p.datasetIDs, history.Items)
		if !history.HasMore || history.Cursor == "" {
			p.indexed = true
			return nil
		}
		cursor = history.Cursor
	}
	return fmt.Errorf("campaign mirror probe: history exceeds 3200 retained records: %w", constants.ErrPublicFeedHistoryIncomplete)
}

func indexMirrorDatasets(datasetIDs map[string]struct{}, projections []models.PublicFeedObject) {
	for _, item := range projections {
		kind, _ := item.StringField("kind")
		if kind == "catalog_snapshot" {
			if datasetID, ok := item.StringField("dataset_id"); ok && datasetID != "" {
				datasetIDs[datasetID] = struct{}{}
			}
		}
		if recordBytes, ok := item["record"]; ok {
			var record models.PublicFeedObject
			if err := json.Unmarshal(recordBytes, &record); err == nil {
				indexMirrorDatasets(datasetIDs, []models.PublicFeedObject{record})
			}
		}
	}
}

func mirrorProjectionHasDataset(projections []models.PublicFeedObject, datasetID string) bool {
	for _, item := range projections {
		if mirrorCatalogDatasetPresent(item, datasetID) {
			return true
		}
	}
	return false
}

func mirrorCatalogDatasetPresent(item models.PublicFeedObject, datasetID string) bool {
	kind, _ := item.StringField("kind")
	if kind == "catalog_snapshot" {
		if currentDatasetID, ok := item.StringField("dataset_id"); ok && currentDatasetID == datasetID {
			return true
		}
	}
	if recordBytes, ok := item["record"]; ok {
		var record models.PublicFeedObject
		if err := json.Unmarshal(recordBytes, &record); err == nil {
			return mirrorCatalogDatasetPresent(record, datasetID)
		}
	}
	return false
}

func fetchPublicMirrorHistory(ctx context.Context, client *http.Client, sourceID, cursor string, limit int) (models.PublicFeedCursorPage, error) {
	historyURL := publicMirrorHistoryURL
	if historyURL == "" {
		historyURL = fmt.Sprintf("http://127.0.0.1:%d/history", constants.PublicSpectatorPublicPort)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, historyURL, nil)
	if err != nil {
		return models.PublicFeedCursorPage{}, err
	}
	query := req.URL.Query()
	if sourceID != "" {
		query.Set("source", sourceID)
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	query.Set("kind", "catalog_snapshot")
	req.URL.RawQuery = query.Encode()
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second} //nolint:gosec
	}
	resp, err := doPublicMirrorRead(ctx, client, req, "public mirror history")
	if err != nil {
		return models.PublicFeedCursorPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return models.PublicFeedCursorPage{}, fmt.Errorf("public mirror history: status %d", resp.StatusCode)
	}
	var page models.PublicFeedCursorPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return models.PublicFeedCursorPage{}, fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}
	return page, nil
}

func fetchPublicMirrorBootstrap(ctx context.Context) (models.PublicFeedBootstrap, error) {
	return fetchPublicMirrorBootstrapWithClient(ctx, &http.Client{Timeout: 5 * time.Second}) //nolint:gosec
}

func fetchPublicMirrorBootstrapWithClient(ctx context.Context, client *http.Client) (models.PublicFeedBootstrap, error) {
	bootstrapURL := publicMirrorBootstrapURL
	if bootstrapURL == "" {
		bootstrapURL = fmt.Sprintf("http://127.0.0.1:%d/bootstrap", constants.PublicSpectatorPublicPort)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bootstrapURL, nil)
	if err != nil {
		return models.PublicFeedBootstrap{}, err
	}
	resp, err := doPublicMirrorRead(ctx, client, req, "public mirror bootstrap")
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

func doPublicMirrorRead(ctx context.Context, client *http.Client, req *http.Request, operation string) (*http.Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		_ = resp.Body.Close()
		if attempt == 1 {
			return nil, fmt.Errorf("%s: status %d: %w", operation, resp.StatusCode, constants.ErrPublicFeedRateLimited)
		}
		if err := waitForPublicMirrorRetry(ctx, resp.Header.Get("Retry-After")); err != nil {
			return nil, err
		}
	}
	return nil, constants.ErrPublicFeedMaxRetriesExceeded
}

func waitForPublicMirrorRetry(ctx context.Context, retryAfter string) error {
	delay := time.Second
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		delay = time.Duration(seconds) * time.Second
	} else if retryAt, err := http.ParseTime(retryAfter); err == nil {
		delay = time.Until(retryAt)
		if delay < 0 {
			delay = 0
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("public mirror rate-limit wait: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
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

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package gateway

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/testutil"
)

const capacityTrustedProxyCIDR = "127.0.0.0/8"

type capacityMirrorEnv struct {
	helper   *mirrorTestEnv
	baseURL  string
	sourceID string
	client   *http.Client
}

func newCapacityMirrorEnv(t *testing.T, recordCount int) *capacityMirrorEnv {
	t.Helper()
	fileSvc := newProducerFileSvc(t)
	mirror, err := NewPublicMirrorServer(testutil.NewTestLogger(), NewRuntimePublicMirrorStore(fileSvc), PublicMirrorConfig{
		TrustedProxyCIDRs: []string{capacityTrustedProxyCIDR},
	})
	require.NoError(t, err)

	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	keyID := "capacity-key"
	sourceID := "capacity-source"
	require.NoError(t, mirror.RegisterSourceKey(context.Background(), sourceID, keyID, pub))

	helper := &mirrorTestEnv{
		t:        t,
		mirror:   mirror,
		pub:      pub,
		priv:     priv,
		keyID:    keyID,
		sourceID: sourceID,
		fileSvc:  fileSvc,
	}
	records := make([]models.PublicFeedRecord, recordCount)
	for index := range records {
		records[index] = helper.makeRecord(int64(index+1), applyProjectionDefaults(models.NewPublicFeedObject(map[string]string{
			"kind":       "catalog_snapshot",
			"dataset_id": fmt.Sprintf("dataset-%d", index%5),
		})))
	}
	batch := helper.buildBatch(records, constants.PublicFeedZeroHashHex)
	status, ingestResp := sendIngestToHandler(t, mirror.Handler(), batch)
	require.Equal(t, http.StatusOK, status)
	require.True(t, ingestResp.Accepted)

	server := httptest.NewServer(mirror.Handler())
	t.Cleanup(server.Close)
	helper.server = server
	helper.client = server.Client()

	return &capacityMirrorEnv{
		helper:   helper,
		baseURL:  server.URL,
		sourceID: sourceID,
		client:   server.Client(),
	}
}

func sendIngestToHandler(t *testing.T, handler http.Handler, batch models.PublicFeedBatch) (int, models.PublicIngestResponse) {
	t.Helper()
	bodyBytes, err := json.Marshal(models.PublicIngestRequest{Batch: batch})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/ingest", bytes.NewReader(bodyBytes))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var ingestResp models.PublicIngestResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &ingestResp))
	return recorder.Code, ingestResp
}

func capacitySyntheticClientAddress(globalIndex int) string {
	return net.IPv4(198, 18, byte(globalIndex/254), byte(globalIndex%254+1)).String()
}

func (env *capacityMirrorEnv) coldLifecycle(ctx context.Context, clientIP string) (outcome string, rateLimited bool) {
	origin := env.baseURL
	source := env.sourceID

	bootstrap, status, err := capacityGetJSON[bootstrapCapacityResponse](ctx, env.client, origin+"/bootstrap", clientIP)
	if err != nil || status != http.StatusOK {
		return "bootstrap", status == http.StatusTooManyRequests
	}
	source = bootstrap.Snapshot.SourceID
	cursor := int64(0)
	for _, record := range bootstrap.RecentProjections {
		if record.Sequence > cursor {
			cursor = record.Sequence
		}
	}
	for range 10 {
		for range 100 {
			history, status, err := capacityGetJSON[historyCapacityResponse](ctx, env.client, capacityEndpoint(origin, "/history", url.Values{
				"source": {source},
				"cursor": {strconv.FormatInt(cursor, 10)},
				"limit":  {"500"},
			}), clientIP)
			if err != nil || status != http.StatusOK {
				return "history", status == http.StatusTooManyRequests
			}
			if len(history.Items) > 0 {
				cursor = history.Items[len(history.Items)-1].Sequence
			}
			if !history.HasMore {
				break
			}
			if len(history.Items) == 0 {
				return "history_stalled", false
			}
		}
		snapshot, status, err := capacityGetJSON[snapshotCapacityResponse](ctx, env.client, capacityEndpoint(origin, "/snapshot", url.Values{"source": {source}}), clientIP)
		if err != nil || status != http.StatusOK {
			return "snapshot", status == http.StatusTooManyRequests
		}
		if snapshot.HighWaterSequence == cursor {
			survived, status, outcome := env.openStream(ctx, source, cursor, clientIP, 2*time.Second)
			if !survived && outcome != "complete" {
				return outcome, status == http.StatusTooManyRequests
			}
			return outcome, status == http.StatusTooManyRequests
		}
	}
	return "sequence_divergence", false
}

func (env *capacityMirrorEnv) openStream(ctx context.Context, source string, cursor int64, clientIP string, hold time.Duration) (survived bool, status int, outcome string) {
	streamCtx, cancel := context.WithTimeout(ctx, hold+10*time.Second)
	defer cancel()
	address := capacityEndpoint(env.baseURL, "/stream", url.Values{
		"source":   {source},
		"since_id": {strconv.FormatInt(cursor, 10)},
	})
	request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, address, nil)
	if err != nil {
		return false, 0, "stream_request"
	}
	request.Header.Set("Accept", "text/event-stream")
	if clientIP != "" {
		request.Header.Set("CF-Connecting-IP", clientIP)
	}
	response, err := env.client.Do(request)
	if err != nil {
		return false, 0, "stream"
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return false, response.StatusCode, "stream"
	}
	deadline := time.Now().Add(hold)
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		if !time.Now().Before(deadline) {
			return true, response.StatusCode, "complete"
		}
	}
	if !time.Now().Before(deadline) {
		return true, response.StatusCode, "complete"
	}
	return false, response.StatusCode, "stream_disconnected"
}

type bootstrapCapacityResponse struct {
	Snapshot          snapshotCapacityResponse   `json:"snapshot"`
	RecentProjections []projectionCapacityCursor `json:"recent_projections"`
}

type historyCapacityResponse struct {
	Items   []projectionCapacityCursor `json:"items"`
	HasMore bool                       `json:"has_more"`
}

type snapshotCapacityResponse struct {
	SourceID          string `json:"source_id"`
	HighWaterSequence int64  `json:"high_water_sequence"`
}

type projectionCapacityCursor struct {
	Sequence int64 `json:"sequence"`
}

func capacityEndpoint(origin, path string, query url.Values) string {
	address := strings.TrimSuffix(origin, "/") + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	return address
}

func capacityGetJSON[T any](ctx context.Context, client *http.Client, address, clientIP string) (T, int, error) {
	var value T
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return value, 0, err
	}
	request.Header.Set("Accept", "application/json")
	if clientIP != "" {
		request.Header.Set("CF-Connecting-IP", clientIP)
	}
	response, err := client.Do(request)
	if err != nil {
		return value, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return value, response.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<20)).Decode(&value); err != nil {
		return value, response.StatusCode, err
	}
	return value, response.StatusCode, nil
}

func TestPublicSpectatorColdLoad_ThreeHundredDistinctClientsReconcile(t *testing.T) {
	env := newCapacityMirrorEnv(t, 60)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const clients = 300
	results := make(chan string, clients)
	var rateLimited atomic.Int32
	var wg sync.WaitGroup
	wg.Add(clients)
	for index := range clients {
		go func(clientIndex int) {
			defer wg.Done()
			outcome, limited := env.coldLifecycle(ctx, capacitySyntheticClientAddress(clientIndex))
			results <- outcome
			if limited {
				rateLimited.Add(1)
			}
		}(index)
	}
	wg.Wait()
	close(results)

	outcomes := make(map[string]int)
	for outcome := range results {
		outcomes[outcome]++
	}
	assert.Zero(t, rateLimited.Load(), "distinct client identities must not share one rate window")
	assert.Equal(t, clients, outcomes["complete"], "outcomes: %v", outcomes)
}

func TestPublicSpectatorPublicationDeliversDuringSSEHold(t *testing.T) {
	env := newCapacityMirrorEnv(t, 5)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const subscribers = 32
	hold := 5 * time.Second
	var delivered atomic.Int32
	var wg sync.WaitGroup
	wg.Add(subscribers)
	for index := range subscribers {
		go func(clientIndex int) {
			defer wg.Done()
			address := capacityEndpoint(env.baseURL, "/stream", url.Values{
				"source":   {env.sourceID},
				"since_id": {"5"},
			})
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
			if err != nil {
				return
			}
			request.Header.Set("Accept", "text/event-stream")
			request.Header.Set("CF-Connecting-IP", capacitySyntheticClientAddress(clientIndex))
			response, err := env.client.Do(request)
			if err != nil || response.StatusCode != http.StatusOK {
				if response != nil {
					response.Body.Close()
				}
				return
			}
			defer response.Body.Close()
			scanner := bufio.NewScanner(response.Body)
			scanner.Buffer(make([]byte, 64<<10), 1<<20)
			deadline := time.Now().Add(hold)
			for scanner.Scan() {
				if strings.Contains(scanner.Text(), "dataset-publish") {
					delivered.Add(1)
					return
				}
				if !time.Now().Before(deadline) {
					return
				}
			}
		}(index)
	}

	time.Sleep(500 * time.Millisecond)
	var snapshot models.PublicFeedSnapshot
	require.Equal(t, http.StatusOK, env.helper.getJSON("/snapshot?source="+env.sourceID, &snapshot))
	publishRecord := env.helper.makeRecord(6, applyProjectionDefaults(models.NewPublicFeedObject(map[string]string{
		"kind":       "catalog_snapshot",
		"dataset_id": "dataset-publish",
	})))
	batch := env.helper.buildBatch([]models.PublicFeedRecord{publishRecord}, snapshot.FeedChainHash)
	status, ingestResp := sendIngestToHandler(t, env.helper.mirror.Handler(), batch)
	require.Equal(t, http.StatusOK, status)
	require.True(t, ingestResp.Accepted)

	wg.Wait()
	assert.GreaterOrEqual(t, int(delivered.Load()), subscribers/2, "connected subscribers should receive the published record during hold")
}

func TestPublicSpectatorStreamHold_ThousandDistinctClientsSurvive(t *testing.T) {
	env := newCapacityMirrorEnv(t, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	const clients = constants.PublicFeedSSEMaxSubscribers
	hold := 3 * time.Second
	results := make(chan bool, clients)
	var wg sync.WaitGroup
	wg.Add(clients)
	for index := range clients {
		go func(clientIndex int) {
			defer wg.Done()
			survived, status, _ := env.openStream(ctx, env.sourceID, 0, capacitySyntheticClientAddress(clientIndex), hold)
			results <- survived && status == http.StatusOK
		}(index)
	}
	wg.Wait()
	close(results)

	survived := 0
	for ok := range results {
		if ok {
			survived++
		}
	}
	assert.Equal(t, clients, survived)
}

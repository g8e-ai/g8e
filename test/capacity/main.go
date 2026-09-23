// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxResponseBytes = 64 << 20
	historyLimit     = 500
	maxHistoryPages  = 100
	maxHistoryRounds = 10
)

type options struct {
	target             string
	mode               string
	clients            int
	hold               time.Duration
	requestTimeout     time.Duration
	allowPublic        bool
	syntheticClientIPs bool
	dockerContainer    string
	sampleInterval     time.Duration
}

type bootstrapResponse struct {
	Snapshot          snapshotResponse   `json:"snapshot"`
	RecentProjections []projectionCursor `json:"recent_projections"`
}

type projectionCursor struct {
	Sequence int64 `json:"sequence"`
}

type historyResponse struct {
	Items   []projectionCursor `json:"items"`
	HasMore bool               `json:"has_more"`
}

type snapshotResponse struct {
	SourceID          string `json:"source_id"`
	HighWaterSequence int64  `json:"high_water_sequence"`
	FeedChainHash     string `json:"feed_chain_hash"`
	BatchCount        int64  `json:"batch_count"`
}

type requestResult struct {
	Stage   string
	Status  int
	Latency time.Duration
}

type clientResult struct {
	Outcome        string
	Requests       []requestResult
	StreamSurvived bool
}

type dockerStats struct {
	Container string `json:"Container"`
	CPUPerc   string `json:"CPUPerc"`
	MemUsage  string `json:"MemUsage"`
	MemPerc   string `json:"MemPerc"`
	NetIO     string `json:"NetIO"`
	BlockIO   string `json:"BlockIO"`
	PIDs      string `json:"PIDs"`
}

type resourceSample struct {
	ObservedAt time.Time   `json:"observed_at"`
	Values     dockerStats `json:"values"`
	Error      string      `json:"error,omitempty"`
}

type latencySummary struct {
	P50 float64 `json:"p50_ms"`
	P95 float64 `json:"p95_ms"`
	P99 float64 `json:"p99_ms"`
	Max float64 `json:"max_ms"`
}

type summary struct {
	Target             string           `json:"target"`
	Mode               string           `json:"mode"`
	Clients            int              `json:"clients"`
	ElapsedSeconds     float64          `json:"elapsed_seconds"`
	Outcomes           map[string]int   `json:"outcomes"`
	Statuses           map[string]int   `json:"statuses"`
	Latency            latencySummary   `json:"latency"`
	StreamsEstablished int              `json:"streams_established"`
	StreamsSurvived    int              `json:"streams_survived"`
	ResourceSamples    []resourceSample `json:"resource_samples,omitempty"`
}

func main() {
	opts := parseFlags()
	if err := validateOptions(opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	result, err := run(context.Background(), opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("capacity: encode summary: %w", err))
		os.Exit(1)
	}
	fmt.Println(string(encoded))
	if result.Outcomes["complete"] != opts.clients || result.StreamsSurvived != opts.clients {
		os.Exit(1)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.target, "target", "", "public mirror origin")
	flag.StringVar(&opts.mode, "mode", "cold", "cold or stream")
	flag.IntVar(&opts.clients, "clients", 300, "concurrent client count")
	flag.DurationVar(&opts.hold, "hold", 5*time.Minute, "SSE hold duration")
	flag.DurationVar(&opts.requestTimeout, "request-timeout", 30*time.Second, "bounded request timeout")
	flag.BoolVar(&opts.allowPublic, "allow-public", false, "allow a non-loopback target")
	flag.BoolVar(&opts.syntheticClientIPs, "synthetic-client-ips", false, "send distinct test client addresses to a loopback trusted proxy")
	flag.StringVar(&opts.dockerContainer, "docker-container", "", "optional container sampled with docker stats")
	flag.DurationVar(&opts.sampleInterval, "sample-interval", 30*time.Second, "resource sample interval")
	flag.Parse()
	return opts
}

func validateOptions(opts options) error {
	if opts.target == "" {
		return fmt.Errorf("capacity: --target is required")
	}
	parsed, err := url.Parse(opts.target)
	if err != nil {
		return fmt.Errorf("capacity: parse target: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("capacity: target scheme must be http or https")
	}
	if parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return fmt.Errorf("capacity: target must be a bare origin")
	}
	isLoopback := loopbackHost(parsed.Hostname())
	if !opts.allowPublic && !isLoopback {
		return fmt.Errorf("capacity: public target requires --allow-public")
	}
	if opts.syntheticClientIPs && !isLoopback {
		return fmt.Errorf("capacity: synthetic client addresses require a loopback target")
	}
	if opts.mode != "cold" && opts.mode != "stream" {
		return fmt.Errorf("capacity: --mode must be cold or stream")
	}
	if opts.clients <= 0 || opts.hold <= 0 || opts.requestTimeout <= 0 || opts.sampleInterval <= 0 {
		return fmt.Errorf("capacity: clients and durations must be positive")
	}
	return nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func syntheticClientAddress(index int) string {
	return net.IPv4(198, 18, byte(index/254), byte(index%254+1)).String()
}

func run(ctx context.Context, opts options) (summary, error) {
	origin := strings.TrimSuffix(opts.target, "/")
	var source string
	var cursor int64
	if opts.mode == "stream" {
		client := newClient(opts.requestTimeout)
		bootstrap, requestInfo, err := fetchBootstrap(ctx, client, origin, "")
		if err != nil {
			return summary{}, fmt.Errorf("capacity: stream bootstrap: %w", err)
		}
		if requestInfo.Status != http.StatusOK {
			return summary{}, fmt.Errorf("capacity: stream bootstrap returned %d", requestInfo.Status)
		}
		source = bootstrap.Snapshot.SourceID
		cursor = bootstrap.Snapshot.HighWaterSequence
		for _, record := range bootstrap.RecentProjections {
			if record.Sequence > cursor {
				cursor = record.Sequence
			}
		}
	}

	started := time.Now()
	results := make(chan clientResult, opts.clients)
	resourceCtx, stopResources := context.WithCancel(ctx)
	resourceResults := make(chan resourceSample, 128)
	if opts.dockerContainer != "" {
		go sampleResources(resourceCtx, opts.dockerContainer, opts.sampleInterval, resourceResults)
	}
	var wg sync.WaitGroup
	wg.Add(opts.clients)
	for index := range opts.clients {
		clientIP := ""
		if opts.syntheticClientIPs {
			clientIP = syntheticClientAddress(index)
		}
		go func() {
			defer wg.Done()
			if opts.mode == "stream" {
				results <- runStreamClient(ctx, opts, origin, source, cursor, clientIP)
				return
			}
			results <- runColdClient(ctx, opts, origin, clientIP)
		}()
	}
	wg.Wait()
	close(results)
	stopResources()

	all := make([]clientResult, 0, opts.clients)
	for result := range results {
		all = append(all, result)
	}
	resources := make([]resourceSample, 0)
	if opts.dockerContainer != "" {
		for {
			select {
			case sample := <-resourceResults:
				resources = append(resources, sample)
			default:
				return summarize(opts, time.Since(started), all, resources), nil
			}
		}
	}
	return summarize(opts, time.Since(started), all, resources), nil
}

func runColdClient(ctx context.Context, opts options, origin, clientIP string) clientResult {
	client := newClient(opts.requestTimeout)
	bootstrap, requestInfo, err := fetchBootstrap(ctx, client, origin, clientIP)
	result := clientResult{Requests: []requestResult{requestInfo}}
	if err != nil || requestInfo.Status != http.StatusOK {
		result.Outcome = "bootstrap"
		return result
	}
	source := bootstrap.Snapshot.SourceID
	cursor := int64(0)
	for _, record := range bootstrap.RecentProjections {
		if record.Sequence > cursor {
			cursor = record.Sequence
		}
	}
	for range maxHistoryRounds {
		for range maxHistoryPages {
			page, requestInfo, pageErr := fetchHistory(ctx, client, origin, source, cursor, clientIP)
			result.Requests = append(result.Requests, requestInfo)
			if pageErr != nil || requestInfo.Status != http.StatusOK {
				result.Outcome = "history"
				return result
			}
			if len(page.Items) > 0 {
				cursor = page.Items[len(page.Items)-1].Sequence
			}
			if !page.HasMore {
				break
			}
			if len(page.Items) == 0 {
				result.Outcome = "history_stalled"
				return result
			}
		}
		snapshot, requestInfo, snapshotErr := fetchSnapshot(ctx, client, origin, source, clientIP)
		result.Requests = append(result.Requests, requestInfo)
		if snapshotErr != nil || requestInfo.Status != http.StatusOK {
			result.Outcome = "snapshot"
			return result
		}
		if snapshot.HighWaterSequence == cursor {
			streamResult := openStream(ctx, opts, client, origin, source, cursor, clientIP)
			result.Requests = append(result.Requests, streamResult.Requests...)
			result.StreamSurvived = streamResult.StreamSurvived
			result.Outcome = streamResult.Outcome
			return result
		}
	}
	result.Outcome = "sequence_divergence"
	return result
}

func runStreamClient(ctx context.Context, opts options, origin, source string, cursor int64, clientIP string) clientResult {
	return openStream(ctx, opts, newClient(opts.requestTimeout), origin, source, cursor, clientIP)
}

func openStream(ctx context.Context, opts options, client *http.Client, origin, source string, cursor int64, clientIP string) clientResult {
	streamCtx, cancel := context.WithCancel(ctx)
	path := endpoint(origin, "/stream", url.Values{"source": {source}, "since_id": {strconv.FormatInt(cursor, 10)}})
	request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, path, nil)
	if err != nil {
		cancel()
		return clientResult{Outcome: "stream_request"}
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("User-Agent", "g8e-public-capacity/1.0")
	if clientIP != "" {
		request.Header.Set("CF-Connecting-IP", clientIP)
	}
	started := time.Now()
	response, err := client.Do(request)
	latency := time.Since(started)
	if err != nil {
		cancel()
		return clientResult{Outcome: "stream", Requests: []requestResult{{Stage: "stream", Latency: latency}}}
	}
	defer response.Body.Close()
	streamRequest := requestResult{Stage: "stream", Status: response.StatusCode, Latency: latency}
	if response.StatusCode != http.StatusOK {
		cancel()
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return clientResult{Outcome: "stream", Requests: []requestResult{streamRequest}}
	}
	deadline := time.Now().Add(opts.hold)
	timer := time.AfterFunc(opts.hold, cancel)
	defer timer.Stop()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		if !time.Now().Before(deadline) {
			return clientResult{Outcome: "complete", Requests: []requestResult{streamRequest}, StreamSurvived: true}
		}
	}
	if !time.Now().Before(deadline) {
		return clientResult{Outcome: "complete", Requests: []requestResult{streamRequest}, StreamSurvived: true}
	}
	return clientResult{Outcome: "stream_disconnected", Requests: []requestResult{streamRequest}}
}

func fetchBootstrap(ctx context.Context, client *http.Client, origin, clientIP string) (bootstrapResponse, requestResult, error) {
	var value bootstrapResponse
	result, err := getJSON(ctx, client, endpoint(origin, "/bootstrap", nil), "bootstrap", clientIP, &value)
	return value, result, err
}

func fetchHistory(ctx context.Context, client *http.Client, origin, source string, cursor int64, clientIP string) (historyResponse, requestResult, error) {
	var value historyResponse
	query := url.Values{"source": {source}, "cursor": {strconv.FormatInt(cursor, 10)}, "limit": {strconv.Itoa(historyLimit)}}
	result, err := getJSON(ctx, client, endpoint(origin, "/history", query), "history", clientIP, &value)
	return value, result, err
}

func fetchSnapshot(ctx context.Context, client *http.Client, origin, source, clientIP string) (snapshotResponse, requestResult, error) {
	var value snapshotResponse
	result, err := getJSON(ctx, client, endpoint(origin, "/snapshot", url.Values{"source": {source}}), "snapshot", clientIP, &value)
	return value, result, err
}

type mirrorResponse interface {
	bootstrapResponse | historyResponse | snapshotResponse
}

func getJSON[T mirrorResponse](ctx context.Context, client *http.Client, address, stage, clientIP string, value *T) (requestResult, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return requestResult{Stage: stage}, fmt.Errorf("%s request: %w", stage, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "g8e-public-capacity/1.0")
	if clientIP != "" {
		request.Header.Set("CF-Connecting-IP", clientIP)
	}
	started := time.Now()
	response, err := client.Do(request)
	latency := time.Since(started)
	if err != nil {
		return requestResult{Stage: stage, Latency: latency}, fmt.Errorf("%s request: %w", stage, err)
	}
	defer response.Body.Close()
	result := requestResult{Stage: stage, Status: response.StatusCode, Latency: latency}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBytes))
		return result, nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(value); err != nil {
		return result, fmt.Errorf("%s response: %w", stage, err)
	}
	return result, nil
}

func endpoint(origin, path string, query url.Values) string {
	address := strings.TrimSuffix(origin, "/") + path
	if len(query) > 0 {
		address += "?" + query.Encode()
	}
	return address
}

func newClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 4
	transport.MaxIdleConnsPerHost = 4
	transport.IdleConnTimeout = timeout
	transport.ResponseHeaderTimeout = timeout
	transport.TLSHandshakeTimeout = timeout
	return &http.Client{Transport: transport}
}

func sampleResources(ctx context.Context, container string, interval time.Duration, results chan<- resourceSample) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		results <- dockerSample(ctx, container)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func dockerSample(ctx context.Context, container string) resourceSample {
	sample := resourceSample{ObservedAt: time.Now().UTC()}
	command := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}", container)
	encoded, err := command.Output()
	if err != nil {
		sample.Error = err.Error()
		return sample
	}
	if err := json.Unmarshal(encoded, &sample.Values); err != nil {
		sample.Error = err.Error()
	}
	return sample
}

func summarize(opts options, elapsed time.Duration, results []clientResult, resources []resourceSample) summary {
	outcomes := make(map[string]int)
	statuses := make(map[string]int)
	latencies := make([]time.Duration, 0)
	established := 0
	survived := 0
	for _, result := range results {
		outcomes[result.Outcome]++
		if result.StreamSurvived {
			survived++
		}
		for _, request := range result.Requests {
			statuses[request.Stage+":"+strconv.Itoa(request.Status)]++
			latencies = append(latencies, request.Latency)
			if request.Stage == "stream" && request.Status == http.StatusOK {
				established++
			}
		}
	}
	return summary{
		Target:             opts.target,
		Mode:               opts.mode,
		Clients:            opts.clients,
		ElapsedSeconds:     elapsed.Seconds(),
		Outcomes:           outcomes,
		Statuses:           statuses,
		Latency:            summarizeLatency(latencies),
		StreamsEstablished: established,
		StreamsSurvived:    survived,
		ResourceSamples:    resources,
	}
}

func summarizeLatency(values []time.Duration) latencySummary {
	if len(values) == 0 {
		return latencySummary{}
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	milliseconds := func(index int) float64 { return float64(values[index]) / float64(time.Millisecond) }
	percentile := func(value float64) float64 {
		index := int(value * float64(len(values)))
		if index >= len(values) {
			index = len(values) - 1
		}
		return milliseconds(index)
	}
	middle := len(values) / 2
	median := milliseconds(middle)
	if len(values)%2 == 0 {
		median = (milliseconds(middle-1) + milliseconds(middle)) / 2
	}
	return latencySummary{P50: median, P95: percentile(0.95), P99: percentile(0.99), Max: milliseconds(len(values) - 1)}
}

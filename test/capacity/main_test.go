// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateOptions_RequiresExplicitSafeTargetAndValidBounds(t *testing.T) {
	valid := options{
		target:         "http://127.0.0.1:8082",
		mode:           "cold",
		clients:        300,
		hold:           5 * time.Minute,
		requestTimeout: 30 * time.Second,
		sampleInterval: 30 * time.Second,
	}
	tests := []struct {
		name    string
		mutate  func(options) options
		wantErr bool
	}{
		{name: "loopback target", mutate: func(value options) options { return value }},
		{name: "public target requires opt in", mutate: func(value options) options { value.target = "https://example.com"; return value }, wantErr: true},
		{name: "public target explicitly allowed", mutate: func(value options) options {
			value.target = "https://example.com"
			value.allowPublic = true
			return value
		}},
		{name: "target path rejected", mutate: func(value options) options { value.target = "http://127.0.0.1:8082/bootstrap"; return value }, wantErr: true},
		{name: "synthetic addresses allowed on loopback", mutate: func(value options) options { value.syntheticClientIPs = true; return value }},
		{name: "synthetic addresses rejected for public target", mutate: func(value options) options {
			value.target = "https://example.com"
			value.allowPublic = true
			value.syntheticClientIPs = true
			return value
		}, wantErr: true},
		{name: "unsupported mode rejected", mutate: func(value options) options { value.mode = "burst"; return value }, wantErr: true},
		{name: "zero clients rejected", mutate: func(value options) options { value.clients = 0; return value }, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOptions(test.mutate(valid))
			if test.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestSyntheticClientAddress_ReturnsDistinctUnicastAddresses(t *testing.T) {
	first := net.ParseIP(syntheticClientAddress(0))
	last := net.ParseIP(syntheticClientAddress(299))
	assert.NotEqual(t, first.String(), last.String())
	assert.True(t, first.IsGlobalUnicast())
	assert.True(t, last.IsGlobalUnicast())
}

func TestSummarizeLatency_ComputesDeterministicPercentiles(t *testing.T) {
	values := []time.Duration{5 * time.Millisecond, time.Millisecond, 3 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	assert.Equal(t, latencySummary{P50: 3, P95: 5, P99: 5, Max: 5}, summarizeLatency(values))
}

func TestSummarizeLatency_AveragesEvenMedian(t *testing.T) {
	values := []time.Duration{4 * time.Millisecond, time.Millisecond, 3 * time.Millisecond, 2 * time.Millisecond}
	assert.Equal(t, latencySummary{P50: 2.5, P95: 4, P99: 4, Max: 4}, summarizeLatency(values))
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package provider_observer

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

// ProcMeminfoCollector reads host RAM usage from /proc/meminfo.
type ProcMeminfoCollector struct {
	Path string
}

// NewProcMeminfoCollector constructs the default host RAM collector.
func NewProcMeminfoCollector() *ProcMeminfoCollector {
	return &ProcMeminfoCollector{Path: "/proc/meminfo"}
}

func (c *ProcMeminfoCollector) Collect(_ context.Context, observedAt time.Time) (*evalv1.ProviderBoundaryHardwareSample, error) {
	if c == nil || c.Path == "" {
		return unavailableRAMSample(observedAt), nil
	}
	file, err := os.Open(c.Path)
	if err != nil {
		return unavailableRAMSample(observedAt), nil
	}
	defer file.Close()

	var totalKB, availableKB uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			totalKB = parseMeminfoKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			availableKB = parseMeminfoKB(line)
		}
	}
	if totalKB == 0 {
		return unavailableRAMSample(observedAt), nil
	}
	usedBytes := (totalKB - availableKB) * 1024
	return &evalv1.ProviderBoundaryHardwareSample{
		ObservedAtUnixNanos: uint64(observedAt.UTC().UnixNano()),
		HostRamAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		HostRamUsedBytes:    usedBytes,
		HostRamTotalBytes:   totalKB * 1024,
	}, nil
}

func parseMeminfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	value, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return value
}

func unavailableRAMSample(observedAt time.Time) *evalv1.ProviderBoundaryHardwareSample {
	return &evalv1.ProviderBoundaryHardwareSample{
		ObservedAtUnixNanos: uint64(observedAt.UTC().UnixNano()),
		HostRamAvailability: evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_UNAVAILABLE,
	}
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows

package provider_observer

import (
	"context"
	"time"
	"unsafe"

	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
	"golang.org/x/sys/windows"
)

// WindowsHostRAMCollector reads host RAM usage through GlobalMemoryStatusEx.
type WindowsHostRAMCollector struct{}

func newPlatformHostRAMCollector() HardwareCollector {
	return &WindowsHostRAMCollector{}
}

func (c *WindowsHostRAMCollector) Collect(_ context.Context, observedAt time.Time) (*evalv1.ProviderBoundaryHardwareSample, error) {
	if c == nil {
		return unavailableRAMSample(observedAt), nil
	}
	totalBytes, usedBytes, ok := readWindowsMemoryStatus()
	if !ok || totalBytes == 0 {
		return unavailableRAMSample(observedAt), nil
	}
	return &evalv1.ProviderBoundaryHardwareSample{
		ObservedAtUnixNanos: uint64(observedAt.UTC().UnixNano()),
		HostRamAvailability:   evalv1.ProviderHardwareMetricAvailability_PROVIDER_HARDWARE_METRIC_AVAILABILITY_REPORTED,
		HostRamUsedBytes:      usedBytes,
		HostRamTotalBytes:     totalBytes,
	}, nil
}

type windowsMemoryStatusEx struct {
	length                uint32
	memoryLoad            uint32
	totalPhys             uint64
	availPhys             uint64
	totalPageFile         uint64
	availPageFile         uint64
	totalVirtual          uint64
	availVirtual          uint64
	availExtendedVirtual  uint64
}

func readWindowsMemoryStatus() (totalBytes, usedBytes uint64, ok bool) {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	status := windowsMemoryStatusEx{
		length: uint32(unsafe.Sizeof(windowsMemoryStatusEx{})),
	}
	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 || status.totalPhys == 0 {
		return 0, 0, false
	}
	used := status.totalPhys - status.availPhys
	return status.totalPhys, used, true
}

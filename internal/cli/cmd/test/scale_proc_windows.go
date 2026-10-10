// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows

package testcmd

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	psapi                     = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo  = psapi.NewProc("GetProcessMemoryInfo")
	procGetProcessHandleCount = kernel32.NewProc("GetProcessHandleCount")
)

type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// readScaleProcStats reads RSS, threads, and handle count for a Windows process.
// It reports false when the process no longer exists.
func readScaleProcStats(pid int) (scaleProcStats, bool) {
	var stats scaleProcStats
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		handle, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
		if err != nil {
			return stats, false
		}
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var pmc processMemoryCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))
	ret, _, _ := procGetProcessMemoryInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.cb),
	)
	if ret != 0 {
		stats.RSSKB = int64(pmc.workingSetSize / 1024)
	}

	var handleCount uint32
	ret, _, _ = procGetProcessHandleCount.Call(uintptr(handle), uintptr(unsafe.Pointer(&handleCount)))
	if ret != 0 {
		stats.FDs = int(handleCount)
	}

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err == nil {
		defer func() { _ = windows.CloseHandle(snapshot) }()
		var entry windows.ProcessEntry32
		entry.Size = uint32(unsafe.Sizeof(entry))
		if err := windows.Process32First(snapshot, &entry); err == nil {
			for {
				if entry.ProcessID == uint32(pid) {
					stats.Threads = int(entry.Threads)
					break
				}
				if err := windows.Process32Next(snapshot, &entry); err != nil {
					break
				}
			}
		}
	}

	return stats, true
}

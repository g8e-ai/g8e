// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build linux

package testcmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// readScaleProcStats reads RSS and threads from /proc/<pid>/status, open FDs
// from /proc/<pid>/fd, and PSS from /proc/<pid>/smaps_rollup. It reports false
// when the process no longer exists.
func readScaleProcStats(pid int) (scaleProcStats, bool) {
	var stats scaleProcStats
	status, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return stats, false
	}
	defer status.Close()
	scanner := bufio.NewScanner(status)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "VmRSS:":
			stats.RSSKB, _ = strconv.ParseInt(fields[1], 10, 64)
		case "Threads:":
			stats.Threads, _ = strconv.Atoi(fields[1])
		}
	}
	if fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid)); err == nil {
		stats.FDs = len(fds)
	}
	if rollup, err := os.ReadFile(fmt.Sprintf("/proc/%d/smaps_rollup", pid)); err == nil {
		for _, line := range strings.Split(string(rollup), "\n") {
			if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "Pss:" {
				stats.PSSKB, _ = strconv.ParseInt(fields[1], 10, 64)
			}
		}
	}
	return stats, true
}

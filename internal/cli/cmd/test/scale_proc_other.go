// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build !linux

package testcmd

// readScaleProcStats has no portable process accounting outside Linux; the
// scale run records Gateway database sizes only on these platforms.
func readScaleProcStats(int) (scaleProcStats, bool) {
	return scaleProcStats{}, false
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration && race

package tests

import (
	"testing"
	"time"
)

// Under the race detector the burst checks the same contract at a size the
// detector can carry; the full scale runs without it.
const (
	burstOperators         = 16
	burstHeartbeatInterval = 2 * time.Second
	burstTimeout           = 5 * time.Minute
)

// requireBurstBuild admits every race build: this size runs anywhere.
func requireBurstBuild(*testing.T) {}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import "time"

const (
	EvaluationReceiptPollInterval = 500 * time.Millisecond
	EvaluationReceiptPollTimeout  = 30 * time.Second
	InferenceRequestSchemaVersion = "1.0"
)

// Platform binary names.
const (
	BinaryNameWindows = "g8e-windows-amd64.exe"
	BinaryNameLinux   = "g8e-linux-amd64"
	BinaryNameDarwin  = "g8e-darwin-amd64"
)

// Supported architectures.
const (
	ArchAMD64 = "amd64"
	ArchARM64 = "arm64"
	Arch386   = "386"
)

// Supported operating systems.
const (
	OSLinux   = "linux"
	OSDarwin  = "darwin"
	OSWindows = "windows"
)

// Governance posture names.
const (
	PostureDoctrine  = "doctrine"
	PostureConsensus = "consensus"
	PostureRatify    = "ratify"
	PostureNotary    = "notary"
)

type GovernancePostureRequirements struct {
	RequiresL2 bool
	RequiresL3 bool
}

func GetGovernancePostureRequirements(posture string) (GovernancePostureRequirements, bool) {
	switch posture {
	case PostureDoctrine:
		return GovernancePostureRequirements{}, true
	case PostureConsensus:
		return GovernancePostureRequirements{RequiresL2: true}, true
	case PostureRatify:
		return GovernancePostureRequirements{RequiresL3: true}, true
	case PostureNotary:
		return GovernancePostureRequirements{RequiresL2: true, RequiresL3: true}, true
	default:
		return GovernancePostureRequirements{}, false
	}
}

func GovernancePostureMeetsFloor(active, floor string) bool {
	activeRequirements, activeValid := GetGovernancePostureRequirements(active)
	floorRequirements, floorValid := GetGovernancePostureRequirements(floor)
	if !activeValid || !floorValid {
		return false
	}
	return (!floorRequirements.RequiresL2 || activeRequirements.RequiresL2) &&
		(!floorRequirements.RequiresL3 || activeRequirements.RequiresL3)
}

// Log level names.
const (
	LogLevelInfo    = "info"
	LogLevelError   = "error"
	LogLevelDebug   = "debug"
	LogLevelDefault = LogLevelInfo
)

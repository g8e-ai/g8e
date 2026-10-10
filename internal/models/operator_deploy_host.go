// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

// DeployHostAction represents an action requested from the remote host agent.
type DeployHostAction string

const (
	DeployHostActionPrepare   DeployHostAction = "prepare"
	DeployHostActionInstall   DeployHostAction = "install"
	DeployHostActionLink      DeployHostAction = "link"
	DeployHostActionStart     DeployHostAction = "start"
	DeployHostActionState     DeployHostAction = "state"
	DeployHostActionPreflight DeployHostAction = "preflight"
	DeployHostActionStop      DeployHostAction = "stop"
)

// DeployHostRequest is sent via stdin to `g8e operator deploy-host`.
type DeployHostRequest struct {
	Action        DeployHostAction `json:"action"`
	Dirs          []string         `json:"dirs,omitempty"`
	Source        string           `json:"source,omitempty"`
	Target        string           `json:"target,omitempty"`
	BinaryDir     string           `json:"binary_dir,omitempty"`
	DestDir       string           `json:"dest_dir,omitempty"`
	WorkingDir    string           `json:"working_dir,omitempty"`
	Binary        string           `json:"binary,omitempty"`
	Args          []string         `json:"args,omitempty"`
	PreflightArgs []string         `json:"preflight_args,omitempty"`
}

// DeployHostResponse is emitted via stdout from `g8e operator deploy-host`.
type DeployHostResponse struct {
	Success      bool                     `json:"success"`
	Error        string                   `json:"error,omitempty"`
	ResolvedDirs []string                 `json:"resolved_dirs,omitempty"`
	PID          int                      `json:"pid,omitempty"`
	State        *OperatorDeploymentState `json:"state,omitempty"`
	StoppedCount int                      `json:"stopped_count,omitempty"`
	StoppedPIDs  []int                    `json:"stopped_pids,omitempty"`
}

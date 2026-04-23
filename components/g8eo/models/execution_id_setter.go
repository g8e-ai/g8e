// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

// ExecutionIDSetter is implemented by result payloads that carry an
// execution_id correlating them back to the originating command. Publishers
// use this to stamp the execution_id without caring about the concrete type,
// so new result payloads only need to implement this interface to participate.
type ExecutionIDSetter interface {
	SetExecutionID(string)
}

func (p *ExecutionResultsPayload) SetExecutionID(id string)       { p.ExecutionID = id }
func (p *CancellationResultPayload) SetExecutionID(id string)     { p.ExecutionID = id }
func (p *FileEditResultPayload) SetExecutionID(id string)         { p.ExecutionID = id }
func (p *FsListResultPayload) SetExecutionID(id string)           { p.ExecutionID = id }
func (p *ExecutionStatusPayload) SetExecutionID(id string)        { p.ExecutionID = id }
func (p *PortCheckResultPayload) SetExecutionID(id string)        { p.ExecutionID = id }
func (p *FetchLogsResultPayload) SetExecutionID(id string)        { p.ExecutionID = id }
func (p *FsReadResultPayload) SetExecutionID(id string)           { p.ExecutionID = id }
func (p *LFAAErrorPayload) SetExecutionID(id string)              { p.ExecutionID = id }
func (p *FetchFileDiffResultPayload) SetExecutionID(id string)    { p.ExecutionID = id }
func (p *FetchHistoryResultPayload) SetExecutionID(id string)     { p.ExecutionID = id }
func (p *FetchFileHistoryResultPayload) SetExecutionID(id string) { p.ExecutionID = id }
func (p *RestoreFileResultPayload) SetExecutionID(id string)      { p.ExecutionID = id }

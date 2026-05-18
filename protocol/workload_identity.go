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

package protocol

import (
	"fmt"
	"net/url"
)

const (
	// TrustDomain is the SPIFFE trust domain for g8e.
	TrustDomain = "g8e.local"
)

// WorkloadIdentity provides helper functions for generating SPIFFE workload identities.
type WorkloadIdentity struct{}

// NewWorkloadIdentity creates a new WorkloadIdentity helper.
func NewWorkloadIdentity() *WorkloadIdentity {
	return &WorkloadIdentity{}
}

// OperatorSPIFFEID generates the SPIFFE ID for an operator workload.
// Format: spiffe://g8e.local/operator/<organization_id>/<operator_id>/<operator_session_id>
func (w *WorkloadIdentity) OperatorSPIFFEID(organizationID, operatorID, sessionID string) string {
	return fmt.Sprintf("spiffe://%s/operator/%s/%s/%s", TrustDomain, organizationID, operatorID, sessionID)
}

// OperatorSPIFFEURL generates the SPIFFE URL for an operator workload.
func (w *WorkloadIdentity) OperatorSPIFFEURL(organizationID, operatorID, sessionID string) (*url.URL, error) {
	return url.Parse(w.OperatorSPIFFEID(organizationID, operatorID, sessionID))
}

// CLISPIFFEID generates the SPIFFE ID for a CLI (BYO client) workload.
// Format: spiffe://g8e.local/cli/<user_id>/<cli_session_id>
func (w *WorkloadIdentity) CLISPIFFEID(userID, sessionID string) string {
	return fmt.Sprintf("spiffe://%s/cli/%s/%s", TrustDomain, userID, sessionID)
}

// CLISPIFFEURL generates the SPIFFE URL for a CLI workload.
func (w *WorkloadIdentity) CLISPIFFEURL(userID, sessionID string) (*url.URL, error) {
	return url.Parse(w.CLISPIFFEID(userID, sessionID))
}

// AppSPIFFEID generates the SPIFFE ID for an application (agent) workload.
// Format: spiffe://g8e.local/app/<operator_id>
func (w *WorkloadIdentity) AppSPIFFEID(operatorID string) string {
	return fmt.Sprintf("spiffe://%s/app/%s", TrustDomain, operatorID)
}

// AppSPIFFEURL generates the SPIFFE URL for an application workload.
func (w *WorkloadIdentity) AppSPIFFEURL(operatorID string) (*url.URL, error) {
	return url.Parse(w.AppSPIFFEID(operatorID))
}

// HubSPIFFEID generates the SPIFFE ID for the hub (operator-listen) workload.
// Format: spiffe://g8e.local/hub/operator-listen
func (w *WorkloadIdentity) HubSPIFFEID() string {
	return fmt.Sprintf("spiffe://%s/hub/operator-listen", TrustDomain)
}

// HubSPIFFEURL generates the SPIFFE URL for the hub workload.
func (w *WorkloadIdentity) HubSPIFFEURL() (*url.URL, error) {
	return url.Parse(w.HubSPIFFEID())
}

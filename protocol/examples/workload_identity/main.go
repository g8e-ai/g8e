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

package main

import (
	"fmt"
	"log"

	"github.com/g8e-ai/g8e/protocol"
)

func main() {
	wid := protocol.NewWorkloadIdentity()

	// Generate SPIFFE IDs for different workload types
	orgID := "org-123"
	operatorID := "op-456"
	sessionID := "session-789"
	userID := "user-abc"
	gatewayID := "gw-12345"

	// Operator workload identity
	operatorSPIFFE := wid.OperatorSPIFFEID(orgID, operatorID, sessionID)
	fmt.Printf("Operator SPIFFE ID: %s\n", operatorSPIFFE)

	// CLI workload identity
	cliSPIFFE := wid.CLISPIFFEID(userID, sessionID)
	fmt.Printf("CLI SPIFFE ID: %s\n", cliSPIFFE)

	// App workload identity
	appSPIFFE := wid.AppSPIFFEID(operatorID)
	fmt.Printf("App SPIFFE ID: %s\n", appSPIFFE)

	// Hub workload identity
	hubSPIFFE := wid.HubSPIFFEID()
	fmt.Printf("Hub SPIFFE ID: %s\n", hubSPIFFE)

	// User workload identity (human delegator)
	userSPIFFE := wid.UserSPIFFEID(userID)
	fmt.Printf("User SPIFFE ID: %s\n", userSPIFFE)

	// Gateway peer workload identity
	gatewaySPIFFE := wid.GatewayPeerSPIFFEID(gatewayID)
	fmt.Printf("Gateway Peer SPIFFE ID: %s\n", gatewaySPIFFE)

	// Validate identities
	if wid.MatchesOperator(operatorSPIFFE, orgID, operatorID, sessionID) {
		fmt.Println("✓ Operator identity valid")
	}

	if wid.MatchesCLI(cliSPIFFE, userID, sessionID) {
		fmt.Println("✓ CLI identity valid")
	}

	if wid.MatchesCLISessionOnly(cliSPIFFE, sessionID) {
		fmt.Println("✓ CLI session-only match valid")
	}

	if wid.MatchesApp(appSPIFFE, operatorID) {
		fmt.Println("✓ App identity valid")
	}

	if wid.MatchesHub(hubSPIFFE) {
		fmt.Println("✓ Hub identity valid")
	}

	if wid.MatchesGatewayPeer(gatewaySPIFFE, gatewayID) {
		fmt.Println("✓ Gateway peer identity valid")
	}

	// Extract session ID from CLI SPIFFE ID
	extractedSession, ok := wid.ExtractCLISessionID(cliSPIFFE)
	if ok {
		fmt.Printf("✓ Extracted CLI session ID: %s\n", extractedSession)
	}

	// Extract user ID from CLI SPIFFE ID
	extractedUser, ok := wid.ExtractUserID(cliSPIFFE)
	if ok {
		fmt.Printf("✓ Extracted user ID: %s\n", extractedUser)
	}

	// Extract user ID from User SAN SPIFFE ID
	extractedUserFromSAN, ok := wid.ExtractUserIDFromUserSAN(userSPIFFE)
	if ok {
		fmt.Printf("✓ Extracted user ID from SAN: %s\n", extractedUserFromSAN)
	}

	// Extract operator session ID from Operator SPIFFE ID
	extractedOpSession, ok := wid.ExtractOperatorSessionID(operatorSPIFFE)
	if ok {
		fmt.Printf("✓ Extracted operator session ID: %s\n", extractedOpSession)
	}

	// Extract gateway ID from Gateway Peer SPIFFE ID
	extractedGatewayID, ok := wid.ExtractGatewayID(gatewaySPIFFE)
	if ok {
		fmt.Printf("✓ Extracted gateway ID: %s\n", extractedGatewayID)
	}

	// Parse SPIFFE URLs
	operatorURL, err := wid.OperatorSPIFFEURL(orgID, operatorID, sessionID)
	if err != nil {
		log.Fatalf("Failed to parse Operator SPIFFE URL: %v", err)
	}
	fmt.Printf("Operator SPIFFE URL: %s\n", operatorURL.String())

	gatewayURL, err := wid.GatewayPeerSPIFFEURL(gatewayID)
	if err != nil {
		log.Fatalf("Failed to parse Gateway Peer SPIFFE URL: %v", err)
	}
	fmt.Printf("Gateway Peer SPIFFE URL: %s\n", gatewayURL.String())
}

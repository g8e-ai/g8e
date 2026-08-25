// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package main

import (
	"fmt"
	"log"

	"github.com/g8e-ai/g8e/v2/protocol"
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

	// Ensemble app workload identity (centralized event broker)
	ensembleSPIFFE := protocol.EnsembleAppID
	fmt.Printf("Ensemble App SPIFFE ID: %s\n", ensembleSPIFFE)

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
		fmt.Println("OK: Operator identity valid")
	}

	if wid.MatchesCLI(cliSPIFFE, userID, sessionID) {
		fmt.Println("OK: CLI identity valid")
	}

	if wid.MatchesCLISessionOnly(cliSPIFFE, sessionID) {
		fmt.Println("OK: CLI session-only match valid")
	}

	if wid.MatchesApp(appSPIFFE, operatorID) {
		fmt.Println("OK: App identity valid")
	}

	if wid.IsAppSAN(appSPIFFE) {
		fmt.Println("OK: App SAN workload identity recognized")
	}

	if wid.IsEnsembleApp(ensembleSPIFFE) {
		fmt.Println("OK: Ensemble app identity recognized")
	}

	if wid.MatchesHub(hubSPIFFE) {
		fmt.Println("OK: Hub identity valid")
	}

	if wid.MatchesGatewayPeer(gatewaySPIFFE, gatewayID) {
		fmt.Println("OK: Gateway peer identity valid")
	}

	if wid.IsUserSAN(userSPIFFE) {
		fmt.Println("OK: User SAN workload identity recognized")
	}

	// Extract session ID from CLI SPIFFE ID
	extractedSession, ok := wid.ExtractCLISessionID(cliSPIFFE)
	if ok {
		fmt.Printf("OK: Extracted CLI session ID: %s\n", extractedSession)
	}

	// Extract user ID from CLI SPIFFE ID
	extractedUser, ok := wid.ExtractUserID(cliSPIFFE)
	if ok {
		fmt.Printf("OK: Extracted user ID: %s\n", extractedUser)
	}

	// Extract user ID from User SAN SPIFFE ID
	extractedUserFromSAN, ok := wid.ExtractUserIDFromUserSAN(userSPIFFE)
	if ok {
		fmt.Printf("OK: Extracted user ID from SAN: %s\n", extractedUserFromSAN)
	}

	// Extract operator session ID from Operator SPIFFE ID
	extractedOpSession, ok := wid.ExtractOperatorSessionID(operatorSPIFFE)
	if ok {
		fmt.Printf("OK: Extracted operator session ID: %s\n", extractedOpSession)
	}

	// Extract gateway ID from Gateway Peer SPIFFE ID
	extractedGatewayID, ok := wid.ExtractGatewayID(gatewaySPIFFE)
	if ok {
		fmt.Printf("OK: Extracted gateway ID: %s\n", extractedGatewayID)
	}

	// Parse SPIFFE URLs
	operatorURL, err := wid.OperatorSPIFFEURL(orgID, operatorID, sessionID)
	if err != nil {
		log.Fatalf("Failed to parse Operator SPIFFE URL: %v", err)
	}
	fmt.Printf("Operator SPIFFE URL: %s\n", operatorURL.String())

	cliURL, err := wid.CLISPIFFEURL(userID, sessionID)
	if err != nil {
		log.Fatalf("Failed to parse CLI SPIFFE URL: %v", err)
	}
	fmt.Printf("CLI SPIFFE URL: %s\n", cliURL.String())

	appURL, err := wid.AppSPIFFEURL(operatorID)
	if err != nil {
		log.Fatalf("Failed to parse App SPIFFE URL: %v", err)
	}
	fmt.Printf("App SPIFFE URL: %s\n", appURL.String())

	userURL, err := wid.UserSPIFFEURL(userID)
	if err != nil {
		log.Fatalf("Failed to parse User SPIFFE URL: %v", err)
	}
	fmt.Printf("User SPIFFE URL: %s\n", userURL.String())

	hubURL, err := wid.HubSPIFFEURL()
	if err != nil {
		log.Fatalf("Failed to parse Hub SPIFFE URL: %v", err)
	}
	fmt.Printf("Hub SPIFFE URL: %s\n", hubURL.String())

	gatewayURL, err := wid.GatewayPeerSPIFFEURL(gatewayID)
	if err != nil {
		log.Fatalf("Failed to parse Gateway Peer SPIFFE URL: %v", err)
	}
	fmt.Printf("Gateway Peer SPIFFE URL: %s\n", gatewayURL.String())
}

package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
)

// printNextSteps outputs guidance after the gateway starts. The passkey
// enrollment guidance is posture-aware: ratify and notary require a
// human-registered passkey because they enforce L3 proof. Doctrine and
// consensus do not require a passkey at startup, and the user can optionally
// enroll one before restarting in ratify or notary mode.
func printNextSteps(cmd *cobra.Command, posture governance.GovernancePosture, externalIP, hostname string) {
	bin := getBinaryName()
	httpPort := constants.Ports.OperatorHttp
	httpsPort := constants.Ports.OperatorHttps

	publicHost := hostname
	if publicHost == "" {
		publicHost = externalIP
	}

	// Endpoints
	cmd.Println("Endpoints:")
	cmd.Printf("  Operator Bootstrap: https://%s:%d\n", externalIP, httpsPort)
	cmd.Printf("  Public API:         https://%s:%d (Public browser/BYO bootstrap)\n", publicHost, httpsPort)
	cmd.Printf("  Console UI:         https://%s:%d/console/ (WebAuthn/passkey dashboard)\n", publicHost, httpsPort)
	cmd.Printf("  MCP HTTP:           http://127.0.0.1:%d (Plain HTTP for MCP calls)\n", httpPort)
	cmd.Println()

	// Status
	cmd.Println("Gateway Status: Online")
	cmd.Println()
	cmd.Println("The g8e Gateway is online as a stateless relay.")
	cmd.Println()

	if posture != nil && posture.RequiresL3Proof() {
		cmd.Println("Passkey Enrollment: Required")
		cmd.Println()
		cmd.Printf("Proof of human presence required for %s posture (L3 enforced).\n", posture.Name())
		cmd.Println()
	} else {
		cmd.Println("Passkey Enrollment: Optional")
		cmd.Println()
		cmd.Printf("No passkey required for %s posture (L3 audited, not enforced).\n", posture.Name())
		cmd.Println("Enroll a passkey if you plan to restart in ratify or notary mode (L3 enforced).")
		cmd.Println()
	}

	// Next Steps
	cmd.Println("Next Steps:")
	cmd.Println()

	// GUI section
	cmd.Println("  ===")
	cmd.Println("  GUI")
	cmd.Println("  ===")
	cmd.Println()
	if posture != nil && posture.RequiresL3Proof() {
		cmd.Println("  1. Enroll your CLI identity and register a passkey from a workstation")
		cmd.Println("     that can reach this gateway:")
	} else {
		cmd.Println("  1. Enroll your CLI identity from a workstation that can reach this")
		cmd.Println("     gateway (passkey enrollment is optional for this posture):")
	}
	cmd.Println()
	cmd.Printf("       %s auth enroll user -e %s\n", bin, externalIP)
	cmd.Println()
	cmd.Println("     `auth enroll user` performs the human passkey ceremony and prepares the")
	cmd.Println("     CLI credentials needed to access the console and MCP endpoints. It also")
	cmd.Println("     installs the gateway root CA into the OS trust store before opening the")
	cmd.Println("     browser; pass `--no-system-trust` to skip that step (an administrator")
	cmd.Println("     must pre-install trust in that case).")
	cmd.Println()
	cmd.Println("     `auth enroll user` verifies the local identity against the live gateway")
	cmd.Println("     before reuse: if the gateway's PKI has been regenerated (e.g., after")
	cmd.Println("     `gw clean`), the local identity is stale and enrollment automatically")
	cmd.Println("     routes through the human-approved recovery flow to obtain a fresh")
	cmd.Println("     certificate signed by the new CA.")
	cmd.Println()

	// CLI section
	cmd.Println("  ===")
	cmd.Println("  CLI")
	cmd.Println("  ===")
	cmd.Println()
	if posture != nil && posture.RequiresL3Proof() {
		cmd.Println("  2. Enroll your passkey (in this terminal):")
	} else {
		cmd.Println("  2. Enroll a passkey (optional, in this terminal):")
	}
	cmd.Printf("       %s auth enroll user\n", bin)
	cmd.Println()

	// Operators
	cmd.Println("  3. Operators:")
	cmd.Printf("       Local Host:   %s operator deploy --hosts <host1,host2>\n", bin)
	cmd.Printf("                     %s operator stream --hosts <host1,host2>\n", bin)
	cmd.Printf("       Remote Host:  %s operator start -e %s\n", bin, externalIP)
	cmd.Println()
}

// pickHostname selects the best hostname from a NetworkIdentity for display.
// It prefers an FQDN (contains a dot), falls back to the first hostname,
// or returns an empty string if no hostnames are available.
func pickHostname(identity *network.NetworkIdentity) string {
	if identity == nil || len(identity.Hostnames) == 0 {
		return ""
	}
	for _, hn := range identity.Hostnames {
		if strings.Contains(hn, ".") {
			return hn
		}
	}
	return identity.Hostnames[0]
}

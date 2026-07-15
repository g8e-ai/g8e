package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/governance"
	"github.com/g8e-ai/g8e/internal/services/network"
)

// printNextSteps outputs guidance after the gateway starts.
func printNextSteps(cmd *cobra.Command, _ governance.GovernancePosture, externalIP, hostname string) {
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
	cmd.Println("Passkey Enrollment: Pending")
	cmd.Println()
	cmd.Println("Proof of human presence required.")
	cmd.Println()

	// Next Steps
	cmd.Println("Next Steps:")
	cmd.Println()

	// GUI section
	cmd.Println("  ===")
	cmd.Println("  GUI")
	cmd.Println("  ===")
	cmd.Println()
	cmd.Println("  1. Trust the gateway CA for HTTPS on your workstation:")
	cmd.Println()
	cmd.Println("       Linux/macOS")
	cmd.Printf("       curl -fsSL http://%s:%d/web-cert.sh | sh\n", externalIP, httpPort)
	cmd.Println()
	cmd.Println("       Windows")
	cmd.Printf("       irm http://%s:%d/web-cert.ps1 | iex\n", externalIP, httpPort)
	cmd.Println()
	cmd.Println("  2. Close all web browser windows on the remote workstation")
	cmd.Println()

	// CLI section
	cmd.Println("  ===")
	cmd.Println("  CLI")
	cmd.Println("  ===")
	cmd.Println()
	cmd.Println("  3. Enroll your passkey (in this terminal):")
	cmd.Printf("       %s auth enroll\n", bin)
	cmd.Println()

	// Operators
	cmd.Println("  4. Operators:")
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

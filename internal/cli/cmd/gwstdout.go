package cmd

import (
	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/netutil"
	"github.com/g8e-ai/g8e/internal/services/governance"
)

// printNextSteps outputs guidance after the gateway starts.
func printNextSteps(cmd *cobra.Command, _ governance.GovernancePosture, externalIP string) {
	bin := getBinaryName()
	httpPort := constants.Ports.OperatorHttp

	cmd.Println("Next Steps:")
	cmd.Println()

	// Step 1: User Workstation
	cmd.Println("  1. User Workstation")
	cmd.Println("     a. Trust the gateway CA for HTTPS (run on your workstation):")
	cmd.Println()
	cmd.Println("          Linux/macOS")
	cmd.Printf("          curl -fsSL http://%s:%d/web-cert.sh | sh\n", externalIP, httpPort)
	cmd.Println()
	cmd.Println("          Windows")
	cmd.Printf("          irm http://%s:%d/web-cert.ps1 | iex\n", externalIP, httpPort)
	cmd.Println()
	cmd.Println("     b. Close all web browser windows on the remote workstation")
	cmd.Println()

	// Step 2: This Terminal
	cmd.Println("  2. This Terminal")
	cmd.Printf("       %s auth enroll\n", bin)
	cmd.Println()

	// Step 3: Operators
	cmd.Println("  3. Operators")
	cmd.Printf("       Local Host:   %s operator deploy --hosts <host1,host2>\n", bin)
	cmd.Printf("                     %s operator stream --hosts <host1,host2>\n", bin)
	cmd.Printf("       Remote Host:  %s operator start -e %s\n", bin, externalIP)
	cmd.Println()

	// Console UI
	cmd.Println("Console UI:")
	cmd.Printf("  %s/console/  (WebAuthn/passkey dashboard)\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
}

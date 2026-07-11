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

	// Step 1: Trust the gateway CA (for remote workstations accessing HTTPS)
	cmd.Println("  1. Trust the gateway CA for HTTPS (run on each remote workstation):")
	cmd.Printf("       curl -fsSL http://%s:%d/web-cert.sh | sh   (Linux/macOS)\n", externalIP, httpPort)
	cmd.Printf("       irm http://%s:%d/web-cert.ps1 | iex        (Windows)\n", externalIP, httpPort)
	cmd.Println()

	// Step 2: Enroll CLI credentials
	cmd.Println("  2. Enroll CLI credentials:")
	cmd.Printf("       %s auth enroll\n", bin)
	cmd.Println()

	// Step 3: Connect remote operators
	cmd.Println("  3. Connect remote operators:")
	cmd.Printf("       Local Host:   %s operator deploy --hosts <host1,host2>\n", bin)
	cmd.Printf("                     %s operator stream --hosts <host1,host2>\n", bin)
	cmd.Printf("       Remote Host:  %s gw security pki enroll -e %s\n", bin, externalIP)
	cmd.Printf("                     %s operator start\n", bin)
	cmd.Println()

	// Console UI
	cmd.Println("Console UI:")
	cmd.Printf("  %s/console/  (WebAuthn/passkey dashboard)\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
}

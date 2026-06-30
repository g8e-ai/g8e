package cmd

import (
	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/netutil"
	"github.com/g8e-ai/g8e/internal/services/governance"
)

// printNextSteps outputs posture-aware guidance after the gateway starts.
func printNextSteps(cmd *cobra.Command, posture governance.GovernancePosture, externalIP string) {
	bin := getBinaryName()
	httpPort := constants.Ports.OperatorHttp

	cmd.Println("Next Steps:")
	cmd.Println()

	// Step 1: Trust the gateway CA (enables trusted HTTPS)
	cmd.Println("  1. Trust the gateway CA for HTTPS (run on this machine):")
	cmd.Printf("       curl -fsSL http://%s:%d/bootstrap-ca | sh        (Linux)\n", externalIP, httpPort)
	cmd.Printf("       curl -fsSL http://%s:%d/bootstrap-ca-macos | sh  (macOS)\n", externalIP, httpPort)
	cmd.Printf("       irm http://%s:%d/bootstrap-ca.ps1 | iex           (Windows)\n", externalIP, httpPort)
	cmd.Println()

	// Step 2: Enroll CLI credentials (all postures)
	cmd.Println("  2. Enroll CLI credentials:")
	cmd.Printf("       %s auth enroll\n", bin)
	cmd.Println()

	// Step 3: Posture-specific governance setup
	stepNum := 3
	switch posture.Name() {
	case "doctrine":
		cmd.Printf("  %d. Governance posture: doctrine (L1 enforced, L2/L3 audited)\n", stepNum)
		cmd.Println("       No additional setup required. L2 consensus and L3 notary")
		cmd.Println("       results are recorded for audit but do not block execution.")
		cmd.Println()
	case "consensus":
		cmd.Printf("  %d. Configure L2 Tribunal for consensus:\n", stepNum)
		cmd.Println("       L2 multi-agent quorum is enforced. Mutations without a valid")
		cmd.Println("       Tribunal quorum will be rejected.")
		cmd.Printf("       Bootstrap:  %s gw start --posture consensus \\\n", bin)
		cmd.Printf("                     --tribunal-id <id> --tribunal-url <url>\n")
		cmd.Printf("       Seed file:  --tribunal-bootstrap <policy.json>\n")
		cmd.Println()
		cmd.Println("       L3 notary is audited only -- no human approval required.")
		cmd.Println()
	case "notary":
		cmd.Printf("  %d. Configure L2 Tribunal + L3 Notary:\n", stepNum)
		cmd.Println("       L2 quorum AND L3 human approval are both enforced.")
		cmd.Println("       All mutations require a WebAuthn/passkey ceremony.")
		cmd.Printf("       Bootstrap:  %s gw start --posture notary \\\n", bin)
		cmd.Printf("                     --tribunal-id <id> --tribunal-url <url>\n")
		cmd.Println()
		cmd.Printf("       Passkey:    %s auth enroll  (registers WebAuthn credential)\n", bin)
		cmd.Println()
	}
	stepNum++

	// Step 3: Connect remote operators
	cmd.Printf("  %d. Connect remote operators (choose one):\n", stepNum)
	cmd.Printf("       Deploy:        %s operator deploy --hosts <host1,host2>\n", bin)
	cmd.Printf("       Stream:        %s operator stream --hosts <host1,host2>\n", bin)
	cmd.Printf("       PKI enroll:    %s gw security pki enroll -e %s\n", bin, externalIP)
	cmd.Printf("       Remote script: curl -fsSL http://%s:%d/g8e-operator.sh | bash  (Linux/macOS)\n", externalIP, httpPort)
	cmd.Printf("                      irm http://%s:%d/g8e-operator.ps1 | iex          (Windows)\n", externalIP, httpPort)
	cmd.Println()
	stepNum++

	// Step 4: Connect AI agents
	cmd.Printf("  %d. Connect AI agents:\n", stepNum)
	cmd.Printf("       %s mcp agent show <agent>   Print MCP client configuration for a specific agent\n", bin)
	cmd.Printf("       %s mcp agent list           List supported agents (goose, claude, cursor, ...)\n", bin)
	cmd.Printf("       %s mcp stdio   Run Operator as MCP stdio server (L1-L5 governance)\n", bin)
	cmd.Println()

	// Console UI
	cmd.Println("Console UI:")
	cmd.Printf("  %s/console/  (WebAuthn/passkey dashboard)\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
	cmd.Println()

	// Manage & Monitor
	cmd.Println("Manage & Monitor:")
	cmd.Printf("  %s gw status | logs -f | restart | settings | reset | clean\n", bin)
	cmd.Printf("  %s gw data operators | users | audit list --operator-session-id <session-id>\n", bin)
}

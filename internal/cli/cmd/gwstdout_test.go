package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/netutil"
	"github.com/g8e-ai/g8e/internal/services/governance"
)

// newOutputCmd creates a cobra.Command whose output is captured in the returned buffer.
func newOutputCmd() (*cobra.Command, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	return cmd, buf
}

func TestPrintNextSteps_DoctrinePosture(t *testing.T) {
	cmd, buf := newOutputCmd()
	posture := &governance.DoctrinePosture{}
	externalIP := "192.168.1.100"

	printNextSteps(cmd, posture, externalIP)

	out := buf.String()
	bin := getBinaryName()

	// Step 1: CA bootstrap URLs with external IP and HTTP port
	assert.Contains(t, out, "Next Steps:")
	assert.Contains(t, out, "1. Trust the gateway CA for HTTPS")
	assert.Contains(t, out, "curl -fsSL http://192.168.1.100:8080/bootstrap-ca | sh")
	assert.Contains(t, out, "curl -fsSL http://192.168.1.100:8080/bootstrap-ca-macos | sh")
	assert.Contains(t, out, "irm http://192.168.1.100:8080/bootstrap-ca.ps1 | iex")

	// Step 2: Enroll
	assert.Contains(t, out, "2. Enroll CLI credentials:")
	assert.Contains(t, out, bin+" auth enroll")

	// Step 3: Doctrine-specific text
	assert.Contains(t, out, "3. Governance posture: doctrine (L1 enforced, L2/L3 audited)")
	assert.Contains(t, out, "No additional setup required.")
	assert.Contains(t, out, "L2 consensus and L3 notary")
	assert.Contains(t, out, "results are recorded for audit but do not block execution.")

	// Step 4: Remote operators (numbered 4 for doctrine)
	assert.Contains(t, out, "4. Connect remote operators (choose one):")
	assert.Contains(t, out, bin+" operator deploy --hosts <host1,host2>")
	assert.Contains(t, out, bin+" operator stream --hosts <host1,host2>")
	assert.Contains(t, out, bin+" gw security pki enroll -e 192.168.1.100")
	assert.Contains(t, out, "curl -fsSL http://192.168.1.100:8080/"+constants.DeployScriptFilenameLinux+" | bash")
	assert.Contains(t, out, "irm http://192.168.1.100:8080/"+constants.DeployScriptFilenameWindows+" | iex")

	// Step 5: AI agents (numbered 5 for doctrine)
	assert.Contains(t, out, "5. Connect AI agents:")
	assert.Contains(t, out, bin+" mcp agent show <agent>")
	assert.Contains(t, out, bin+" mcp agent list")
	assert.Contains(t, out, bin+" mcp stdio")

	// Console UI
	assert.Contains(t, out, "Console UI:")
	assert.Contains(t, out, netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps)+"/console/")

	// Manage & Monitor
	assert.Contains(t, out, "Manage & Monitor:")
	assert.Contains(t, out, bin+" gw status | logs -f | restart | settings | reset | clean")
	assert.Contains(t, out, bin+" gw data operators | users | audit list --operator-session-id <session-id>")
}

func TestPrintNextSteps_ConsensusPosture(t *testing.T) {
	cmd, buf := newOutputCmd()
	posture := &governance.ConsensusPosture{}
	externalIP := "10.0.0.50"

	printNextSteps(cmd, posture, externalIP)

	out := buf.String()
	bin := getBinaryName()

	// Step 3: Consensus-specific text
	assert.Contains(t, out, "3. Configure L2 Tribunal for consensus:")
	assert.Contains(t, out, "L2 multi-agent quorum is enforced.")
	assert.Contains(t, out, "Mutations without a valid")
	assert.Contains(t, out, "Tribunal quorum will be rejected.")
	assert.Contains(t, out, "Bootstrap:  "+bin+" gw start --posture consensus \\")
	assert.Contains(t, out, "--tribunal-id <id> --tribunal-url <url>")
	assert.Contains(t, out, "Seed file:  --tribunal-bootstrap <policy.json>")
	assert.Contains(t, out, "L3 notary is audited only -- no human approval required.")

	// Step 4: Remote operators (numbered 4 for consensus)
	assert.Contains(t, out, "4. Connect remote operators (choose one):")

	// Step 5: AI agents
	assert.Contains(t, out, "5. Connect AI agents:")

	// External IP interpolated correctly
	assert.Contains(t, out, "http://10.0.0.50:8080/bootstrap-ca")
}

func TestPrintNextSteps_NotaryPosture(t *testing.T) {
	cmd, buf := newOutputCmd()
	posture := &governance.NotaryPosture{}
	externalIP := "172.16.0.1"

	printNextSteps(cmd, posture, externalIP)

	out := buf.String()
	bin := getBinaryName()

	// Step 3: Notary-specific text
	assert.Contains(t, out, "3. Configure L2 Tribunal + L3 Notary:")
	assert.Contains(t, out, "L2 quorum AND L3 human approval are both enforced.")
	assert.Contains(t, out, "All mutations require a WebAuthn/passkey ceremony.")
	assert.Contains(t, out, "Bootstrap:  "+bin+" gw start --posture notary \\")
	assert.Contains(t, out, "Passkey:    "+bin+" auth enroll  (registers WebAuthn credential)")

	// Step 4: Remote operators (numbered 4 for notary)
	assert.Contains(t, out, "4. Connect remote operators (choose one):")

	// Step 5: AI agents
	assert.Contains(t, out, "5. Connect AI agents:")

	// External IP interpolated correctly
	assert.Contains(t, out, "http://172.16.0.1:8080/bootstrap-ca")
}

func TestPrintNextSteps_StepNumberingPerPosture(t *testing.T) {
	// All three postures produce the same step numbering:
	// 1=CA, 2=Enroll, 3=Posture-specific, 4=Remote operators, 5=AI agents
	postures := []struct {
		name    string
		posture governance.GovernancePosture
	}{
		{"doctrine", &governance.DoctrinePosture{}},
		{"consensus", &governance.ConsensusPosture{}},
		{"notary", &governance.NotaryPosture{}},
	}

	for _, p := range postures {
		t.Run(p.name, func(t *testing.T) {
			cmd, buf := newOutputCmd()
			printNextSteps(cmd, p.posture, "127.0.0.1")
			out := buf.String()

			// Verify step numbering is sequential: 1, 2, 3, 4, 5
			assert.Contains(t, out, "  1. Trust the gateway CA")
			assert.Contains(t, out, "  2. Enroll CLI credentials:")
			assert.Contains(t, out, "  3. ")
			assert.Contains(t, out, "  4. Connect remote operators")
			assert.Contains(t, out, "  5. Connect AI agents:")
		})
	}
}

func TestPrintNextSteps_ExternalIPInterpolation(t *testing.T) {
	testIPs := []string{
		"127.0.0.1",
		"192.168.1.100",
		"10.0.0.1",
		"203.0.113.42",
	}

	for _, ip := range testIPs {
		t.Run(ip, func(t *testing.T) {
			cmd, buf := newOutputCmd()
			printNextSteps(cmd, &governance.DoctrinePosture{}, ip)
			out := buf.String()

			// The IP should appear in bootstrap-ca URL, PKI enroll, and remote script URLs
			assert.Contains(t, out, "http://"+ip+":8080/bootstrap-ca")
			assert.Contains(t, out, "pki enroll -e "+ip)
			assert.Contains(t, out, "http://"+ip+":8080/"+constants.DeployScriptFilenameLinux)
			assert.Contains(t, out, "http://"+ip+":8080/"+constants.DeployScriptFilenameWindows)
		})
	}
}

func TestPrintNextSteps_PortInterpolation(t *testing.T) {
	cmd, buf := newOutputCmd()
	printNextSteps(cmd, &governance.DoctrinePosture{}, "localhost")
	out := buf.String()

	httpPort := constants.Ports.OperatorHttp
	httpsPort := constants.Ports.OperatorHttps

	// HTTP port should appear in bootstrap and operator script URLs
	assert.Contains(t, out, "localhost:"+itoa(httpPort)+"/bootstrap-ca")
	assert.Contains(t, out, "localhost:"+itoa(httpPort)+"/"+constants.DeployScriptFilenameLinux)

	// HTTPS port should appear in the console URL
	assert.Contains(t, out, netutil.LocalhostHTTPSURL(httpsPort)+"/console/")
}

func TestPrintNextSteps_BinaryNameInterpolation(t *testing.T) {
	cmd, buf := newOutputCmd()
	printNextSteps(cmd, &governance.DoctrinePosture{}, "localhost")
	out := buf.String()
	bin := getBinaryName()

	// Binary name should appear in multiple commands
	assert.Contains(t, out, bin+" auth enroll")
	assert.Contains(t, out, bin+" operator deploy")
	assert.Contains(t, out, bin+" operator stream")
	assert.Contains(t, out, bin+" mcp agent show")
	assert.Contains(t, out, bin+" mcp agent list")
	assert.Contains(t, out, bin+" mcp stdio")
	assert.Contains(t, out, bin+" gw status")
	assert.Contains(t, out, bin+" gw data operators")
}

func TestPrintNextSteps_AllPosturesContainCommonSections(t *testing.T) {
	postures := []struct {
		name    string
		posture governance.GovernancePosture
	}{
		{"doctrine", &governance.DoctrinePosture{}},
		{"consensus", &governance.ConsensusPosture{}},
		{"notary", &governance.NotaryPosture{}},
	}

	commonSubstrings := []string{
		"Next Steps:",
		"Trust the gateway CA",
		"Enroll CLI credentials",
		"Connect remote operators",
		"Connect AI agents",
		"Console UI:",
		"Manage & Monitor:",
	}

	for _, p := range postures {
		t.Run(p.name, func(t *testing.T) {
			cmd, buf := newOutputCmd()
			printNextSteps(cmd, p.posture, "192.168.1.1")
			out := buf.String()

			for _, sub := range commonSubstrings {
				assert.Contains(t, out, sub, "posture %s output missing common section: %s", p.name, sub)
			}
		})
	}
}

func TestPrintNextSteps_PostureSpecificContentNotLeaked(t *testing.T) {
	t.Run("doctrine does not contain consensus or notary specific text", func(t *testing.T) {
		cmd, buf := newOutputCmd()
		printNextSteps(cmd, &governance.DoctrinePosture{}, "localhost")
		out := buf.String()

		assert.NotContains(t, out, "Configure L2 Tribunal")
		assert.NotContains(t, out, "Configure L2 Tribunal + L3 Notary")
		assert.NotContains(t, out, "WebAuthn/passkey ceremony")
		assert.NotContains(t, out, "Tribunal quorum will be rejected")
	})

	t.Run("consensus does not contain doctrine or notary specific text", func(t *testing.T) {
		cmd, buf := newOutputCmd()
		printNextSteps(cmd, &governance.ConsensusPosture{}, "localhost")
		out := buf.String()

		assert.NotContains(t, out, "No additional setup required.")
		assert.NotContains(t, out, "Configure L2 Tribunal + L3 Notary")
		assert.NotContains(t, out, "WebAuthn/passkey ceremony")
	})

	t.Run("notary does not contain doctrine or consensus specific text", func(t *testing.T) {
		cmd, buf := newOutputCmd()
		printNextSteps(cmd, &governance.NotaryPosture{}, "localhost")
		out := buf.String()

		assert.NotContains(t, out, "No additional setup required.")
		assert.NotContains(t, out, "Configure L2 Tribunal for consensus:")
		assert.NotContains(t, out, "L3 notary is audited only")
	})
}

func TestPrintNextSteps_OutputNotEmpty(t *testing.T) {
	postures := []governance.GovernancePosture{
		&governance.DoctrinePosture{},
		&governance.ConsensusPosture{},
		&governance.NotaryPosture{},
	}

	for i, p := range postures {
		cmd, buf := newOutputCmd()
		printNextSteps(cmd, p, "localhost")
		require.NotEmpty(t, buf.String(), "posture index %d produced empty output", i)
	}
}

func TestPrintNextSteps_LineCountReasonable(t *testing.T) {
	cmd, buf := newOutputCmd()
	printNextSteps(cmd, &governance.ConsensusPosture{}, "localhost")
	out := buf.String()

	lineCount := strings.Count(out, "\n")
	// The function prints a substantial number of lines across all sections.
	// Doctrine produces the fewest lines; consensus and notary produce more.
	// A reasonable lower bound ensures no section was silently skipped.
	assert.Greater(t, lineCount, 20, "expected substantial output, got %d lines", lineCount)
}

// itoa is a helper to avoid importing strconv just for int-to-string.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

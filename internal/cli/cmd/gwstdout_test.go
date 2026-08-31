package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/governance"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
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
	hostname := "dev.g8e.local"

	printNextSteps(cmd, posture, externalIP, hostname)

	out := buf.String()
	bin := getBinaryName()

	// Endpoints section
	assert.Contains(t, out, "Endpoints:")
	assert.Contains(t, out, "Operator Bootstrap: https://192.168.1.100:8443")
	assert.Contains(t, out, "Public API:         https://dev.g8e.local:8443")
	assert.Contains(t, out, "Console UI:         https://dev.g8e.local:8443/console/")
	assert.Contains(t, out, "MCP HTTP:           http://127.0.0.1:8080")

	// Status section
	assert.Contains(t, out, "Gateway Status: Online")
	assert.Contains(t, out, "The g8e Gateway is online as a stateless relay.")
	assert.Contains(t, out, "Passkey Enrollment: Optional")
	assert.Contains(t, out, "No passkey required for doctrine posture (L3 audited, not enforced).")
	assert.Contains(t, out, "Enroll a passkey if you plan to restart in ratify or notary mode (L3 enforced).")

	// Next Steps with GUI section
	assert.Contains(t, out, "Next Steps:")
	assert.Contains(t, out, "GUI")
	assert.Contains(t, out, "1. Enroll your CLI identity from a workstation that can reach this")
	assert.Contains(t, out, "(passkey enrollment is optional for this posture)")
	assert.Contains(t, out, bin+" auth enroll user -e 192.168.1.100")
	assert.Contains(t, out, "`auth enroll user` performs the human passkey ceremony")
	// Built-in OS trust installation is mentioned, including the
	// --no-system-trust opt-out (administrator-managed trust).
	assert.Contains(t, out, "installs the gateway root CA into the OS trust store")
	assert.Contains(t, out, "--no-system-trust")

	// CLI section
	assert.Contains(t, out, "CLI")
	assert.Contains(t, out, "2. Enroll a passkey (optional, in this terminal)")
	assert.Contains(t, out, bin+" auth enroll user")

	// Operators (step 3)
	assert.Contains(t, out, "3. Operators:")
	assert.Contains(t, out, "Local Host:")
	assert.Contains(t, out, bin+" operator deploy --hosts <host1,host2>")
	assert.Contains(t, out, bin+" operator stream --hosts <host1,host2>")
	assert.Contains(t, out, "Remote Host:")
	assert.Contains(t, out, bin+" operator start -e 192.168.1.100")

	// Removed sections
	assert.NotContains(t, out, "Connect AI agents")
	assert.NotContains(t, out, "Manage & Monitor")
	assert.NotContains(t, out, "Governance posture:")
	assert.NotContains(t, out, constants.DeployScriptFilenameLinux)
	assert.NotContains(t, out, constants.DeployScriptFilenameWindows)
	// Trust-script references must be gone
	assert.NotContains(t, out, "web-cert.sh")
	assert.NotContains(t, out, "web-cert.ps1")
	assert.NotContains(t, out, "Trust the gateway CA")
	assert.NotContains(t, out, "Close all web browser")
}

func TestPrintNextSteps_ConsensusPosture(t *testing.T) {
	cmd, buf := newOutputCmd()
	posture := &governance.ConsensusPosture{}
	externalIP := "10.0.0.50"

	printNextSteps(cmd, posture, externalIP, "")

	out := buf.String()

	// Consensus does not require L3 proof — passkey is optional
	assert.Contains(t, out, "Passkey Enrollment: Optional")
	assert.Contains(t, out, "No passkey required for consensus posture (L3 audited, not enforced).")
	assert.Contains(t, out, "1. Enroll your CLI identity")
	assert.Contains(t, out, "2. Enroll a passkey (optional, in this terminal)")
	assert.Contains(t, out, "3. Operators:")

	// External IP interpolated correctly
	assert.Contains(t, out, "auth enroll user -e 10.0.0.50")

	// No posture-specific text from the old posture-specific output
	assert.NotContains(t, out, "Configure L2 Consensus")
	assert.NotContains(t, out, "Connect AI agents")
	assert.NotContains(t, out, "Manage & Monitor")
}

func TestPrintNextSteps_RatifyPosture(t *testing.T) {
	cmd, buf := newOutputCmd()
	posture := &governance.RatifyPosture{}
	externalIP := "172.16.0.1"

	printNextSteps(cmd, posture, externalIP, "")

	out := buf.String()

	// Ratify requires L3 proof — passkey is required
	assert.Contains(t, out, "Passkey Enrollment: Required")
	assert.Contains(t, out, "Proof of human presence required for ratify posture (L3 enforced).")
	assert.Contains(t, out, "1. Enroll your CLI identity and register a passkey")
	assert.Contains(t, out, "2. Enroll your passkey (in this terminal)")
	assert.Contains(t, out, "3. Operators:")

	// External IP interpolated correctly
	assert.Contains(t, out, "auth enroll user -e 172.16.0.1")

	// No posture-specific text from the old posture-specific output
	assert.NotContains(t, out, "Configure L2 Consensus")
	assert.NotContains(t, out, "Connect AI agents")
	assert.NotContains(t, out, "Manage & Monitor")
}

func TestPrintNextSteps_NotaryPosture(t *testing.T) {
	cmd, buf := newOutputCmd()
	posture := &governance.NotaryPosture{}
	externalIP := "172.16.0.2"

	printNextSteps(cmd, posture, externalIP, "")

	out := buf.String()

	// Notary requires L3 proof — passkey is required
	assert.Contains(t, out, "Passkey Enrollment: Required")
	assert.Contains(t, out, "Proof of human presence required for notary posture (L3 enforced).")
	assert.Contains(t, out, "1. Enroll your CLI identity and register a passkey")
	assert.Contains(t, out, "2. Enroll your passkey (in this terminal)")
	assert.Contains(t, out, "3. Operators:")

	// External IP interpolated correctly
	assert.Contains(t, out, "auth enroll user -e 172.16.0.2")

	// No posture-specific text from the old posture-specific output
	assert.NotContains(t, out, "Configure L2 Consensus")
	assert.NotContains(t, out, "Connect AI agents")
	assert.NotContains(t, out, "Manage & Monitor")
}

func TestPrintNextSteps_StepNumberingPerPosture(t *testing.T) {
	// All four postures produce the same step numbering (1, 2, 3), but the
	// passkey wording differs: ratify and notary say "Enroll your passkey",
	// while doctrine and consensus say "Enroll a passkey (optional, ...)".
	postures := []struct {
		name    string
		posture governance.GovernancePosture
	}{
		{"doctrine", &governance.DoctrinePosture{}},
		{"consensus", &governance.ConsensusPosture{}},
		{"ratify", &governance.RatifyPosture{}},
		{"notary", &governance.NotaryPosture{}},
	}

	for _, p := range postures {
		t.Run(p.name, func(t *testing.T) {
			cmd, buf := newOutputCmd()
			printNextSteps(cmd, p.posture, "127.0.0.1", "")
			out := buf.String()

			// Verify step numbering is sequential: 1, 2, 3
			assert.Contains(t, out, "  1. Enroll your CLI identity")
			assert.Contains(t, out, "  2. Enroll")
			assert.Contains(t, out, "passkey")
			assert.Contains(t, out, "  3. Operators:")
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
			printNextSteps(cmd, &governance.DoctrinePosture{}, ip, "")
			out := buf.String()

			// The IP should appear in auth enroll user -e and operator start -e
			assert.Contains(t, out, "auth enroll user -e "+ip)
			assert.Contains(t, out, "operator start -e "+ip)
		})
	}
}

func TestPrintNextSteps_PortInterpolation(t *testing.T) {
	cmd, buf := newOutputCmd()
	printNextSteps(cmd, &governance.DoctrinePosture{}, "localhost", "")
	out := buf.String()

	httpPort := constants.Ports.OperatorHttp
	httpsPort := constants.Ports.OperatorHttps

	// HTTP port should appear in the MCP HTTP endpoint line
	assert.Contains(t, out, "http://127.0.0.1:"+itoa(httpPort))

	// HTTPS port should appear in the endpoints section
	assert.Contains(t, out, "localhost:"+itoa(httpsPort))
	assert.Contains(t, out, "/console/")

	// Trust-script URLs must not appear
	assert.NotContains(t, out, "/web-cert.sh")
	assert.NotContains(t, out, "/web-cert.ps1")
}

func TestPrintNextSteps_BinaryNameInterpolation(t *testing.T) {
	cmd, buf := newOutputCmd()
	printNextSteps(cmd, &governance.DoctrinePosture{}, "localhost", "")
	out := buf.String()
	bin := getBinaryName()

	// Binary name should appear in multiple commands
	assert.Contains(t, out, bin+" auth enroll user")
	assert.Contains(t, out, bin+" operator deploy")
	assert.Contains(t, out, bin+" operator stream")
	assert.Contains(t, out, bin+" operator start -e")
}

func TestPrintNextSteps_AllPosturesContainCommonSections(t *testing.T) {
	postures := []struct {
		name    string
		posture governance.GovernancePosture
	}{
		{"doctrine", &governance.DoctrinePosture{}},
		{"consensus", &governance.ConsensusPosture{}},
		{"ratify", &governance.RatifyPosture{}},
		{"notary", &governance.NotaryPosture{}},
	}

	commonSubstrings := []string{
		"Next Steps:",
		"Endpoints:",
		"Enroll your CLI identity",
		"auth enroll",
		"Operators",
		"Gateway Status: Online",
	}

	for _, p := range postures {
		t.Run(p.name, func(t *testing.T) {
			cmd, buf := newOutputCmd()
			printNextSteps(cmd, p.posture, "192.168.1.1", "")
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
		printNextSteps(cmd, &governance.DoctrinePosture{}, "localhost", "")
		out := buf.String()

		assert.NotContains(t, out, "Configure L2 Consensus")
		assert.NotContains(t, out, "Configure L2 Consensus + L3 Notary")
		assert.NotContains(t, out, "WebAuthn/passkey ceremony")
		assert.NotContains(t, out, "Consensus quorum will be rejected")
	})

	t.Run("consensus does not contain doctrine or notary specific text", func(t *testing.T) {
		cmd, buf := newOutputCmd()
		printNextSteps(cmd, &governance.ConsensusPosture{}, "localhost", "")
		out := buf.String()

		assert.NotContains(t, out, "No additional setup required.")
		assert.NotContains(t, out, "Configure L2 Consensus + L3 Notary")
		assert.NotContains(t, out, "WebAuthn/passkey ceremony")
	})

	t.Run("notary does not contain doctrine or consensus specific text", func(t *testing.T) {
		cmd, buf := newOutputCmd()
		printNextSteps(cmd, &governance.NotaryPosture{}, "localhost", "")
		out := buf.String()

		assert.NotContains(t, out, "No additional setup required.")
		assert.NotContains(t, out, "Configure L2 Consensus:")
		assert.NotContains(t, out, "L3 notary is audited only")
	})
}

func TestPrintNextSteps_OutputNotEmpty(t *testing.T) {
	postures := []governance.GovernancePosture{
		&governance.DoctrinePosture{},
		&governance.ConsensusPosture{},
		&governance.RatifyPosture{},
		&governance.NotaryPosture{},
	}

	for i, p := range postures {
		cmd, buf := newOutputCmd()
		printNextSteps(cmd, p, "localhost", "")
		require.NotEmpty(t, buf.String(), "posture index %d produced empty output", i)
	}
}

func TestPrintNextSteps_LineCountReasonable(t *testing.T) {
	cmd, buf := newOutputCmd()
	printNextSteps(cmd, &governance.ConsensusPosture{}, "localhost", "")
	out := buf.String()

	lineCount := strings.Count(out, "\n")
	// The function prints endpoints, status, passkey enrollment, and 4 steps.
	// A reasonable lower bound ensures no section was silently skipped.
	assert.Greater(t, lineCount, 15, "expected substantial output, got %d lines", lineCount)
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

func TestPickHostname(t *testing.T) {
	t.Run("nil identity returns empty", func(t *testing.T) {
		assert.Equal(t, "", pickHostname(nil))
	})

	t.Run("empty hostnames returns empty", func(t *testing.T) {
		id := &network.NetworkIdentity{Hostnames: []string{}}
		assert.Equal(t, "", pickHostname(id))
	})

	t.Run("prefers FQDN over short hostname", func(t *testing.T) {
		id := &network.NetworkIdentity{Hostnames: []string{"dev", "dev.g8e.local"}}
		assert.Equal(t, "dev.g8e.local", pickHostname(id))
	})

	t.Run("falls back to first hostname if no FQDN", func(t *testing.T) {
		id := &network.NetworkIdentity{Hostnames: []string{"dev", "workstation"}}
		assert.Equal(t, "dev", pickHostname(id))
	})

	t.Run("returns first FQDN when multiple FQDNs exist", func(t *testing.T) {
		id := &network.NetworkIdentity{Hostnames: []string{"dev.g8e.local", "dev.example.com"}}
		assert.Equal(t, "dev.g8e.local", pickHostname(id))
	})
}

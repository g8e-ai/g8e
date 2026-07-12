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

	// Step 1: User Workstation (1a scripts, 1b close browsers)
	assert.Contains(t, out, "Next Steps:")
	assert.Contains(t, out, "1. User Workstation")
	assert.Contains(t, out, "a. Trust the gateway CA for HTTPS")
	assert.Contains(t, out, "Linux/macOS")
	assert.Contains(t, out, "curl -fsSL http://192.168.1.100:8080/web-cert.sh | sh")
	assert.Contains(t, out, "Windows")
	assert.Contains(t, out, "irm http://192.168.1.100:8080/web-cert.ps1 | iex")
	assert.Contains(t, out, "b. Close all web browser windows on the remote workstation")

	// Step 2: This Terminal (enroll)
	assert.Contains(t, out, "2. This Terminal")
	assert.Contains(t, out, bin+" auth enroll")

	// Step 3: Operators with Local/Remote Host labels
	assert.Contains(t, out, "3. Operators")
	assert.Contains(t, out, "Local Host:")
	assert.Contains(t, out, bin+" operator deploy --hosts <host1,host2>")
	assert.Contains(t, out, bin+" operator stream --hosts <host1,host2>")
	assert.Contains(t, out, "Remote Host:")
	assert.Contains(t, out, bin+" operator start -e 192.168.1.100")

	// Console UI
	assert.Contains(t, out, "Console UI:")
	assert.Contains(t, out, netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps)+"/console/")

	// Removed sections
	assert.NotContains(t, out, "Connect AI agents")
	assert.NotContains(t, out, "Manage & Monitor")
	assert.NotContains(t, out, "Governance posture:")
	assert.NotContains(t, out, constants.DeployScriptFilenameLinux)
	assert.NotContains(t, out, constants.DeployScriptFilenameWindows)
}

func TestPrintNextSteps_ConsensusPosture(t *testing.T) {
	cmd, buf := newOutputCmd()
	posture := &governance.ConsensusPosture{}
	externalIP := "10.0.0.50"

	printNextSteps(cmd, posture, externalIP)

	out := buf.String()

	// All postures now produce the same output (no posture-specific steps)
	assert.Contains(t, out, "1. User Workstation")
	assert.Contains(t, out, "2. This Terminal")
	assert.Contains(t, out, "3. Operators")

	// External IP interpolated correctly
	assert.Contains(t, out, "http://10.0.0.50:8080/web-cert.sh")

	// No posture-specific text
	assert.NotContains(t, out, "Configure L2 Tribunal")
	assert.NotContains(t, out, "Connect AI agents")
	assert.NotContains(t, out, "Manage & Monitor")
}

func TestPrintNextSteps_NotaryPosture(t *testing.T) {
	cmd, buf := newOutputCmd()
	posture := &governance.NotaryPosture{}
	externalIP := "172.16.0.1"

	printNextSteps(cmd, posture, externalIP)

	out := buf.String()

	// All postures now produce the same output (no posture-specific steps)
	assert.Contains(t, out, "1. User Workstation")
	assert.Contains(t, out, "2. This Terminal")
	assert.Contains(t, out, "3. Operators")

	// External IP interpolated correctly
	assert.Contains(t, out, "http://172.16.0.1:8080/web-cert.sh")

	// No posture-specific text
	assert.NotContains(t, out, "Configure L2 Tribunal")
	assert.NotContains(t, out, "WebAuthn/passkey ceremony")
	assert.NotContains(t, out, "Connect AI agents")
	assert.NotContains(t, out, "Manage & Monitor")
}

func TestPrintNextSteps_StepNumberingPerPosture(t *testing.T) {
	// All three postures produce the same step numbering:
	// 1=Web cert, 2=Close browser, 3=Enroll, 4=Remote operators
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

			// Verify step numbering is sequential: 1, 2, 3
			assert.Contains(t, out, "  1. User Workstation")
			assert.Contains(t, out, "  2. This Terminal")
			assert.Contains(t, out, "  3. Operators")
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

			// The IP should appear in web-cert URL and operator start -e
			assert.Contains(t, out, "http://"+ip+":8080/web-cert.sh")
			assert.Contains(t, out, "http://"+ip+":8080/web-cert.ps1")
			assert.Contains(t, out, "operator start -e "+ip)
		})
	}
}

func TestPrintNextSteps_PortInterpolation(t *testing.T) {
	cmd, buf := newOutputCmd()
	printNextSteps(cmd, &governance.DoctrinePosture{}, "localhost")
	out := buf.String()

	httpPort := constants.Ports.OperatorHttp
	httpsPort := constants.Ports.OperatorHttps

	// HTTP port should appear in web-cert URLs
	assert.Contains(t, out, "localhost:"+itoa(httpPort)+"/web-cert.sh")
	assert.Contains(t, out, "localhost:"+itoa(httpPort)+"/web-cert.ps1")

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
	assert.Contains(t, out, bin+" operator start -e")
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
		"User Workstation",
		"Trust the gateway CA",
		"Close all web browser windows",
		"This Terminal",
		"auth enroll",
		"Operators",
		"Console UI:",
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
	// The function prints 3 steps plus console UI.
	// A reasonable lower bound ensures no section was silently skipped.
	assert.Greater(t, lineCount, 10, "expected substantial output, got %d lines", lineCount)
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

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/v2/internal/cli/tui"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	compliancecatalog "github.com/g8e-ai/g8e/v2/internal/services/compliance/catalog"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/tools/agent_harness/scenarios"
	compliancev1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/compliance/v1"
)

// demoVerbose controls demo output verbosity. When false (default), step-by-step
// command output is suppressed and only scenario results + data dump are shown.
var demoVerbose bool

// demoPrintln prints a line only when demoVerbose is true. Used for scenario
// step descriptions and supplementary output that should be suppressed in
// concise (default) mode. Scenario result lines, error messages, the results
// table, and the data dump always use fmt.Println directly.
func demoPrintln(a ...any) {
	if demoVerbose {
		fmt.Println(a...)
	}
}

// demoPrintf prints formatted output only when demoVerbose is true.
func demoPrintf(format string, a ...any) {
	if demoVerbose {
		fmt.Printf(format, a...)
	}
}

// demoEmitter is the active TUI event emitter, or nil when --tui is not set.
// It is a package-level variable so that deep call chains (dhsScenarioStep,
// demoStep, etc.) can emit TUI events without threading an emitter through
// every function signature.
var demoEmitter DemoEmitter

// DemoEmitter translates demo scenario events into TUI messages.
// When --tui is not active, demoEmitter is nil and all methods are no-ops.
type demoProgram interface {
	Run() (tea.Model, error)
	Send(tea.Msg)
}

type DemoEmitter struct {
	program demoProgram
}

// NewDemoEmitter creates a DemoEmitter backed by the given bubbletea program.
func NewDemoEmitter(p *tea.Program) *DemoEmitter {
	if p == nil {
		return &DemoEmitter{}
	}
	return &DemoEmitter{program: p}
}

// Pipeline emits a pipeline stage update to the TUI.
func (e *DemoEmitter) Pipeline(stage tui.PipelineStage, status tui.PipelineStatus, txID, detail string) {
	if e == nil || e.program == nil {
		return
	}
	e.program.Send(tui.PipelineMsg{Stage: stage, Status: status, TxID: txID, Detail: detail})
}

// Ledger emits a ledger entry to the TUI.
func (e *DemoEmitter) Ledger(level tui.LedgerLevel, message string) {
	if e == nil || e.program == nil {
		return
	}
	e.program.Send(tui.LedgerMsg{Level: level, Message: message})
}

// Consensus emits an L2 consensus update to the TUI.
func (e *DemoEmitter) Consensus(member constants.ConsensusMember, decision, signed bool, quorum, total int, result tui.ConsensusResult, hash string) {
	if e == nil || e.program == nil {
		return
	}
	e.program.Send(tui.ConsensusMsg{Member: member, Decision: decision, Signed: signed, Quorum: quorum, Total: total, Result: result, Hash: hash})
}

// toDockerPath converts a filepath to a Docker-compatible path format.
// On Windows, Docker expects forward slashes even though the OS uses backslashes.
func toDockerPath(path string) string {
	if runtime.GOOS == "windows" {
		return filepath.ToSlash(path)
	}
	return path
}

// checkDockerAvailable verifies that Docker is installed and the daemon is running.
// It returns a user-friendly error with platform-specific guidance if Docker is
// not available.
func checkDockerAvailable() error {
	if _, err := exec.LookPath("docker"); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("%w: Docker is not installed or not on PATH. Install Docker Desktop from https://www.docker.com/products/docker-desktop/", constants.ErrServiceUnavailable)
		}
		return fmt.Errorf("%w: Docker is not installed or not on PATH. Install Docker and ensure 'docker' is in your PATH", constants.ErrServiceUnavailable)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	infoCmd := exec.CommandContext(ctx, "docker", "info")
	infoCmd.Stdout = nil
	infoCmd.Stderr = nil
	if err := infoCmd.Run(); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("%w: Docker daemon is not running. Start Docker Desktop and wait for it to be ready, then try again", constants.ErrServiceUnavailable)
		}
		return fmt.Errorf("%w: Docker daemon is not running. Start the Docker daemon (e.g. 'sudo systemctl start docker') and try again", constants.ErrServiceUnavailable)
	}

	return nil
}

func checkDemoDirExists(demoDir, org string) error {
	if _, err := os.Stat(demoDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: demo environment '%s'. Run 'g8e demos list' to see available demos", constants.ErrNotFound, org)
		}
		return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
	}
	return nil
}

func checkComposeFileExists(composePath, org string) error {
	if _, err := os.Stat(composePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: compose.yml in demo directory '%s'", constants.ErrNotFound, org)
		}
		return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
	}
	return nil
}

func demosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "demos",
		Aliases: []string{"demo"},
		Short:   "Manage g8e demo environments",
		Long: `Manage Docker Compose demo environments for org-specific g8e deployments.
Each org environment is hermetically sealed with no shared state, volumes, or cross-org dependencies.`,
	}

	cmd.AddCommand(
		demosListCmd(),
		demosStartCmd(),
		demosStopCmd(),
		demosStatusCmd(),
		demosCleanCmd(),
		demosResetCmd(),
		demosRebuildCmd(),
		demosRunCmd(),
		demosScenariosCmd(),
		demosPullCmd(),
		demosExportCmd(),
		demosImportCmd(),
		demosImagesCmd(),
	)

	return cmd
}

func demosListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available demo environments",
		RunE:  runDemosList,
	}

	return cmd
}

func demosScenariosCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenarios",
		Short: "List and run demo scenarios against a real Gateway/Operator",
		Long: `List and run demo scenarios against a REAL g8e Gateway + Operator,
exercising the full protocol surface (MCP, A2A, A2A protobuf, and official
governance envelopes with mock consensus + principal signing).

Subcommands:
  list    List all scenarios in run order
  run     Run one or more scenarios against a real Gateway/Operator`,
	}

	cmd.AddCommand(
		demosScenariosListCmd(),
		demosScenariosRunCmd(),
	)

	return cmd
}

func demosScenariosListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all demo scenarios",
		Long: `List all demo scenarios in run order, grouped by governance posture.
Each scenario shows its name, required posture, agent persona, and description.`,
		RunE: runDemosScenarios,
	}

	return cmd
}

func runDemosScenarios(cmd *cobra.Command, args []string) error {
	cmd.Println("scenarios (in run order):")
	for _, s := range scenarios.Registry() {
		cmd.Printf("  %-18s %-9s %-18s %s\n", s.Name, s.RequiresPosture, s.Persona.ID, s.Title)
	}
	return nil
}

func runDemosList(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	demosDir := filepath.Join(cwd, constants.DemosDirname)
	entries, err := os.ReadDir(demosDir)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirectoryRead, err)
	}

	cmd.Println("Available demo environments:")
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != constants.DemosBinDirname {
			composePath := filepath.Join(demosDir, entry.Name(), constants.DemosComposeFile)
			if _, err := os.Stat(composePath); err == nil {
				cmd.Printf("  - %s\n", entry.Name())
			}
		}
	}

	return nil
}

// imageManifestEntry represents a single image in demos/images.json.
type imageManifestEntry struct {
	Image  string   `json:"image"`
	Tag    string   `json:"tag"`
	Digest string   `json:"digest"`
	Demos  []string `json:"demos"`
}

func demosPullCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pre-pull all external images for air-gapped deployment",
		Long: `Pulls all external Docker images listed in demos/images.json.
This is the first step for air-gapped deployment: run this on a connected machine,
then use 'g8e demos export' to create a tar bundle for transfer.`,
		RunE: runDemosPull,
	}

	return cmd
}

func runDemosPull(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	manifestPath := filepath.Join(cwd, constants.DemosDirname, constants.DemosImagesManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading images manifest %s: %w", manifestPath, err)
	}
	var entries []imageManifestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parsing images manifest: %w", err)
	}

	if err := checkDockerAvailable(); err != nil {
		return err
	}

	cmd.Printf("Pulling %d images from demos/images.json...\n", len(entries))
	for i, e := range entries {
		ref := fmt.Sprintf("%s@%s", e.Image, e.Digest)
		cmd.Printf("[%d/%d] Pulling %s\n", i+1, len(entries), ref)
		pullCmd := exec.Command("docker", "pull", ref)
		pullCmd.Stdout = os.Stdout
		pullCmd.Stderr = os.Stderr
		if err := pullCmd.Run(); err != nil {
			return fmt.Errorf("pulling %s: %w", ref, err)
		}
	}

	cmd.Println("\nAll images pulled successfully.")
	cmd.Println("Next step: run 'g8e demos export' to create a transfer bundle.")
	return nil
}

func demosExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export [output-dir]",
		Short: "Save all manifest images to tar files for air-gapped transfer",
		Long: `Saves all Docker images listed in demos/images.json to .tar files.
Run 'g8e demos pull' first on a connected machine, then export.
Defaults to demos/images-export/ if no output directory is specified.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runDemosExport,
	}

	return cmd
}

func runDemosExport(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	manifestPath := filepath.Join(cwd, constants.DemosDirname, constants.DemosImagesManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading images manifest %s: %w", manifestPath, err)
	}
	var entries []imageManifestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parsing images manifest: %w", err)
	}

	outDir := filepath.Join(cwd, constants.DemosDirname, "images-export")
	if len(args) == 1 {
		outDir = args[0]
	}
	if err := os.MkdirAll(outDir, constants.PermDirStandard); err != nil {
		return fmt.Errorf("creating output directory %s: %w", outDir, err)
	}

	if err := checkDockerAvailable(); err != nil {
		return err
	}

	total := len(entries)
	cmd.Printf("Exporting %d images to %s...\n", total, outDir)
	for i, e := range entries {
		ref := fmt.Sprintf("%s@%s", e.Image, e.Digest)
		filename := strings.NewReplacer("/", "_", ":", "_", "@", "_").Replace(ref) + ".tar"
		tarPath := filepath.Join(outDir, filename)

		if _, err := os.Stat(tarPath); err == nil {
			cmd.Printf("[%d/%d] Skipping %s (already exists)\n", i+1, total, ref)
			continue
		}

		cmd.Printf("[%d/%d] Saving %s...\n", i+1, total, ref)
		saveCmd := exec.Command("docker", "save", "-o", tarPath, ref)
		saveCmd.Stdout = os.Stdout
		saveCmd.Stderr = os.Stderr
		if err := saveCmd.Run(); err != nil {
			return fmt.Errorf("saving %s: %w", ref, err)
		}
	}

	cmd.Printf("\nExport complete. %d images saved to %s/\n", total, outDir)
	cmd.Printf("Transfer this directory to the air-gapped machine and run:\n")
	cmd.Printf("  g8e demos import %s\n", outDir)
	return nil
}

func demosImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import [input-dir]",
		Short: "Load tar files into local Docker for air-gapped deployment",
		Long: `Loads all .tar files from the specified directory into the local Docker daemon.
Defaults to demos/images-export/ if no input directory is specified.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runDemosImport,
	}

	return cmd
}

func runDemosImport(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	inDir := filepath.Join(cwd, constants.DemosDirname, "images-export")
	if len(args) == 1 {
		inDir = args[0]
	}
	if _, err := os.Stat(inDir); err != nil {
		return fmt.Errorf("%w: directory %s", constants.ErrPathNotFound, inDir)
	}

	entries, err := os.ReadDir(inDir)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirectoryRead, err)
	}

	var tarFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tar") {
			tarFiles = append(tarFiles, filepath.Join(inDir, entry.Name()))
		}
	}

	if len(tarFiles) == 0 {
		return fmt.Errorf("%w: no .tar files found in %s", constants.ErrNotFound, inDir)
	}

	total := len(tarFiles)
	cmd.Printf("Loading %d images from %s...\n", total, inDir)
	for i, tf := range tarFiles {
		cmd.Printf("[%d/%d] Loading %s...\n", i+1, total, filepath.Base(tf))
		loadCmd := exec.Command("docker", "load", "-i", tf)
		loadCmd.Stdout = os.Stdout
		loadCmd.Stderr = os.Stderr
		if err := loadCmd.Run(); err != nil {
			return fmt.Errorf("loading %s: %w", filepath.Base(tf), err)
		}
	}

	cmd.Printf("\nImport complete. %d images loaded.\n", total)
	cmd.Println("You can now build and run demos in air-gapped mode:")
	cmd.Println("  make build")
	cmd.Println("  g8e demos start <org>")
	return nil
}

func demosImagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "images",
		Short: "List all images in the demo manifest",
		Long:  `Lists all external Docker images from demos/images.json with their pinned digests and associated demos.`,
		RunE:  runDemosImages,
	}

	return cmd
}

func runDemosImages(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	manifestPath := filepath.Join(cwd, constants.DemosDirname, constants.DemosImagesManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading images manifest %s: %w", manifestPath, err)
	}
	var entries []imageManifestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("parsing images manifest: %w", err)
	}

	cmd.Printf("Images in manifest (%s):\n\n", manifestPath)
	for _, e := range entries {
		demos := strings.Join(e.Demos, ", ")
		cmd.Printf("  %s@%s\n", e.Image, e.Digest)
		cmd.Printf("    tag: %s\n", e.Tag)
		cmd.Printf("    demos: %s\n\n", demos)
	}
	return nil
}

func demosStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <org>",
		Short: "Start a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosStart,
	}

	return cmd
}

func runDemosStart(cmd *cobra.Command, args []string) error {
	org := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	if err := checkDemoDirExists(demoDir, org); err != nil {
		return err
	}

	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if err := checkComposeFileExists(composePath, org); err != nil {
		return err
	}

	// Check if g8e binary exists in demos/bin
	binPath := filepath.Join(cwd, constants.DemosDirname, constants.DemosBinDirname, constants.DemosBinaryName)
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}
	if _, err := os.Stat(binPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
		}
		cmd.Printf("Warning: g8e binary not found at %s\n", binPath)
		if runtime.GOOS == "windows" {
			cmd.Printf("Run 'make build' from the repository root, then copy the binary:\n  copy g8e.exe %s\\%s\\g8e.exe\n", constants.DemosDirname, constants.DemosBinDirname)
		} else {
			cmd.Printf("Run 'make build && cp g8e %s/%s/%s' from the repository root to build it.\n", constants.DemosDirname, constants.DemosBinDirname, constants.DemosBinaryName)
		}
	}

	// Pre-flight: verify Docker is available and running
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	// Start the demo environment
	cmd.Printf("Starting demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "up", "-d")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	cmd.Printf("\nDemo environment '%s' started successfully.\n", org)
	cmd.Printf("Run 'g8e demos status %s' to check service status.\n", org)
	cmd.Printf("Run 'g8e demos stop %s' to stop the environment.\n", org)

	// Print endpoint information
	printDemoEndpoints(cmd, org)

	// Print platform enrollment instructions. The operator and its dependents
	// stay not-ready until the owner approves their platform enrollment.
	printPlatformEnrollmentInstructions(cmd, org)

	return nil
}

func printDemoEndpoints(cmd *cobra.Command, org string) {
	cmd.Println("\nAvailable endpoints:")
	switch org {
	case constants.DemosOrgHealthcare:
		cmd.Println("  Gateway HTTP:  http://localhost:8081")
		cmd.Println("  Gateway HTTPS: https://localhost:8444")
		cmd.Println("  Console:       https://localhost:8444/console/")
		cmd.Println("  RabbitMQ UI:   http://localhost:15673")
		cmd.Println("  PostgreSQL:    localhost:5433")
		cmd.Println("  Metabase:      http://localhost:3001")
	case constants.DemosOrgFinance:
		cmd.Println("  Gateway HTTP:  http://localhost:8082")
		cmd.Println("  Gateway HTTPS: https://localhost:8445")
		cmd.Println("  Console:       https://localhost:8445/console/")
		cmd.Println("  Demo UI:       http://localhost:3002")
	case constants.DemosOrgDHS:
		cmd.Println("  Gateway HTTP:  http://localhost:8087")
		cmd.Println("  Gateway HTTPS: https://localhost:8450")
		cmd.Println("  Console:       https://localhost:8450/console/")
	case constants.DemosOrgFedRAMP:
		cmd.Println("  Gateway HTTP:  http://localhost:8088")
		cmd.Println("  Gateway HTTPS: https://localhost:8451")
		cmd.Println("  Console:       https://localhost:8451/console/")
	default:
		cmd.Printf("  No endpoint information available for '%s'\n", org)
	}
}

// demoGatewayHTTPPort maps each demo org to its gateway HTTP discovery port,
// used for the platform enrollment instructions.
var demoGatewayHTTPPort = map[string]string{
	constants.DemosOrgHealthcare: "8081",
	constants.DemosOrgFinance:    "8082",
	constants.DemosOrgDHS:        "8087",
	constants.DemosOrgFedRAMP:    "8088",
}

// demoOperatorContainer maps each demo org to its operator container name,
// used to check operator enrollment readiness before running scenarios.
var demoOperatorContainer = map[string]string{
	constants.DemosOrgHealthcare: "healthcare-operator",
	constants.DemosOrgFinance:    "finance-operator",
	constants.DemosOrgDHS:        "dhs-operator",
	constants.DemosOrgFedRAMP:    "g8e-fedramp-operator",
}

// printPlatformEnrollmentInstructions prints the owner-approved platform
// enrollment flow for a demo org. The gateway starts with zero
// users, so platform workloads (operator and its dependents) stay not-ready
// until the owner enrolls and approves their platform enrollment requests.
func printPlatformEnrollmentInstructions(cmd *cobra.Command, org string) {
	port, ok := demoGatewayHTTPPort[org]
	if !ok {
		return
	}
	cmd.Println()
	cmd.Println("Platform enrollment required:")
	cmd.Println("  The gateway starts with zero users. The operator and its dependents stay")
	cmd.Println("  not-ready until the owner approves their platform enrollment requests.")
	cmd.Println("  Complete the enrollment flow:")
	cmd.Println()
	cmd.Println("  1. Enroll the first owner (creates CLI mTLS credentials):")
	cmd.Printf("     ./g8e auth enroll user -e localhost:%s\n", port)
	cmd.Println()
	cmd.Println("  2. List pending platform enrollment requests:")
	cmd.Println("     ./g8e auth pending-platform-enrollments")
	cmd.Println()
	cmd.Println("  3. Approve each request by ID (operator first, then dependents):")
	cmd.Println("     ./g8e auth approve-platform-enrollment <request-id> --yes")
	cmd.Println()
	cmd.Println("  4. Wait for workload health:")
	cmd.Printf("     g8e demos status %s\n", org)
	cmd.Println()
	cmd.Println("  The console at the HTTPS endpoint above also supports approval.")
}

// operatorEnrolled reports whether the demo operator container's healthcheck
// reports healthy, indicating its platform enrollment has been approved and
// credentials written. Returns false if the container or healthcheck is
// unavailable.
func operatorEnrolled(org string) bool {
	container, ok := demoOperatorContainer[org]
	if !ok {
		return false
	}
	state, err := containerHealthStateFor(container)
	if err != nil {
		return false
	}
	return state == "healthy"
}

// containerHealthStateFor returns the Docker healthcheck state for a container
// ("healthy", "starting", "unhealthy", or "none").
func containerHealthStateFor(container string) (string, error) {
	c := exec.Command("docker", "inspect", "-f", "{{.State.Health.Status}}", container)
	output, err := c.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func demosStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <org>",
		Short: "Stop a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosStop,
	}

	return cmd
}

func runDemosStop(cmd *cobra.Command, args []string) error {
	org := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	if err := checkDemoDirExists(demoDir, org); err != nil {
		return err
	}

	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if err := checkComposeFileExists(composePath, org); err != nil {
		return err
	}

	// Pre-flight: verify Docker is available and running
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	// Stop the demo environment
	cmd.Printf("Stopping demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "down")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
	}

	cmd.Printf("\nDemo environment '%s' stopped successfully.\n", org)

	return nil
}

func demosStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <org>",
		Short: "Show status of a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosStatus,
	}

	return cmd
}

func runDemosStatus(cmd *cobra.Command, args []string) error {
	org := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	if err := checkDemoDirExists(demoDir, org); err != nil {
		return err
	}

	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if err := checkComposeFileExists(composePath, org); err != nil {
		return err
	}

	// Pre-flight: verify Docker is available and running
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	// Show status
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "ps")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInternal, err)
	}

	return nil
}

func demosCleanCmd() *cobra.Command {
	var skipConfirm bool

	cmd := &cobra.Command{
		Use:   "clean [org]",
		Short: "Remove containers, volumes, and networks for demo environments",
		Long: `Remove containers, volumes, and networks for demo environments.
If no org is specified, all demo environments are cleaned.

This is a destructive operation that removes all associated Docker volumes and networks.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDemosClean(cmd, args, skipConfirm)
		},
	}

	cmd.Flags().BoolVar(&skipConfirm, "yes", true, "Skip interactive confirmation (default: true)")

	return cmd
}

func runDemosClean(cmd *cobra.Command, args []string, skipConfirm bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	demosDir := filepath.Join(cwd, constants.DemosDirname)

	if len(args) == 1 {
		org := args[0]
		return cleanSingleDemo(cmd, demosDir, org, skipConfirm)
	}

	return cleanAllDemos(cmd, demosDir, skipConfirm)
}

func cleanSingleDemo(cmd *cobra.Command, demosDir, org string, skipConfirm bool) error {
	demoDir := filepath.Join(demosDir, org)

	if err := checkDemoDirExists(demoDir, org); err != nil {
		return err
	}

	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if err := checkComposeFileExists(composePath, org); err != nil {
		return err
	}

	if !skipConfirm {
		running := isDemoRunning(demoDir, composePath)
		cmd.Printf("WARNING: This will remove all containers, volumes, and networks for '%s'.\n", org)
		if running {
			cmd.Printf("Status: RUNNING\n")
		} else {
			cmd.Printf("Status: not running\n")
		}
		if !confirmAction(cmd, "Proceed with clean?") {
			cmd.Println("Clean cancelled.")
			return nil
		}
	}

	if err := checkDockerAvailable(); err != nil {
		cmd.Printf("Docker not available — nothing to clean for '%s'.\n", org)
		return nil
	}

	cmd.Printf("Cleaning demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "down", "-v", "--remove-orphans", "-t", "0")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		cmd.Printf("Warning: compose down for '%s' had issues: %v\n", org, err)
	}

	forceRemoveLeftovers(cmd, org+"-demo")

	cmd.Printf("\nDemo environment '%s' cleaned successfully.\n", org)
	return nil
}

func cleanAllDemos(cmd *cobra.Command, demosDir string, skipConfirm bool) error {
	entries, err := os.ReadDir(demosDir)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrDirectoryRead, err)
	}

	type demoInfo struct {
		name        string
		demoDir     string
		composePath string
		running     bool
	}

	var demos []demoInfo
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == constants.DemosBinDirname {
			continue
		}
		composePath := filepath.Join(demosDir, entry.Name(), constants.DemosComposeFile)
		if _, err := os.Stat(composePath); err != nil {
			continue
		}
		demoDir := filepath.Join(demosDir, entry.Name())
		demos = append(demos, demoInfo{
			name:        entry.Name(),
			demoDir:     demoDir,
			composePath: composePath,
			running:     isDemoRunning(demoDir, composePath),
		})
	}

	if len(demos) == 0 {
		cmd.Println("No demo environments found.")
		return nil
	}

	cmd.Println("The following demo environments will be cleaned:")
	cmd.Println()
	var runningCount int
	for _, d := range demos {
		status := "stopped"
		if d.running {
			status = "RUNNING"
			runningCount++
		}
		cmd.Printf("  - %-15s  [%s]\n", d.name, status)
	}
	cmd.Println()
	cmd.Printf("Total: %d demo environment(s) (%d running)\n", len(demos), runningCount)
	cmd.Println()
	cmd.Println("WARNING: This will remove ALL containers, volumes, and networks for the above demos.")

	if !skipConfirm {
		if !confirmAction(cmd, "Proceed with cleaning all demos?") {
			cmd.Println("Clean cancelled.")
			return nil
		}
	}

	if err := checkDockerAvailable(); err != nil {
		cmd.Println("Docker not available — nothing to clean.")
		return nil
	}

	for _, d := range demos {
		cmd.Printf("\nCleaning demo environment: %s\n", d.name)
		dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(d.composePath), "down", "-v", "--remove-orphans", "-t", "0")
		dockerComposeCmd.Dir = d.demoDir
		dockerComposeCmd.Stdout = os.Stdout
		dockerComposeCmd.Stderr = os.Stderr

		if err := dockerComposeCmd.Run(); err != nil {
			cmd.Printf("Warning: compose down for '%s' had issues: %v\n", d.name, err)
		}

		forceRemoveLeftovers(cmd, d.name+"-demo")

		cmd.Printf("Demo environment '%s' cleaned successfully.\n", d.name)
	}

	cmd.Println()
	cmd.Printf("All %d demo environment(s) cleaned successfully.\n", len(demos))
	return nil
}

func forceRemoveLeftovers(cmd *cobra.Command, projectPrefix string) {
	// Force-remove any leftover volumes matching the project prefix
	volList := exec.Command("docker", "volume", "ls", "-q", "--filter", "name="+projectPrefix+"_")
	volOut, err := volList.Output()
	if err == nil {
		volumes := strings.Fields(string(volOut))
		for _, v := range volumes {
			rm := exec.Command("docker", "volume", "rm", "-f", v)
			rm.Stdout = os.Stdout
			rm.Stderr = os.Stderr
			if err := rm.Run(); err != nil {
				cmd.Printf("Warning: could not force-remove volume '%s': %v\n", v, err)
			}
		}
	}

	// Force-remove any leftover networks matching the project prefix
	netList := exec.Command("docker", "network", "ls", "-q", "--filter", "name="+projectPrefix+"_")
	netOut, err := netList.Output()
	if err == nil {
		networks := strings.Fields(string(netOut))
		for _, n := range networks {
			rm := exec.Command("docker", "network", "rm", "-f", n)
			rm.Stdout = os.Stdout
			rm.Stderr = os.Stderr
			if err := rm.Run(); err != nil {
				cmd.Printf("Warning: could not force-remove network '%s': %v\n", n, err)
			}
		}
	}
}

func confirmAction(cmd *cobra.Command, prompt string) bool {
	reader := bufio.NewReader(cmd.InOrStdin())
	cmd.Printf("%s [y/N]: ", prompt)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(input))
	return answer == "y" || answer == "yes"
}

func demosRebuildCmd() *cobra.Command {
	var noCache bool

	cmd := &cobra.Command{
		Use:   "rebuild <org>",
		Short: "Rebuild Docker images and restart a demo environment",
		Long: `Rebuild Docker images for a demo environment and restart it.
Stops the environment, rebuilds all images, and starts it again.

Use --no-cache=false to reuse the Docker build cache.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDemosRebuild(cmd, args, noCache)
		},
	}

	cmd.Flags().BoolVar(&noCache, "no-cache", true, "Rebuild without using Docker cache")

	return cmd
}

func runDemosRebuild(cmd *cobra.Command, args []string, noCache bool) error {
	org := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	if err := checkDemoDirExists(demoDir, org); err != nil {
		return err
	}

	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if err := checkComposeFileExists(composePath, org); err != nil {
		return err
	}

	// Pre-flight: verify Docker is available and running
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	cmd.Printf("Stopping demo environment: %s\n", org)
	stopCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "down")
	stopCmd.Dir = demoDir
	stopCmd.Stdout = os.Stdout
	stopCmd.Stderr = os.Stderr
	if err := stopCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
	}

	cmd.Printf("\nRebuilding images for: %s\n", org)
	buildArgs := []string{"compose", "-f", toDockerPath(composePath), "build"}
	if noCache {
		buildArgs = append(buildArgs, "--no-cache")
	}
	buildCmd := exec.Command("docker", buildArgs...)
	buildCmd.Dir = demoDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	cmd.Printf("\nStarting demo environment: %s\n", org)
	upCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "up", "-d")
	upCmd.Dir = demoDir
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	cmd.Printf("\nDemo environment '%s' rebuilt and started successfully.\n", org)
	printDemoEndpoints(cmd, org)
	printPlatformEnrollmentInstructions(cmd, org)

	return nil
}

func demosResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset <org>",
		Short: "Clean and restart a demo environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemosReset,
	}

	return cmd
}

func runDemosReset(cmd *cobra.Command, args []string) error {
	// First clean the environment (skip confirmation since reset is already explicit)
	if err := runDemosClean(cmd, args, true); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
	}

	// Then start it again
	if err := runDemosStart(cmd, args); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	return nil
}

// scenarioCounts maps each org to the number of defined scenarios.
var scenarioCounts = map[string]int{
	constants.DemosOrgHealthcare: 4,
	constants.DemosOrgFinance:    1,
	constants.DemosOrgDHS:        4,
	constants.DemosOrgFedRAMP:    4,
}

func demosRunCmd() *cobra.Command {
	return demosRunCmdWithConfig(newFileSvc)
}

func demosRunCmdWithConfig(fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error)) *cobra.Command {
	var useTUI bool
	cmd := &cobra.Command{
		Use:   "run <org> [scenario]",
		Short: "Run demo scenarios",
		Long: `Run one or all scenarios for a demo environment.
Omit the scenario number to run all scenarios in sequence.

Available scenarios:
  healthcare: 1-4
    1 - Authorized Agent Submits a FHIR PA Request
    2 - Gold Card Auto-Approval
    3 - SLA Breach and OHA Reporting
    4 - Bad Actor PHI Exfiltration Blocked
  finance: 1
    1 - Unauthorized Trade Blocked
  dhs: 1-4
    1 - Sovereign Multi-Source Ingest (chain-of-custody) (LOE 1)
    2 - Resilient Disconnected Operations / Continuity of Coverage (LOE 2)
    3 - Governed Predictive Cueing (LOE 3 & 4)
    4 - Sovereign Destruction + tamper-proof audit (LOE 2)
  fedramp: 1-4
    1 - Governed Cloud Resource Provisioning
    2 - Unauthorized Audit Trail Destruction Blocked (CR-26)
    3 - Governed Configuration Revert (CM-7)
    4 - Gateway Audit Vault Destruction Blocked (CR-26)`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}
			return runDemosRun(cmd, args, useTUI, fileSvc)
		},
	}

	cmd.Flags().BoolVar(&useTUI, "tui", false, "Launch tactical governance TUI overlay")
	cmd.Flags().BoolVarP(&demoVerbose, "verbose", "v", false, "Show step-by-step command output")

	return cmd
}

func runDemosRun(cmd *cobra.Command, args []string, useTUI bool, fileSvc fs.RuntimeFileService) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: demo environment name", constants.ErrMissingRequiredField)
	}
	if len(args) > 2 {
		return fmt.Errorf("%w: accepts at most 2 arguments (demo environment and optional scenario name)", constants.ErrValidationFailed)
	}

	org := args[0]
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}
	demoDir := filepath.Join(cwd, constants.DemosDirname, org)

	if err := checkDemoDirExists(demoDir, org); err != nil {
		return err
	}

	composePath := filepath.Join(demoDir, constants.DemosComposeFile)
	if err := checkComposeFileExists(composePath, org); err != nil {
		return err
	}

	// Pre-flight: verify Docker is available before checking running state
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	// Check if demo is running, start if not
	if !isDemoRunning(demoDir, composePath) {
		cmd.Printf("Demo environment '%s' is not running. Starting it now...\n", org)
		if err := runDemosStart(cmd, args); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
		}
	}

	// Warn if the operator is not yet enrolled. Scenarios require the operator
	// to be healthy (operator.crt exists), which only happens after the owner
	// approves the operator's platform enrollment request.
	if !operatorEnrolled(org) {
		cmd.Println()
		cmd.Printf("Warning: the '%s' operator is not healthy. Scenarios require the operator\n", org)
		cmd.Println("to be enrolled and approved before they can run. If the demo was just")
		cmd.Println("started, complete the platform enrollment flow printed above, then re-run")
		cmd.Printf("this command. Run 'g8e demos status %s' to check readiness.\n", org)
	}

	if useTUI {
		return runDemosWithTUI(cmd, fileSvc, org, demoDir, args)
	}

	if len(args) >= 2 {
		return runScenario(cmd.Context(), fileSvc, org, demoDir, args[1]) //nolint:gosec // length checked above
	}

	return runAllScenarios(cmd.Context(), fileSvc, cmd, org, demoDir)
}

// runDemosWithTUI launches the bubbletea TUI, sets the package-level
// demoEmitter so scenario code can send events, runs the requested scenarios
// in a goroutine, and then waits for the user to quit the TUI. After the TUI
// exits, it cancels and waits for scenario execution so errors are not lost.
func runDemosWithTUI(cmd *cobra.Command, fileSvc fs.RuntimeFileService, org, demoDir string, args []string) error {
	m := tui.NewModel(tui.Options{
		Version:  "tactical",
		NodeName: "tactical-edge-01",
		NetLabel: "AIR-GAP",
	})
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(cmd.Context()))

	demoEmitter = *NewDemoEmitter(p)
	defer func() { demoEmitter = DemoEmitter{} }()

	return runDemosWithTUILifecycle(cmd.Context(), p, func(ctx context.Context) error {
		if len(args) >= 2 {
			return runScenario(ctx, fileSvc, org, demoDir, args[1])
		}
		return runAllScenarios(ctx, fileSvc, cmd, org, demoDir)
	})
}

func runDemosWithTUILifecycle(ctx context.Context, program demoProgram, runScenario func(context.Context) error) error {
	scenarioCtx, cancelScenario := context.WithCancel(ctx)
	defer cancelScenario()

	scenarioErrCh := make(chan error, 1)
	go func() {
		err := runScenario(scenarioCtx)
		status := tui.ScenarioSucceeded
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			status = tui.ScenarioCancelled
		} else if err != nil {
			status = tui.ScenarioFailed
		}
		program.Send(tui.ScenarioCompleteMsg{Status: status})
		scenarioErrCh <- err
	}()

	_, tuiErr := program.Run()
	cancelScenario()
	scenarioErr := <-scenarioErrCh
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%w: %w", constants.ErrDemoScenarioCancelled, ctxErr)
	}
	if tuiErr != nil {
		return fmt.Errorf("tui: run: %w", tuiErr)
	}
	if errors.Is(scenarioErr, context.Canceled) {
		return fmt.Errorf("%w: %w", constants.ErrDemoScenarioCancelled, scenarioErr)
	}
	return scenarioErr
}

func isDemoRunning(demoDir, composePath string) bool {
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "ps", "-q")
	dockerComposeCmd.Dir = demoDir
	output, err := dockerComposeCmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

func runAllScenarios(ctx context.Context, fileSvc fs.RuntimeFileService, cmd *cobra.Command, org, demoDir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	count, ok := scenarioCounts[org]
	if !ok {
		return fmt.Errorf("%w: no scenarios defined for demo environment '%s'", constants.ErrNotFound, org)
	}

	cmd.Printf("\n%s\n  Running all %s demo scenarios\n%s\n",
		strings.Repeat("═", 60), org, strings.Repeat("═", 60))

	results := make([]*compliancev1.DemoScenarioResult, 0, count)

	for i := 1; i <= count; i++ {
		scenarioNum := fmt.Sprintf("%d", i)
		scenarioResults, err := runScenarioWithResults(ctx, fileSvc, org, demoDir, scenarioNum)
		if err != nil {
			return err
		}
		results = append(results, scenarioResults...)
	}

	if org == constants.DemosOrgFedRAMP {
		if !runFedRAMPKSIEvidence(ctx, demoDir) {
			results = append(results, newDemoScenarioResult("KSI", "KSI Evidence Export", demoStatusFailed, "snapshot emission or verification failed"))
		} else {
			results = append(results, newDemoScenarioResult("KSI", "KSI Evidence Export", demoStatusPassed, "snapshots emitted and verified"))
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	printResultsTable(cmd, org, results)

	return summarizeScenarioResults(cmd, org, results)
}

// summarizeScenarioResults prints the pass/fail/skip summary banner for a demo
// run and returns ErrDemoScenarioFailed when any scenario result is FAIL. All
// result rows are retained in the printed table regardless of outcome; this
// only determines the command's exit status so automation cannot treat a failed
// run as a gate pass.
// Demo scenario status values matching the protocol-owned DemoScenarioResult
// status field. These replace the legacy uppercase "PASS"/"FAIL"/"SKIP" strings.
const (
	demoStatusPassed  = "passed"
	demoStatusFailed  = "failed"
	demoStatusSkipped = "skipped"
)

// newDemoScenarioResult constructs a minimal typed DemoScenarioResult populated
// with the display fields used by the results table and summary banner. The
// full evidence-grade fields (assertion refs, framework refs, step results,
// receipt/state refs) are populated by evidence-grade scenario runners.
func newDemoScenarioResult(number, title, status, metrics string) *compliancev1.DemoScenarioResult {
	return &compliancev1.DemoScenarioResult{
		DisplayNumber:  number,
		Title:          title,
		Status:         status,
		MetricsSummary: metrics,
	}
}

func loadDemoScenarioDefinition(scenarioID string) (*compliancev1.DemoScenarioDefinition, error) {
	assertions, frameworks, _, err := compliancecatalog.LoadCanonicalCatalogs()
	if err != nil {
		return nil, fmt.Errorf("load canonical compliance catalogs: %w", err)
	}
	scenarios, err := compliancecatalog.LoadDemoScenarioCatalog(assertions, frameworks)
	if err != nil {
		return nil, fmt.Errorf("load canonical demo scenario catalog: %w", err)
	}
	definition := compliancecatalog.FindDemoScenarioDefinition(scenarios, scenarioID, "1.0.0")
	if definition == nil {
		return nil, fmt.Errorf("%w: %s@1.0.0", constants.ErrUnresolvedReference, scenarioID)
	}
	return definition, nil
}

func newDemoEvidenceScenarioResult(startedAt time.Time, definition *compliancev1.DemoScenarioDefinition, demoID, scopeID, metricsSummary string) *compliancev1.DemoScenarioResult {
	return &compliancev1.DemoScenarioResult{
		ResultId:             fmt.Sprintf("%s-run:%s:%s", demoID, startedAt.Format("20060102T150405Z"), definition.ScenarioId),
		ScenarioRef:          &compliancev1.VersionedReference{Id: definition.ScenarioId, Version: definition.ScenarioVersion},
		DemoId:               demoID,
		ScopeId:              scopeID,
		RunId:                fmt.Sprintf("%s-run-%s", demoID, startedAt.Format("20060102T150405Z")),
		StartedAt:            timestamppb.New(startedAt),
		Status:               demoStatusPassed,
		AssertionRefs:        cloneVersionedRefs(definition.AssertionRefs),
		FrameworkControlRefs: cloneFrameworkControlRefs(definition.FrameworkControlRefs),
		DisplayNumber:        definition.DisplayNumber,
		Title:                definition.Title,
		MetricsSummary:       metricsSummary,
	}
}

// cloneVersionedRefs returns a deep copy of the given versioned references so
// callers cannot mutate the package-level canonical slices.
func cloneVersionedRefs(refs []*compliancev1.VersionedReference) []*compliancev1.VersionedReference {
	clone := make([]*compliancev1.VersionedReference, len(refs))
	for i, ref := range refs {
		clone[i] = &compliancev1.VersionedReference{Id: ref.Id, Version: ref.Version}
	}
	return clone
}

// cloneFrameworkControlRefs returns a deep copy of the given framework control
// references so callers cannot mutate the package-level canonical slices.
func cloneFrameworkControlRefs(refs []*compliancev1.FrameworkControlReference) []*compliancev1.FrameworkControlReference {
	clone := make([]*compliancev1.FrameworkControlReference, len(refs))
	for i, ref := range refs {
		clone[i] = &compliancev1.FrameworkControlReference{
			FrameworkRef: &compliancev1.VersionedReference{Id: ref.FrameworkRef.Id, Version: ref.FrameworkRef.Version},
			ControlId:    ref.ControlId,
		}
	}
	return clone
}

// demoResultFailureError returns ErrDemoScenarioFailed when the typed result
// has a failed status, or nil for passing, skipped, or other non-failing statuses.
func demoResultFailureError(result *compliancev1.DemoScenarioResult, org string) error {
	if result == nil || result.Status != demoStatusFailed {
		return nil
	}
	return fmt.Errorf("%w: %s scenario %s", constants.ErrDemoScenarioFailed, org, result.DisplayNumber)
}

func summarizeScenarioResults(cmd *cobra.Command, org string, results []*compliancev1.DemoScenarioResult) error {
	hasFail, hasSkip := false, false
	for _, r := range results {
		switch r.Status {
		case demoStatusFailed:
			hasFail = true
		case demoStatusSkipped:
			hasSkip = true
		}
	}

	switch {
	case hasFail:
		cmd.Printf("\n%s\n  One or more %s scenarios FAILED.\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
		return fmt.Errorf("%w: %s", constants.ErrDemoScenarioFailed, org)
	case hasSkip:
		cmd.Printf("\n%s\n  All active %s scenarios passed (some skipped — see table).\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	default:
		cmd.Printf("\n%s\n  All %s scenarios passed.\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	}

	return nil
}

func runScenario(ctx context.Context, fileSvc fs.RuntimeFileService, org, demoDir, scenario string) error {
	results, err := runScenarioWithResults(ctx, fileSvc, org, demoDir, scenario)
	if err != nil {
		return err
	}
	for _, result := range results {
		if err := demoResultFailureError(result, org); err != nil {
			return err
		}
	}
	return nil
}

func runScenarioWithResults(ctx context.Context, fileSvc fs.RuntimeFileService, org, demoDir, scenario string) ([]*compliancev1.DemoScenarioResult, error) {
	var results []*compliancev1.DemoScenarioResult
	var err error
	switch org {
	case constants.DemosOrgHealthcare:
		var result *compliancev1.DemoScenarioResult
		result, err = runHealthcareScenario(ctx, demoDir, scenario)
		results = []*compliancev1.DemoScenarioResult{result}
	case constants.DemosOrgFinance:
		var result *compliancev1.DemoScenarioResult
		result, err = runFinanceScenario(ctx, demoDir, scenario)
		results = []*compliancev1.DemoScenarioResult{result}
	case constants.DemosOrgDHS:
		results, err = runDHSScenario(ctx, demoDir, scenario)
	case constants.DemosOrgFedRAMP:
		var result *compliancev1.DemoScenarioResult
		result, err = runFedRAMPScenario(ctx, demoDir, scenario)
		results = []*compliancev1.DemoScenarioResult{result}
	default:
		err = fmt.Errorf("%w: no scenarios defined for demo environment '%s'", constants.ErrNotFound, org)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return results, ctxErr
	}
	for _, result := range results {
		if persistErr := persistDemoScenarioResult(ctx, fileSvc, result); persistErr != nil {
			err = errors.Join(err, persistErr)
		}
	}
	return results, err
}

// titleCase capitalizes the first letter of each word in s, leaving the rest lowercase.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

func printResultsTable(cmd *cobra.Command, org string, results []*compliancev1.DemoScenarioResult) {
	cmd.Printf("\n%s\n  %s Scenario Results Summary\n%s\n",
		strings.Repeat("═", 60), titleCase(org), strings.Repeat("═", 60))
	cmd.Println()

	// Print header
	cmd.Printf("%-10s\t%-50s\t%-12s\t%s\n",
		"Scenario", "Name", "Status", "Key Metrics")
	cmd.Println(strings.Repeat("─", 120))

	// Print rows
	for _, r := range results {
		cmd.Printf("%-10s\t%-50s\t%-12s\t%s\n",
			r.DisplayNumber, r.Title, strings.ToUpper(r.Status), r.MetricsSummary)
	}
	cmd.Println()
}

// demoStep prints a labeled command and runs it, streaming output inline.
// In non-verbose mode, output is suppressed. Always returns error if command
// fails, but only stops execution if fatal is true.
func demoStep(ctx context.Context, demoDir, label string, fatal bool, args ...string) error {
	if demoVerbose {
		fmt.Printf("  $ %s\n", strings.Join(args, " "))
	}
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Dir = demoDir
	if demoVerbose {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	} else {
		c.Stdout = io.Discard
		c.Stderr = io.Discard
	}
	err := c.Run()
	if demoVerbose {
		fmt.Println()
	}
	if err != nil {
		if fatal {
			return fmt.Errorf("%s: %w", label, err)
		}
		return fmt.Errorf("%s failed (non-fatal): %w", label, err)
	}
	return nil
}

// harnessResult is the typed JSON output emitted by `demos scenarios run --json`.
// The parent demo runner parses this to extract authoritative transaction IDs and
// receipt projections retained by the child harness inside the container.
type harnessResult struct {
	Name            string           `json:"name"`
	Title           string           `json:"title"`
	Persona         string           `json:"persona"`
	RequiresPosture string           `json:"requires_posture"`
	StartedAt       time.Time        `json:"started_at"`
	DurationMS      int64            `json:"duration_ms"`
	RunID           string           `json:"run_id,omitempty"`
	ScenarioID      string           `json:"scenario_id,omitempty"`
	TransactionIDs  []string         `json:"transaction_ids,omitempty"`
	Receipts        []harnessReceipt `json:"receipts,omitempty"`
	Notes           []string         `json:"notes,omitempty"`
	OK              bool             `json:"ok"`
	Err             string           `json:"error,omitempty"`
}

type harnessReceipt struct {
	TransactionID   string `json:"transaction_id"`
	TransactionHash string `json:"transaction_hash"`
	SignerKeyID     string `json:"signer_key_id"`
	Signature       string `json:"signature"`
}

// runHarnessWithJSON runs the harness command with --json and parses the typed
// result from stdout. It returns the parsed result and the command error
// separately so the caller can correlate authoritative identity even when the
// harness exits nonzero (e.g. a blocked L1 scenario that still emits a result).
func runHarnessWithJSON(ctx context.Context, demoDir, label string, args []string) ([]harnessResult, error) {
	if demoVerbose {
		fmt.Printf("  $ %s\n", strings.Join(args, " "))
	}
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Dir = demoDir
	var stdout strings.Builder
	c.Stdout = &stdout
	if demoVerbose {
		c.Stderr = os.Stderr
	} else {
		c.Stderr = io.Discard
	}
	runErr := c.Run()
	if demoVerbose {
		fmt.Println()
	}
	raw := strings.TrimSpace(stdout.String())
	if raw == "" {
		return nil, fmt.Errorf("%s: no JSON output: %w", label, runErr)
	}
	var results []harnessResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return nil, fmt.Errorf("%s: decode harness JSON: %w", label, err)
	}
	return results, runErr
}

// applyHarnessAuthoritativeIdentity replaces synthetic transaction and receipt
// references on the DemoScenarioResult with the authoritative values retained
// by the child harness. It fails closed: if the harness emitted no transaction
// IDs or receipts, the result keeps its existing references and the caller is
// expected to mark the step as failed.
func applyHarnessAuthoritativeIdentity(result *compliancev1.DemoScenarioResult, hr *harnessResult) bool {
	if hr == nil || len(hr.TransactionIDs) == 0 || len(hr.Receipts) == 0 {
		return false
	}
	result.TransactionIds = append(result.TransactionIds, hr.TransactionIDs...)
	for _, rcpt := range hr.Receipts {
		result.ReceiptRefs = append(result.ReceiptRefs, "action-receipt:"+rcpt.TransactionID)
	}
	return true
}

// demoStepHTTP runs a curl command that writes the HTTP status code to stdout
// (via -o /dev/null -w "%{http_code}") and validates the response against the
// single expected status code. Unlike demoStep, this checks the actual HTTP
// response status, not just the curl exit code.
func demoStepHTTP(ctx context.Context, demoDir, label string, expectedCode string, args ...string) error {
	return demoStepHTTPAny(ctx, demoDir, label, []string{expectedCode}, args...)
}

// demoStepHTTPAny runs a curl command that writes the HTTP status code to stdout
// (via -o /dev/null -w "%{http_code}") and validates the response against any
// of the expected status codes. Use for endpoints whose correct behavior spans
// multiple codes (e.g. a passkey challenge endpoint that returns 200 for a
// valid body but 400 for a malformed one — both prove the endpoint is
// reachable and enforcing input validation).
func demoStepHTTPAny(ctx context.Context, demoDir, label string, expectedCodes []string, args ...string) error {
	if demoVerbose {
		fmt.Printf("  $ %s\n", strings.Join(args, " "))
	}
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Dir = demoDir
	var stdout strings.Builder
	c.Stdout = &stdout
	if demoVerbose {
		c.Stderr = os.Stderr
	} else {
		c.Stderr = io.Discard
	}
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", label, err)
	}
	actual := strings.TrimSpace(stdout.String())
	if demoVerbose {
		fmt.Printf("  HTTP status: %s (expected one of: %s)\n\n", actual, strings.Join(expectedCodes, ", "))
	}
	for _, code := range expectedCodes {
		if actual == code {
			return nil
		}
	}
	return fmt.Errorf("%s: expected HTTP %s, got %s", label, strings.Join(expectedCodes, " or "), actual)
}

// demoScenarioStep prints the step description, runs the command via demoStep,
// prints pass/fail, and returns whether it succeeded. This extracts the
// repetitive print → demoStep → error pattern from each scenario case block.
func demoScenarioStep(ctx context.Context, demoDir, desc string, cmd []string) bool {
	demoPrintf("  ── %s ──\n", desc)
	if err := demoStep(ctx, demoDir, desc, false, cmd...); err != nil {
		fmt.Printf("  (%s failed)\n\n", desc)
		return false
	}
	return true
}

// demoStepWarn runs a non-critical demoStep and prints a warning on failure
// without setting hasErrors. Use for supplementary verification steps whose
// failure does not invalidate the scenario result.
func demoStepWarn(ctx context.Context, demoDir, label string, args ...string) {
	if err := demoStep(ctx, demoDir, label, false, args...); err != nil {
		fmt.Printf("  (warning: %s failed)\n", label)
	}
}

// harnessConfig holds the fixed connection parameters for building a docker
// compose exec/run command for a demos scenarios run. Centralising these in a
// struct avoids positional-argument drift across demos.
type harnessConfig struct {
	Container  string
	MTLSURL    string
	PublicURL  string
	CertPath   string
	KeyPath    string
	CAPath     string
	RunID      string
	ScenarioID string
	UseRun     bool // true for `docker compose run --rm`, false for `exec`
	JSON       bool // true emits typed JSON results to stdout for parent parsing
}

// defaultHarnessConfig returns the config matching the standard demo topology:
// g8e.local gateway on 8443/8080, operator mTLS certs in the container PKI dir.
func defaultHarnessConfig(container string) harnessConfig {
	return harnessConfig{
		Container: container,
		MTLSURL:   "https://g8e.local:8443",
		PublicURL: "http://g8e.local:8080",
		CertPath:  constants.ContainerOperatorCert,
		KeyPath:   constants.ContainerOperatorKey,
		CAPath:    constants.ContainerCABundle,
	}
}

func harnessConfigForResult(container string, result *compliancev1.DemoScenarioResult) harnessConfig {
	return bindHarnessConfig(defaultHarnessConfig(container), result)
}

func bindHarnessConfig(cfg harnessConfig, result *compliancev1.DemoScenarioResult) harnessConfig {
	cfg.RunID = result.GetRunId()
	cfg.ScenarioID = result.GetScenarioRef().GetId()
	return cfg
}

// harnessRun builds the docker compose command for a demos scenarios run.
// Uses exec by default (long-running sleep-infinity container with a fixed IP).
// When cfg.UseRun is true, uses `docker compose run --rm` instead.
func harnessRun(scenario string, cfg harnessConfig) []string {
	var cmd []string
	if cfg.UseRun {
		cmd = []string{"docker", "compose", "run", "--rm", "-T", "--no-deps"}
	} else {
		cmd = []string{"docker", "compose", "exec", "-T"}
	}
	if cfg.RunID != "" {
		cmd = append(cmd, "-e", string(constants.EnvVar.DemoRunID)+"="+cfg.RunID)
	}
	scenarioID := cfg.ScenarioID
	if scenarioID == "" {
		scenarioID = scenario
	}
	cmd = append(cmd, "-e", string(constants.EnvVar.DemoScenarioID)+"="+scenarioID)
	cmd = append(cmd, cfg.Container)
	if !cfg.UseRun {
		cmd = append(cmd, "/g8e")
	}
	cmd = append(cmd, "demos", "scenarios", "run")
	cmd = append(cmd,
		"--mtls-url", cfg.MTLSURL,
		"--public-url", cfg.PublicURL,
		"--cert", cfg.CertPath,
		"--key", cfg.KeyPath,
		"--ca", cfg.CAPath,
	)
	if cfg.JSON {
		cmd = append(cmd, "--json")
	}
	cmd = append(cmd, scenario)
	return cmd
}

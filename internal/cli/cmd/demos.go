// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/internal/cli/tui"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/tools/agent_harness/scenarios"
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
type DemoEmitter struct {
	program *tea.Program
}

// NewDemoEmitter creates a DemoEmitter backed by the given bubbletea program.
func NewDemoEmitter(p *tea.Program) *DemoEmitter {
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
	case constants.DemosOrgFrontend:
		cmd.Println("  Gateway HTTP:  http://localhost:8083")
		cmd.Println("  Gateway HTTPS: https://localhost:8446")
		cmd.Println("  Console:       https://localhost:8446/console/")
		cmd.Println("  Frontend App:  http://localhost:3003")
	default:
		cmd.Printf("  No endpoint information available for '%s'\n", org)
	}
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
	constants.DemosOrgFrontend:   1,
}

func demosRunCmd() *cobra.Command {
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
    4 - Gateway Audit Vault Destruction Blocked (CR-26)
  frontend: 1
    1 - Third-Party Frontend Enrollment`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDemosRun(cmd, args, useTUI)
		},
	}

	cmd.Flags().BoolVar(&useTUI, "tui", false, "Launch tactical governance TUI overlay")
	cmd.Flags().BoolVarP(&demoVerbose, "verbose", "v", false, "Show step-by-step command output")

	return cmd
}

func runDemosRun(cmd *cobra.Command, args []string, useTUI bool) error {
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

	if useTUI {
		return runDemosWithTUI(cmd, org, demoDir, args)
	}

	if len(args) >= 2 {
		return runScenario(org, demoDir, args[1]) //nolint:gosec // length checked above
	}

	return runAllScenarios(cmd, org, demoDir)
}

// runDemosWithTUI launches the bubbletea TUI, sets the package-level
// demoEmitter so scenario code can send events, runs the requested scenarios
// in a goroutine, and then waits for the user to quit the TUI. After the TUI
// exits, it waits up to 5 seconds for the scenario goroutine to finish so
// errors are not silently lost.
func runDemosWithTUI(cmd *cobra.Command, org, demoDir string, args []string) error {
	m := tui.NewModel(tui.Options{
		Version:  "tactical",
		NodeName: "tactical-edge-01",
		NetLabel: "AIR-GAP",
	})
	p := tea.NewProgram(m, tea.WithAltScreen())

	demoEmitter = *NewDemoEmitter(p)
	defer func() { demoEmitter = DemoEmitter{} }()

	scenarioErrCh := make(chan error, 1)
	go func() {
		if len(args) >= 2 {
			scenarioErrCh <- runScenario(org, demoDir, args[1])
		} else {
			scenarioErrCh <- runAllScenarios(cmd, org, demoDir)
		}
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: run: %w", err)
	}

	select {
	case err := <-scenarioErrCh:
		return err
	case <-time.After(5 * time.Second):
		return nil
	}
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

func runAllScenarios(cmd *cobra.Command, org, demoDir string) error {
	count, ok := scenarioCounts[org]
	if !ok {
		return fmt.Errorf("%w: no scenarios defined for demo environment '%s'", constants.ErrNotFound, org)
	}

	cmd.Printf("\n%s\n  Running all %s demo scenarios\n%s\n",
		strings.Repeat("═", 60), org, strings.Repeat("═", 60))

	results := make([]scenarioResult, 0, count)

	for i := 1; i <= count; i++ {
		scenarioNum := fmt.Sprintf("%d", i)
		result, err := runScenarioWithResult(org, demoDir, scenarioNum)
		if err != nil {
			return err
		}
		results = append(results, result)
	}

	if org == constants.DemosOrgFedRAMP {
		if !runFedRAMPKSIEvidence(demoDir) {
			results = append(results, scenarioResult{
				number:  "KSI",
				name:    "KSI Evidence Export",
				status:  "FAIL",
				metrics: "snapshot emission or verification failed",
			})
		} else {
			results = append(results, scenarioResult{
				number:  "KSI",
				name:    "KSI Evidence Export",
				status:  "PASS",
				metrics: "snapshots emitted and verified",
			})
		}
	}

	printResultsTable(cmd, org, results)

	hasFail, hasSkip := false, false
	for _, r := range results {
		switch r.status {
		case "FAIL":
			hasFail = true
		case "SKIP":
			hasSkip = true
		}
	}

	switch {
	case hasFail:
		cmd.Printf("\n%s\n  One or more %s scenarios FAILED.\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	case hasSkip:
		cmd.Printf("\n%s\n  All active %s scenarios passed (some skipped — see table).\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	default:
		cmd.Printf("\n%s\n  All %s scenarios passed.\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	}

	return nil
}

type scenarioResult struct {
	number  string
	name    string
	status  string
	metrics string
}

func runScenario(org, demoDir, scenario string) error {
	_, err := runScenarioWithResult(org, demoDir, scenario)
	return err
}

func runScenarioWithResult(org, demoDir, scenario string) (scenarioResult, error) {
	switch org {
	case constants.DemosOrgHealthcare:
		return runHealthcareScenario(demoDir, scenario)
	case constants.DemosOrgFinance:
		return runFinanceScenario(demoDir, scenario)
	case constants.DemosOrgDHS:
		return runDHSScenario(demoDir, scenario)
	case constants.DemosOrgFedRAMP:
		return runFedRAMPScenario(demoDir, scenario)
	case constants.DemosOrgFrontend:
		return runFrontendScenario(demoDir, scenario)
	default:
		return scenarioResult{}, fmt.Errorf("%w: no scenarios defined for demo environment '%s'", constants.ErrNotFound, org)
	}
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

func printResultsTable(cmd *cobra.Command, org string, results []scenarioResult) {
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
			r.number, r.name, r.status, r.metrics)
	}
	cmd.Println()
}

// demoStep prints a labeled command and runs it, streaming output inline.
// In non-verbose mode, output is suppressed. Always returns error if command
// fails, but only stops execution if fatal is true.
func demoStep(demoDir, label string, fatal bool, args ...string) error {
	if demoVerbose {
		fmt.Printf("  $ %s\n", strings.Join(args, " "))
	}
	c := exec.Command(args[0], args[1:]...)
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

// demoStepHTTP runs a curl command that writes the HTTP status code to stdout
// (via -o /dev/null -w "%{http_code}") and validates the response against the
// expected status code. Unlike demoStep, this checks the actual HTTP response
// status, not just the curl exit code.
func demoStepHTTP(demoDir, label string, expectedCode string, args ...string) error {
	if demoVerbose {
		fmt.Printf("  $ %s\n", strings.Join(args, " "))
	}
	c := exec.Command(args[0], args[1:]...)
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
		fmt.Printf("  HTTP status: %s (expected: %s)\n\n", actual, expectedCode)
	}
	if actual != expectedCode {
		return fmt.Errorf("%s: expected HTTP %s, got %s", label, expectedCode, actual)
	}
	return nil
}

// demoScenarioStep prints the step description, runs the command via demoStep,
// prints pass/fail, and returns whether it succeeded. This extracts the
// repetitive print → demoStep → error pattern from each scenario case block.
func demoScenarioStep(demoDir, desc string, cmd []string) bool {
	demoPrintf("  ── %s ──\n", desc)
	if err := demoStep(demoDir, desc, false, cmd...); err != nil {
		fmt.Printf("  (%s failed)\n\n", desc)
		return false
	}
	return true
}

// demoStepWarn runs a non-critical demoStep and prints a warning on failure
// without setting hasErrors. Use for supplementary verification steps whose
// failure does not invalidate the scenario result.
func demoStepWarn(demoDir, label string, args ...string) {
	if err := demoStep(demoDir, label, false, args...); err != nil {
		fmt.Printf("  (warning: %s failed)\n", label)
	}
}

type twoLayerScenarioConfig struct {
	scenarioName      string
	metrics           string
	httpPort          string
	harnessScenario   string
	provesDescription string
	step3Label        string
	step3Description  string
	passMessage       string
}

// harnessConfig holds the fixed connection parameters for building a docker
// compose exec/run command for a demos scenarios run. Centralising these in a
// struct avoids positional-argument drift across demos.
type harnessConfig struct {
	Container string
	MTLSURL   string
	PublicURL string
	CertPath  string
	KeyPath   string
	CAPath    string
	UseRun    bool // true for `docker compose run --rm`, false for `exec`
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

// harnessRun builds the docker compose command for a demos scenarios run.
// Uses exec by default (long-running sleep-infinity container with a fixed IP).
// When cfg.UseRun is true, uses `docker compose run --rm` instead.
func harnessRun(scenario string, cfg harnessConfig) []string {
	var cmd []string
	if cfg.UseRun {
		cmd = []string{"docker", "compose", "run", "--rm", "-T", "--no-deps", cfg.Container, "demos", "scenarios", "run"}
	} else {
		cmd = []string{"docker", "compose", "exec", "-T", cfg.Container, "/g8e", "demos", "scenarios", "run"}
	}
	cmd = append(cmd,
		"--mtls-url", cfg.MTLSURL,
		"--public-url", cfg.PublicURL,
		"--cert", cfg.CertPath,
		"--key", cfg.KeyPath,
		"--ca", cfg.CAPath,
		scenario,
	)
	return cmd
}

func runTwoLayerScenario(demoDir string, cfg twoLayerScenarioConfig) (scenarioResult, error) {
	var result scenarioResult
	var hasErrors bool

	result.number = "1"
	result.name = cfg.scenarioName
	result.status = "PASS"
	result.metrics = cfg.metrics

	demoPrintf("\n%s\n", strings.Repeat("─", 60))
	demoPrintf("  Scenario 1 — %s\n", cfg.scenarioName)
	demoPrintln(strings.Repeat("─", 60))
	demoPrintln()
	demoPrintf("  PROVES: %s\n", cfg.provesDescription)
	demoPrintln()

	demoPrintln("  ── Step 1: Confirm g8e gateway is live ──────────────────────")
	if err := demoStep(demoDir, "gateway health",
		false,
		"curl", "-s", "http://localhost:"+cfg.httpPort+"/api/v1/health",
	); err != nil {
		fmt.Println("  (gateway health check failed — is the demo running?)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  ── Step 2: Verify operator enrollment (mTLS certs) ────────────")
	if err := demoStep(demoDir, "enrollment check",
		false,
		"docker", "compose", "exec", "-T", "operator",
		"test", "-f", constants.ContainerOperatorCert,
	); err != nil {
		fmt.Println("  (operator cert not found — operator may not have enrolled correctly)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintf("  ── Step 3: %s ───────\n", cfg.step3Label)
	demoPrintf("  %s\n", cfg.step3Description)
	demoPrintln()
	hcfg := defaultHarnessConfig("agent-runtime")
	hcfg.PublicURL = "http://g8e.local:" + cfg.httpPort
	if err := demoStep(demoDir, cfg.harnessScenario+" via agent",
		false,
		harnessRun(cfg.harnessScenario, hcfg)...,
	); err != nil {
		fmt.Println("  (agent scenario failed)")
		fmt.Println()
		hasErrors = true
	}

	demoPrintln("  ── Step 4: Verify doctrine rejection in gateway logs ──────────")
	if err := demoStep(demoDir, "audit tail",
		false,
		"docker", "compose", "logs", "observability", "--tail", "10",
	); err != nil {
		fmt.Println("  (audit tail failed)")
	}

	demoPrintln("  ── Step 5: Network isolation (supplementary proof) ───────────")
	demoPrintln("  bad-actor (net_untrusted) → target-system (net_secure) — should timeout")
	demoPrintln()
	if err := demoStep(demoDir, "network isolation",
		false,
		"docker", "compose", "exec", "-T", "bad-actor",
		"sh", "-c", "wget -qO- -T 5 http://10.23.0.30:8000/var/g8e/target/ 2>&1 || echo 'BLOCKED: no route from net_untrusted to net_secure'",
	); err != nil {
		fmt.Println("  (network isolation check failed)")
	}

	demoPrintln("  Inspect with: g8e audit receipts | g8e audit events | g8e audit summary")

	if hasErrors {
		result.status = "FAIL"
		fmt.Printf("  [FAIL] Scenario 1 — One or more steps failed.\n")
	} else {
		fmt.Printf("  [PASS] %s\n", cfg.passMessage)
	}

	return result, nil
}

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
	"bytes"
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
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/pathutil"
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

// Consensus emits a tribunal consensus update to the TUI.
func (e *DemoEmitter) Consensus(member string, decision, signed bool, quorum, total int, result tui.ConsensusResult, hash string) {
	if e == nil || e.program == nil {
		return
	}
	e.program.Send(tui.ConsensusMsg{Member: member, Decision: decision, Signed: signed, Quorum: quorum, Total: total, Result: result, Hash: hash})
}

// Quit sends a quit signal to the TUI.
func (e *DemoEmitter) Quit() {
	if e == nil || e.program == nil {
		return
	}
	e.program.Send(tui.QuitMsg{})
}

// DoctrineRule represents a single doctrine rule from the JSON file
type DoctrineRule struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Severity    string  `json:"severity"`
	Pattern     string  `json:"pattern"`
	MitreAttack string  `json:"mitre_attack"`
	MitreTactic string  `json:"mitre_tactic"`
	Confidence  float64 `json:"confidence"`
	Enabled     bool    `json:"enabled"`
}

// DoctrineFile represents the structure of a doctrine JSON file
type DoctrineFile struct {
	Source      string         `json:"source"`
	Version     string         `json:"version"`
	LastUpdated string         `json:"last_updated"`
	License     string         `json:"license"`
	Doctrines   []DoctrineRule `json:"doctrines"`
}

// readDoctrineRule reads a doctrine file and returns a specific rule by ID
func readDoctrineRule(demoDir, doctrineFile, ruleID string) (*DoctrineRule, error) {
	doctrinePath := pathutil.SafeJoin(demoDir, constants.DemosDoctrineDir, doctrineFile)
	data, err := os.ReadFile(doctrinePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrPathNotFound, err)
	}

	var docFile DoctrineFile
	if err := json.Unmarshal(data, &docFile); err != nil {
		return nil, fmt.Errorf("%w: %w", constants.ErrInvalidJSONBody, err)
	}

	for _, rule := range docFile.Doctrines {
		if rule.ID == ruleID {
			return &rule, nil
		}
	}

	return nil, fmt.Errorf("%w: doctrine rule %q", constants.ErrNotFound, ruleID)
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

	infoCmd := exec.Command("docker", "info")
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
		demosAuditCmd(),
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

	fmt.Println("Available demo environments:")
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != constants.DemosBinDirname {
			composePath := filepath.Join(demosDir, entry.Name(), constants.DemosComposeFile)
			if _, err := os.Stat(composePath); err == nil {
				fmt.Printf("  - %s\n", entry.Name())
			}
		}
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
		fmt.Printf("Warning: g8e binary not found at %s\n", binPath)
		if runtime.GOOS == "windows" {
			fmt.Printf("Run 'make build' from the repository root, then copy the binary:\n  copy g8e.exe %s\\%s\\g8e.exe\n", constants.DemosDirname, constants.DemosBinDirname)
		} else {
			fmt.Printf("Run 'make build && cp g8e %s/%s/%s' from the repository root to build it.\n", constants.DemosDirname, constants.DemosBinDirname, constants.DemosBinaryName)
		}
	}

	// Pre-flight: verify Docker is available and running
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	// Start the demo environment
	fmt.Printf("Starting demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "up", "-d")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	fmt.Printf("\nDemo environment '%s' started successfully.\n", org)
	fmt.Printf("Run 'g8e demos status %s' to check service status.\n", org)
	fmt.Printf("Run 'g8e demos stop %s' to stop the environment.\n", org)

	// Print endpoint information
	printDemoEndpoints(org)

	return nil
}

func printDemoEndpoints(org string) {
	fmt.Println("\nAvailable endpoints:")
	switch org {
	case constants.DemosOrgHealthcare:
		fmt.Println("  Gateway HTTP:  http://localhost:8081")
		fmt.Println("  Gateway HTTPS: https://localhost:8444")
		fmt.Println("  Console:       https://localhost:8444/console/")
		fmt.Println("  RabbitMQ UI:   http://localhost:15673")
		fmt.Println("  PostgreSQL:    localhost:5433")
		fmt.Println("  Metabase:      http://localhost:3001")
	case constants.DemosOrgGov:
		fmt.Println("  Gateway HTTP:  http://localhost:8080")
		fmt.Println("  Gateway HTTPS: https://localhost:8443")
		fmt.Println("  Console:       https://localhost:8443/console/")
		fmt.Println("  Demo UI:       http://localhost:3000")
	case constants.DemosOrgFinance:
		fmt.Println("  Gateway HTTP:  http://localhost:8082")
		fmt.Println("  Gateway HTTPS: https://localhost:8445")
		fmt.Println("  Console:       https://localhost:8445/console/")
		fmt.Println("  Demo UI:       http://localhost:3002")
	case constants.DemosOrgSecureData:
		fmt.Println("  Gateway HTTP:  http://localhost:8083")
		fmt.Println("  Gateway HTTPS: https://localhost:8446")
		fmt.Println("  Console:       https://localhost:8446/console/")
		fmt.Println("  Demo UI:       http://localhost:3003")
	case constants.DemosOrgDoW:
		fmt.Println("  Gateway HTTP:  http://localhost:8086")
		fmt.Println("  Gateway HTTPS: https://localhost:8449")
		fmt.Println("  Console:       https://localhost:8449/console/")
	case constants.DemosOrgDHS:
		fmt.Println("  Gateway HTTP:  http://localhost:8087")
		fmt.Println("  Gateway HTTPS: https://localhost:8450")
		fmt.Println("  Console:       https://localhost:8450/console/")
	case constants.DemosOrgSwarm:
		fmt.Println("  Gateway HTTP:  http://localhost:8085")
		fmt.Println("  Gateway HTTPS: https://localhost:8448")
		fmt.Println("  Console:       https://localhost:8448/console/")
	default:
		fmt.Printf("  No endpoint information available for '%s'\n", org)
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
	fmt.Printf("Stopping demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "down")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
	}

	fmt.Printf("\nDemo environment '%s' stopped successfully.\n", org)

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

	cmd.Flags().BoolVar(&skipConfirm, "yes", false, "Skip interactive confirmation")

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

	// Pre-flight: verify Docker is available and running
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	cmd.Printf("Cleaning demo environment: %s\n", org)
	dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "down", "-v")
	dockerComposeCmd.Dir = demoDir
	dockerComposeCmd.Stdout = os.Stdout
	dockerComposeCmd.Stderr = os.Stderr

	if err := dockerComposeCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
	}

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

	// Pre-flight: verify Docker is available and running
	if err := checkDockerAvailable(); err != nil {
		return err
	}

	var failed []string
	for _, d := range demos {
		cmd.Printf("\nCleaning demo environment: %s\n", d.name)
		dockerComposeCmd := exec.Command("docker", "compose", "-f", toDockerPath(d.composePath), "down", "-v")
		dockerComposeCmd.Dir = d.demoDir
		dockerComposeCmd.Stdout = os.Stdout
		dockerComposeCmd.Stderr = os.Stderr

		if err := dockerComposeCmd.Run(); err != nil {
			cmd.Printf("Error cleaning '%s': %v\n", d.name, err)
			failed = append(failed, d.name)
			continue
		}
		cmd.Printf("Demo environment '%s' cleaned successfully.\n", d.name)
	}

	cmd.Println()
	if len(failed) > 0 {
		cmd.Printf("Completed with errors. Failed: %s\n", strings.Join(failed, ", "))
		return fmt.Errorf("%w: failed to clean: %s", constants.ErrProcessStopFailed, strings.Join(failed, ", "))
	}
	cmd.Printf("All %d demo environment(s) cleaned successfully.\n", len(demos))
	return nil
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

	fmt.Printf("Stopping demo environment: %s\n", org)
	stopCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "down")
	stopCmd.Dir = demoDir
	stopCmd.Stdout = os.Stdout
	stopCmd.Stderr = os.Stderr
	if err := stopCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStopFailed, err)
	}

	fmt.Printf("\nRebuilding images for: %s\n", org)
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

	fmt.Printf("\nStarting demo environment: %s\n", org)
	upCmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "up", "-d")
	upCmd.Dir = demoDir
	upCmd.Stdout = os.Stdout
	upCmd.Stderr = os.Stderr
	if err := upCmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
	}

	fmt.Printf("\nDemo environment '%s' rebuilt and started successfully.\n", org)
	printDemoEndpoints(org)

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
	constants.DemosOrgGov:        1,
	constants.DemosOrgFinance:    1,
	constants.DemosOrgSecureData: 3,
	constants.DemosOrgDoW:        3,
	constants.DemosOrgDHS:        5,
	constants.DemosOrgSwarm:      3,
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
  gov: 1
    1 - CUI Exfiltration Attempt Blocked
  finance: 1
    1 - Unauthorized Trade Blocked
  secure-data: 1-3
    1 - Governed Migration with Chain-of-Custody Receipts
    2 - Connector Bypass Attempt Blocked
    3 - Cross-Tenant Leak Doctrine Triggered
  dow: 1-3
    1 - Autonomous SIGINT-to-EO/IR Cross-Cueing (Challenge 5)
    2 - BFT Spoofing Defense (Challenge 8)
    3 - Disconnected Operations (Challenge 6)
  dhs: 1-5
    1 - Sovereign Multi-Source Ingest (chain-of-custody) (LOE 1)
    2 - Cross-Domain Release requires Notary authority (LOE 1 & 2)
    3 - Resilient Disconnected Operations / Continuity of Coverage (LOE 2)
    4 - Governed Predictive Cueing (quorum vs veto) (LOE 3 & 4)
    5 - Sovereign Destruction + tamper-proof audit (LOE 2)
  swarm: 1-3
    1 - Authorized Recon Mission (Governed Drone Deployment)
    2 - Weapons Safety Doctrine Block
    3 - Navigation Boundary Violation Block`,
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

	// Check if demo is running, start if not
	if !isDemoRunning(demoDir, composePath) {
		fmt.Printf("Demo environment '%s' is not running. Starting it now...\n", org)
		if err := runDemosStart(cmd, args); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrProcessStartFailed, err)
		}
	}

	if useTUI {
		return runDemosWithTUI(org, demoDir, args)
	}

	if len(args) >= 2 {
		return runScenario(org, demoDir, args[1]) //nolint:gosec // length checked above
	}

	return runAllScenarios(org, demoDir)
}

// runDemosWithTUI launches the bubbletea TUI, sets the package-level
// demoEmitter so scenario code can send events, runs the requested scenarios
// in a goroutine, and then waits for the user to quit the TUI. After the TUI
// exits, it waits up to 5 seconds for the scenario goroutine to finish so
// errors are not silently lost.
func runDemosWithTUI(org, demoDir string, args []string) error {
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
			scenarioErrCh <- runAllScenarios(org, demoDir)
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

func runAllScenarios(org, demoDir string) error {
	count, ok := scenarioCounts[org]
	if !ok {
		return fmt.Errorf("%w: no scenarios defined for demo environment '%s'", constants.ErrNotFound, org)
	}

	fmt.Printf("\n%s\n  Running all %s demo scenarios\n%s\n",
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

	printResultsTable(org, results)

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
		fmt.Printf("\n%s\n  One or more %s scenarios FAILED.\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	case hasSkip:
		fmt.Printf("\n%s\n  All active %s scenarios passed (some skipped — see table).\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	default:
		fmt.Printf("\n%s\n  All %s scenarios passed.\n%s\n",
			strings.Repeat("═", 60), org, strings.Repeat("═", 60))
	}

	printDataDump(demoDir)
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
		return runHealthcareScenarioWithResult(demoDir, scenario)
	case constants.DemosOrgGov:
		return runGovScenarioWithResult(demoDir, scenario)
	case constants.DemosOrgFinance:
		return runFinanceScenarioWithResult(demoDir, scenario)
	case constants.DemosOrgSecureData:
		return runSecureDataScenarioWithResult(demoDir, scenario)
	case constants.DemosOrgDoW:
		return runDoWScenarioWithResult(demoDir, scenario)
	case constants.DemosOrgDHS:
		return runDHSScenarioWithResult(demoDir, scenario)
	case constants.DemosOrgSwarm:
		return runSwarmScenarioWithResult(demoDir, scenario)
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

func printResultsTable(org string, results []scenarioResult) {
	fmt.Printf("\n%s\n  %s Scenario Results Summary\n%s\n",
		strings.Repeat("═", 60), titleCase(org), strings.Repeat("═", 60))
	fmt.Println()

	// Print header
	fmt.Printf("%-10s\t%-50s\t%-12s\t%s\n",
		"Scenario", "Name", "Status", "Key Metrics")
	fmt.Println(strings.Repeat("─", 120))

	// Print rows
	for _, r := range results {
		fmt.Printf("%-10s\t%-50s\t%-12s\t%s\n",
			r.number, r.name, r.status, r.metrics)
	}
	fmt.Println()
}

// printDataDump queries the gateway's audit API and ledger from inside the
// Docker demo containers, then prints the actual data in tabbed tables.
// This gives the user real receipts, events, and ledger state after a demo run.
func printDataDump(demoDir string) {
	composePath := filepath.Join(demoDir, constants.DemosComposeFile)

	fmt.Printf("\n%s\n  Platform Data Dump\n%s\n",
		strings.Repeat("═", 60), strings.Repeat("═", 60))

	// Build the curl command to query the gateway mTLS API from the operator container.
	// The operator has the client certs and network access to the gateway.
	curlBase := []string{
		"docker", "compose", "-f", toDockerPath(composePath),
		"exec", "-T", "operator",
		"curl", "-s",
		"--cert", constants.ContainerOperatorCert,
		"--key", constants.ContainerOperatorKey,
		"--cacert", constants.ContainerCABundle,
	}

	mtlsURL := "https://g8e.local:8443"

	// ── Receipts ──
	receiptsJSON, err := captureCommand(demoDir, append(curlBase, mtlsURL+constants.APIPaths.AuditReceipts)...)
	if err != nil || strings.TrimSpace(receiptsJSON) == "" {
		fmt.Println("\n  [Receipts] (unavailable)")
	} else {
		var resp models.AuditReceiptsResponse
		if err := json.Unmarshal([]byte(receiptsJSON), &resp); err != nil || !resp.Success {
			fmt.Println("\n  [Receipts] (parse error)")
		} else if len(resp.Receipts) == 0 {
			fmt.Println("\n  [Receipts] None recorded")
		} else {
			fmt.Printf("\n  [Receipts] (%d total)\n", len(resp.Receipts))
			fmt.Printf("  %-16s %-20s %-40s %-8s %s\n",
				"TX HASH", "ACTION", "RESOURCE", "STATUS", "EXECUTED AT")
			fmt.Printf("  %s\n", strings.Repeat("─", 100))
			for _, r := range resp.Receipts {
				txHash := r.TransactionHash
				if len(txHash) > 14 {
					txHash = txHash[:14] + "…"
				}
				resource := r.TargetResource
				if len(resource) > 38 {
					resource = resource[:38] + "…"
				}
				action := string(r.ActionType)
				if len(action) > 18 {
					action = action[:18] + "…"
				}
				fmt.Printf("  %-16s %-20s %-40s %-8s %s\n",
					txHash, action, resource, r.Status.String(),
					r.ExecutedAt.Format(time.RFC3339))
			}
		}
	}

	// ── Audit Events ──
	eventsJSON, err := captureCommand(demoDir, append(curlBase, mtlsURL+constants.APIPaths.AuditEvents+"?limit=20")...)
	if err != nil || strings.TrimSpace(eventsJSON) == "" {
		fmt.Println("\n  [Audit Events] (unavailable)")
	} else {
		var resp models.AuditEventsResponse
		if err := json.Unmarshal([]byte(eventsJSON), &resp); err != nil || !resp.Success {
			fmt.Println("\n  [Audit Events] (parse error)")
		} else if len(resp.Events) == 0 {
			fmt.Println("\n  [Audit Events] None recorded")
		} else {
			fmt.Printf("\n  [Audit Events] (%d shown, %d total)\n", len(resp.Events), resp.Count)
			fmt.Printf("  %-6s %-22s %-30s %-6s %s\n",
				"ID", "TIMESTAMP", "TYPE", "EXIT", "COMMAND")
			fmt.Printf("  %s\n", strings.Repeat("─", 100))
			for _, e := range resp.Events {
				ts := e.Timestamp
				if len(ts) > 20 {
					ts = ts[:20]
				}
				et := e.Type
				if len(et) > 28 {
					et = et[:28] + "…"
				}
				cmd := e.CommandRaw
				if len(cmd) > 35 {
					cmd = cmd[:35] + "…"
				}
				exitCode := "-"
				if e.CommandExitCode != 0 {
					exitCode = fmt.Sprintf("%d", e.CommandExitCode)
				}
				fmt.Printf("  %-6d %-22s %-30s %-6s %s\n", e.ID, ts, et, exitCode, cmd)
			}
		}
	}

	// ── Summary ──
	summaryJSON, err := captureCommand(demoDir, append(curlBase, mtlsURL+constants.APIPaths.AuditSummary)...)
	if err != nil || strings.TrimSpace(summaryJSON) == "" {
		fmt.Println("\n  [Summary] (unavailable)")
	} else {
		var resp models.AuditSummaryResponse
		if err := json.Unmarshal([]byte(summaryJSON), &resp); err != nil || !resp.Success {
			fmt.Println("\n  [Summary] (parse error)")
		} else {
			fmt.Printf("\n  [Summary]\n")
			if resp.EventsTotal > 0 {
				fmt.Printf("  Events (%d total):\n", resp.EventsTotal)
				for et, count := range resp.EventsSummary {
					fmt.Printf("    %-40s %d\n", et, count)
				}
			}
			if resp.ReceiptsTotal > 0 {
				fmt.Printf("  Receipts (%d total):\n", resp.ReceiptsTotal)
				for key, count := range resp.ReceiptsSummary {
					fmt.Printf("    %-40s %d\n", key, count)
				}
			}
			fmt.Printf("  Total records: %d\n", resp.TotalRecords)
		}
	}

	// ── Ledger Files ──
	ledgerOut, err := captureCommand(demoDir,
		"docker", "compose", "-f", toDockerPath(composePath),
		"exec", "-T", "operator",
		"ls", "-la", constants.ContainerLedgerFilesDir+"/",
	)
	if err != nil || strings.TrimSpace(ledgerOut) == "" {
		fmt.Println("\n  [Ledger] (unavailable)")
	} else {
		fmt.Println("\n  [Ledger Files]")
		for _, line := range strings.Split(strings.TrimSpace(ledgerOut), "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	// ── Recent Gateway Logs ──
	logsOut, err := captureCommand(demoDir,
		"docker", "compose", "-f", toDockerPath(composePath),
		"logs", "--tail", "15", "gateway",
	)
	if err != nil || strings.TrimSpace(logsOut) == "" {
		fmt.Println("\n  [Gateway Logs] (unavailable)")
	} else {
		fmt.Println("\n  [Gateway Logs] (last 15 lines)")
		for _, line := range strings.Split(strings.TrimSpace(logsOut), "\n") {
			fmt.Printf("  %s\n", line)
		}
	}

	fmt.Println()
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

// captureCommand runs a command and returns its stdout as a string.
// Used by printDataDump to fetch data from the gateway API.
func captureCommand(demoDir string, args ...string) (string, error) {
	var buf bytes.Buffer
	c := exec.Command(args[0], args[1:]...)
	c.Dir = demoDir
	c.Stdout = &buf
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		return "", err
	}
	return buf.String(), nil
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

// harnessConfig holds the parameters for building a docker compose exec/run
// command for an agent-harness scenario. Centralising these in a struct
// avoids positional-argument drift as flags are added across demos.
type harnessConfig struct {
	Container     string
	MTLSURL       string
	PublicURL     string
	CertPath      string
	KeyPath       string
	CAPath        string
	EnsembleSize  int
	L3Mode        string
	Posture       string
	ConsensusSeed string
	TribunalID    string
	UseRun        bool // true for `docker compose run --rm`, false for `exec`
}

// defaultHarnessConfig returns the config matching the standard demo topology:
// g8e.local gateway on 8443/8080, operator mTLS certs in the container PKI dir,
// ensemble size 3, mock L3 mode.
func defaultHarnessConfig(container string) harnessConfig {
	return harnessConfig{
		Container:    container,
		MTLSURL:      "https://g8e.local:8443",
		PublicURL:    "http://g8e.local:8080",
		CertPath:     constants.ContainerOperatorCert,
		KeyPath:      constants.ContainerOperatorKey,
		CAPath:       constants.ContainerCABundle,
		EnsembleSize: 3,
		L3Mode:       "mock",
	}
}

// harnessRun builds the docker compose command for an agent-harness scenario.
// Uses exec by default (long-running sleep-infinity container with a fixed IP).
// When cfg.UseRun is true, uses `docker compose run --rm` instead.
func harnessRun(scenario string, cfg harnessConfig) []string {
	var cmd []string
	if cfg.UseRun {
		cmd = []string{"docker", "compose", "run", "--rm", "-T", "--no-deps", cfg.Container, "agent", "run"}
	} else {
		cmd = []string{"docker", "compose", "exec", "-T", cfg.Container, "/g8e", "agent", "run"}
	}
	cmd = append(cmd,
		"--mtls-url", cfg.MTLSURL,
		"--public-url", cfg.PublicURL,
		"--cert", cfg.CertPath,
		"--key", cfg.KeyPath,
		"--ca", cfg.CAPath,
	)
	if cfg.EnsembleSize > 0 {
		cmd = append(cmd, "--ensemble", fmt.Sprintf("%d", cfg.EnsembleSize))
	}
	if cfg.L3Mode != "" {
		cmd = append(cmd, "--l3-mode", cfg.L3Mode)
	}
	if cfg.ConsensusSeed != "" {
		cmd = append(cmd, "--consensus-seed", cfg.ConsensusSeed)
	}
	if cfg.TribunalID != "" {
		cmd = append(cmd, "--tribunal-id", cfg.TribunalID)
	}
	cmd = append(cmd, scenario)
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

func demosAuditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit <org> [action]",
		Short: "View audit logs and ledger history for a demo environment",
		Long: `View audit logs and ledger history for a demo environment.
Without an action, it prints a summary of available audit resources.

Actions:
  logs              Tail the observability logs
  receipts          List signed receipts from the gateway
  events            Query raw audit events from the gateway
  summary           Aggregate audit events and receipts by type
  ledger-log        View the git ledger log
  ledger-files      List all files in the git ledger
  ledger-history <file> View git history for a specific file
  ledger-show <hash> View a specific git commit diff
  vault             Open the execution vault database (SQLite)`,
		Args: cobra.MinimumNArgs(1),
		RunE: runDemosAudit,
	}

	return cmd
}

func runDemosAudit(cmd *cobra.Command, args []string) error {
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

	// Check if demo is running
	if !isDemoRunning(demoDir, composePath) {
		return fmt.Errorf("%w: demo environment '%s' is not running. Run 'g8e demos start %s' first", constants.ErrServiceUnavailable, org, org)
	}

	gatewayService := "gateway"

	if len(args) == 1 {
		fmt.Printf("Audit logs and ledger history for: %s\n", org)
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println("Run 'g8e demos audit <org> <action>' to execute these directly.")
		fmt.Println()

		// View operator log (real-time audit stream)
		fmt.Println("1. Real-time audit log stream (operator.log):")
		fmt.Printf("   Action: logs\n")
		fmt.Printf("   Command: docker compose -f %s logs -f observability\n", composePath)
		fmt.Println()

		// Query audit data via g8e CLI
		fmt.Println("2. Audit receipts (signed governance receipts):")
		fmt.Printf("   Action: receipts\n")
		fmt.Printf("   Command: g8e audit receipts --json\n")
		fmt.Println()

		fmt.Println("3. Audit events (raw event log):")
		fmt.Printf("   Action: events\n")
		fmt.Printf("   Command: g8e audit events --limit 20\n")
		fmt.Println()

		fmt.Println("4. Audit summary (aggregated by type):")
		fmt.Printf("   Action: summary\n")
		fmt.Printf("   Command: g8e audit summary\n")
		fmt.Println()

		// View git ledger
		fmt.Println("5. Git ledger history:")
		fmt.Printf("   Action: ledger-log\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sh -c 'cd %s && git log --oneline'\n", composePath, gatewayService, constants.ContainerLedgerFilesDir)
		fmt.Println()
		fmt.Printf("   Action: ledger-files\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sh -c 'cd %s && git ls-files'\n", composePath, gatewayService, constants.ContainerLedgerFilesDir)
		fmt.Println()
		fmt.Printf("   Action: ledger-history <file>\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sh -c 'cd %s && git log --follow -- path/to/file'\n", composePath, gatewayService, constants.ContainerLedgerFilesDir)
		fmt.Println()
		fmt.Printf("   Action: ledger-show <hash>\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sh -c 'cd %s && git show <commit-hash>'\n", composePath, gatewayService, constants.ContainerLedgerFilesDir)
		fmt.Println()

		// View execution vault
		fmt.Println("6. Execution vault (command results and file diffs):")
		fmt.Printf("   Action: vault\n")
		fmt.Printf("   Command: docker compose -f %s exec %s sqlite3 %s\n", composePath, gatewayService, constants.ContainerExecutionVaultDB)
		fmt.Println()
		fmt.Println("   Useful SQL queries:")
		fmt.Println("   SELECT * FROM execution_log ORDER BY timestamp_utc DESC LIMIT 20;")
		fmt.Println("   SELECT * FROM file_diff_log ORDER BY timestamp_utc DESC LIMIT 20;")
		fmt.Println()
		return nil
	}

	action := args[1]
	switch action {
	case "logs":
		return runDockerComposeLogs(demoDir, composePath, "observability")
	case "receipts":
		return runG8EAuditCmd(demoDir, composePath, "receipts")
	case "events":
		return runG8EAuditCmd(demoDir, composePath, "events")
	case "summary":
		return runG8EAuditCmd(demoDir, composePath, "summary")
	case "ledger-log":
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sh", "-c", "cd "+constants.ContainerLedgerFilesDir+" && git log --oneline")
	case "ledger-files":
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sh", "-c", "cd "+constants.ContainerLedgerFilesDir+" && git ls-files")
	case "ledger-history":
		if len(args) < 3 {
			return fmt.Errorf("%w: ledger-history requires a file path", constants.ErrMissingRequiredField)
		}
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sh", "-c", "cd "+constants.ContainerLedgerFilesDir+" && git log --follow -- \"$1\"", "--", args[2])
	case "ledger-show":
		if len(args) < 3 {
			return fmt.Errorf("%w: ledger-show requires a commit hash", constants.ErrMissingRequiredField)
		}
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sh", "-c", "cd "+constants.ContainerLedgerFilesDir+" && git show \"$1\"", "--", args[2])
	case "vault":
		return runDockerComposeExec(demoDir, composePath, gatewayService, "sqlite3", constants.ContainerExecutionVaultDB)
	default:
		return fmt.Errorf("%w: unknown audit action: %s", constants.ErrValidationFailed, action)
	}
}

func runDockerComposeExec(demoDir, composePath, service string, args ...string) error {
	fullArgs := []string{"compose", "-f", toDockerPath(composePath), "exec", service}
	fullArgs = append(fullArgs, args...)

	cmd := exec.Command("docker", fullArgs...)
	cmd.Dir = demoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func runDockerComposeLogs(demoDir, composePath, service string) error {
	cmd := exec.Command("docker", "compose", "-f", toDockerPath(composePath), "logs", "-f", service)
	cmd.Dir = demoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// runG8EAuditCmd runs `g8e audit <subcommand>` inside the operator container,
// which has the mTLS credentials and network access to query the gateway API.
func runG8EAuditCmd(demoDir, composePath, subcommand string) error {
	fullArgs := []string{"compose", "-f", toDockerPath(composePath), "exec", "-T", "operator", "/g8e", "audit", subcommand}
	cmd := exec.Command("docker", fullArgs...)
	cmd.Dir = demoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

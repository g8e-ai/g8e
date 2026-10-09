// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package testcmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

// Environment contract consumed by test/e2e/operator_fleet_e2e_test.go and
// test/e2e/config_helpers.go. The names must match the scenario's constants.
const (
	scaleEnvRuntimeRoot        = "G8E_E2E_RUNTIME_ROOT"
	scaleEnvFleetSize          = "G8E_E2E_FLEET_SIZE"
	scaleEnvFleetBin           = "G8E_E2E_FLEET_BIN"
	scaleEnvFleetSoak          = "G8E_E2E_FLEET_SOAK"
	scaleEnvFleetConcurrency   = "G8E_E2E_FLEET_CONCURRENCY"
	scaleEnvFleetRounds        = "G8E_E2E_FLEET_ROUNDS"
	scaleEnvFleetReport        = "G8E_E2E_FLEET_REPORT"
	scaleEnvFleetRestartReport = "G8E_E2E_FLEET_RESTART_REPORT"
	scaleEnvGatewayHTTPPort    = "G8E_E2E_GATEWAY_HTTP_PORT"
	scaleEnvGatewayHTTPSPort   = "G8E_E2E_GATEWAY_HTTPS_PORT"
)

const (
	scaleFanOutScenario  = "^TestOperatorFleet_HoldsUnderFanOut$"
	scaleRestartScenario = "^TestOperatorFleet_RecoversAfterGatewayRestart$"

	scaleMaxCount    = 5000                                                // operator deploy --start-index ceiling
	scaleMaxParallel = constants.PlatformEnrollmentMaxLiveOperatorRequests // operator deploy --parallel ceiling
	scaleStopGrace   = 5 * time.Second

	// scaleFanOutAll selects one fan-out wave across every target: the fleet
	// plus the embedded Operator.
	scaleFanOutAll       = "all"
	scaleTeardownTimeout = 2 * time.Minute
)

// scaleConfig is the validated input of one scale run.
type scaleConfig struct {
	Count             int
	BatchSize         int
	Parallel          int
	Soak              time.Duration
	FanOutConcurrency string
	Rounds            int
	ScenarioTimeout   time.Duration
	SampleInterval    time.Duration
	Root              string
	Binary            string
	Clean             bool
	SkipRestart       bool
}

// scaleLayout names the directories of one run under its scratch root.
type scaleLayout struct {
	Root  string // evidence root, printed at start and end
	Run   string // Gateway working directory; its .g8e/ tree lives here
	Fleet string // op-NNNNN Operator working directories
	Home  string // isolated HOME/USERPROFILE for every child process
	Out   string // logs, CSV samples, JSON reports
}

func newScaleLayout(root string) scaleLayout {
	return scaleLayout{
		Root:  root,
		Run:   filepath.Join(root, "run"),
		Fleet: filepath.Join(root, "fleet"),
		Home:  filepath.Join(root, "home"),
		Out:   filepath.Join(root, "out"),
	}
}

// scaleProcess is one child process started by the scale run.
type scaleProcess struct {
	Dir  string
	Env  []string
	Log  string // optional file that receives a copy of stdout and stderr
	Name string
	Args []string
}

type scaleRunner func(ctx context.Context, p scaleProcess) (int, error)

// scalePorts are the Gateway listener ports of one run. Every child that dials
// the Gateway is given them explicitly, so the run never depends on, or
// collides with, a Gateway already listening on the default ports.
type scalePorts struct {
	HTTP  int
	HTTPS int
}

// scaleDeps holds the side effects of a scale run so Tier 1 tests can verify
// the orchestration without starting a Gateway or Operator processes.
type scaleDeps struct {
	run           scaleRunner
	reservePorts  func() (scalePorts, error)
	verifyGateway func(runDir string, ports scalePorts) error
	stopWorkers   func(ctx context.Context, fleetDir string) (int, error)
	sample        func(ctx context.Context, layout scaleLayout, interval time.Duration)
	stdout        io.Writer
}

// scaleBatch is one `operator deploy` invocation.
type scaleBatch struct {
	StartIndex int     `json:"start_index"`
	Count      int     `json:"count"`
	Seconds    float64 `json:"seconds"`
}

// scalePhase records the outcome of one phase in the run summary.
type scalePhase struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"seconds"`
	Error   string  `json:"error,omitempty"`
}

// scaleSummary is the machine-readable result of a scale run. It carries no
// identifiers, keys, or tokens.
type scaleSummary struct {
	Count            int          `json:"count"`
	BatchSize        int          `json:"batch_size"`
	Parallel         int          `json:"parallel"`
	Binary           string       `json:"binary"`
	OS               string       `json:"os"`
	GatewayHTTPPort  int          `json:"gateway_http_port"`
	GatewayHTTPSPort int          `json:"gateway_https_port"`
	Batches          []scaleBatch `json:"batches"`
	Phases           []scalePhase `json:"phases"`
	StoppedPIDs      int          `json:"stopped_workers"`
	Passed           bool         `json:"passed"`
	StartedAt        time.Time    `json:"started_at"`
	FinishedAt       time.Time    `json:"finished_at"`
}

func scaleCmd() *cobra.Command {
	return scaleCmdWithDeps(scaleDeps{
		run:           realScaleRunner(os.Stdout, os.Stderr),
		reservePorts:  reserveScalePorts,
		verifyGateway: verifyScaleGatewayOwnership,
		stopWorkers:   stopScaleWorkers,
		sample:        sampleScaleResources,
		stdout:        os.Stdout,
	})
}

func scaleCmdWithDeps(deps scaleDeps) *cobra.Command {
	var cfg scaleConfig

	cmd := &cobra.Command{
		Use:   "scale",
		Short: "Run the Operator fleet scale qualification (isolated Gateway, N real Operators)",
		Long: `Run the Operator fleet scale qualification end to end.

The run starts an isolated doctrine Gateway in a fresh scratch root, enrolls a
headless CLI owner, deploys --count real Operator processes with
"operator deploy --local --background --approve" (by default the whole fleet in
one invocation, staged all at once up to the Gateway live request quota), then
runs the Tier 3 scenarios:

  1. TestOperatorFleet_HoldsUnderFanOut: every Operator enrolls and heartbeats,
     an idle soak of --soak keeps every Operator active, governed fan-out at
     each --fan-out-concurrency level ("all" = every target in one wave)
     succeeds on every target, and a settle
     window afterwards still has no stale Operator.
  2. TestOperatorFleet_RecoversAfterGatewayRestart (unless --skip-restart):
     the Gateway restarts and every Operator reconnects and heartbeats again
     without re-enrolling.

Gateway and fleet resources are sampled every --sample-interval (Linux /proc;
other platforms record database sizes only). Teardown always stops all of this
run's workers at once by their recorded PIDs, then stops the Gateway. It never
touches processes outside the scratch root.

The Gateway listens on two free ports chosen for the run, never the defaults,
so it runs alongside any other Gateway on the host; the Operators, the CLI
owner, and the scenarios are all pointed at those ports. Evidence (logs, CSVs, fleet-report.json,
restart-report.json, scale-summary.json) is kept under <root>/out.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cfg.validate(); err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runScale(ctx, cfg, deps)
		},
	}

	cmd.Flags().IntVar(&cfg.Count, "count", 100, fmt.Sprintf("Remote Operators to deploy (1..%d)", scaleMaxCount))
	cmd.Flags().IntVar(&cfg.BatchSize, "batch-size", 0, "Operators per operator deploy invocation; later batches append with --start-index (default: the whole fleet)")
	cmd.Flags().IntVar(&cfg.Parallel, "parallel", scaleMaxParallel, fmt.Sprintf("Concurrent staging workers per batch (1..%d, the Gateway live Operator request quota)", scaleMaxParallel))
	cmd.Flags().DurationVar(&cfg.Soak, "soak", 5*time.Minute, "Idle steady-state soak before fan-out")
	cmd.Flags().StringVar(&cfg.FanOutConcurrency, "fan-out-concurrency", scaleFanOutAll, `Comma-separated governed fan-out concurrency levels; "all" dispatches to every target at once`)
	cmd.Flags().IntVar(&cfg.Rounds, "rounds", 1, "Fan-out rounds per concurrency level")
	cmd.Flags().DurationVar(&cfg.ScenarioTimeout, "timeout", 0, "Time limit for each Tier 3 scenario (default: soak plus 20m)")
	cmd.Flags().DurationVar(&cfg.SampleInterval, "sample-interval", 10*time.Second, "Resource sampling interval")
	cmd.Flags().StringVar(&cfg.Root, "root", "", "Scratch and evidence root (default: .local.dev/scale/<UTC timestamp> under the working directory)")
	cmd.Flags().StringVar(&cfg.Binary, "binary", "", "g8e binary for the Gateway and Operators (default: this executable)")
	cmd.Flags().BoolVar(&cfg.Clean, "clean", false, "Remove an existing non-empty --root before the run")
	cmd.Flags().BoolVar(&cfg.SkipRestart, "skip-restart", false, "Skip the Gateway restart recovery scenario")

	return cmd
}

func (c *scaleConfig) validate() error {
	switch {
	case c.Count < 1 || c.Count > scaleMaxCount:
		return fmt.Errorf("%w: --count must be 1..%d, got %d", constants.ErrScaleTestInvalidInput, scaleMaxCount, c.Count)
	case c.BatchSize < 0:
		return fmt.Errorf("%w: --batch-size must not be negative, got %d", constants.ErrScaleTestInvalidInput, c.BatchSize)
	case c.Parallel < 1 || c.Parallel > scaleMaxParallel:
		return fmt.Errorf("%w: --parallel must be 1..%d, got %d", constants.ErrScaleTestInvalidInput, scaleMaxParallel, c.Parallel)
	case c.Rounds < 1:
		return fmt.Errorf("%w: --rounds must be positive, got %d", constants.ErrScaleTestInvalidInput, c.Rounds)
	case c.Soak < 0:
		return fmt.Errorf("%w: --soak must not be negative", constants.ErrScaleTestInvalidInput)
	case c.SampleInterval < time.Second:
		return fmt.Errorf("%w: --sample-interval must be at least 1s", constants.ErrScaleTestInvalidInput)
	}
	if c.BatchSize == 0 {
		c.BatchSize = c.Count
	}
	levels := strings.Split(c.FanOutConcurrency, ",")
	for i, part := range levels {
		part = strings.TrimSpace(part)
		if part == scaleFanOutAll {
			levels[i] = strconv.Itoa(c.Count + 1)
			continue
		}
		if level, err := strconv.Atoi(part); err != nil || level < 1 {
			return fmt.Errorf("%w: --fan-out-concurrency entries must be positive integers or %q, got %q", constants.ErrScaleTestInvalidInput, scaleFanOutAll, part)
		}
		levels[i] = part
	}
	c.FanOutConcurrency = strings.Join(levels, ",")
	if c.ScenarioTimeout == 0 {
		c.ScenarioTimeout = c.Soak + 20*time.Minute
	}
	return nil
}

// scaleBatches splits count Operators into deploy invocations of at most size.
func scaleBatches(count, size int) []scaleBatch {
	var batches []scaleBatch
	for start := 1; start <= count; start += size {
		batches = append(batches, scaleBatch{StartIndex: start, Count: min(size, count-start+1)})
	}
	return batches
}

func runScale(ctx context.Context, cfg scaleConfig, deps scaleDeps) (err error) {
	bin, err := resolveScaleBinary(cfg.Binary)
	if err != nil {
		return err
	}
	ports, err := deps.reservePorts()
	if err != nil {
		return fmt.Errorf("%w: reserve Gateway ports: %w", constants.ErrScaleTestFailed, err)
	}
	httpPort, httpsPort := strconv.Itoa(ports.HTTP), strconv.Itoa(ports.HTTPS)
	layout, err := prepareScaleRoot(cfg.Root, cfg.Clean)
	if err != nil {
		return err
	}

	env, err := scaleChildEnv(layout)
	if err != nil {
		return err
	}
	summary := scaleSummary{
		Count: cfg.Count, BatchSize: cfg.BatchSize, Parallel: cfg.Parallel,
		Binary: bin, OS: runtime.GOOS, GatewayHTTPPort: ports.HTTP, GatewayHTTPSPort: ports.HTTPS,
		StartedAt: time.Now().UTC(),
	}
	fmt.Fprintf(deps.stdout, "Scale run root: %s\n", layout.Root)
	fmt.Fprintf(deps.stdout, "Scale Gateway ports: http=%d https=%d\n", ports.HTTP, ports.HTTPS)

	sampleCtx, stopSampling := context.WithCancel(context.Background())
	var sampling sync.WaitGroup
	gatewayStarted := false
	defer func() {
		// Teardown runs on success, failure, and interruption, with its own
		// deadline so a cancelled run context cannot skip it.
		teardownCtx, cancel := context.WithTimeout(context.Background(), scaleTeardownTimeout)
		defer cancel()
		stopSampling()
		sampling.Wait()
		started := time.Now()
		stopped, stopErr := deps.stopWorkers(teardownCtx, layout.Fleet)
		summary.StoppedPIDs = stopped
		if gatewayStarted {
			if _, gwErr := deps.run(teardownCtx, scaleProcess{
				Dir: layout.Run, Env: env, Log: filepath.Join(layout.Out, "gateway-stop.log"),
				Name: bin, Args: []string{"gw", "stop"},
			}); gwErr != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("stop gateway: %w", gwErr))
			}
		}
		summary.Phases = append(summary.Phases, scalePhase{Name: "teardown", Seconds: time.Since(started).Seconds(), Error: errString(stopErr)})
		summary.Passed = err == nil && stopErr == nil
		summary.FinishedAt = time.Now().UTC()
		if writeErr := writeScaleSummary(layout, &summary); writeErr != nil {
			stopErr = errors.Join(stopErr, writeErr)
		}
		printScaleSummary(deps.stdout, layout, &summary)
		if err == nil && stopErr != nil {
			err = fmt.Errorf("%w: teardown: %w", constants.ErrScaleTestFailed, stopErr)
		}
	}()

	phase := func(name string, fn func() error) error {
		fmt.Fprintf(deps.stdout, "== %s\n", name)
		started := time.Now()
		phaseErr := fn()
		summary.Phases = append(summary.Phases, scalePhase{Name: name, Seconds: time.Since(started).Seconds(), Error: errString(phaseErr)})
		if phaseErr != nil {
			return fmt.Errorf("%w: %s: %w", constants.ErrScaleTestFailed, name, phaseErr)
		}
		return nil
	}
	child := func(log string, args ...string) error {
		code, runErr := deps.run(ctx, scaleProcess{Dir: layout.Run, Env: env, Log: filepath.Join(layout.Out, log), Name: bin, Args: args})
		return exitError(code, runErr)
	}

	if err := phase("gateway", func() error {
		if err := child("gateway-start.log", "gw", "start", "--cert-mode", "localhost", "--posture", "doctrine",
			"--http-port", httpPort, "--https-port", httpsPort,
			"--public-spectator=false", "--rate-limit-rps", "0", "--log", "info"); err != nil {
			return fmt.Errorf("gw start: %w", err)
		}
		if deps.verifyGateway != nil {
			if err := deps.verifyGateway(layout.Run, ports); err != nil {
				return fmt.Errorf("verify gateway ownership: %w", err)
			}
		}
		gatewayStarted = true
		if err := child("auth-enroll.log", "auth", "enroll", "user", "-e", "localhost:"+httpPort, "-p", httpsPort, "--headless"); err != nil {
			return fmt.Errorf("auth enroll user: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	sampling.Add(1)
	go func() {
		defer sampling.Done()
		deps.sample(sampleCtx, layout, cfg.SampleInterval)
	}()

	if err := phase("enrollment", func() error {
		for i, batch := range scaleBatches(cfg.Count, cfg.BatchSize) {
			started := time.Now()
			runErr := child(fmt.Sprintf("deploy-%03d.log", i+1), "operator", "deploy", "--local",
				"--dest-dir", layout.Fleet, "--count", strconv.Itoa(batch.Count), "--start-index", strconv.Itoa(batch.StartIndex),
				"--roles", "data", "-e", "localhost", "--gateway-http-port", httpPort, "--gateway-https-port", httpsPort, "--background", "--approve", "--parallel", strconv.Itoa(cfg.Parallel), "--log", "info")
			batch.Seconds = time.Since(started).Seconds()
			summary.Batches = append(summary.Batches, batch)
			fmt.Fprintf(deps.stdout, "batch %d: op-%05d..op-%05d in %.1fs\n", i+1, batch.StartIndex, batch.StartIndex+batch.Count-1, batch.Seconds)
			if runErr != nil {
				return fmt.Errorf("operator deploy --start-index %d --count %d: %w", batch.StartIndex, batch.Count, runErr)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	scenarioEnv := append(scaleScenarioEnv(env, layout, cfg, bin, ports), scaleEnvFleetReport+"="+filepath.Join(layout.Out, "fleet-report.json"))
	if err := phase("steady state, soak, and fan-out", func() error {
		return scaleScenario(ctx, deps, scenarioEnv, layout, cfg, scaleFanOutScenario, "scenario-fan-out.log")
	}); err != nil {
		return err
	}

	if cfg.SkipRestart {
		return nil
	}
	restartEnv := append(scenarioEnv, scaleEnvFleetRestartReport+"="+filepath.Join(layout.Out, "restart-report.json"))
	return phase("gateway restart recovery", func() error {
		return scaleScenario(ctx, deps, restartEnv, layout, cfg, scaleRestartScenario, "scenario-restart.log")
	})
}

// scaleScenario runs one Tier 3 fleet scenario through go test from the
// repository root, matching the arguments of `g8e test e2e`.
func scaleScenario(ctx context.Context, deps scaleDeps, env []string, layout scaleLayout, cfg scaleConfig, run, log string) error {
	repo, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	args := []string{"test", "-tags=e2e", "-count=1", "-parallel=1", "-timeout", cfg.ScenarioTimeout.String(), "-v"}
	if runtime.GOOS != "windows" {
		args = append(args, "-race")
	}
	args = append(args, "-run", run, "./test/e2e/...")
	code, runErr := deps.run(ctx, scaleProcess{Dir: repo, Env: env, Log: filepath.Join(layout.Out, log), Name: "go", Args: args})
	return exitError(code, runErr)
}

func scaleScenarioEnv(env []string, layout scaleLayout, cfg scaleConfig, bin string, ports scalePorts) []string {
	return append(append([]string(nil), env...),
		scaleEnvRuntimeRoot+"="+layout.Run,
		scaleEnvFleetSize+"="+strconv.Itoa(cfg.Count),
		scaleEnvFleetBin+"="+bin,
		scaleEnvFleetSoak+"="+cfg.Soak.String(),
		scaleEnvFleetConcurrency+"="+cfg.FanOutConcurrency,
		scaleEnvFleetRounds+"="+strconv.Itoa(cfg.Rounds),
		scaleEnvGatewayHTTPPort+"="+strconv.Itoa(ports.HTTP),
		scaleEnvGatewayHTTPSPort+"="+strconv.Itoa(ports.HTTPS),
	)
}

// reserveScalePorts asks the OS for two distinct free TCP ports on the
// wildcard address the Gateway binds, skipping the Gateway's own auxiliary
// listener ports. The ports are released before return, so the Gateway bind
// can still race another process; a Gateway that then falls back to other
// ports fails the deploy preflight, which dials exactly these.
func reserveScalePorts() (scalePorts, error) {
	var ports []int
	var listeners []net.Listener
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	for len(ports) < 2 {
		l, err := net.Listen("tcp", ":0")
		if err != nil {
			return scalePorts{}, fmt.Errorf("%w: %w", constants.ErrPortUnavailable, err)
		}
		listeners = append(listeners, l)
		port := l.Addr().(*net.TCPAddr).Port
		if _, reserved := constants.GatewayReservedLoopbackPorts[port]; !reserved {
			ports = append(ports, port)
		}
	}
	return scalePorts{HTTP: ports[0], HTTPS: ports[1]}, nil
}

// verifyScaleGatewayOwnership asserts that the Gateway running in runDir was
// started by this scale run and bound to the reserved ports.
func verifyScaleGatewayOwnership(runDir string, ports scalePorts) error {
	fileSvc, err := fs.NewRuntimeFileService(runDir, slog.Default())
	if err != nil {
		return fmt.Errorf("create runtime file service: %w", err)
	}
	pm, err := platform.NewProcessManager(fileSvc)
	if err != nil {
		return fmt.Errorf("create process manager: %w", err)
	}
	pid, err := pm.ReadPIDFile(constants.OperatorPIDFilename)
	if err != nil {
		return fmt.Errorf("read gateway pid: %w", err)
	}
	if pid == 0 {
		return fmt.Errorf("gateway pid file missing or empty in %s", runDir)
	}
	if !pm.IsProcessRunning(pid) {
		return fmt.Errorf("gateway process (pid %d) is not running", pid)
	}
	profile, err := serve.ReadLaunchProfile(fileSvc)
	if err != nil {
		return fmt.Errorf("read gateway launch profile: %w", err)
	}
	if profile.Config.HTTPPort != ports.HTTP {
		return fmt.Errorf("gateway launch profile http port %d does not match reserved port %d", profile.Config.HTTPPort, ports.HTTP)
	}
	if profile.Config.HTTPSPort != ports.HTTPS {
		return fmt.Errorf("gateway launch profile https port %d does not match reserved port %d", profile.Config.HTTPSPort, ports.HTTPS)
	}
	return nil
}


// scaleChildEnv isolates HOME and USERPROFILE from the developer's while
// keeping the Go caches, so the scenario neither recompiles everything nor
// needs the network.
func scaleChildEnv(layout scaleLayout) ([]string, error) {
	goEnv := map[string]string{}
	for _, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		value := os.Getenv(key)
		if value == "" {
			out, err := exec.Command("go", "env", key).Output()
			if err != nil {
				return nil, fmt.Errorf("%w: go env %s: %w", constants.ErrScaleTestFailed, key, err)
			}
			value = strings.TrimSpace(string(out))
		}
		goEnv[key] = value
	}
	var env []string
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(key) {
		case "HOME", "USERPROFILE", "GOCACHE", "GOMODCACHE", "GOPATH",
			scaleEnvRuntimeRoot, scaleEnvFleetSize, scaleEnvFleetBin, scaleEnvFleetSoak,
			scaleEnvFleetConcurrency, scaleEnvFleetRounds, scaleEnvFleetReport, scaleEnvFleetRestartReport,
			scaleEnvGatewayHTTPPort, scaleEnvGatewayHTTPSPort,
			string(constants.EnvVar.E2EFleetSessions):
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+layout.Home, "USERPROFILE="+layout.Home)
	for _, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		env = append(env, key+"="+goEnv[key])
	}
	return env, nil
}

func resolveScaleBinary(binary string) (string, error) {
	if binary == "" {
		exe, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("%w: resolve current executable: %w", constants.ErrScaleTestFailed, err)
		}
		binary = exe
	}
	abs, err := filepath.Abs(binary)
	if err != nil {
		return "", fmt.Errorf("%w: resolve binary %s: %w", constants.ErrScaleTestFailed, binary, err)
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		return "", fmt.Errorf("%w: binary %s is not a file", constants.ErrScaleTestInvalidInput, abs)
	}
	return abs, nil
}

// defaultScaleRoot is the per-run root under the working directory's
// .local.dev/scale, named by the UTC start time.
func defaultScaleRoot(now time.Time) string {
	return filepath.Join(filepath.FromSlash(constants.ScaleRunsDirPath), now.UTC().Format(constants.ScaleRunTimestampFormat))
}

// prepareScaleRoot creates a fresh scratch root. An existing non-empty root is
// refused unless clean is set, so a run never mixes with an earlier Gateway's
// registry or Operator identities.
func prepareScaleRoot(root string, clean bool) (scaleLayout, error) {
	if root == "" {
		root = defaultScaleRoot(time.Now())
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return scaleLayout{}, fmt.Errorf("%w: resolve root: %w", constants.ErrScaleTestFailed, err)
	}
	entries, err := os.ReadDir(root)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return scaleLayout{}, fmt.Errorf("%w: read root %s: %w", constants.ErrScaleTestFailed, root, err)
	case len(entries) > 0 && !clean:
		return scaleLayout{}, fmt.Errorf("%w: root %s is not empty; choose a new --root or pass --clean", constants.ErrScaleTestInvalidInput, root)
	case len(entries) > 0:
		if err := os.RemoveAll(root); err != nil {
			return scaleLayout{}, fmt.Errorf("%w: clean root %s: %w", constants.ErrScaleTestFailed, root, err)
		}
	}
	layout := newScaleLayout(root)
	for _, dir := range []string{layout.Run, layout.Fleet, layout.Home, layout.Out} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return scaleLayout{}, fmt.Errorf("%w: create %s: %w", constants.ErrScaleTestFailed, dir, err)
		}
	}
	return layout, nil
}

// realScaleRunner runs a child process, streaming its output to the parent and
// to the optional log file.
func realScaleRunner(stdout, stderr io.Writer) scaleRunner {
	return func(ctx context.Context, p scaleProcess) (int, error) {
		c := exec.CommandContext(ctx, p.Name, p.Args...)
		c.Dir, c.Env = p.Dir, p.Env
		c.Stdout, c.Stderr = stdout, stderr
		if p.Log != "" {
			log, err := os.OpenFile(p.Log, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return -1, fmt.Errorf("open log %s: %w", p.Log, err)
			}
			defer log.Close()
			c.Stdout, c.Stderr = io.MultiWriter(stdout, log), io.MultiWriter(stderr, log)
		}
		if err := c.Run(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				return exitErr.ExitCode(), err
			}
			return -1, err
		}
		return 0, nil
	}
}

// scaleWorkerPIDs reads the PID recorded by `operator deploy --background` in
// each op-NNNNN directory of this run's fleet.
func scaleWorkerPIDs(fleetDir string) ([]int, error) {
	dirs, err := filepath.Glob(filepath.Join(fleetDir, "op-*"))
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, dir := range dirs {
		data, err := os.ReadFile(filepath.Join(dir, constants.OperatorPIDFilename))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return pids, fmt.Errorf("read worker PID in %s: %w", dir, err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			return pids, fmt.Errorf("invalid worker PID in %s: %q", dir, strings.TrimSpace(string(data)))
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// stopScaleWorkers stops all of this run's workers at once: TERM where the
// platform supports it, then KILL after a grace period. Only PIDs recorded
// under fleetDir are signalled, so no unrelated Operator is affected.
func stopScaleWorkers(ctx context.Context, fleetDir string) (int, error) {
	pids, err := scaleWorkerPIDs(fleetDir)
	var wg sync.WaitGroup
	for _, pid := range pids {
		wg.Go(func() { stopScaleWorker(ctx, pid) })
	}
	wg.Wait()
	return len(pids), err
}

func stopScaleWorker(ctx context.Context, pid int) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return // already gone (Windows opens a handle here)
	}
	defer proc.Release()
	if runtime.GOOS == "windows" {
		_ = proc.Kill()
		return
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return // already gone
	}
	deadline := time.Now().Add(scaleStopGrace)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if proc.Signal(syscall.Signal(0)) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = proc.Kill()
}

// sampleScaleResources appends one row per interval to gateway-resources.csv
// and fleet-resources.csv until ctx is cancelled. The Gateway PID is re-read
// each tick so samples continue across the restart scenario.
func sampleScaleResources(ctx context.Context, layout scaleLayout, interval time.Duration) {
	gwFile, err := os.OpenFile(filepath.Join(layout.Out, "gateway-resources.csv"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer gwFile.Close()
	fleetFile, err := os.OpenFile(filepath.Join(layout.Out, "fleet-resources.csv"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer fleetFile.Close()
	gwCSV, fleetCSV := csv.NewWriter(gwFile), csv.NewWriter(fleetFile)
	_ = gwCSV.Write([]string{"timestamp", "pid", "rss_kb", "threads", "open_fds", "db_bytes", "wal_bytes"})
	_ = fleetCSV.Write([]string{"timestamp", "procs", "rss_kb", "pss_kb", "pss_kb_per_op", "threads"})

	fileSvc, fsErr := fs.NewRuntimeFileService(layout.Run, nil)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		ts := time.Now().UTC().Format(time.RFC3339)

		var gw scaleProcStats
		pid := 0
		var dbBytes, walBytes int64
		if fsErr == nil {
			if data, err := fileSvc.ReadFile(ctx, filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)); err == nil {
				pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			}
			if entries, err := fileSvc.ReadDir(ctx, constants.DataDirname); err == nil {
				for _, entry := range entries {
					info, err := entry.Info()
					if err != nil {
						continue
					}
					switch {
					case strings.HasSuffix(entry.Name(), ".db"):
						dbBytes += info.Size()
					case strings.HasSuffix(entry.Name(), ".db-wal"):
						walBytes += info.Size()
					}
				}
			}
		}
		if pid > 0 {
			gw, _ = readScaleProcStats(pid)
		}
		_ = gwCSV.Write([]string{ts, strconv.Itoa(pid), itoa64(gw.RSSKB), strconv.Itoa(gw.Threads), strconv.Itoa(gw.FDs),
			itoa64(dbBytes), itoa64(walBytes)})
		gwCSV.Flush()

		var fleet scaleProcStats
		procs := 0
		pids, _ := scaleWorkerPIDs(layout.Fleet)
		for _, workerPID := range pids {
			stats, ok := readScaleProcStats(workerPID)
			if !ok {
				continue
			}
			procs++
			fleet.RSSKB += stats.RSSKB
			fleet.PSSKB += stats.PSSKB
			fleet.Threads += stats.Threads
		}
		perOp := int64(0)
		if procs > 0 {
			perOp = fleet.PSSKB / int64(procs)
		}
		_ = fleetCSV.Write([]string{ts, strconv.Itoa(procs), itoa64(fleet.RSSKB), itoa64(fleet.PSSKB), itoa64(perOp), strconv.Itoa(fleet.Threads)})
		fleetCSV.Flush()

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// scaleProcStats is one process observation; fields are zero where the
// platform does not expose them.
type scaleProcStats struct {
	RSSKB   int64
	PSSKB   int64
	Threads int
	FDs     int
}

func writeScaleSummary(layout scaleLayout, summary *scaleSummary) error {
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("encode scale summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(layout.Out, "scale-summary.json"), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write scale summary: %w", err)
	}
	return nil
}

func printScaleSummary(w io.Writer, layout scaleLayout, summary *scaleSummary) {
	fmt.Fprintln(w, strings.Repeat("=", 72))
	fmt.Fprintf(w, "Operator scale run: count=%d batch=%d parallel=%d passed=%t\n", summary.Count, summary.BatchSize, summary.Parallel, summary.Passed)
	for _, p := range summary.Phases {
		status := "ok"
		if p.Error != "" {
			status = "FAILED: " + p.Error
		}
		fmt.Fprintf(w, "  %-34s %9.1fs  %s\n", p.Name, p.Seconds, status)
	}
	fmt.Fprintf(w, "  workers stopped: %d\n", summary.StoppedPIDs)
	fmt.Fprintf(w, "Evidence: %s\n", layout.Out)
}

func exitError(code int, err error) error {
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit code %d", code)
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

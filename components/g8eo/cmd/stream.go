package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

const (
	defaultConcurrency = 50
	defaultTimeout     = 60 * time.Second
	defaultArch        = "amd64"
	defaultBinaryDir   = "/home/g8e"
)

// StreamStatusEvent is written as a JSON line to stdout for each host event.
type StreamStatusEvent struct {
	Host      string                 `json:"host,omitempty"`
	Status    constants.StreamStatus `json:"status"`
	SizeBytes int64                  `json:"size_bytes,omitempty"`
	Error     string                 `json:"error,omitempty"`
	ElapsedMs int64                  `json:"elapsed_ms,omitempty"`
	Ts        time.Time              `json:"ts"`

	// Set only on the terminal summary line
	Summary bool  `json:"summary,omitempty"`
	Total   int   `json:"total,omitempty"`
	Success int   `json:"success,omitempty"`
	Failed  int   `json:"failed,omitempty"`
	TotalMs int64 `json:"total_ms,omitempty"`
}

// RunStream is the entry point for `g8e.operator stream`.
// It runs inside the g8ep container and streams the operator binary to
// one or more remote hosts concurrently via native Go crypto/ssh.
func RunStream(args []string) {
	fs := flag.NewFlagSet("stream", flag.ContinueOnError)

	var (
		arch         string
		hostsFile    string
		concurrency  int
		timeoutSec   int
		endpoint     string
		deviceToken  string
		apiKey       string
		noGit        bool
		sshConfigArg string
		binaryDir    string
	)

	fs.StringVar(&arch, "arch", defaultArch, "Target architecture: amd64, arm64, 386")
	fs.StringVar(&hostsFile, "hosts", "", "File of hosts (one per line) or - for stdin")
	fs.IntVar(&concurrency, "concurrency", defaultConcurrency, "Max parallel SSH sessions")
	fs.IntVar(&timeoutSec, "timeout", int(defaultTimeout.Seconds()), "Per-host dial+inject timeout in seconds")
	fs.StringVar(&endpoint, "endpoint", "", "Platform endpoint — if set, starts operator on each remote host")
	fs.StringVar(&deviceToken, "device-token", "", "Device link token (supports single and mass deployment via max_uses)")
	fs.StringVar(&apiKey, "key", "", "API key auth")
	fs.BoolVar(&noGit, "no-git", false, "Disable ledger")
	fs.StringVar(&sshConfigArg, "ssh-config", "", "Path to SSH config file (default: ~/.ssh/config)")
	fs.StringVar(&binaryDir, "binary-dir", defaultBinaryDir, "Directory containing arch-specific operator builds")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			printStreamUsage()
			os.Exit(constants.ExitSuccess)
		}
		fmt.Fprintf(os.Stderr, "[stream] flag error: %v\n", err)
		os.Exit(constants.ExitGeneralError)
	}

	// Collect positional host arguments
	positionalHosts := fs.Args()

	// Build host list from all sources
	hosts, err := collectHosts(positionalHosts, hostsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[stream] error reading hosts: %v\n", err)
		os.Exit(constants.ExitGeneralError)
	}

	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "[stream] no hosts specified")
		fmt.Fprintln(os.Stderr, "  Usage: g8e.operator stream [host...] [--hosts file] [flags]")
		fmt.Fprintln(os.Stderr, "  Run:   g8e.operator stream --help")
		os.Exit(constants.ExitGeneralError)
	}

	// Validate arch
	switch arch {
	case "amd64", "arm64", "386":
	default:
		fmt.Fprintf(os.Stderr, "[stream] unknown arch '%s' (valid: amd64, arm64, 386)\n", arch)
		os.Exit(constants.ExitConfigError)
	}

	// Load the binary into memory once
	// Try simple path first (g8ep build), then arch-specific path (local build)
	binPath := fmt.Sprintf("%s/g8e.operator", binaryDir)
	binaryData, err := os.ReadFile(binPath)
	if err != nil {
		binPath = fmt.Sprintf("%s/linux-%s/g8e.operator", binaryDir, arch)
		binaryData, err = os.ReadFile(binPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[stream] binary not found at %s\n", binPath)
		fmt.Fprintf(os.Stderr, "  Run: ./g8e operator build\n")
		os.Exit(constants.ExitGeneralError)
	}

	// Build operator invocation args for the remote shell
	operatorArgs := buildOperatorArgs(endpoint, deviceToken, apiKey, noGit)

	dialTimeout := time.Duration(timeoutSec) * time.Second

	settings := config.LoadSettings()

	// Set up context with signal cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\n[stream] signal received — cancelling all sessions...")
		cancel()
	}()

	// Print human-readable header to stderr
	fmt.Fprintf(os.Stderr, "[stream] linux/%s  %d hosts  concurrency=%d  timeout=%ds\n",
		arch, len(hosts), concurrency, timeoutSec)
	fmt.Fprintf(os.Stderr, "[stream] binary: %s (%s)\n", binPath, humanBytes(int64(len(binaryData))))
	if endpoint != "" {
		fmt.Fprintf(os.Stderr, "[stream] endpoint: %s\n", endpoint)
	}
	fmt.Fprintln(os.Stderr, "[stream] streaming...")

	// Run concurrent streaming
	wallStart := time.Now()
	results := runConcurrentStream(ctx, hosts, binaryData, operatorArgs, sshConfigArg, concurrency, dialTimeout, settings.SSHAuthSock, settings.User)

	// Tally results
	var succeeded, failed int
	for _, res := range results {
		if res.Error != "" {
			failed++
		} else {
			succeeded++
		}
	}

	totalMs := time.Since(wallStart).Milliseconds()
	summary := StreamStatusEvent{
		Summary: true,
		Status:  constants.StreamStatusSummary,
		Total:   len(hosts),
		Success: succeeded,
		Failed:  failed,
		TotalMs: totalMs,
		Ts:      time.Now().UTC(),
	}
	emitJSON(summary)

	fmt.Fprintf(os.Stderr, "[stream] done: %d/%d succeeded in %dms\n",
		succeeded, len(hosts), totalMs)

	if failed > 0 {
		os.Exit(constants.ExitGeneralError)
	}
}

// runConcurrentStream fans out streamToHost across all hosts using a bounded
// semaphore and collects all results.
func runConcurrentStream(
	ctx context.Context,
	hosts []string,
	binaryData []byte,
	operatorArgs string,
	sshConfigPath string,
	concurrency int,
	dialTimeout time.Duration,
	sshAuthSock string,
	username string,
) []streamResult {
	resultCh := make(chan streamResult, len(hosts))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, host := range hosts {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			streamToHost(ctx, h, binaryData, operatorArgs, sshConfigPath, dialTimeout, sshAuthSock, username, resultCh)
		}(host)
	}

	// Close resultCh once all goroutines finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect and emit results as they arrive (streaming output)
	var results []streamResult
	for res := range resultCh {
		results = append(results, res)
		// Emit per-host status immediately as JSON line
		evt := StreamStatusEvent{
			Host:      res.Host,
			Status:    res.Status,
			SizeBytes: res.SizeBytes,
			ElapsedMs: res.Elapsed.Milliseconds(),
			Ts:        time.Now().UTC(),
		}
		if res.Error != "" {
			evt.Error = res.Error
		}
		emitJSON(evt)
		if res.Error != "" {
			fmt.Fprintf(os.Stderr, "[stream] FAIL  %-30s %s\n", res.Host, res.Error)
		} else {
			fmt.Fprintf(os.Stderr, "[stream] OK    %-30s %dms\n", res.Host, res.Elapsed.Milliseconds())
		}
	}

	return results
}

// collectHosts merges positional CLI args, a --hosts file/stdin, deduplicates,
// and returns the final list.
func collectHosts(positional []string, hostsFile string) ([]string, error) {
	seen := make(map[string]struct{})
	var hosts []string

	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" || strings.HasPrefix(h, "#") {
			return
		}
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			hosts = append(hosts, h)
		}
	}

	for _, h := range positional {
		add(h)
	}

	if hostsFile != "" {
		var scanner *bufio.Scanner
		if hostsFile == "-" {
			scanner = bufio.NewScanner(os.Stdin)
		} else {
			f, err := os.Open(hostsFile)
			if err != nil {
				return nil, fmt.Errorf("open hosts file: %w", err)
			}
			defer f.Close()
			scanner = bufio.NewScanner(f)
		}
		for scanner.Scan() {
			add(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read hosts file: %w", err)
		}
	}

	return hosts, nil
}

// buildOperatorArgs constructs the shell-safe argument string for the remote
// operator invocation. Returns empty string when no endpoint is specified
// (inject-only mode).
//
// NOTE: Host-key policy is not passed here because strict mode is enforced on
// the outgoing dial from g8ep (stream phase). If remote operators ever make
// their own SSH connections, a --strict arg should be added here.
func buildOperatorArgs(endpoint, deviceToken, apiKey string, noGit bool) string {
	if endpoint == "" {
		return ""
	}
	parts := []string{"-e", shellQuote(endpoint)}
	if deviceToken != "" {
		parts = append(parts, "-D", shellQuote(deviceToken))
	}
	if apiKey != "" {
		parts = append(parts, "-k", shellQuote(apiKey))
	}
	if noGit {
		parts = append(parts, "--no-git")
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps a string in single quotes for safe inline shell embedding.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// emitJSON writes a StreamStatusEvent as a JSON line to stdout.
func emitJSON(evt StreamStatusEvent) {
	data, _ := json.Marshal(evt)
	fmt.Println(string(data))
}

// humanBytes formats a byte count as a human-readable string.
func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func printStreamUsage() {
	fmt.Print(`
g8e.operator stream -- concurrent ephemeral SSH operator injection

Streams the operator binary from the g8ep container directly to one or
more remote hosts over SSH. The binary is written to a tmpfile, optionally
started, and automatically deleted when the SSH session closes.

USAGE
  g8e.operator stream [host...] [flags]

HOSTS
  Hosts can be specified as positional arguments, via --hosts <file>, or both.
  Each host is an SSH alias from ~/.ssh/config or a user@host[:port] string.

  g8e.operator stream host1 host2 host3
  g8e.operator stream --hosts hosts.txt
  cat hosts.txt | g8e.operator stream --hosts -

FLAGS
  --arch amd64|arm64|386        Target architecture (default: amd64)
  --hosts <file|->              File of hosts (one per line), - for stdin
  --concurrency <N>             Max parallel SSH sessions (default: 50)
  --timeout <secs>              Per-host dial+inject timeout (default: 60)
  --endpoint <host>             Platform endpoint: starts operator if set
  --device-token <tok>          Device link token (single or mass deployment via max_uses)
  --key <apikey>                API key auth
  --no-git                      Disable ledger on remote operator
  --ssh-config <path>           SSH config path (default: ~/.ssh/config)
  --binary-dir <path>           Operator build dir (default: /home/g8e)

OUTPUT
  Per-host status events are written as JSON lines to stdout.
  Human-readable progress is written to stderr.

EXAMPLES
  # Inject to 3 hosts, start operator on each
  g8e.operator stream host1 host2 host3 \
    --endpoint 10.0.0.1 --device-token dlk_xxx

  # 1,000-node mass deployment from a file (device link with max_uses=1000)
  g8e.operator stream --hosts /etc/g8e/fleet.txt \
    --concurrency 100 --endpoint 10.0.0.1 --device-token dlk_xxx

  # Inject only (start manually on each remote)
  g8e.operator stream --hosts hosts.txt
`)
}

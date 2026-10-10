// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package stream

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/paths"
	"github.com/g8e-ai/g8e/v2/internal/pathutil"
)

const (
	defaultConcurrency = 50
	defaultTimeout     = 60 * time.Second
)

func getDefaultG8eBinaryDir() string {
	return pathutil.SafeJoin(paths.Infra.RuntimeDir, constants.PathParentDir, constants.BinDirname)
}

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
// It runs inside the g8ep container and streams the Node binary to
// one or more remote hosts concurrently via native Go crypto/ssh.
func RunStream(args []string) {
	if err := paths.Init(); err != nil {
		if fallbackErr := paths.InitWithBase(constants.PathCurrentDir); fallbackErr != nil {
			fmt.Fprintf(os.Stderr, "[stream] failed to initialize paths: %v\n", fallbackErr)
			os.Exit(constants.ExitGeneralError)
		}
	}

	fs := flag.NewFlagSet("stream", flag.ContinueOnError)

	var (
		arch             string
		hostsFile        string
		concurrency      int
		timeoutSec       int
		endpoint         string
		gatewayHTTPPort  int
		gatewayHTTPSPort int
		noGit            bool
		sshConfigArg     string
		sshKnownHosts    string
		binaryDir        string
		sshIdentityFile  string
		sshUser          string
		sshPassphrase    string
		preFlightCheck   bool
	)

	fs.StringVar(&arch, "arch", constants.ArchAMD64, "Target architecture: amd64, arm64, 386")
	fs.StringVar(&hostsFile, "hosts", "", "File of hosts (one per line) or - for stdin")
	fs.IntVar(&concurrency, "concurrency", defaultConcurrency, "Max parallel SSH sessions")
	fs.IntVar(&timeoutSec, "timeout", int(defaultTimeout.Seconds()), "Per-host dial+inject timeout in seconds")
	fs.StringVar(&endpoint, "endpoint", "", "Platform endpoint - if set, starts Operator on each remote host")
	fs.StringVar(&endpoint, "e", "", "Platform endpoint (shorthand)")
	fs.IntVar(&gatewayHTTPPort, "gateway-http-port", constants.Ports.OperatorHttp, "Gateway HTTP port to forward")
	fs.IntVar(&gatewayHTTPSPort, "gateway-https-port", constants.Ports.OperatorHttps, "Gateway HTTPS port to forward")
	fs.BoolVar(&noGit, "no-git", false, "Disable ledger")
	fs.StringVar(&sshConfigArg, "ssh-config", "", "Path to SSH config file (default: ~/.ssh/config)")
	fs.StringVar(&sshKnownHosts, "known-hosts", "", "Path to SSH known_hosts file (default: ~/.ssh/known_hosts)")
	fs.StringVar(&binaryDir, "binary-dir", getDefaultG8eBinaryDir(), "Directory containing arch-specific Operator builds")
	fs.StringVar(&sshIdentityFile, "ssh-identity-file", "", "SSH identity file path")
	fs.StringVar(&sshUser, "ssh-user", "", "SSH username")
	fs.StringVar(&sshPassphrase, "ssh-passphrase", "", "Passphrase for encrypted SSH private keys")
	fs.BoolVar(&preFlightCheck, "preflight", false, "Enable pre-flight SSH connectivity check before binary transfer")

	positionalHosts, err := parseInterleavedArgs(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printStreamUsage()
			os.Exit(constants.ExitSuccess)
		}
		fmt.Fprintf(os.Stderr, "[stream] flag error: %v\n", err)
		os.Exit(constants.ExitGeneralError)
	}

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
	case constants.ArchAMD64, constants.ArchARM64, constants.Arch386:
	default:
		fmt.Fprintf(os.Stderr, "[stream] unknown arch '%s' (valid: %s, %s, %s)\n",
			arch, constants.ArchAMD64, constants.ArchARM64, constants.Arch386)
		os.Exit(constants.ExitConfigError)
	}

	// Load the binary into memory once
	// Try simple path first (g8ep build), then arch-specific path (local build)
	binPath := pathutil.SafeJoin(binaryDir, constants.OperatorBinaryFilename)
	binaryData, err := os.ReadFile(binPath)
	if err != nil {
		binPath = pathutil.SafeJoin(binaryDir, fmt.Sprintf("%s-%s", constants.OSLinux, arch), constants.OperatorBinaryFilename)
		binaryData, err = os.ReadFile(binPath)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[stream] binary not found at %s\n", binPath)
		fmt.Fprintf(os.Stderr, "  Run: ./g8e operator build\n")
		os.Exit(constants.ExitGeneralError)
	}

	dialTimeout := time.Duration(timeoutSec) * time.Second

	// Set up context with signal cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "\n[stream] signal received - cancelling all sessions...")
		cancel()
	}()

	// Human-readable header
	fmt.Fprintf(os.Stderr, "[stream] %s/%s  %d hosts  concurrency=%d  timeout=%ds\n",
		constants.OSLinux, arch, len(hosts), concurrency, timeoutSec)
	fmt.Fprintf(os.Stderr, "[stream] binary: %s (%s)\n", binPath, humanBytes(int64(len(binaryData))))
	if endpoint != "" {
		fmt.Fprintf(os.Stderr, "[stream] endpoint: %s\n", endpoint)
	}
	fmt.Fprintln(os.Stderr, "[stream] streaming...")

	opts := StreamHostOptions{
		BinaryData:        binaryData,
		NoGit:             noGit,
		GatewayHTTPPort:   gatewayHTTPPort,
		GatewayHTTPSPort:  gatewayHTTPSPort,
		SSHConfigPath:     sshConfigArg,
		SSHKnownHostsPath: sshKnownHosts,
		DialTimeout:       dialTimeout,
		SSHAuthSock:       os.Getenv(string(constants.EnvVar.SSHAuthSock)),
		Username:          os.Getenv(string(constants.EnvVar.User)),
		SSHIdentityFile:   sshIdentityFile,
		SSHUser:           sshUser,
		SSHPassphrase:     sshPassphrase,
		PreFlightCheck:    preFlightCheck,
	}

	// Run concurrent streaming
	wallStart := time.Now()
	results := runConcurrentStream(ctx, hosts, opts, concurrency)

	// Tally results
	var succeeded, failed int
	for _, res := range results {
		if res.Error != nil {
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
	if err := emitJSON(&summary); err != nil {
		fmt.Fprintf(os.Stderr, "[stream] %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "[stream] done: %d/%d succeeded in %dms\n",
		succeeded, len(hosts), totalMs)

	if failed > 0 {
		cancel()
		os.Exit(constants.ExitGeneralError)
	}
}

// runConcurrentStream fans out streamToHost across all hosts using a bounded
// semaphore and collects all results.
func runConcurrentStream(
	ctx context.Context,
	hosts []string,
	opts StreamHostOptions,
	concurrency int,
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
			streamToHost(ctx, h, opts, resultCh)
		}(host)
	}

	// Close resultCh once all goroutines finish
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect and emit results as they arrive (streaming output)
	results := make([]streamResult, 0, len(hosts))
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
		if res.Error != nil {
			evt.Error = res.Error.Error()
		}
		if err := emitJSON(&evt); err != nil {
			fmt.Fprintf(os.Stderr, "[stream] %v\n", err)
		}
		if res.Error != nil {
			fmt.Fprintf(os.Stderr, "[stream] FAIL  %-30s %v\n", res.Host, res.Error)
		} else {
			fmt.Fprintf(os.Stderr, "[stream] OK    %-30s %dms\n", res.Host, res.Elapsed.Milliseconds())
		}
	}

	return results
}

// parseInterleavedArgs parses args against fs while permitting positional
// arguments and flags to appear in any order. Go's stdlib flag.Parse stops at
// the first non-flag token, which would cause a command like
// `stream host1 --key xxx` to mis-parse `--key`/`xxx` as additional hosts.
//
// We work around this by repeatedly calling fs.Parse on the unconsumed tail:
// each pass consumes leading flags up to the next positional arg, which is
// then peeled off into the returned slice before parsing resumes on the rest.
//
// flag.ErrHelp and other parse errors are returned unchanged so callers can
// distinguish help-requested from genuine errors.
func parseInterleavedArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	remaining := args
	for {
		if err := fs.Parse(remaining); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		remaining = fs.Args()[1:]
	}
}

// collectHosts merges positional CLI args, a --hosts file/stdin, deduplicates,
// and returns the final list.
func collectHosts(positional []string, hostsFile string) (hosts []string, err error) {
	seen := make(map[string]struct{})

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
			f, openErr := os.Open(hostsFile)
			if openErr != nil {
				return nil, fmt.Errorf("stream: collect hosts: %w", openErr)
			}
			defer func() {
				if closeErr := f.Close(); closeErr != nil && err == nil {
					err = fmt.Errorf("stream: collect hosts: close: %w", closeErr)
				}
			}()
			scanner = bufio.NewScanner(f)
		}
		for scanner.Scan() {
			add(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("stream: collect hosts: scan: %w", err)
		}
	}

	return hosts, nil
}

// buildOperatorCommand constructs the direct execution command for the remote
// Operator invocation, binding to loopback through the SSH forwarded tunnels.
func buildOperatorCommand(remotePath string, fwdHTTPPort, fwdHTTPSPort int, noGit bool) string {
	cmd := fmt.Sprintf("%s operator start --endpoint %s --gateway-http-port %d --gateway-https-port %d",
		remotePath, constants.LocalhostIP, fwdHTTPPort, fwdHTTPSPort)
	if noGit {
		cmd += " --no-git"
	}
	return cmd
}

// shellQuote wraps a string in single quotes for safe inline shell embedding.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// emitJSON writes a StreamStatusEvent as a JSON line to stdout.
func emitJSON(evt *StreamStatusEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("stream: emit: %w: %w", constants.ErrStreamMarshalEvent, err)
	}
	fmt.Println(string(data))
	return nil
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
g8e.operator stream -- concurrent ephemeral SSH Operator injection

Streams the Node binary from the g8ep container directly to one or
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
  --endpoint, -e <host>         Platform endpoint: starts Operator if set
  --gateway-http-port <port>    Gateway HTTP discovery port to forward (default: 8080)
  --gateway-https-port <port>   Gateway HTTPS/mTLS port to forward (default: 8443)
  --no-git                      Disable ledger on remote operator
  --ssh-config <path>           SSH config path (default: ~/.ssh/config)
  --known-hosts <path>          SSH known_hosts path (default: ~/.ssh/known_hosts)
  --binary-dir <path>           Operator build dir (default: <project-root>/bin)
  --ssh-identity-file <path>    SSH identity file path
  --ssh-user <user>             SSH username
  --ssh-passphrase <pass>       SSH private key passphrase
  --preflight                   Enable pre-flight SSH connectivity check

OUTPUT
  Per-host status events are written as JSON lines to stdout.
  Human-readable progress is written to stderr.

EXAMPLES
  # Inject to 3 hosts, start Operator on each
  g8e.operator stream host1 host2 host3 \
    --endpoint 10.0.0.1

  # 1,000-node mass deployment from a file
  g8e.operator stream --hosts /etc/g8e/fleet.txt \
    --concurrency 100 --endpoint 10.0.0.1

  # Inject only (start manually on each remote)
  g8e.operator stream --hosts hosts.txt
`)
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
)

const (
	// operatorDeployDirPrefix names the per-Operator directories created under
	// --remote-dir when --count is greater than one.
	operatorDeployDirPrefix = "op"

	// operatorDeployEnrollAttempts bounds restarts of one Operator whose
	// enrollment request was rejected or never appeared. The Gateway allows
	// only constants.PlatformEnrollmentMaxLiveOperatorRequests live
	// Operator requests platform-wide, so a busy Gateway answers 429 and the
	// worker exits; a later attempt succeeds once earlier requests complete.
	operatorDeployEnrollAttempts = 3

	operatorDeployPollInterval      = 100 * time.Millisecond
	operatorDeployEnrollTimeout     = 30 * time.Second
	operatorDeployOnlineBaseTimeout = time.Minute
	operatorDeployOnlinePerOperator = time.Second

	// operatorDeployStartLog is written by the started worker in its working
	// directory. Its "Approve with:" line and enrollment-completed record are
	// how deploy ties an enrollment request and an Operator session to the
	// directory that produced them.
	operatorDeployStartLog = "start.log"
)

// operatorDeployRemoteDirPattern and operatorDeployEndpointPattern restrict the values
// interpolated into the remote shell command; a leading ~ must stay unquoted so the
// remote shell expands it.
var (
	operatorDeployRemoteDirPattern = regexp.MustCompile(`^(~|~?/?[A-Za-z0-9_.-]+)(/[A-Za-z0-9_.-]+)*/?$`)
	// Worker configuration adds the standard HTTP and HTTPS ports itself, so
	// this value must be a host/IP without a scheme or explicit port.
	operatorDeployEndpointPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

	operatorDeployRequestIDPattern    = regexp.MustCompile(`auth enroll approve ([0-9a-f-]{36})`)
	operatorDeploySessionIDPattern    = regexp.MustCompile(`operator_session_id:\s+([0-9a-f-]{36})`)
	operatorDeployEnrollFailedPattern = regexp.MustCompile(`(?m)^Enrollment failed: .+$`)
)

// deploySSH runs commands on, and copies files to, one remote host.
type deploySSH struct {
	host         string
	port         int
	identityFile string
	stderr       io.Writer
	local        bool
	binaryDir    string
}

// deployTarget is the lifecycle surface shared by process/SSH and Docker
// deployments. Enrollment deliberately lives above this boundary so every
// transport retains the same request-specific approval and readiness checks.
type deployTarget interface {
	name() string
	startOperator(context.Context, string, string, ...string) error
	readStartLog(context.Context, string) ([]byte, error)
	awaitRequestID(context.Context, string) (string, error)
	awaitSessionID(context.Context, string) (string, error)
	awaitReady(context.Context, string) error
	markReady(context.Context, string) error
}

// deployedOperator is one Operator working directory that deploy set up.
type deployedOperator struct {
	target    deployTarget
	Dir       string
	RequestID string
	SessionID string
}

type operatorDeployOptions struct {
	endpoint   string
	background bool
	startArgs  []string
	// client is set only with --approve; it approves each enrollment request.
	client     authcmd.APIClient
	approvalMu *sync.Mutex
}

func operatorDeployCmd() *cobra.Command {
	return operatorDeployCmdWithConfig(shared.LoadConfig, authcmd.DefaultAPIClientFactory, shared.NewFileSvc)
}

func operatorDeployCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var hosts string
	var port int
	var identityFile string
	var background bool
	var remoteDir string
	var count int
	var approve bool
	var local bool
	var startIndex int
	var parallel int
	var operatorEndpoint string
	var dockerContext string
	var dockerImage string
	var dockerMounts []string
	start := operatorStartCmd()

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy the operator binary locally, over SSH, or to a Docker context",
		Long: `Deploy up to 5000 Operators locally (--local), to each SSH host (--hosts),
or as one container per Operator on --docker-context using --docker-image.
Use --dest-dir for the binary and isolated runtime state; --remote-dir remains an alias.
With --count N, directories are op-00001 through op-N. --start-index adds later batches
without replacing earlier Operators. An explicit --start-index always uses numbered directories.
The binary is installed once per host and hard-linked into each directory.
--parallel bounds concurrent deployments (default 4, maximum 4).

--roles selects any combination of data (default), provenance, inference, and observer.
Capability flags and role-specific settings use the same flags as operator start.
Role IDs default to unique, stable values per host/directory. Flag values support
{name} (directory basename), {dir} (absolute directory), and {host} substitutions.
--background starts workers; --endpoint or --operator-endpoint is required.
--operator-endpoint selects the worker-facing Gateway host and defaults to --endpoint.
--approve approves enrollment as
the authenticated owner and verifies all deployed sessions are active. Enrollment is
paced to respect Gateway limits. Repeating a deployment replaces only its own workers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configLoader("")
			if err != nil {
				return err
			}

			fileSvc, err := fileSvcFactory("", slog.Default())
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
			}

			creds, err := auth.LoadCredentials(fileSvc, cfg)
			if err != nil || creds == nil {
				return fmt.Errorf("%w: Please run './g8e auth enroll user' first", constants.ErrNotAuthenticated)
			}

			hosts, workerEndpoint, err := validateOperatorDeployFlags(
				cmd, local, hosts, dockerContext, dockerImage, remoteDir, parallel, count, startIndex, background, operatorEndpoint, approve,
			)
			if err != nil {
				return err
			}

			startArgs, err := operatorDeployStartArgs(cmd)
			if err != nil {
				return err
			}
			opts := operatorDeployOptions{endpoint: workerEndpoint, background: background, startArgs: startArgs, approvalMu: &sync.Mutex{}}
			if approve {
				opts.client, err = clientFactory(fileSvc, cfg)
				if err != nil {
					return fmt.Errorf("operator deploy: create API client: %w", err)
				}
			}

			var sourceBinary string
			if dockerContext == "" {
				sourceBinary, err = os.Executable()
				if err != nil {
					return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
				}
				if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
					return fmt.Errorf("%w: %s", constants.ErrPathNotFound, sourceBinary)
				}
			}

			hostList := strings.Split(hosts, ",")
			dirs := operatorDeployDirs(remoteDir, count)
			if cmd.Flags().Changed("start-index") || startIndex != 1 {
				dirs = make([]string, count)
				for i := range dirs {
					dirs[i] = fmt.Sprintf("%s/op-%05d", strings.TrimSuffix(remoteDir, "/"), startIndex+i)
				}
			}
			if dockerContext != "" {
				hostList = []string{dockerContext}
			}
			seen := make(map[string]bool)
			for i, host := range hostList {
				host = strings.TrimSpace(host)
				if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\r\n") || seen[host] {
					return fmt.Errorf("%w: invalid or duplicate host %q", constants.ErrMissingRequiredField, host)
				}
				seen[host] = true
				hostList[i] = host
			}
			cmd.Printf("Deploying %d operator(s) to %d target(s): %s\n", len(dirs), len(hostList), strings.Join(hostList, ","))

			ctx := cmd.Context()
			var deployed []deployedOperator
			var failed []string
			if dockerContext != "" {
				deployed, failed, err = executeDeployDocker(ctx, cmd, dockerContext, dockerImage, remoteDir, dockerMounts, dirs, opts, parallel, len(hostList)*len(dirs))
				if err != nil {
					return err
				}
			} else {
				deployed, failed, err = executeDeploySSH(ctx, cmd, hostList, port, identityFile, local, remoteDir, sourceBinary, dirs, opts, parallel)
				if err != nil {
					return err
				}
			}

			if approve && len(deployed) > 0 {
				cmd.Printf("Waiting for %d operator(s) to come online...\n", len(deployed))
				if err := awaitOperatorsOnline(ctx, opts.client, creds.UserID, deployed); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrOperatorDeployFailed, err)
				}
				for _, op := range deployed {
					cmd.Printf("  %s:%s  session %s\n", op.target.name(), op.Dir, op.SessionID)
				}
			}

			if len(failed) > 0 {
				return fmt.Errorf("%w: %d of %d operators failed: %s", constants.ErrOperatorDeployFailed, len(failed), len(hostList)*len(dirs), strings.Join(failed, ", "))
			}
			cmd.Println("\nDeployment complete")
			return nil
		},
	}

	cmd.Flags().StringVar(&hosts, "hosts", "", "Comma-separated SSH hosts (required unless --local)")
	cmd.Flags().StringVar(&dockerContext, "docker-context", "", "Docker context on which to create one container per Operator")
	cmd.Flags().StringVar(&dockerImage, "docker-image", "", "Existing Operator image on the selected Docker daemon")
	cmd.Flags().StringArrayVar(&dockerMounts, "docker-mount", nil, "Additional Docker mount spec (repeatable; source=,target=,readonly)")
	cmd.Flags().StringVar(&operatorEndpoint, "operator-endpoint", "", "Gateway address used by deployed Operators (defaults to --endpoint)")
	cmd.Flags().IntVar(&parallel, "parallel", 4, "Maximum concurrent deployments/enrollments (1..4)")
	cmd.Flags().IntVarP(&port, "port", "P", 0, "SSH port to connect to on remote hosts")
	cmd.Flags().StringVarP(&identityFile, "identity", "i", "", "SSH identity file (private key)")
	cmd.Flags().BoolVar(&background, "background", false, "Start operator in background after deployment (requires --endpoint)")
	cmd.Flags().StringVar(&remoteDir, "dest-dir", "~", "Destination directory for binary and isolated Operator runtimes")
	cmd.Flags().StringVar(&remoteDir, "remote-dir", "~", "Alias for --dest-dir")
	cmd.MarkFlagsMutuallyExclusive("dest-dir", "remote-dir")
	cmd.Flags().BoolVar(&local, "local", false, "Deploy on this system without SSH (requires --dest-dir)")
	cmd.Flags().IntVar(&startIndex, "start-index", 1, "First numbered operator directory (1..5000)")
	for _, name := range operatorDeployForwardFlags {
		cmd.Flags().AddFlag(start.Flags().Lookup(name))
	}
	cmd.Flags().IntVar(&count, "count", 1, "Operators to deploy per host, each in its own directory under --dest-dir when greater than 1")
	cmd.Flags().BoolVar(&approve, "approve", false, "Approve each Operator's enrollment request as the owner and wait until it is online (requires --background)")

	return cmd
}

// operatorDeployDirs returns the working directories for count Operators: the
// remote directory itself for one, numbered subdirectories for more.
func operatorDeployDirs(remoteDir string, count int) []string {
	if count == 1 {
		return []string{remoteDir}
	}
	dirs := make([]string, count)
	for i := range dirs {
		dirs[i] = fmt.Sprintf("%s/%s-%05d", strings.TrimSuffix(remoteDir, "/"), operatorDeployDirPrefix, i+1)
	}
	return dirs
}

// deployOperator installs the binary into dir on one host and, with
// --background, starts the worker there and (with --approve) enrolls it.
func deployOperator(ctx context.Context, cmd *cobra.Command, s deploySSH, sourceBinary, dir string, opts operatorDeployOptions) (deployedOperator, error) {
	absDir, err := s.prepareDir(ctx, dir)
	if err != nil {
		return deployedOperator{}, err
	}
	if err := s.installBinary(ctx, sourceBinary, absDir); err != nil {
		return deployedOperator{}, err
	}
	opts.startArgs = operatorDeployArgsForDir(opts.startArgs, s.host, absDir)
	op := deployedOperator{target: s, Dir: absDir}
	if !opts.background {
		cmd.Printf("Operator deployed to %s:%s (use --background to auto-start)\n", s.host, absDir)
		return op, nil
	}
	if opts.client == nil {
		if err := s.startOperator(ctx, absDir, opts.endpoint, opts.startArgs...); err != nil {
			return deployedOperator{}, err
		}
		cmd.Printf("Started operator in background on %s (working dir %s)\n", s.host, absDir)
		return op, nil
	}
	op.RequestID, err = enrollOperator(ctx, s, opts, absDir)
	if err != nil {
		return deployedOperator{}, err
	}
	op.SessionID, err = s.awaitSessionID(ctx, absDir)
	if err != nil {
		return deployedOperator{}, err
	}
	if err := s.awaitReady(ctx, absDir); err != nil {
		return deployedOperator{}, err
	}
	if err := s.markReady(ctx, absDir); err != nil {
		return deployedOperator{}, err
	}
	if op.RequestID == "" {
		cmd.Printf("Started operator on %s (working dir %s, already enrolled)\n", s.host, absDir)
		return op, nil
	}
	cmd.Printf("Started and approved operator on %s (working dir %s, enrollment request %s)\n", s.host, absDir, op.RequestID)
	return op, nil
}

// enrollOperator starts the worker in dir and approves the enrollment request
// it submits, restarting the worker up to operatorDeployEnrollAttempts times.
// The returned request ID is empty when the worker was already enrolled.
func enrollOperator(ctx context.Context, s deployTarget, opts operatorDeployOptions, dir string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= operatorDeployEnrollAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt-1) * time.Second):
			}
		}
		requestID, err := startAndApprove(ctx, s, opts, dir)
		if err == nil {
			return requestID, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("enrollment failed after %d attempts: %w", operatorDeployEnrollAttempts, lastErr)
}

func startAndApprove(ctx context.Context, s deployTarget, opts operatorDeployOptions, dir string) (string, error) {
	if err := s.startOperator(ctx, dir, opts.endpoint, opts.startArgs...); err != nil {
		return "", err
	}
	requestID, err := s.awaitRequestID(ctx, dir)
	if err != nil || requestID == "" {
		return "", err
	}
	if opts.approvalMu != nil {
		opts.approvalMu.Lock()
	}
	_, err = authcmd.PostPlatformEnrollmentDecision(opts.client, models.PlatformEnrollmentDecisionRequest{
		RequestID: requestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	if opts.approvalMu != nil {
		opts.approvalMu.Unlock()
	}
	if err != nil {
		return "", fmt.Errorf("approve %s: %w", requestID, err)
	}
	// Wait for completion before opening another enrollment slot.
	if _, err := s.awaitSessionID(ctx, dir); err != nil {
		return "", err
	}
	return requestID, nil
}

// awaitOperatorsOnline resolves each Operator's session ID from its start log,
// then waits until the Gateway lists every one of them as active.
func awaitOperatorsOnline(ctx context.Context, client authcmd.APIClient, userID string, ops []deployedOperator) error {
	ctx, cancel := context.WithTimeout(ctx, operatorDeployOnlineBaseTimeout+time.Duration(len(ops))*operatorDeployOnlinePerOperator)
	defer cancel()

	for i := range ops {
		if ops[i].SessionID != "" {
			continue
		}
		sessionID, err := ops[i].target.awaitSessionID(ctx, ops[i].Dir)
		if err != nil {
			return fmt.Errorf("%s:%s: %w", ops[i].target.name(), ops[i].Dir, err)
		}
		ops[i].SessionID = sessionID
	}

	return pollUntil(ctx, func() (bool, error) {
		operators, err := listUserOperators(client, userID)
		if err != nil {
			return false, err
		}
		active := make(map[string]struct{}, len(operators))
		for _, op := range operators {
			if op.Status == constants.OperatorStatusActive {
				active[op.OperatorSessionID] = struct{}{}
			}
		}
		for _, op := range ops {
			if _, ok := active[op.SessionID]; !ok {
				return false, nil
			}
		}
		return true, nil
	})
}

// pollUntil calls check every operatorDeployPollInterval until it reports done,
// returns an error, or ctx ends.
func pollUntil(ctx context.Context, check func() (bool, error)) error {
	for {
		done, err := check()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", constants.ErrOperatorDeployFailed, ctx.Err())
		case <-time.After(operatorDeployPollInterval):
		}
	}
}

func (s deploySSH) sshOptions(portFlag string) []string {
	var args []string
	if s.port != 0 {
		args = append(args, portFlag, fmt.Sprintf("%d", s.port))
	}
	if s.identityFile != "" {
		args = append(args, "-i", s.identityFile)
	}
	return args
}

func (s deploySSH) name() string { return s.host }

func (s deploySSH) markReady(context.Context, string) error { return nil }

func (s deploySSH) run(ctx context.Context, remoteCommand string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ssh", append(s.sshOptions("-p"), s.host, remoteCommand)...)
	if s.local {
		cmd = exec.CommandContext(ctx, "sh", "-c", remoteCommand)
	}
	cmd.Stderr = s.stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ssh %s: %w", s.host, err)
	}
	return out, nil
}

// prepareDir creates dir on the host and returns its absolute path, which is
// the value the worker's --working-dir carries.
func (s deploySSH) prepareDir(ctx context.Context, dir string) (string, error) {
	if s.local {
		if dir == "~" || strings.HasPrefix(dir, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			if dir == "~" {
				dir = home
			} else {
				dir = filepath.Join(home, strings.TrimPrefix(dir, "~/"))
			}
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		dir, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		return filepath.EvalSymlinks(dir)
	}
	out, err := s.run(ctx, fmt.Sprintf("mkdir -p %[1]s && cd %[1]s && pwd", dir))
	if err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	absDir := strings.TrimSpace(string(out))
	if !operatorDeployRemoteDirPattern.MatchString(absDir) {
		return "", fmt.Errorf("%w: resolved directory %q must match %s", constants.ErrPathValidation, absDir, operatorDeployRemoteDirPattern)
	}
	return absDir, nil
}

// installBinary uploads beside the target and renames into place: scp cannot
// open a running (or hard-linked, shared) g8e for writing (ETXTBSY), but a
// rename replaces the directory entry and leaves the old inode alone.
func (s deploySSH) installBinary(ctx context.Context, sourceBinary, dir string) error {
	if s.binaryDir != "" {
		if s.local {
			tmp, err := os.CreateTemp(dir, ".g8e-link-*")
			if err != nil {
				return err
			}
			staging := tmp.Name()
			if err := tmp.Close(); err != nil {
				return err
			}
			defer os.Remove(staging)
			if err := os.Remove(staging); err != nil {
				return err
			}
			if err := os.Link(filepath.Join(s.binaryDir, "g8e"), staging); err != nil {
				return err
			}
			return os.Rename(staging, filepath.Join(dir, "g8e"))
		}
		_, err := s.run(ctx, fmt.Sprintf("ln -f %s/g8e %s/g8e.new && mv -f %s/g8e.new %s/g8e", s.binaryDir, dir, dir, dir))
		return err
	}
	staging := dir + "/g8e.new"
	if s.local {
		// Copy once, then hard-link into each runtime directory. The source may be rebuilt.
		tmp, err := os.CreateTemp(dir, ".g8e-copy-*")
		if err != nil {
			return err
		}
		staging = tmp.Name()
		if err := tmp.Close(); err != nil {
			return err
		}
		defer os.Remove(staging)
		if err := CopyFile(sourceBinary, staging); err != nil {
			return err
		}
		return os.Rename(staging, filepath.Join(dir, "g8e"))
	}
	scp := exec.CommandContext(ctx, "scp", append(s.sshOptions("-P"), sourceBinary, fmt.Sprintf("%s:%s", s.host, staging))...)
	scp.Stderr = s.stderr
	if err := scp.Run(); err != nil {
		return fmt.Errorf("copy binary to %s: %w", s.host, err)
	}
	if _, err := s.run(ctx, fmt.Sprintf("chmod +x %[1]s && mv -f %[1]s %[2]s/g8e", staging, dir)); err != nil {
		return fmt.Errorf("install binary: %w", err)
	}
	return nil
}

// startOperator stops any worker previously started from dir, clears the start
// log so stale enrollment output cannot be read back, and starts a new worker.
// The stop pattern is anchored to the worker's own command line so it cannot
// match the shell running this command.
func (s deploySSH) startOperator(ctx context.Context, dir, endpoint string, startArgs ...string) error {
	quotedArgs := make([]string, len(startArgs))
	for i, arg := range startArgs {
		quotedArgs[i] = operatorDeployShellQuote(arg)
	}
	running := operatorDeployShellQuote(`^\./g8e operator start .*--working-dir ` + regexp.QuoteMeta(dir) + `$`)
	stop := fmt.Sprintf(`cd %[1]s && for i in $(seq 1 50); do pkill -f %[2]s || break; sleep 0.1; done && ! pgrep -f %[2]s >/dev/null`, operatorDeployShellQuote(dir), running)
	if _, err := s.run(ctx, stop); err != nil {
		return fmt.Errorf("stop previous operator: %w", err)
	}
	if s.local {
		if err := ctx.Err(); err != nil {
			return err
		}
		log, err := os.OpenFile(filepath.Join(dir, operatorDeployStartLog), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer log.Close()
		args := append([]string{"operator", "start", "--endpoint", endpoint}, startArgs...)
		args = append(args, "--working-dir", dir)
		worker := exec.Command(filepath.Join(dir, "g8e"), args...)
		// Preserve the exact argv prefix used by directory-scoped replacement.
		worker.Args[0] = "./g8e"
		worker.Dir = dir
		worker.Stdout, worker.Stderr = log, log
		detachDeployedOperator(worker)
		if err := worker.Start(); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "operator.pid"), []byte(fmt.Sprintf("%d\n", worker.Process.Pid)), 0o600); err != nil {
			_ = worker.Process.Kill()
			_ = worker.Wait()
			return err
		}
		go func() { _ = worker.Wait() }()
		return nil
	}
	script := fmt.Sprintf(
		`umask 077; cd %[1]s && rm -f %[3]s && { nohup ./g8e operator start --endpoint %[2]s %[4]s --working-dir %[1]s > %[3]s 2>&1 < /dev/null & echo $! > operator.pid; }`,
		operatorDeployShellQuote(dir), endpoint, operatorDeployStartLog, strings.Join(quotedArgs, " "))
	if _, err := s.run(ctx, script); err != nil {
		return fmt.Errorf("start operator: %w", err)
	}
	return nil
}

func (s deploySSH) readStartLog(ctx context.Context, dir string) ([]byte, error) {
	if s.local {
		data, err := os.ReadFile(filepath.Join(dir, operatorDeployStartLog))
		if os.IsNotExist(err) {
			return nil, nil
		}
		return data, err
	}
	return s.run(ctx, fmt.Sprintf("cat %s/%s 2>/dev/null || true", dir, operatorDeployStartLog))
}

// awaitRequestID returns the enrollment request ID the worker in dir printed,
// or the worker's enrollment failure. A worker that already holds issued
// credentials submits no request and goes straight to its session; that
// returns an empty request ID.
func (s deploySSH) awaitRequestID(ctx context.Context, dir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, operatorDeployEnrollTimeout)
	defer cancel()

	var requestID string
	err := pollUntil(ctx, func() (bool, error) {
		log, err := s.readStartLog(ctx, dir)
		if err != nil {
			return false, err
		}
		if m := operatorDeployRequestIDPattern.FindSubmatch(log); m != nil {
			requestID = string(m[1])
			return true, nil
		}
		if m := operatorDeployEnrollFailedPattern.Find(log); m != nil {
			return false, fmt.Errorf("%w: %s", constants.ErrOperatorDeployFailed, m)
		}
		return operatorDeploySessionIDPattern.Match(log), nil
	})
	if err != nil {
		return "", fmt.Errorf("await enrollment request in %s: %w", dir, err)
	}
	return requestID, nil
}

// awaitSessionID returns the operator session ID the worker in dir logged when
// its enrollment completed.
func (s deploySSH) awaitSessionID(ctx context.Context, dir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, operatorDeployEnrollTimeout)
	defer cancel()
	var sessionID string
	err := pollUntil(ctx, func() (bool, error) {
		log, err := s.readStartLog(ctx, dir)
		if err != nil {
			return false, err
		}
		if m := operatorDeployEnrollFailedPattern.Find(log); m != nil {
			return false, fmt.Errorf("%w: %s", constants.ErrOperatorDeployFailed, m)
		}
		if m := operatorDeploySessionIDPattern.FindSubmatch(log); m != nil {
			sessionID = string(m[1])
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("await operator session: %w", err)
	}
	return sessionID, nil
}

// Reuse start's flag definitions so deploy and start cannot drift in types/defaults.
var operatorDeployForwardFlags = []string{
	"roles", "inference-enabled", "inference-ollama-endpoint", "inference-keep-alive",
	"provider-boundary-observer-enabled", "provider-boundary-observer-id",
	"provenance-operator-enabled", "provenance-operator-id", "model-storage-root",
	"heartbeat-interval", "no-git", "execution-vault", "log", "trust-bundle",
}

func operatorDeployStartArgs(cmd *cobra.Command) ([]string, error) {
	var args []string
	for _, name := range operatorDeployForwardFlags {
		flag := cmd.Flags().Lookup(name)
		if flag.Changed {
			args = append(args, "--"+name+"="+flag.Value.String())
		}
	}
	heartbeat, _ := cmd.Flags().GetInt("heartbeat-interval")
	if err := validateHeartbeatInterval(heartbeat); err != nil {
		return nil, err
	}
	return args, nil
}

// Output is buffered per job and emitted by the caller, never concurrently to Cobra's writer.
type operatorDeployResult struct {
	op     deployedOperator
	dir    string
	output string
	err    error
}

func deployOperatorBatch(ctx context.Context, host deploySSH, source string, dirs []string, opts operatorDeployOptions, parallel int) <-chan operatorDeployResult {
	jobs := make(chan string)
	results := make(chan operatorDeployResult)
	var workers sync.WaitGroup
	for i := 0; i < min(parallel, len(dirs)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for dir := range jobs {
				var output bytes.Buffer
				cmd := &cobra.Command{}
				cmd.SetOut(&output)
				cmd.SetErr(&output)
				s := host
				s.stderr = &output
				op, err := deployOperator(ctx, cmd, s, source, dir, opts)
				// Do not retain the per-job output buffer through the stored transport.
				if target, ok := op.target.(deploySSH); ok {
					target.stderr = host.stderr
					op.target = target
				}
				results <- operatorDeployResult{op: op, dir: dir, output: output.String(), err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, dir := range dirs {
			select {
			case jobs <- dir:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	return results
}

func operatorDeployShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func operatorDeployArgsForDir(args []string, host, dir string) []string {
	replace := strings.NewReplacer("{host}", host, "{dir}", dir, "{name}", filepath.Base(dir))
	result := make([]string, 0, len(args)+2)
	flags := make(map[string]string)
	for _, arg := range args {
		arg = replace.Replace(arg)
		result = append(result, arg)
		key, value, _ := strings.Cut(arg, "=")
		flags[key] = value
	}
	digest := sha256.Sum256([]byte(host + "\x00" + dir))
	for enabled, id := range map[string]string{
		"--provenance-operator-enabled":        "--provenance-operator-id",
		"--provider-boundary-observer-enabled": "--provider-boundary-observer-id",
	} {
		var roles constants.OperatorRoles
		if value := flags["--roles"]; value != "" {
			_ = roles.Set(value)
		}
		roleEnabled := (enabled == "--provenance-operator-enabled" && roles.Has(constants.OperatorRoleProvenance)) || (enabled == "--provider-boundary-observer-enabled" && roles.Has(constants.OperatorRoleObserver))
		if (flags[enabled] == "true" || roleEnabled) && flags[id] == "" {
			result = append(result, fmt.Sprintf("%s=operator-%x", id, digest[:16]))
		}
	}
	return result
}

// A retained registry session may still be marked active from the old process.
// Require this launch to establish its command subscription before trusting it.
func (s deploySSH) awaitReady(ctx context.Context, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, operatorDeployEnrollTimeout)
	defer cancel()
	return pollUntil(ctx, func() (bool, error) {
		log, err := s.readStartLog(ctx, dir)
		if err != nil {
			return false, err
		}
		if bytes.Contains(log, []byte("Failed to start g8e")) || bytes.Contains(log, []byte("Enrollment failed:")) {
			return false, fmt.Errorf("operator startup failed; see %s/%s", dir, operatorDeployStartLog)
		}
		return bytes.Contains(log, []byte("Channel established - Ready to receive")), nil
	})
}

func validateOperatorDeployFlags(
	cmd *cobra.Command,
	local bool,
	hosts string,
	dockerContext string,
	dockerImage string,
	remoteDir string,
	parallel int,
	count int,
	startIndex int,
	background bool,
	operatorEndpoint string,
	approve bool,
) (effectiveHosts string, workerEndpoint string, err error) {
	selectedTransports := 0
	if local {
		selectedTransports++
	}
	if hosts != "" {
		selectedTransports++
	}
	if dockerContext != "" {
		selectedTransports++
	}
	if selectedTransports != 1 {
		return "", "", fmt.Errorf("%w: exactly one of --local, --hosts, or --docker-context is required", constants.ErrMissingRequiredField)
	}
	if local && !cmd.Flags().Changed("dest-dir") && !cmd.Flags().Changed("remote-dir") {
		return "", "", fmt.Errorf("%w: --local requires --dest-dir", constants.ErrMissingRequiredField)
	}
	effectiveHosts = hosts
	if local {
		effectiveHosts = "local"
	}
	if dockerContext != "" && strings.TrimSpace(dockerImage) == "" {
		return "", "", fmt.Errorf("%w: --docker-image is required with --docker-context", constants.ErrMissingRequiredField)
	}
	if remoteDir == "" || strings.ContainsAny(remoteDir, "\x00\r\n") || (dockerContext == "" && !local && (!operatorDeployRemoteDirPattern.MatchString(remoteDir) || strings.HasPrefix(remoteDir, "-"))) {
		return "", "", fmt.Errorf("%w: --dest-dir %q must match %s", constants.ErrPathValidation, remoteDir, operatorDeployRemoteDirPattern)
	}
	if dockerContext != "" && (!filepath.IsAbs(remoteDir) || filepath.Clean(remoteDir) == "/") {
		return "", "", fmt.Errorf("%w: Docker --dest-dir must be an absolute non-root container path", constants.ErrPathValidation)
	}
	if parallel < 1 || parallel > constants.PlatformEnrollmentMaxLiveOperatorRequests {
		return "", "", fmt.Errorf("%w: --parallel must be between 1 and %d", constants.ErrMissingRequiredField, constants.PlatformEnrollmentMaxLiveOperatorRequests)
	}
	if count < 1 || count > 5000 || startIndex < 1 || startIndex > 5000 || count > 5001-startIndex {
		return "", "", fmt.Errorf("%w: --count and --start-index must select operators within 1..5000", constants.ErrMissingRequiredField)
	}
	endpoint, _ := cmd.Flags().GetString("endpoint")
	endpoint = strings.TrimSpace(endpoint)
	workerEndpoint = strings.TrimSpace(operatorEndpoint)
	if workerEndpoint == "" {
		workerEndpoint = endpoint
	}
	if background && workerEndpoint == "" {
		return "", "", fmt.Errorf("%w: --operator-endpoint or --endpoint is required with --background", constants.ErrMissingRequiredField)
	}
	if background && !operatorDeployEndpointPattern.MatchString(workerEndpoint) {
		return "", "", fmt.Errorf("%w: worker endpoint %q must match %s", constants.ErrPathValidation, workerEndpoint, operatorDeployEndpointPattern)
	}
	if approve && !background {
		return "", "", fmt.Errorf("%w: --approve requires --background", constants.ErrMissingRequiredField)
	}
	logLevel, _ := cmd.Flags().GetString("log")
	if approve && logLevel != "info" && logLevel != "debug" {
		return "", "", fmt.Errorf("%w: --approve requires --log info or debug to observe worker readiness", constants.ErrMissingRequiredField)
	}
	return effectiveHosts, workerEndpoint, nil
}

func executeDeployDocker(
	ctx context.Context,
	cmd *cobra.Command,
	dockerContext, dockerImage, remoteDir string,
	dockerMounts []string,
	dirs []string,
	opts operatorDeployOptions,
	parallel int,
	totalTargets int,
) ([]deployedOperator, []string, error) {
	d := newDeployDocker(dockerContext, dockerImage, remoteDir, dockerMounts)
	preflightEndpoint := ""
	if opts.background {
		preflightEndpoint = opts.endpoint
	}
	if err := d.prepare(ctx, preflightEndpoint); err != nil {
		return nil, nil, err
	}
	var deployed []deployedOperator
	var failed []string
	results := deployDockerBatch(ctx, d, dirs, opts, parallel)
	for result := range results {
		cmd.Print(result.output)
		if result.err != nil {
			cmd.Printf("Failed to deploy %s:%s: %v\n", dockerContext, result.dir, result.err)
			failed = append(failed, dockerContext+":"+result.dir)
			continue
		}
		deployed = append(deployed, result.op)
	}
	if err := ctx.Err(); err != nil {
		notStarted := totalTargets - len(deployed) - len(failed)
		return deployed, failed, fmt.Errorf("%w: Docker deployment canceled (completed=%d failed=%d not-started=%d)", err, len(deployed), len(failed), notStarted)
	}
	return deployed, failed, nil
}

func executeDeploySSH(
	ctx context.Context,
	cmd *cobra.Command,
	hostList []string,
	port int,
	identityFile string,
	local bool,
	remoteDir, sourceBinary string,
	dirs []string,
	opts operatorDeployOptions,
	parallel int,
) ([]deployedOperator, []string, error) {
	var deployed []deployedOperator
	var failed []string
	for _, host := range hostList {
		s := deploySSH{host: strings.TrimSpace(host), port: port, identityFile: identityFile, stderr: cmd.ErrOrStderr(), local: local}
		cache, err := s.prepareDir(ctx, strings.TrimSuffix(remoteDir, "/")+"/.deploy-bin")
		if err != nil {
			return deployed, failed, err
		}
		if err := s.installBinary(ctx, sourceBinary, cache); err != nil {
			return deployed, failed, err
		}
		s.binaryDir = cache
		results := deployOperatorBatch(ctx, s, sourceBinary, dirs, opts, parallel)
		for result := range results {
			cmd.Print(result.output)
			if result.err != nil {
				cmd.Printf("Failed to deploy %s:%s: %v\n", s.host, result.dir, result.err)
				failed = append(failed, s.host+":"+result.dir)
				continue
			}
			deployed = append(deployed, result.op)
		}
		if err := ctx.Err(); err != nil {
			return deployed, failed, err
		}
	}
	return deployed, failed, nil
}

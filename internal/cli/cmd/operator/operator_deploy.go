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
	"encoding/json"
	"errors"
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
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

const (
	// operatorDeployDirPrefix names the per-Operator directories created under
	// --remote-dir when --count is greater than one.
	operatorDeployDirPrefix   = "op"
	operatorDeployMaxParallel = 100

	operatorDeployDiagnoseTimeout   = 10 * time.Second
	operatorDeployOnlineBaseTimeout = time.Minute
	operatorDeployOnlinePerOperator = time.Second

	// operatorDeployStartLog is written by the started worker in its working
	// directory for human diagnostics. Discovery uses structured runtime state.
	operatorDeployStartLog = "start.log"
	operatorDeployPIDFile  = "operator.pid"
)

// operatorDeployEnrollTimeout bounds how long deploy waits for the Gateway to
// announce one worker's pending request. Exposed as a var so tests can shorten it.
var operatorDeployEnrollTimeout = 30 * time.Second

// operatorDeployRemoteDirPattern and operatorDeployEndpointPattern restrict the values
// interpolated into the remote shell command; a leading ~ must stay unquoted so the
// remote shell expands it.
var (
	operatorDeployRemoteDirPattern = regexp.MustCompile(`^(~|~?/?[A-Za-z0-9_.-]+)(/[A-Za-z0-9_.-]+)*/?$`)
	// Worker ports travel separately as --gateway-http-port and
	// --gateway-https-port, so this value must be a host/IP without a scheme
	// or explicit port.
	operatorDeployEndpointPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// deploySSH runs commands on, and copies files to, one remote host.
type deploySSH struct {
	host         string
	port         int
	identityFile string
	stderr       io.Writer
	local        bool
	binaryDir    string
	launchID     string
}

// deployTarget is the lifecycle surface shared by process/SSH and Docker
// deployments. Enrollment deliberately lives above this boundary so every
// transport retains the same request-specific approval and readiness checks.
type deployTarget interface {
	name() string
	startOperator(context.Context, string, string, ...string) error
	// readDeploymentState reads the worker's recorded progress once. Waiting is
	// event driven; this only explains a wait that failed.
	readDeploymentState(context.Context, string) (*models.OperatorDeploymentState, error)
	markReady(context.Context, string) error
}

// deployedOperator is one Operator working directory that deploy set up.
type deployedOperator struct {
	target    deployTarget
	Dir       string
	RequestID string
	SessionID string
	watch     *deploymentWatch
}

type operatorDeployOptions struct {
	endpoint   string
	background bool
	startArgs  []string
	// preflight is the operator gateway-preflight argument list each target runs
	// before any worker starts; it is set whenever workers are started.
	preflight []string
	// client is set only with --approve; the staged cohort is approved once.
	client authcmd.APIClient
	// events delivers the Gateway's staging and readiness announcements for the
	// workers this deploy starts. It is set whenever workers are started.
	events deploymentWatcher
}

// deploymentWatcher registers a launch to be announced by the Gateway.
type deploymentWatcher interface {
	watch(launchID string) *deploymentWatch
}

// deploymentEventsConnector opens the deploying CLI's event stream.
type deploymentEventsConnector func(context.Context, fs.RuntimeFileService, *config.Config) (deploymentWatcher, func(), error)

func connectGatewayDeploymentEvents(ctx context.Context, fileSvc fs.RuntimeFileService, cfg *config.Config) (deploymentWatcher, func(), error) {
	creds, err := auth.LoadCredentials(fileSvc, cfg)
	if err != nil || creds == nil || creds.CLISessionID == "" {
		return nil, nil, fmt.Errorf("%w: Please run './g8e auth enroll user' first", constants.ErrNotAuthenticated)
	}
	httpClient, err := auth.BuildMTLSClient(fileSvc, cfg, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("operator deploy: build mTLS client: %w", err)
	}
	return connectDeploymentEvents(ctx, httpClient, cfg.OperatorPublicURL(), creds.CLISessionID)
}

// explainDeployWait adds what the worker itself recorded to a failed wait, so a
// worker that exited or reported a failure is named instead of only timing out.
func explainDeployWait(target deployTarget, dir string, waitErr error) error {
	ctx, cancel := context.WithTimeout(context.Background(), operatorDeployDiagnoseTimeout)
	defer cancel()
	state, err := target.readDeploymentState(ctx, dir)
	switch {
	case err != nil:
		return fmt.Errorf("%w; worker state: %w", waitErr, err)
	case state != nil && state.Phase == models.OperatorDeploymentPhaseFailed:
		return fmt.Errorf("%w; worker reported: %s", waitErr, state.Error)
	}
	return waitErr
}

func operatorDeployCmd() *cobra.Command {
	return operatorDeployCmdWithConfig(shared.LoadConfig, authcmd.DefaultAPIClientFactory, shared.NewFileSvc, connectGatewayDeploymentEvents)
}

func operatorDeployCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
	connectEvents deploymentEventsConnector,
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
--parallel bounds concurrent deployments (default 100, maximum 100).

--roles selects any combination of data (default), provenance, inference, and observer.
Capability flags and role-specific settings use the same flags as operator start.
Role IDs default to unique, stable values per host/directory. Flag values support
{name} (directory basename), {dir} (absolute directory), and {host} substitutions.
--background starts workers; --endpoint or --operator-endpoint is required.
--operator-endpoint selects the worker-facing Gateway host and defaults to --endpoint.
Workers dialing a non-loopback host need a Gateway started with --listen-host 0.0.0.0.
--approve approves enrollment as
the authenticated owner in one batch after every worker is staged, then verifies
every worker has established its command subscription. Without --approve, workers remain pending. Repeating a deployment replaces only its own workers.`,
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
			opts := operatorDeployOptions{endpoint: workerEndpoint, background: background, startArgs: startArgs}
			if background {
				opts.preflight = operatorDeployPreflightArgs(cmd, workerEndpoint)
			}
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
			if background {
				events, stopEvents, err := connectEvents(ctx, fileSvc, cfg)
				if err != nil {
					return err
				}
				defer stopEvents()
				opts.events = events
			}
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

			if len(failed) > 0 {
				return fmt.Errorf("%w: staging failed for %d of %d operators; no approvals submitted: %s", constants.ErrOperatorDeployFailed, len(failed), len(hostList)*len(dirs), strings.Join(failed, ", "))
			}
			if approve && len(deployed) > 0 {
				if err := approveDeployedOperators(opts.client, deployed); err != nil {
					return err
				}
				cmd.Printf("Waiting for %d operator(s) to come online...\n", len(deployed))
				if err := awaitOperatorsOnline(ctx, deployed); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrOperatorDeployFailed, err)
				}
				for _, op := range deployed {
					cmd.Printf("  %s:%s  session %s\n", op.target.name(), op.Dir, op.SessionID)
				}
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
	cmd.Flags().IntVar(&parallel, "parallel", operatorDeployMaxParallel, fmt.Sprintf("Maximum concurrent staging workers (1..%d)", operatorDeployMaxParallel))
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
	cmd.Flags().BoolVar(&approve, "approve", false, "Approve the fully staged cohort in one batch and wait until it is online (requires --background)")

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
	s.launchID, err = uuid.NewString()
	if err != nil {
		return deployedOperator{}, fmt.Errorf("deployment launch identity: %w", err)
	}
	opts.startArgs = append(opts.startArgs, "--deployment-id="+s.launchID)
	op := deployedOperator{target: s, Dir: absDir}
	if !opts.background {
		cmd.Printf("Operator deployed to %s:%s (use --background to auto-start)\n", s.host, absDir)
		return op, nil
	}
	// Watch before starting: the stream delivers only live events.
	op.watch = opts.events.watch(s.launchID)
	if err := s.startOperator(ctx, absDir, opts.endpoint, opts.startArgs...); err != nil {
		return deployedOperator{}, err
	}
	op.RequestID, err = awaitStaged(ctx, op.watch, s, absDir)
	if err != nil {
		return deployedOperator{}, err
	}
	if op.RequestID == "" {
		cmd.Printf("Staged operator on %s (working dir %s, already enrolled)\n", s.host, absDir)
	} else {
		cmd.Printf("Staged operator on %s (working dir %s, enrollment request %s)\n", s.host, absDir, op.RequestID)
	}
	return op, nil
}

// approveDeployedOperators resolves only this deployment's IDs against one
// pending snapshot and posts one fingerprint-bound decision after staging.
func approveDeployedOperators(client authcmd.APIClient, ops []deployedOperator) error {
	ids := make(map[string]struct{}, len(ops))
	for _, op := range ops {
		if op.RequestID != "" {
			ids[op.RequestID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	body, err := client.Get(constants.APIPaths.AuthPlatformEnrollmentPending)
	if err != nil {
		return fmt.Errorf("operator deploy: fetch staged requests: %w", err)
	}
	var pending models.PlatformEnrollmentPendingResponse
	if err := json.Unmarshal(body, &pending); err != nil {
		return fmt.Errorf("operator deploy: decode staged requests: %w", err)
	}
	req := models.PlatformEnrollmentBatchDecisionRequest{Decision: models.PlatformEnrollmentDecisionApprove}
	for _, target := range pending.Requests {
		if _, ok := ids[target.RequestID]; ok {
			req.Requests = append(req.Requests, models.PlatformEnrollmentDecisionTarget{RequestID: target.RequestID, Fingerprints: target.Fingerprints})
			delete(ids, target.RequestID)
		}
	}
	if len(ids) != 0 {
		return fmt.Errorf("operator deploy: %d staged requests are no longer pending: %w", len(ids), constants.ErrPlatformEnrollmentRequestNotFound)
	}
	if _, err := authcmd.PostPlatformEnrollmentBatchDecision(client, req); err != nil {
		return fmt.Errorf("operator deploy: approve staged cohort: %w", err)
	}
	return nil
}

// awaitOperatorsOnline waits for this launch's acknowledged command subscription
// and reads its session ID from the same deployment state.
func awaitOperatorsOnline(ctx context.Context, ops []deployedOperator) error {
	ctx, cancel := context.WithTimeout(ctx, operatorDeployOnlineBaseTimeout+time.Duration(len(ops))*operatorDeployOnlinePerOperator)
	defer cancel()

	for i := range ops {
		sessionID, err := ops[i].watch.awaitReady(ctx)
		if err != nil {
			return fmt.Errorf("%s:%s: %w", ops[i].target.name(), ops[i].Dir, explainDeployWait(ops[i].target, ops[i].Dir, err))
		}
		ops[i].SessionID = sessionID
		if err := ops[i].target.markReady(ctx, ops[i].Dir); err != nil {
			return err
		}
	}

	return nil
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

// stopPreviousOperator checks only directories with a recorded deployment.
// Fresh fleet members must not each scan the entire host's process table.
// The PID/start log is evidence of a previous launch, not authority to signal a PID:
// the anchored directory pattern still prevents stale/reused PIDs from targeting
// unrelated processes.
func (s deploySSH) stopPreviousOperator(ctx context.Context, dir string) error {
	if s.local {
		previous := false
		for _, name := range []string{operatorDeployPIDFile, operatorDeployStartLog} {
			_, err := os.Lstat(filepath.Join(dir, name))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("read previous deployment: %w", err)
			}
			previous = previous || err == nil
		}
		if !previous {
			return nil
		}
	}
	running := operatorDeployShellQuote(`^\./g8e operator start .*--working-dir ` + regexp.QuoteMeta(dir) + `$`)
	stop := fmt.Sprintf(`cd %[1]s && { { [ ! -e %[3]s ] && [ ! -L %[3]s ] && [ ! -e %[4]s ] && [ ! -L %[4]s ]; } || { for i in $(seq 1 50); do pkill -f %[2]s || break; sleep 0.1; done && ! pgrep -f %[2]s >/dev/null; }; }`, operatorDeployShellQuote(dir), running, operatorDeployPIDFile, operatorDeployStartLog)
	if _, err := s.run(ctx, stop); err != nil {
		return fmt.Errorf("stop previous operator: %w", err)
	}
	return nil
}

// preflightGateway runs the installed binary's gateway-preflight on this host,
// so a Gateway the workers cannot reach fails the deploy before any starts.
func (s deploySSH) preflightGateway(ctx context.Context, args []string) error {
	if s.local {
		check := exec.CommandContext(ctx, filepath.Join(s.binaryDir, "g8e"), append([]string{"operator", "gateway-preflight"}, args...)...)
		check.Stderr = s.stderr
		return check.Run()
	}
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = operatorDeployShellQuote(arg)
	}
	_, err := s.run(ctx, fmt.Sprintf("%s/g8e operator gateway-preflight %s", operatorDeployShellQuote(s.binaryDir), strings.Join(quoted, " ")))
	return err
}

// startOperator stops any worker previously deployed from dir, clears its start
// log, and starts a new worker.
func (s deploySSH) startOperator(ctx context.Context, dir, endpoint string, startArgs ...string) error {
	if err := s.stopPreviousOperator(ctx, dir); err != nil {
		return err
	}
	quotedArgs := make([]string, len(startArgs))
	for i, arg := range startArgs {
		quotedArgs[i] = operatorDeployShellQuote(arg)
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
		if err := os.WriteFile(filepath.Join(dir, operatorDeployPIDFile), []byte(fmt.Sprintf("%d\n", worker.Process.Pid)), 0o600); err != nil {
			_ = worker.Process.Kill()
			_ = worker.Wait()
			return err
		}
		go func() { _ = worker.Wait() }()
		return nil
	}
	script := fmt.Sprintf(
		`umask 077; cd %[1]s && rm -f %[3]s && { nohup ./g8e operator start --endpoint %[2]s %[4]s --working-dir %[1]s > %[3]s 2>&1 < /dev/null & echo $! > %[5]s; }`,
		operatorDeployShellQuote(dir), endpoint, operatorDeployStartLog, strings.Join(quotedArgs, " "), operatorDeployPIDFile)
	if _, err := s.run(ctx, script); err != nil {
		return fmt.Errorf("start operator: %w", err)
	}
	return nil
}

func (s deploySSH) readDeploymentState(ctx context.Context, dir string) (*models.OperatorDeploymentState, error) {
	var state *models.OperatorDeploymentState
	if s.local {
		fileSvc, err := fs.NewRuntimeFileService(dir, slog.Default())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
		}
		state, err = readOperatorDeploymentState(ctx, fileSvc)
		if err != nil {
			return nil, err
		}
	} else {
		// The remote CLI reads its runtime through RuntimeFileService.
		data, err := s.run(ctx, fmt.Sprintf("cd %s && ./g8e operator deployment-state --working-dir %s", operatorDeployShellQuote(dir), operatorDeployShellQuote(dir)))
		if err != nil {
			return nil, err
		}
		state, err = decodeOperatorDeploymentState(data)
		if err != nil {
			return nil, err
		}
	}
	if state != nil && state.LaunchID != s.launchID {
		return nil, nil
	}
	return state, nil
}

// Reuse start's flag definitions so deploy and start cannot drift in types/defaults.
var operatorDeployForwardFlags = []string{
	"roles", "inference-enabled", "inference-ollama-endpoint", "inference-keep-alive",
	"provider-boundary-observer-enabled", "provider-boundary-observer-id",
	"provenance-operator-enabled", "provenance-operator-id", "model-storage-root",
	"gateway-http-port", "gateway-https-port",
	"heartbeat-interval", "no-git", "execution-vault", "log", "trust-bundle", "master-key-file",
}

// operatorDeployPreflightArgs returns the gateway-preflight arguments that probe
// the same host and ports the workers will dial.
func operatorDeployPreflightArgs(cmd *cobra.Command, endpoint string) []string {
	args := []string{endpoint}
	for _, name := range []string{"gateway-http-port", "gateway-https-port"} {
		if flag := cmd.Flags().Lookup(name); flag.Changed {
			args = append(args, "--"+name+"="+flag.Value.String())
		}
	}
	return args
}

// gatewayPreflightError names the likely cause of a failed preflight: the
// Gateway listens on loopback unless started with --listen-host.
func gatewayPreflightError(target, endpoint string, err error) error {
	return fmt.Errorf("%w: %s cannot reach the Gateway at %s (it listens on loopback unless started with --listen-host 0.0.0.0): %w",
		constants.ErrOperatorDeployFailed, target, endpoint, err)
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
	for _, name := range []string{"gateway-http-port", "gateway-https-port"} {
		value, _ := cmd.Flags().GetInt(name)
		if err := validateGatewayPort(name, value); err != nil {
			return nil, err
		}
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
	if parallel < 1 || parallel > operatorDeployMaxParallel {
		return "", "", fmt.Errorf("%w: --parallel must be between 1 and %d", constants.ErrMissingRequiredField, operatorDeployMaxParallel)
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
	if err := d.prepare(ctx, opts.preflight); err != nil {
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
		if opts.preflight != nil {
			if err := s.preflightGateway(ctx, opts.preflight); err != nil {
				return deployed, failed, gatewayPreflightError(s.host, opts.endpoint, err)
			}
		}
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

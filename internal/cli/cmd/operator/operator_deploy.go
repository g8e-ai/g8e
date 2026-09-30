// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
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
	// only constants.PlatformEnrollmentMaxLiveRequestsPerComponent live
	// Operator requests platform-wide, so a busy Gateway answers 429 and the
	// worker exits; a later attempt succeeds once earlier requests complete.
	operatorDeployEnrollAttempts = 3

	operatorDeployPollInterval      = 500 * time.Millisecond
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
	operatorDeployEndpointPattern  = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

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
}

// deployedOperator is one Operator working directory that deploy set up.
type deployedOperator struct {
	ssh       deploySSH
	Dir       string
	RequestID string
	SessionID string
}

type operatorDeployOptions struct {
	endpoint   string
	background bool
	// client is set only with --approve; it approves each enrollment request.
	client authcmd.APIClient
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

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy the operator binary to remote hosts and start it",
		Long: `Deploy the g8e operator binary to remote hosts via SSH and start it in the background. Uses your existing SSH config for authentication. Requires './g8e auth enroll user' first.

The binary is installed at <remote-dir>/g8e (uploaded as g8e.new, then renamed into place, so redeploying over a running Operator is safe). With --background the worker is started as
'g8e operator start --endpoint <endpoint> --working-dir <remote-dir>' from inside <remote-dir>
(the .g8e/ runtime tree is rooted at the process's current directory), so its runtime state
lives under --remote-dir. Distinct --remote-dir values give distinct Operator identities
on the same host. --endpoint is required with --background. Starting replaces any Operator
previously started from the same directory.

--count N deploys N Operators per host into <remote-dir>/op-00001 .. op-N.

--approve approves each Operator's enrollment request as the owner, one Operator at a time
(the Gateway caps live Operator enrollment requests), then waits until every approved
Operator is active and prints its operator session ID for 'operator bind' and 'operator run'.
It requires --background.`,
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

			if hosts == "" {
				return fmt.Errorf("%w: --hosts flag is required (comma-separated list of hosts)", constants.ErrMissingRequiredField)
			}
			if !operatorDeployRemoteDirPattern.MatchString(remoteDir) {
				return fmt.Errorf("%w: --remote-dir %q must match %s", constants.ErrPathValidation, remoteDir, operatorDeployRemoteDirPattern)
			}
			if count < 1 {
				return fmt.Errorf("%w: --count must be at least 1", constants.ErrMissingRequiredField)
			}
			endpoint, _ := cmd.Flags().GetString("endpoint")
			endpoint = strings.TrimSpace(endpoint)
			if background && endpoint == "" {
				return fmt.Errorf("%w: --endpoint is required with --background", constants.ErrMissingRequiredField)
			}
			if background && !operatorDeployEndpointPattern.MatchString(endpoint) {
				return fmt.Errorf("%w: --endpoint %q must match %s", constants.ErrPathValidation, endpoint, operatorDeployEndpointPattern)
			}
			if approve && !background {
				return fmt.Errorf("%w: --approve requires --background", constants.ErrMissingRequiredField)
			}

			opts := operatorDeployOptions{endpoint: endpoint, background: background}
			if approve {
				opts.client, err = clientFactory(fileSvc, cfg)
				if err != nil {
					return fmt.Errorf("operator deploy: create API client: %w", err)
				}
			}

			sourceBinary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("%w: %w", constants.ErrStatFailed, err)
			}
			if _, err := os.Stat(sourceBinary); os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", constants.ErrPathNotFound, sourceBinary)
			}

			hostList := strings.Split(hosts, ",")
			dirs := operatorDeployDirs(remoteDir, count)
			cmd.Printf("Deploying %d operator(s) to %d host(s): %s\n", len(dirs), len(hostList), hosts)

			ctx := cmd.Context()
			var deployed []deployedOperator
			var failed []string
			for _, host := range hostList {
				s := deploySSH{host: strings.TrimSpace(host), port: port, identityFile: identityFile, stderr: cmd.ErrOrStderr()}
				for _, dir := range dirs {
					op, err := deployOperator(ctx, cmd, s, sourceBinary, dir, opts)
					if err != nil {
						cmd.Printf("Failed to deploy %s:%s: %v\n", s.host, dir, err)
						failed = append(failed, s.host+":"+dir)
						continue
					}
					deployed = append(deployed, op)
				}
			}

			if approve && len(deployed) > 0 {
				cmd.Printf("Waiting for %d operator(s) to come online...\n", len(deployed))
				if err := awaitOperatorsOnline(ctx, opts.client, creds.UserID, deployed); err != nil {
					return fmt.Errorf("%w: %w", constants.ErrOperatorDeployFailed, err)
				}
				for _, op := range deployed {
					cmd.Printf("  %s:%s  session %s\n", op.ssh.host, op.Dir, op.SessionID)
				}
			}

			if len(failed) > 0 {
				return fmt.Errorf("%w: %d of %d operators failed: %s", constants.ErrOperatorDeployFailed, len(failed), len(hostList)*len(dirs), strings.Join(failed, ", "))
			}
			cmd.Println("\nDeployment complete")
			return nil
		},
	}

	cmd.Flags().StringVar(&hosts, "hosts", "", "Comma-separated list of hosts to deploy to (required)")
	cmd.Flags().IntVarP(&port, "port", "P", 0, "SSH port to connect to on remote hosts")
	cmd.Flags().StringVarP(&identityFile, "identity", "i", "", "SSH identity file (private key)")
	cmd.Flags().BoolVar(&background, "background", false, "Start operator in background after deployment (requires --endpoint)")
	cmd.Flags().StringVar(&remoteDir, "remote-dir", "~", "Remote directory for the binary and the Operator working directory")
	cmd.Flags().IntVar(&count, "count", 1, "Operators to deploy per host, each in its own directory under --remote-dir when greater than 1")
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
	op := deployedOperator{ssh: s, Dir: absDir}
	if !opts.background {
		cmd.Printf("Operator deployed to %s:%s (use --background to auto-start)\n", s.host, absDir)
		return op, nil
	}
	if opts.client == nil {
		if err := s.startOperator(ctx, absDir, opts.endpoint); err != nil {
			return deployedOperator{}, err
		}
		cmd.Printf("Started operator in background on %s (working dir %s)\n", s.host, absDir)
		return op, nil
	}
	op.RequestID, err = enrollOperator(ctx, s, opts, absDir)
	if err != nil {
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
func enrollOperator(ctx context.Context, s deploySSH, opts operatorDeployOptions, dir string) (string, error) {
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

func startAndApprove(ctx context.Context, s deploySSH, opts operatorDeployOptions, dir string) (string, error) {
	if err := s.startOperator(ctx, dir, opts.endpoint); err != nil {
		return "", err
	}
	requestID, err := s.awaitRequestID(ctx, dir)
	if err != nil || requestID == "" {
		return "", err
	}
	_, err = authcmd.PostPlatformEnrollmentDecision(opts.client, models.PlatformEnrollmentDecisionRequest{
		RequestID: requestID,
		Decision:  models.PlatformEnrollmentDecisionApprove,
	})
	if err != nil {
		return "", fmt.Errorf("approve %s: %w", requestID, err)
	}
	return requestID, nil
}

// awaitOperatorsOnline resolves each Operator's session ID from its start log,
// then waits until the Gateway lists every one of them as active.
func awaitOperatorsOnline(ctx context.Context, client authcmd.APIClient, userID string, ops []deployedOperator) error {
	ctx, cancel := context.WithTimeout(ctx, operatorDeployOnlineBaseTimeout+time.Duration(len(ops))*operatorDeployOnlinePerOperator)
	defer cancel()

	for i := range ops {
		sessionID, err := ops[i].ssh.awaitSessionID(ctx, ops[i].Dir)
		if err != nil {
			return fmt.Errorf("%s:%s: %w", ops[i].ssh.host, ops[i].Dir, err)
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

func (s deploySSH) run(ctx context.Context, remoteCommand string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ssh", append(s.sshOptions("-p"), s.host, remoteCommand)...)
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
	staging := dir + "/g8e.new"
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
func (s deploySSH) startOperator(ctx context.Context, dir, endpoint string) error {
	running := fmt.Sprintf(`'^\./g8e operator start .*--working-dir %s$'`, dir)
	script := fmt.Sprintf(
		`cd %[1]s && for i in $(seq 1 50); do pkill -f %[2]s || break; sleep 0.1; done && ! pgrep -f %[2]s >/dev/null && rm -f %[3]s && { nohup ./g8e operator start --endpoint %[4]s --working-dir %[1]s > %[3]s 2>&1 < /dev/null & }`,
		dir, running, operatorDeployStartLog, endpoint)
	if _, err := s.run(ctx, script); err != nil {
		return fmt.Errorf("start operator: %w", err)
	}
	return nil
}

func (s deploySSH) readStartLog(ctx context.Context, dir string) ([]byte, error) {
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
	var sessionID string
	err := pollUntil(ctx, func() (bool, error) {
		log, err := s.readStartLog(ctx, dir)
		if err != nil {
			return false, err
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

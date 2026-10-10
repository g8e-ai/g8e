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
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/spf13/cobra"
	sshlib "golang.org/x/crypto/ssh"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	sshpkg "github.com/g8e-ai/g8e/v2/internal/pkg/ssh"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

const (
	// operatorDeployDirPrefix names the per-Operator directories created under
	// --remote-dir when --count is greater than one.
	operatorDeployDirPrefix = "op"
	// operatorDeployMaxParallel stages a whole cohort at once up to the
	// Gateway's live Operator request quota; a larger --parallel could only
	// queue requests the Gateway would reject.
	operatorDeployMaxParallel = constants.PlatformEnrollmentMaxLiveOperatorRequests

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

// remoteDeployClient abstracts binary deployment and host agent execution for remote targets.
type remoteDeployClient interface {
	IsWSL() bool
	UploadBinary(ctx context.Context, sourceBinary, remoteDir string) (string, error)
	ExecuteAgent(ctx context.Context, req models.DeployHostRequest) (models.DeployHostResponse, error)
	Close() error
}

// remoteDeployClientFactory constructs a remoteDeployClient for a remote host.
type remoteDeployClientFactory func(ctx context.Context, host string, port int, identityFile string) (remoteDeployClient, error)

func isLocalTarget(host string) bool {
	h := strings.TrimSpace(host)
	return h == "local" || strings.EqualFold(h, constants.LocalhostHostname) || h == "127.0.0.1"
}

func isRemoteTarget(host string) bool {
	return !isLocalTarget(host)
}

func isLoopbackEndpoint(endpoint string) bool {
	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		return false
	}
	if idx := strings.Index(ep, "://"); idx != -1 {
		ep = ep[idx+3:]
	}
	host := ep
	if h, _, err := net.SplitHostPort(ep); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, constants.LocalhostHostname) || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

func isWindowsPath(p string) bool {
	p = strings.TrimSpace(p)
	if len(p) >= 3 && p[0] == '/' && ((p[1] >= 'a' && p[1] <= 'z') || (p[1] >= 'A' && p[1] <= 'Z')) && p[2] == ':' {
		return true
	}
	if len(p) >= 2 && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) && p[1] == ':' {
		return true
	}
	return strings.HasPrefix(p, `\\`) || strings.Contains(p, `\`)
}

func toWindowsFilePath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) >= 3 && p[0] == '/' && ((p[1] >= 'a' && p[1] <= 'z') || (p[1] >= 'A' && p[1] <= 'Z')) && p[2] == ':' {
		return p[1:]
	}
	return p
}

func resolveSFTPPath(wd, p string) string {
	p = strings.TrimSpace(p)
	if p == "~" {
		return wd
	}
	if strings.HasPrefix(p, "~/") {
		return path.Join(wd, strings.TrimPrefix(p, "~/"))
	}
	if strings.HasPrefix(p, `~\`) {
		return path.Join(wd, strings.TrimPrefix(p, `~\`))
	}
	return p
}

type sshRemoteDeployClient struct {
	client        *sshlib.Client
	sftpClient    *sftp.Client
	host          string
	isWSL         bool
	workingDir    string
	agentPath     string
	stopKeepalive func()
}

func defaultRemoteDeployClientFactory(ctx context.Context, host string, port int, identityFile string) (remoteDeployClient, error) {
	var portStr string
	if port > 0 {
		portStr = strconv.Itoa(port)
	}
	r, err := sshpkg.ResolveHost(host, "", "", identityFile, "")
	if err != nil {
		return nil, fmt.Errorf("resolve host %s: %w", host, err)
	}
	if portStr != "" {
		r.Port = portStr
	}

	authSock := os.Getenv(string(constants.EnvVar.SSHAuthSock))
	authMethods, err := sshpkg.BuildAuthMethods(r, authSock, "")
	if err != nil {
		return nil, fmt.Errorf("build ssh auth for %s: %w", host, err)
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no ssh auth methods available for %s", host)
	}

	hostKeyCB, err := sshpkg.BuildHostKeyCallback("")
	if err != nil {
		return nil, fmt.Errorf("ssh host key callback: %w", err)
	}

	clientConfig := &sshlib.ClientConfig{
		User:            r.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCB,
		Timeout:         30 * time.Second,
	}

	addr := net.JoinHostPort(r.Hostname, r.Port)
	client, err := sshpkg.DialSSH(ctx, r, clientConfig, addr)
	if err != nil {
		return nil, fmt.Errorf("dial ssh %s: %w", host, err)
	}

	stopKeepalive := sshpkg.StartKeepalive(ctx, client, constants.SSHKeepaliveInterval, constants.SSHKeepaliveMaxMissed)

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		stopKeepalive()
		_ = client.Close()
		return nil, fmt.Errorf("sftp client %s: %w", host, err)
	}

	wd, err := sftpClient.Getwd()
	if err != nil {
		wd = "."
	}
	isWSL := isWindowsPath(wd)

	return &sshRemoteDeployClient{
		client:        client,
		sftpClient:    sftpClient,
		host:          host,
		isWSL:         isWSL,
		workingDir:    wd,
		stopKeepalive: stopKeepalive,
	}, nil
}

func (c *sshRemoteDeployClient) IsWSL() bool {
	return c.isWSL
}

func (c *sshRemoteDeployClient) Close() error {
	if c.stopKeepalive != nil {
		c.stopKeepalive()
	}
	var errs []error
	if c.sftpClient != nil {
		if err := c.sftpClient.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.client != nil {
		if err := c.client.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (c *sshRemoteDeployClient) runAgentCommand(ctx context.Context, cmdStr string, req models.DeployHostRequest) (models.DeployHostResponse, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return models.DeployHostResponse{}, fmt.Errorf("new ssh session on %s: %w", c.host, err)
	}
	defer session.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-done:
		}
	}()

	reqData, err := json.Marshal(req)
	if err != nil {
		return models.DeployHostResponse{}, fmt.Errorf("encode deploy-host request: %w", err)
	}

	var stdout, stderr bytes.Buffer
	session.Stdin = bytes.NewReader(reqData)
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmdStr); err != nil {
		return models.DeployHostResponse{}, fmt.Errorf("exec deploy-host on %s: %w (stderr: %s)", c.host, err, strings.TrimSpace(stderr.String()))
	}

	var resp models.DeployHostResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return resp, fmt.Errorf("decode deploy-host response from %s: %w (stdout: %s)", c.host, err, strings.TrimSpace(stdout.String()))
	}
	if !resp.Success {
		return resp, fmt.Errorf("deploy-host on %s: %s", c.host, resp.Error)
	}
	return resp, nil
}

func (c *sshRemoteDeployClient) UploadBinary(ctx context.Context, sourceBinary, remoteDir string) (string, error) {
	if c.isWSL {
		// WSL remote host: SFTP writes to the Windows filesystem.
		windowsStagingDir := path.Join(c.workingDir, constants.DeployBinDirname)
		if err := c.sftpClient.MkdirAll(windowsStagingDir); err != nil {
			return "", fmt.Errorf("create windows staging dir on %s: %w", c.host, err)
		}
		stagingFile := path.Join(windowsStagingDir, "g8e.staging")

		srcFile, err := os.Open(sourceBinary)
		if err != nil {
			return "", fmt.Errorf("open source binary: %w", err)
		}
		defer srcFile.Close()

		dstFile, err := c.sftpClient.OpenFile(stagingFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			return "", fmt.Errorf("create staging binary on %s: %w", c.host, err)
		}
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			_ = dstFile.Close()
			return "", fmt.Errorf("upload staging binary to %s: %w", c.host, err)
		}
		if err := dstFile.Chmod(constants.PermFileExecutable); err != nil {
			_ = dstFile.Close()
			return "", fmt.Errorf("chmod staging binary on %s: %w", c.host, err)
		}
		if err := dstFile.Close(); err != nil {
			return "", fmt.Errorf("close staging binary on %s: %w", c.host, err)
		}

		// Bootstrap once via wsl.exe --cd <windows staging dir> -e ./g8e.staging operator deploy-host
		bootstrapCmd := fmt.Sprintf("wsl.exe --cd %s -e ./g8e.staging operator deploy-host", toWindowsFilePath(windowsStagingDir))
		bootstrapReq := models.DeployHostRequest{
			Action:  models.DeployHostActionInstall,
			Source:  "./g8e.staging",
			DestDir: remoteDir,
		}
		resp, err := c.runAgentCommand(ctx, bootstrapCmd, bootstrapReq)
		_ = c.sftpClient.Remove(stagingFile)
		if err != nil {
			return "", fmt.Errorf("%w: WSL bootstrap failed on %s: %v", constants.ErrWSLUnavailable, c.host, err)
		}
		if !resp.Success {
			return "", fmt.Errorf("%w: WSL bootstrap on %s: %s", constants.ErrWSLUnavailable, c.host, resp.Error)
		}

		targetBinDir := path.Join(remoteDir, constants.DeployBinDirname)
		c.agentPath = path.Join(targetBinDir, "g8e")
		return targetBinDir, nil
	}

	// Linux remote host: SFTP upload to <destDir>/.deploy-bin/g8e.new, Chmod, PosixRename
	resolvedDestDir := resolveSFTPPath(c.workingDir, remoteDir)
	targetBinDir := path.Join(resolvedDestDir, constants.DeployBinDirname)
	if err := c.sftpClient.MkdirAll(targetBinDir); err != nil {
		return "", fmt.Errorf("create %s on %s: %w", targetBinDir, c.host, err)
	}

	stagingPath := path.Join(targetBinDir, "g8e.new")
	srcFile, err := os.Open(sourceBinary)
	if err != nil {
		return "", fmt.Errorf("open source binary: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := c.sftpClient.OpenFile(stagingPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return "", fmt.Errorf("create %s on %s: %w", stagingPath, c.host, err)
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		return "", fmt.Errorf("upload binary to %s on %s: %w", stagingPath, c.host, err)
	}
	if err := dstFile.Chmod(constants.PermFileExecutable); err != nil {
		_ = dstFile.Close()
		return "", fmt.Errorf("chmod %s on %s: %w", stagingPath, c.host, err)
	}
	if err := dstFile.Close(); err != nil {
		return "", fmt.Errorf("close %s on %s: %w", stagingPath, c.host, err)
	}

	finalPath := path.Join(targetBinDir, "g8e")
	if err := c.sftpClient.PosixRename(stagingPath, finalPath); err != nil {
		_ = c.sftpClient.Remove(finalPath)
		if err := c.sftpClient.Rename(stagingPath, finalPath); err != nil {
			return "", fmt.Errorf("install binary on %s: %w", c.host, err)
		}
	}

	c.agentPath = finalPath
	return targetBinDir, nil
}

func (c *sshRemoteDeployClient) ExecuteAgent(ctx context.Context, req models.DeployHostRequest) (models.DeployHostResponse, error) {
	cmdStr := fmt.Sprintf("%s operator deploy-host", c.agentPath)
	if c.isWSL {
		cmdStr = fmt.Sprintf("wsl.exe -e %s operator deploy-host", c.agentPath)
	}
	resp, err := c.runAgentCommand(ctx, cmdStr, req)
	if err != nil {
		if c.isWSL {
			return resp, fmt.Errorf("%w: %v", constants.ErrWSLUnavailable, err)
		}
		return resp, err
	}
	return resp, nil
}

// deploySSH runs commands on, and copies files to, one local or remote host.
type deploySSH struct {
	host         string
	port         int
	identityFile string
	stderr       io.Writer
	local        bool
	binaryDir    string
	launchID     string
	remoteClient remoteDeployClient
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
	remoteFactory ...remoteDeployClientFactory,
) *cobra.Command {
	rFactory := defaultRemoteDeployClientFactory
	if len(remoteFactory) > 0 && remoteFactory[0] != nil {
		rFactory = remoteFactory[0]
	}

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
--parallel bounds concurrent deployments (default and maximum: the Gateway live Operator request quota).

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
				useDeployGatewayPorts(cmd)
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
			var cleanup func()
			defer func() {
				if cleanup != nil {
					cleanup()
				}
			}()

			if dockerContext != "" {
				deployed, failed, err = executeDeployDocker(ctx, cmd, dockerContext, dockerImage, remoteDir, dockerMounts, dirs, opts, parallel, len(hostList)*len(dirs))
				if err != nil {
					return err
				}
			} else {
				deployed, failed, cleanup, err = executeDeploySSH(ctx, cmd, hostList, port, identityFile, remoteDir, sourceBinary, dirs, opts, parallel, rFactory)
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

func (s deploySSH) name() string { return s.host }

func (s deploySSH) markReady(context.Context, string) error { return nil }

func (s deploySSH) binaryName() string {
	if s.local && runtime.GOOS == "windows" {
		return "g8e.exe"
	}
	return "g8e"
}

// prepareDir creates dir on the host and returns its absolute path, which is
// the value the worker's --working-dir carries.
func (s deploySSH) prepareDir(ctx context.Context, dir string) (string, error) {
	if s.local {
		dirs, err := ExecuteDeployHostPrepare(ctx, []string{dir})
		if err != nil {
			return "", err
		}
		return dirs[0], nil
	}
	resp, err := s.remoteClient.ExecuteAgent(ctx, models.DeployHostRequest{
		Action: models.DeployHostActionPrepare,
		Dirs:   []string{dir},
	})
	if err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	if len(resp.ResolvedDirs) == 0 {
		return "", fmt.Errorf("prepare %s returned no resolved dirs", dir)
	}
	absDir := resp.ResolvedDirs[0]
	if !operatorDeployRemoteDirPattern.MatchString(absDir) {
		return "", fmt.Errorf("%w: resolved directory %q must match %s", constants.ErrPathValidation, absDir, operatorDeployRemoteDirPattern)
	}
	return absDir, nil
}

// installBinary uploads beside the target and renames into place: scp cannot
// open a running (or hard-linked, shared) g8e for writing (ETXTBSY), but a
// rename replaces the directory entry and leaves the old inode alone.
func (s deploySSH) installBinary(ctx context.Context, sourceBinary, dir string) error {
	binName := s.binaryName()
	if s.local {
		if s.binaryDir != "" {
			return ExecuteDeployHostLink(ctx, s.binaryDir, dir, binName)
		}
		target := filepath.Join(dir, binName)
		return ExecuteDeployHostInstall(ctx, sourceBinary, target)
	}
	_, err := s.remoteClient.ExecuteAgent(ctx, models.DeployHostRequest{
		Action:    models.DeployHostActionLink,
		BinaryDir: s.binaryDir,
		DestDir:   dir,
		Binary:    "g8e",
	})
	return err
}

// preflightGateway runs the installed binary's gateway-preflight on this host,
// so a Gateway the workers cannot reach fails the deploy before any starts.
func (s deploySSH) preflightGateway(ctx context.Context, args []string) error {
	if s.local {
		return ExecuteDeployHostPreflight(ctx, args)
	}
	_, err := s.remoteClient.ExecuteAgent(ctx, models.DeployHostRequest{
		Action:        models.DeployHostActionPreflight,
		PreflightArgs: args,
	})
	return err
}

// startOperator stops any worker previously deployed from dir, clears its start
// log, and starts a new worker.
func (s deploySSH) startOperator(ctx context.Context, dir, endpoint string, startArgs ...string) error {
	args := append([]string{"--endpoint", endpoint}, startArgs...)
	if s.local {
		_, err := ExecuteDeployHostStart(ctx, dir, s.binaryName(), args)
		return err
	}
	_, err := s.remoteClient.ExecuteAgent(ctx, models.DeployHostRequest{
		Action:     models.DeployHostActionStart,
		WorkingDir: dir,
		Binary:     "g8e",
		Args:       args,
	})
	return err
}

func (s deploySSH) readDeploymentState(ctx context.Context, dir string) (*models.OperatorDeploymentState, error) {
	var state *models.OperatorDeploymentState
	if s.local {
		fileSvc, err := fs.NewRuntimeFileService(dir, slog.Default())
		if err != nil {
			return nil, fmt.Errorf("%w: %w", constants.ErrFileServiceInit, err)
		}
		var readErr error
		state, readErr = readOperatorDeploymentState(ctx, fileSvc)
		if readErr != nil {
			return nil, readErr
		}
	} else {
		resp, err := s.remoteClient.ExecuteAgent(ctx, models.DeployHostRequest{
			Action:     models.DeployHostActionState,
			WorkingDir: dir,
		})
		if err != nil {
			return nil, err
		}
		state = resp.State
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

// useDeployGatewayPorts points this CLI's own Gateway calls (cohort approval)
// at the Gateway ports the workers dial. Deploy's SSH --port shadows the root
// -p flag, so --gateway-http-port and --gateway-https-port are the only way to
// name a non-default Gateway port here. An explicit URL endpoint is kept.
func useDeployGatewayPorts(cmd *cobra.Command) {
	endpoint, _ := cmd.Flags().GetString("endpoint")
	host := strings.TrimSpace(endpoint)
	if strings.Contains(host, "://") {
		return
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" {
		host = constants.LocalhostHostname
	}
	if flag := cmd.Flags().Lookup("gateway-http-port"); flag.Changed {
		config.SetHTTPEndpointOverride(net.JoinHostPort(host, flag.Value.String()))
	}
	if flag := cmd.Flags().Lookup("gateway-https-port"); flag.Changed {
		config.SetHTTPSEndpointOverride(net.JoinHostPort(host, flag.Value.String()))
	}
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
	if dockerContext != "" {
		if local || hosts != "" {
			return "", "", fmt.Errorf("%w: --docker-context cannot be combined with --local or --hosts", constants.ErrMissingRequiredField)
		}
	} else if !local && strings.TrimSpace(hosts) == "" {
		return "", "", fmt.Errorf("%w: --hosts or --local is required", constants.ErrMissingRequiredField)
	}

	if local && !cmd.Flags().Changed("dest-dir") && !cmd.Flags().Changed("remote-dir") {
		return "", "", fmt.Errorf("%w: --local requires --dest-dir", constants.ErrMissingRequiredField)
	}

	if dockerContext != "" {
		effectiveHosts = dockerContext
	} else {
		seen := make(map[string]bool)
		var targets []string
		if local {
			targets = append(targets, "local")
			seen["local"] = true
		}
		if hosts != "" {
			for _, raw := range strings.Split(hosts, ",") {
				h := strings.TrimSpace(raw)
				if h == "" || strings.HasPrefix(h, "-") || strings.ContainsAny(h, " \t\r\n") || seen[h] {
					return "", "", fmt.Errorf("%w: invalid or duplicate host %q", constants.ErrMissingRequiredField, raw)
				}
				seen[h] = true
				targets = append(targets, h)
			}
		}
		if len(targets) == 0 {
			return "", "", fmt.Errorf("%w: --hosts or --local is required", constants.ErrMissingRequiredField)
		}
		effectiveHosts = strings.Join(targets, ",")
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

	hasRemote := false
	if dockerContext == "" {
		for _, h := range strings.Split(effectiveHosts, ",") {
			if isRemoteTarget(h) {
				hasRemote = true
				break
			}
		}
	}

	if hasRemote {
		if background {
			if workerEndpoint == "" {
				return "", "", fmt.Errorf("%w: %w: --operator-endpoint is required for remote deployment with --background", constants.ErrMissingRequiredField, constants.ErrOperatorEndpointInvalid)
			}
			if isLoopbackEndpoint(workerEndpoint) {
				return "", "", fmt.Errorf("%w: remote deployment requires a non-loopback --operator-endpoint (got %q)", constants.ErrOperatorEndpointInvalid, workerEndpoint)
			}
		} else if workerEndpoint != "" && isLoopbackEndpoint(workerEndpoint) {
			return "", "", fmt.Errorf("%w: remote deployment requires a non-loopback --operator-endpoint (got %q)", constants.ErrOperatorEndpointInvalid, workerEndpoint)
		}
	} else {
		if workerEndpoint == "" {
			workerEndpoint = endpoint
		}
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
	remoteDir, sourceBinary string,
	dirs []string,
	opts operatorDeployOptions,
	parallel int,
	clientFactory remoteDeployClientFactory,
) ([]deployedOperator, []string, func(), error) {
	var deployed []deployedOperator
	var failed []string
	var clients []remoteDeployClient
	cleanup := func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}
	var err error
	defer func() {
		if err != nil {
			cleanup()
		}
	}()

	for _, host := range hostList {
		host = strings.TrimSpace(host)
		isLocal := isLocalTarget(host)
		s := deploySSH{host: host, port: port, identityFile: identityFile, stderr: cmd.ErrOrStderr(), local: isLocal}

		if isLocal {
			cache, prepErr := s.prepareDir(ctx, strings.TrimSuffix(remoteDir, "/")+"/.deploy-bin")
			if prepErr != nil {
				err = prepErr
				return deployed, failed, cleanup, err
			}
			if instErr := s.installBinary(ctx, sourceBinary, cache); instErr != nil {
				err = instErr
				return deployed, failed, cleanup, err
			}
			s.binaryDir = cache
			if opts.preflight != nil {
				if pfErr := s.preflightGateway(ctx, opts.preflight); pfErr != nil {
					err = gatewayPreflightError(s.host, opts.endpoint, pfErr)
					return deployed, failed, cleanup, err
				}
			}
		} else {
			rc, dialErr := clientFactory(ctx, host, port, identityFile)
			if dialErr != nil {
				err = fmt.Errorf("connect to remote host %s: %w", host, dialErr)
				return deployed, failed, cleanup, err
			}
			clients = append(clients, rc)
			s.remoteClient = rc

			cache, upErr := rc.UploadBinary(ctx, sourceBinary, remoteDir)
			if upErr != nil {
				err = fmt.Errorf("upload binary to %s: %w", host, upErr)
				return deployed, failed, cleanup, err
			}
			s.binaryDir = cache

			if opts.preflight != nil {
				if pfErr := s.preflightGateway(ctx, opts.preflight); pfErr != nil {
					err = gatewayPreflightError(s.host, opts.endpoint, pfErr)
					return deployed, failed, cleanup, err
				}
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
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
			return deployed, failed, cleanup, err
		}
	}
	return deployed, failed, cleanup, nil
}

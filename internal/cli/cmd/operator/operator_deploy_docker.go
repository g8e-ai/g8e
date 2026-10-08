// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.

package operatorcmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/uuid"
)

const (
	dockerManagedLabel    = "io.g8e.operator-deploy.managed"
	dockerDeploymentLabel = "io.g8e.operator-deploy.deployment"
	dockerDirectoryLabel  = "io.g8e.operator-deploy.directory"
	dockerIndexLabel      = "io.g8e.operator-deploy.index"
)

var dockerSafeValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/@+-]*$`)

type dockerCommandRunner interface {
	Run(context.Context, ...string) ([]byte, error)
}

type execDockerRunner struct{}

func (r execDockerRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

type dockerOperatorSpec struct {
	dir, container, volume, hostname string
	startArgs                        []string
	launchTime                       time.Time
	launchID                         string
	launched                         bool
}

type deployDocker struct {
	context, image, imageID, root, deployment string
	mounts                                    []string
	runner                                    dockerCommandRunner
	mu                                        sync.RWMutex
	operators                                 map[string]*dockerOperatorSpec
}

func newDeployDocker(contextName, image, root string, mounts []string) *deployDocker {
	sum := sha256.Sum256([]byte(contextName + "\x00" + path.Clean(root)))
	return &deployDocker{
		context: contextName, image: image, root: path.Clean(root),
		deployment: hex.EncodeToString(sum[:8]), mounts: mounts,
		runner: execDockerRunner{}, operators: make(map[string]*dockerOperatorSpec),
	}
}

func (d *deployDocker) name() string { return "docker:" + d.context }

func (d *deployDocker) docker(ctx context.Context, args ...string) ([]byte, error) {
	return d.runner.Run(ctx, append([]string{"--context", d.context}, args...)...)
}

// prepare performs all shared, non-fleet checks before any persistent resource
// is created. The image ID is then used for every container so a mutable tag
// cannot change halfway through a rollout.
func (d *deployDocker) prepare(ctx context.Context, endpoint string) error {
	if !dockerSafeValue.MatchString(d.context) || !dockerSafeValue.MatchString(d.image) {
		return fmt.Errorf("%w: unsafe Docker context or image name", constants.ErrPathValidation)
	}
	if err := d.validateMounts(); err != nil {
		return err
	}
	daemon, err := d.docker(ctx, "info", "--format", "{{.OSType}}/{{.Architecture}}")
	if err != nil {
		return fmt.Errorf("docker context %q is unavailable: %w", d.context, err)
	}
	image, err := d.docker(ctx, "image", "inspect", "--format", "{{.Id}} {{.Os}}/{{.Architecture}}", d.image)
	if err != nil {
		return fmt.Errorf("docker image %q is unavailable on context %q: %w", d.image, d.context, err)
	}
	parts := strings.Fields(string(image))
	if len(parts) != 2 {
		return fmt.Errorf("unexpected Docker image inspection output %q", strings.TrimSpace(string(image)))
	}
	if got, want := normalizeDockerPlatform(parts[1]), normalizeDockerPlatform(strings.TrimSpace(string(daemon))); got != want || !strings.HasPrefix(got, "linux/") {
		return fmt.Errorf("docker image platform %s is incompatible with daemon platform %s", got, want)
	}
	d.imageID = parts[0]
	if endpoint != "" {
		_, err = d.docker(ctx, "run", "--rm", "--entrypoint", "/gateway-preflight.sh", d.imageID, endpoint)
		if err != nil {
			return fmt.Errorf("docker context %q cannot verify Gateway HTTP/TLS connectivity at %s on ports 8080/8443: %w", d.context, endpoint, err)
		}
	}
	return nil
}

func normalizeDockerPlatform(platform string) string {
	osName, arch, ok := strings.Cut(strings.TrimSpace(platform), "/")
	if !ok {
		return strings.TrimSpace(platform)
	}
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	return osName + "/" + arch
}

func (d *deployDocker) validateMounts() error {
	for _, mount := range d.mounts {
		fields := strings.Split(mount, ",")
		values := map[string]string{}
		readonly := false
		for _, field := range fields {
			if field == "readonly" || field == "ro" {
				readonly = true
				continue
			}
			k, v, ok := strings.Cut(field, "=")
			if !ok {
				return fmt.Errorf("%w: invalid --docker-mount field %q", constants.ErrPathValidation, field)
			}
			values[k] = v
		}
		if values["type"] != "bind" || !path.IsAbs(values["source"]) || !path.IsAbs(values["target"]) || !readonly {
			return fmt.Errorf("%w: --docker-mount must be an absolute read-only bind mount (type=bind,source=/...,target=/...,readonly)", constants.ErrPathValidation)
		}
		target := path.Clean(values["target"])
		if pathContains(target, d.root) || pathContains(target, "/g8e") {
			return fmt.Errorf("%w: mount target %q covers the Operator runtime or executable", constants.ErrPathValidation, target)
		}
	}
	return nil
}

func pathContains(parent, child string) bool {
	parent, child = path.Clean(parent), path.Clean(child)
	return parent == child || strings.HasPrefix(child, strings.TrimSuffix(parent, "/")+"/")
}

func (d *deployDocker) spec(dir string, startArgs []string) *dockerOperatorSpec {
	sum := sha256.Sum256([]byte(d.deployment + "\x00" + dir))
	suffix := hex.EncodeToString(sum[:6])
	name := "g8e-op-" + d.deployment + "-" + suffix
	return &dockerOperatorSpec{dir: dir, container: name, volume: name + "-data", hostname: name, startArgs: startArgs}
}

func dockerNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such container") || strings.Contains(message, "no such volume") || strings.Contains(message, "no such object")
}

func (d *deployDocker) ensureOwnedVolume(ctx context.Context, op *dockerOperatorSpec) error {
	out, err := d.docker(ctx, "volume", "inspect", "--format", "{{index .Labels \""+dockerManagedLabel+"\"}} {{index .Labels \""+dockerDeploymentLabel+"\"}} {{index .Labels \""+dockerDirectoryLabel+"\"}}", op.volume)
	if err == nil {
		if strings.TrimSpace(string(out)) != "true "+d.deployment+" "+op.dir {
			return fmt.Errorf("refusing to adopt Docker volume %q: ownership labels do not match", op.volume)
		}
		return nil
	}
	if !dockerNotFound(err) {
		return err
	}
	_, err = d.docker(ctx, "volume", "create", "--label", dockerManagedLabel+"=true", "--label", dockerDeploymentLabel+"="+d.deployment, "--label", dockerDirectoryLabel+"="+op.dir, "--label", dockerIndexLabel+"="+path.Base(op.dir), op.volume)
	return err
}

func (d *deployDocker) removeOwnedContainer(ctx context.Context, op *dockerOperatorSpec) error {
	out, err := d.docker(ctx, "container", "inspect", "--format", "{{index .Config.Labels \""+dockerManagedLabel+"\"}} {{index .Config.Labels \""+dockerDeploymentLabel+"\"}} {{index .Config.Labels \""+dockerDirectoryLabel+"\"}}", op.container)
	if dockerNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "true "+d.deployment+" "+op.dir {
		return fmt.Errorf("refusing to replace Docker container %q: ownership labels do not match", op.container)
	}
	_, _ = d.docker(ctx, "container", "stop", "--time", "10", op.container)
	_, err = d.docker(ctx, "container", "rm", op.container)
	return err
}

func (d *deployDocker) createContainer(ctx context.Context, op *dockerOperatorSpec, endpoint string) error {
	args := []string{"container", "create", "--name", op.container, "--hostname", op.hostname,
		"--label", dockerManagedLabel + "=true", "--label", dockerDeploymentLabel + "=" + d.deployment,
		"--label", dockerDirectoryLabel + "=" + op.dir, "--label", dockerIndexLabel + "=" + path.Base(op.dir), "--restart", "no",
		"--mount", "type=volume,source=" + op.volume + ",target=" + op.dir, "--workdir", op.dir}
	for _, mount := range d.mounts {
		args = append(args, "--mount", mount)
	}
	args = append(args, d.imageID, "operator", "start", "--endpoint", endpoint)
	args = append(args, op.startArgs...)
	args = append(args, "--working-dir", op.dir, "--deployment-id="+op.launchID)
	_, err := d.docker(ctx, args...)
	return err
}

func (d *deployDocker) prepareOperator(ctx context.Context, dir, endpoint string, startArgs []string) (*dockerOperatorSpec, error) {
	op := d.spec(dir, startArgs)
	var err error
	op.launchID, err = uuid.NewString()
	if err != nil {
		return nil, fmt.Errorf("deployment launch identity: %w", err)
	}
	if err := d.ensureOwnedVolume(ctx, op); err != nil {
		return nil, err
	}
	if err := d.removeOwnedContainer(ctx, op); err != nil {
		return nil, err
	}
	if err := d.createContainer(ctx, op, endpoint); err != nil {
		return nil, err
	}
	d.mu.Lock()
	d.operators[dir] = op
	d.mu.Unlock()
	return op, nil
}

func (d *deployDocker) operator(dir string) (*dockerOperatorSpec, error) {
	d.mu.RLock()
	op := d.operators[dir]
	d.mu.RUnlock()
	if op == nil {
		return nil, fmt.Errorf("docker Operator %q was not prepared", dir)
	}
	return op, nil
}

func (d *deployDocker) startOperator(ctx context.Context, dir, _ string, _ ...string) error {
	op, err := d.operator(dir)
	if err != nil {
		return err
	}
	// Enrollment is retried when a request is rejected, times out, or its
	// approval result is uncertain. The previous attempt may still be running,
	// so give it a bounded stop before starting the next launch. A stopped
	// container also accepts this operation, which keeps the retry path simple.
	if op.launched {
		if _, err := d.docker(ctx, "container", "stop", "--time", "10", op.container); err != nil {
			return err
		}
	}
	op.launchTime = time.Now().UTC().Add(-time.Second)
	if _, err := d.docker(ctx, "container", "start", op.container); err != nil {
		return err
	}
	op.launched = true
	return nil
}

func (d *deployDocker) readStartLog(ctx context.Context, dir string) ([]byte, error) {
	op, err := d.operator(dir)
	if err != nil {
		return nil, err
	}
	args := []string{"container", "logs", "--tail", "200"}
	if !op.launchTime.IsZero() {
		args = append(args, "--since", op.launchTime.Format(time.RFC3339Nano))
	}
	return d.docker(ctx, append(args, op.container)...)
}

func (d *deployDocker) running(ctx context.Context, op *dockerOperatorSpec) (bool, string, error) {
	out, err := d.docker(ctx, "container", "inspect", "--format", "{{.State.Running}} {{.State.ExitCode}} {{.State.Error}}", op.container)
	if err != nil {
		return false, "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return false, "", fmt.Errorf("unexpected container state %q", strings.TrimSpace(string(out)))
	}
	return fields[0] == "true", strings.Join(fields[1:], " "), nil
}

func (d *deployDocker) readDeploymentState(ctx context.Context, dir string) (*models.OperatorDeploymentState, error) {
	op, err := d.operator(dir)
	if err != nil {
		return nil, err
	}
	// Probe process state before exec: an exited process may leave a ready record.
	running, status, err := d.running(ctx, op)
	if err != nil {
		return nil, err
	}
	if !running {
		log, logErr := d.readStartLog(ctx, dir)
		if logErr != nil {
			return nil, logErr
		}
		return nil, fmt.Errorf("%w: container %s exited (%s); recent logs: %s", constants.ErrOperatorDeployFailed, op.container, status, strings.TrimSpace(string(log)))
	}
	data, err := d.docker(ctx, "container", "exec", op.container, "./g8e", "operator", "deployment-state", "--working-dir", op.dir)
	if err != nil {
		return nil, err
	}
	state, err := decodeOperatorDeploymentState(data)
	if err != nil {
		return nil, err
	}
	if state != nil && state.LaunchID != op.launchID {
		return nil, nil
	}
	return state, nil
}

func (d *deployDocker) markReady(ctx context.Context, dir string) error {
	op, err := d.operator(dir)
	if err != nil {
		return err
	}
	_, err = d.docker(ctx, "container", "update", "--restart", "unless-stopped", op.container)
	return err
}

func deployDockerOperator(ctx context.Context, cmd *cobra.Command, d *deployDocker, dir string, opts operatorDeployOptions) (deployedOperator, error) {
	args := operatorDeployArgsForDir(opts.startArgs, d.context, dir)
	op, err := d.prepareOperator(ctx, dir, opts.endpoint, args)
	if err != nil {
		return deployedOperator{}, err
	}
	deployed := deployedOperator{target: d, Dir: dir}
	if !opts.background {
		cmd.Printf("Operator container %s prepared on %s (use --background to start)\n", op.container, d.context)
		return deployed, nil
	}
	// Watch before starting: the stream delivers only live events.
	deployed.watch = opts.events.watch(op.launchID)
	if err := d.startOperator(ctx, dir, opts.endpoint); err != nil {
		return deployedOperator{}, err
	}
	deployed.RequestID, err = awaitStaged(ctx, deployed.watch, d, dir)
	if err != nil {
		return deployedOperator{}, err
	}
	cmd.Printf("Staged Operator container %s on %s (enrollment request %s)\n", op.container, d.context, deployed.RequestID)
	return deployed, nil
}

func deployDockerBatch(ctx context.Context, d *deployDocker, dirs []string, opts operatorDeployOptions, parallel int) <-chan operatorDeployResult {
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
				op, err := deployDockerOperator(ctx, cmd, d, dir, opts)
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

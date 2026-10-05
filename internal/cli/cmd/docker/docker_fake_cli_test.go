// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package docker

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
)

// installFakeDocker puts an executable `docker` shell script first on PATH so
// the commands under test never reach a real Docker daemon. The script logs
// each invocation's arguments (one line per call) and then runs body, a
// /bin/sh fragment that may branch on "$1", "$2" and "$*". It returns the
// path of the call log.
func installFakeDocker(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	callLog := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + callLog + "'\n" + body + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return callLog
}

// withoutDocker removes every `docker` executable from PATH.
func withoutDocker(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

func fakeDockerCalls(t *testing.T, callLog string) []string {
	t.Helper()
	data, err := os.ReadFile(callLog)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// composeLine is the logged form of a compose invocation against the root
// compose file in the current directory.
func composeLine(t *testing.T, profiles []string, tail string) string {
	t.Helper()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	line := "compose -f " + filepath.Join(cwd, constants.DockerComposeFile)
	for _, p := range profiles {
		line += " --profile " + p
	}
	return line + " " + tail
}

func allTeardownProfiles() []string {
	return []string{
		constants.DockerBootstrappedProfile,
		constants.DockerEvaluationProfile,
		constants.DockerCrossEnrollProfile,
		constants.DockerG8ellamaProfile,
	}
}

func runDockerCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	// Build/export tests use a stamped binary identity, as the root command
	// does in production, rather than discovering the developer's source tree.
	cmd.SetContext(shared.ContextWithVersionInfo(t.Context(), serve.VersionInfo{
		SourceTreeStateHash: strings.Repeat("a", 64),
	}))
	err := cmd.RunE(cmd, args)
	return buf.String(), err
}

func tarWithFile(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: name, Mode: int64(constants.PermFileExecutable), Size: int64(len(content)), Typeflag: tar.TypeReg}))
	_, err := writer.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return archive.Bytes()
}

// fakeDockerForImageExport serves the inspect/create/cp/rm sequence the
// runtime-binary export performs, streaming the archive named by
// $FAKE_DOCKER_ARCHIVE for `docker cp`. Other subcommands succeed.
const fakeDockerForImageExport = `
case "$1 $2" in
  "image inspect") printf '{"Id":"sha256:img123","Config":{"Labels":{}}}\n'; exit 0;;
esac
case "$1" in
  create) echo cont-1; exit 0;;
  cp) cat "$FAKE_DOCKER_ARCHIVE"; exit 0;;
esac
exit 0`

func writeRuntimeArchive(t *testing.T, content []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime.tar")
	require.NoError(t, os.WriteFile(path, tarWithFile(t, "g8e", content), 0o644))
	t.Setenv("FAKE_DOCKER_ARCHIVE", path)
}

// ---------------------------------------------------------------------------
// Compose plumbing
// ---------------------------------------------------------------------------

func TestRunDockerCompose_TargetsTheRootComposeFileWithRequestedProfiles(t *testing.T) {
	writeRootCompose(t)
	callLog := installFakeDocker(t, "exit 0")

	require.NoError(t, RunDockerCompose([]string{"up", "-d"}, "bootstrapped", "", "evaluation"))

	calls := fakeDockerCalls(t, callLog)
	require.Len(t, calls, 2)
	assert.Equal(t, "info", calls[0], "availability is verified before any compose command")
	assert.Equal(t, composeLine(t, []string{"bootstrapped", "evaluation"}, "up -d"), calls[1], "empty profiles are skipped")
}

func TestRunDockerCompose_PropagatesComposeExitStatus(t *testing.T) {
	writeRootCompose(t)
	installFakeDocker(t, `case "$1" in compose) exit 7;; esac; exit 0`)

	err := RunDockerCompose([]string{"up"})

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 7, exitErr.ExitCode())
}

func TestRunDockerCompose_FailsClosedWhenDockerIsUnavailable(t *testing.T) {
	t.Run("docker is not installed", func(t *testing.T) {
		writeRootCompose(t)
		withoutDocker(t)

		err := RunDockerCompose([]string{"up"})

		require.ErrorIs(t, err, constants.ErrServiceUnavailable)
		assert.Contains(t, err.Error(), "not installed")
	})

	t.Run("daemon is not running", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, `[ "$1" = info ] && exit 1; exit 0`)

		err := RunDockerCompose([]string{"up"})

		require.ErrorIs(t, err, constants.ErrServiceUnavailable)
		assert.Contains(t, err.Error(), "daemon is not running")
		assert.Equal(t, []string{"info"}, fakeDockerCalls(t, callLog), "compose must not run against an unreachable daemon")
	})
}

func TestRunDockerComposeOutput_ReturnsCombinedOutputAndWrapsFailure(t *testing.T) {
	t.Run("success returns the command output", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) echo "NAME   STATUS"; echo "gw     Up";; esac; exit 0`)

		out, err := runDockerComposeOutput([]string{"ps"}, "bootstrapped")

		require.NoError(t, err)
		assert.Equal(t, "NAME   STATUS\ngw     Up\n", out)
	})

	t.Run("failure keeps the output for diagnosis and wraps ErrInternal", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) echo "no such service" >&2; exit 3;; esac; exit 0`)

		out, err := runDockerComposeOutput([]string{"ps"})

		require.ErrorIs(t, err, constants.ErrInternal)
		assert.Contains(t, out, "no such service")
	})

	t.Run("docker unavailable", func(t *testing.T) {
		writeRootCompose(t)
		withoutDocker(t)

		_, err := runDockerComposeOutput([]string{"ps"})

		require.ErrorIs(t, err, constants.ErrServiceUnavailable)
	})
}

func TestPrintDockerStackStatus_OnlyShowsSectionWhenContainersRunning(t *testing.T) {
	const header = "Docker Compose Stack\n---------------------\n"

	t.Run("no compose file writes nothing", func(t *testing.T) {
		cmdtest.ChdirTemp(t)
		var buf bytes.Buffer

		err := PrintDockerStackStatus(&buf, "")

		require.ErrorIs(t, err, constants.ErrNotFound)
		assert.Empty(t, buf.String())
	})

	t.Run("docker unavailable writes nothing", func(t *testing.T) {
		writeRootCompose(t)
		withoutDocker(t)
		var buf bytes.Buffer

		err := PrintDockerStackStatus(&buf, "")

		require.ErrorIs(t, err, constants.ErrServiceUnavailable)
		assert.Empty(t, buf.String())
	})

	t.Run("compose ps fails writes nothing", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) exit 2;; esac; exit 0`)
		var buf bytes.Buffer

		err := PrintDockerStackStatus(&buf, "")

		require.Error(t, err)
		assert.Empty(t, buf.String())
	})

	t.Run("no running containers writes nothing", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) printf '  \n';; esac; exit 0`)
		var buf bytes.Buffer

		require.NoError(t, PrintDockerStackStatus(&buf, ""))

		assert.Empty(t, buf.String())
	})

	t.Run("header only ps output writes nothing", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) printf 'NAME      IMAGE     COMMAND   SERVICE   CREATED   STATUS    PORTS\n';; esac; exit 0`)
		var buf bytes.Buffer

		require.NoError(t, PrintDockerStackStatus(&buf, ""))

		assert.Empty(t, buf.String())
	})

	t.Run("running containers are listed for the requested profile", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, `case "$1" in compose) printf 'NAME STATUS\ng8e-gateway Up\n\n';; esac; exit 0`)
		var buf bytes.Buffer

		require.NoError(t, PrintDockerStackStatus(&buf, "bootstrapped"))

		assert.Equal(t, header+"NAME STATUS\ng8e-gateway Up\n", buf.String(), "trailing whitespace is trimmed")
		assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, []string{"bootstrapped"}, "ps"))
	})
}

// ---------------------------------------------------------------------------
// Simple compose-driven subcommands
// ---------------------------------------------------------------------------

func TestDockerRestartCmd(t *testing.T) {
	t.Run("defaults to restarting both operators so they pick up the host binary", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")

		out, err := runDockerCommand(t, dockerRestartCmd())

		require.NoError(t, err)
		assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, nil, "restart g8e-data-operator g8e-inference-operator"))
		assert.Contains(t, out, "Restarting g8e-data-operator, g8e-inference-operator...")
		assert.Contains(t, out, "Restart complete")
	})

	t.Run("restarts exactly the named services under the requested profile", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")
		cmd := dockerRestartCmd()
		require.NoError(t, cmd.Flags().Set("profile", "bootstrapped"))

		_, err := runDockerCommand(t, cmd, "g8e-gateway", "g8e-ensemble")

		require.NoError(t, err)
		assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, []string{"bootstrapped"}, "restart g8e-gateway g8e-ensemble"))
	})

	t.Run("compose failure is reported as a process start failure", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) exit 4;; esac; exit 0`)

		out, err := runDockerCommand(t, dockerRestartCmd())

		require.ErrorIs(t, err, constants.ErrProcessStartFailed)
		assert.NotContains(t, out, "Restart complete")
	})
}

func TestDockerStopCmd(t *testing.T) {
	t.Run("tears down every profile so no optional service is left holding volumes", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")

		out, err := runDockerCommand(t, dockerStopCmd())

		require.NoError(t, err)
		assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, allTeardownProfiles(), "down"))
		assert.Contains(t, out, "Docker Compose stack stopped successfully.")
	})

	t.Run("an explicit profile narrows the teardown", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")
		cmd := dockerStopCmd()
		require.NoError(t, cmd.Flags().Set("profile", "evaluation"))

		_, err := runDockerCommand(t, cmd)

		require.NoError(t, err)
		assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, []string{"evaluation"}, "down"))
	})

	t.Run("compose failure is reported as a process stop failure", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) exit 1;; esac; exit 0`)

		out, err := runDockerCommand(t, dockerStopCmd())

		require.ErrorIs(t, err, constants.ErrProcessStopFailed)
		assert.NotContains(t, out, "stopped successfully")
	})
}

func TestDockerStatusCmd(t *testing.T) {
	t.Run("lists the stack with compose ps", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")

		_, err := runDockerCommand(t, dockerStatusCmd())

		require.NoError(t, err)
		assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, nil, "ps"))
	})

	t.Run("compose failure is an internal error", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) exit 1;; esac; exit 0`)

		_, err := runDockerCommand(t, dockerStatusCmd())

		require.ErrorIs(t, err, constants.ErrInternal)
	})
}

func TestDockerLogsCmd(t *testing.T) {
	tests := []struct {
		name     string
		flags    map[string]string
		args     []string
		wantTail string
		profiles []string
	}{
		{name: "all services", wantTail: "logs"},
		{name: "follow a single service", flags: map[string]string{"follow": "true"}, args: []string{"g8e-gateway"}, wantTail: "logs -f g8e-gateway"},
		{name: "a single service under a profile", flags: map[string]string{"profile": "bootstrapped"}, args: []string{"g8e-ensemble"}, wantTail: "logs g8e-ensemble", profiles: []string{"bootstrapped"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeRootCompose(t)
			callLog := installFakeDocker(t, "exit 0")
			cmd := dockerLogsCmd()
			for k, v := range tt.flags {
				require.NoError(t, cmd.Flags().Set(k, v))
			}

			_, err := runDockerCommand(t, cmd, tt.args...)

			require.NoError(t, err)
			assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, tt.profiles, tt.wantTail))
		})
	}

	t.Run("more than one service name is rejected by argument validation", func(t *testing.T) {
		require.Error(t, dockerLogsCmd().Args(dockerLogsCmd(), []string{"a", "b"}))
	})

	t.Run("compose failure is an internal error", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) exit 1;; esac; exit 0`)

		_, err := runDockerCommand(t, dockerLogsCmd())

		require.ErrorIs(t, err, constants.ErrInternal)
	})
}

func TestDockerStartCmd_SkipEnrollStartsTheStackAndPreparesTheHostRuntime(t *testing.T) {
	t.Run("default profile", func(t *testing.T) {
		tmp := writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")
		cmd := dockerStartCmd()
		require.NoError(t, cmd.Flags().Set("skip-enroll", "true"))

		out, err := runDockerCommand(t, cmd)

		require.NoError(t, err)
		assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, nil, "up -d"))
		assert.DirExists(t, filepath.Join(tmp, constants.RuntimeDirname), "the host .g8e tree must exist before containers mount it")
		assert.Contains(t, out, "Starting Docker Compose unified stack...")
		assert.Contains(t, out, "--skip-enroll set")
		assert.Contains(t, out, "g8e auth enroll pending")
	})

	t.Run("explicit profile is reflected in the command and the message", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")
		cmd := dockerStartCmd()
		require.NoError(t, cmd.Flags().Set("skip-enroll", "true"))
		require.NoError(t, cmd.Flags().Set("profile", "bootstrapped"))

		out, err := runDockerCommand(t, cmd)

		require.NoError(t, err)
		assert.Contains(t, fakeDockerCalls(t, callLog), composeLine(t, []string{"bootstrapped"}, "up -d"))
		assert.Contains(t, out, "unified stack (profiles bootstrapped)")
	})

	t.Run("compose failure is a process start failure and prints no success message", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$1" in compose) exit 1;; esac; exit 0`)
		cmd := dockerStartCmd()
		require.NoError(t, cmd.Flags().Set("skip-enroll", "true"))

		out, err := runDockerCommand(t, cmd)

		require.ErrorIs(t, err, constants.ErrProcessStartFailed)
		assert.NotContains(t, out, "started successfully")
	})
}

// ---------------------------------------------------------------------------
// Destructive subcommands
// ---------------------------------------------------------------------------

func confirmedDestructive(t *testing.T, cmd *cobra.Command) *cobra.Command {
	t.Helper()
	require.NoError(t, cmd.Flags().Set("yes", "true"))
	require.NoError(t, cmd.Flags().Set("skip-backup", "true"))
	return cmd
}

const fakeDockerWithLeftovers = `
case "$1 $2" in
  "volume ls") printf 'g8e_vol_a\ng8e_vol_b\n'; exit 0;;
  "network ls") printf 'g8e_net_a\n'; exit 0;;
esac
exit 0`

func TestDockerCleanCmd_WipesStackAndForceRemovesLeftoverVolumesAndNetworks(t *testing.T) {
	writeRootCompose(t)
	callLog := installFakeDocker(t, fakeDockerWithLeftovers)

	out, err := runDockerCommand(t, confirmedDestructive(t, dockerCleanCmd()))

	require.NoError(t, err)
	calls := fakeDockerCalls(t, callLog)
	assert.Contains(t, calls, composeLine(t, allTeardownProfiles(), "down -v --remove-orphans -t 0"))
	assert.Contains(t, calls, "volume ls -q --filter name="+constants.DockerProjectPrefix+"_")
	assert.Contains(t, calls, "volume rm -f g8e_vol_a")
	assert.Contains(t, calls, "volume rm -f g8e_vol_b")
	assert.Contains(t, calls, "network ls -q --filter name="+constants.DockerProjectPrefix+"_")
	assert.Contains(t, calls, "network rm -f g8e_net_a")
	assert.Contains(t, out, "Docker Compose stack cleaned successfully.")
}

func TestDockerCleanCmd_ComposeFailureIsWarnedAboutButLeftoversAreStillRemoved(t *testing.T) {
	writeRootCompose(t)
	callLog := installFakeDocker(t, `case "$1" in compose) exit 5;; esac`+fakeDockerWithLeftovers)

	out, err := runDockerCommand(t, confirmedDestructive(t, dockerCleanCmd()))

	require.NoError(t, err)
	assert.Contains(t, out, "Warning: compose down had issues")
	assert.Contains(t, fakeDockerCalls(t, callLog), "volume rm -f g8e_vol_a", "cleanup of leftovers must not depend on compose succeeding")
	assert.Contains(t, out, "cleaned successfully")
}

func TestDockerCleanCmd_WarnsWhenALeftoverCannotBeRemoved(t *testing.T) {
	writeRootCompose(t)
	installFakeDocker(t, `
case "$1 $2" in
  "volume ls") echo g8e_stuck; exit 0;;
  "volume rm") exit 1;;
esac
exit 0`)

	out, err := runDockerCommand(t, confirmedDestructive(t, dockerCleanCmd()))

	require.NoError(t, err)
	assert.Contains(t, out, "Warning: could not force-remove volume 'g8e_stuck'")
}

func TestDockerCleanCmd_NothingToCleanWhenDockerIsUnavailable(t *testing.T) {
	writeRootCompose(t)
	withoutDocker(t)

	out, err := runDockerCommand(t, confirmedDestructive(t, dockerCleanCmd()))

	require.NoError(t, err)
	assert.Contains(t, out, "Docker not available — nothing to clean.")
	assert.NotContains(t, out, "cleaned successfully")
}

func TestDockerResetCmd_WipesThenRestartsAndRebuildsTheHostRuntime(t *testing.T) {
	t.Run("default restarts the gateway scope", func(t *testing.T) {
		tmp := writeRootCompose(t)
		callLog := installFakeDocker(t, fakeDockerWithLeftovers)

		out, err := runDockerCommand(t, confirmedDestructive(t, dockerResetCmd()))

		require.NoError(t, err)
		calls := fakeDockerCalls(t, callLog)
		down := indexOf(calls, composeLine(t, allTeardownProfiles(), "down -v --remove-orphans -t 0"))
		up := indexOf(calls, composeLine(t, nil, "up -d"))
		require.GreaterOrEqual(t, down, 0, "the stack must be torn down")
		require.GreaterOrEqual(t, up, 0, "the stack must be started again")
		assert.Less(t, down, up, "teardown precedes startup")
		assert.Contains(t, calls, "volume rm -f g8e_vol_a")
		assert.DirExists(t, filepath.Join(tmp, constants.RuntimeDirname))
		assert.Contains(t, out, "Starting Docker Compose gateway...")
		assert.Contains(t, out, "reset successfully")
	})

	t.Run("an explicit profile narrows both teardown and startup", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")
		cmd := confirmedDestructive(t, dockerResetCmd())
		require.NoError(t, cmd.Flags().Set("profile", "bootstrapped"))

		out, err := runDockerCommand(t, cmd)

		require.NoError(t, err)
		calls := fakeDockerCalls(t, callLog)
		assert.Contains(t, calls, composeLine(t, []string{"bootstrapped"}, "down -v --remove-orphans -t 0"))
		assert.Contains(t, calls, composeLine(t, []string{"bootstrapped"}, "up -d"))
		assert.Contains(t, out, "full stack (profiles bootstrapped)")
	})

	t.Run("startup failure after a successful wipe is a process start failure", func(t *testing.T) {
		writeRootCompose(t)
		installFakeDocker(t, `case "$*" in *" up "*) exit 6;; esac; exit 0`)

		out, err := runDockerCommand(t, confirmedDestructive(t, dockerResetCmd()))

		require.ErrorIs(t, err, constants.ErrProcessStartFailed)
		assert.NotContains(t, out, "reset successfully")
	})
}

func indexOf(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Build and rebuild (image build + runtime binary export)
// ---------------------------------------------------------------------------

func TestDockerBuildCmd_BuildsImagesAndExportsTheRuntimeBinaryToBothLocations(t *testing.T) {
	tmp := writeRootCompose(t)
	callLog := installFakeDocker(t, fakeDockerForImageExport)
	writeRuntimeArchive(t, []byte("exported-runtime-binary"))

	out, err := runDockerCommand(t, dockerBuildCmd())

	require.NoError(t, err)
	calls := fakeDockerCalls(t, callLog)
	var composeBuild string
	for _, c := range calls {
		if strings.HasPrefix(c, "compose -f ") && strings.Contains(c, " build ") {
			composeBuild = c
		}
	}
	require.NotEmpty(t, composeBuild, "images are built through compose")
	assert.Contains(t, composeBuild, "--build-arg VERSION=")
	assert.Contains(t, composeBuild, "--build-arg SOURCE_TREE_HASH=")
	assert.Equal(t, 2, countCalls(calls, "image inspect --format={{json .}} "+defaultDockerGatewayImage), "the runtime binary is exported twice: ./g8e and bin/g8e")
	assert.Equal(t, 2, countCalls(calls, "create sha256:img123"), "the immutable image ID, not the tag, is used to create the export container")
	assert.Equal(t, 2, countCalls(calls, "rm cont-1"), "the export container is always removed")
	for _, rel := range []string{"g8e", filepath.Join(constants.BinDirname, "g8e")} {
		path := filepath.Join(tmp, rel)
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr, rel)
		assert.Equal(t, "exported-runtime-binary", string(data), rel)
		info, statErr := os.Stat(path)
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(constants.PermFileExecutable), info.Mode().Perm(), "%s must be executable", rel)
	}
	assert.Contains(t, out, "Docker images built and runtime binary exported to ./g8e.")
}

func TestDockerBuildCmd_NoCacheAndProfileFlowThroughToCompose(t *testing.T) {
	writeRootCompose(t)
	callLog := installFakeDocker(t, fakeDockerForImageExport)
	writeRuntimeArchive(t, []byte("bin"))
	cmd := dockerBuildCmd()
	require.NoError(t, cmd.Flags().Set("no-cache", "true"))
	require.NoError(t, cmd.Flags().Set("profile", "bootstrapped"))

	_, err := runDockerCommand(t, cmd)

	require.NoError(t, err)
	var found bool
	for _, c := range fakeDockerCalls(t, callLog) {
		if strings.Contains(c, "--profile bootstrapped build ") && strings.HasSuffix(c, "--no-cache") {
			found = true
		}
	}
	assert.True(t, found, "--no-cache and --profile must reach the compose build invocation")
}

func TestDockerBuildCmd_FailureModes(t *testing.T) {
	t.Run("compose build failure stops before any export", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, `case "$1" in compose) exit 9;; esac; exit 0`)

		out, err := runDockerCommand(t, dockerBuildCmd())

		require.ErrorIs(t, err, constants.ErrProcessStartFailed)
		assert.Contains(t, err.Error(), "build images")
		assert.Zero(t, countCalls(fakeDockerCalls(t, callLog), "create sha256:img123"))
		assert.NotContains(t, out, "built and runtime binary exported")
	})

	t.Run("image inspection failure is an export failure and publishes nothing", func(t *testing.T) {
		tmp := writeRootCompose(t)
		installFakeDocker(t, `case "$1 $2" in "image inspect") exit 1;; esac; exit 0`)

		_, err := runDockerCommand(t, dockerBuildCmd())

		require.ErrorIs(t, err, constants.ErrG8eBinaryExport)
		assert.NoFileExists(t, filepath.Join(tmp, "g8e"))
	})
}

func TestDockerRebuildCmd(t *testing.T) {
	t.Run("stops, rebuilds, exports, prepares the runtime and starts again", func(t *testing.T) {
		tmp := writeRootCompose(t)
		callLog := installFakeDocker(t, fakeDockerForImageExport)
		writeRuntimeArchive(t, []byte("rebuilt"))
		cmd := dockerRebuildCmd()
		require.NoError(t, cmd.Flags().Set("profile", "bootstrapped"))

		out, err := runDockerCommand(t, cmd)

		require.NoError(t, err)
		calls := fakeDockerCalls(t, callLog)
		down := indexOf(calls, composeLine(t, []string{"bootstrapped"}, "down"))
		up := indexOf(calls, composeLine(t, []string{"bootstrapped"}, "up -d"))
		require.GreaterOrEqual(t, down, 0)
		require.GreaterOrEqual(t, up, 0)
		assert.Less(t, down, up)
		assert.DirExists(t, filepath.Join(tmp, constants.RuntimeDirname))
		data, readErr := os.ReadFile(filepath.Join(tmp, "g8e"))
		require.NoError(t, readErr)
		assert.Equal(t, "rebuilt", string(data))
		assert.Contains(t, out, "rebuilt and started successfully.")
	})

	t.Run("stop failure is a process stop failure and nothing is rebuilt", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, `case "$*" in *" down"*) exit 2;; esac; exit 0`)

		_, err := runDockerCommand(t, dockerRebuildCmd())

		require.ErrorIs(t, err, constants.ErrProcessStopFailed)
		for _, c := range fakeDockerCalls(t, callLog) {
			assert.NotContains(t, c, " build ")
		}
	})

	t.Run("build failure leaves the stack down", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, `case "$*" in *" build "*) exit 3;; esac; exit 0`)

		_, err := runDockerCommand(t, dockerRebuildCmd())

		require.ErrorIs(t, err, constants.ErrProcessStartFailed)
		assert.Zero(t, countCalls(fakeDockerCalls(t, callLog), composeLine(t, nil, "up -d")), "a failed build must not start the stack")
	})
}

func countCalls(calls []string, want string) int {
	n := 0
	for _, c := range calls {
		if c == want {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Init pre-flight and early failures
// ---------------------------------------------------------------------------

func writeInitEnv(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("G8E_OLLAMA_ENDPOINT=http://127.0.0.1:11434\n"), 0o644))
}

func initCmdWithStubs(t *testing.T, configLoader func(string) (*config.Config, error), fileSvcErr error) *cobra.Command {
	t.Helper()
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	if configLoader == nil {
		configLoader = cmdtest.ConfigLoaderFor(cfg)
	}
	factory := cmdtest.FileSvcFactoryFor(fileSvc)
	if fileSvcErr != nil {
		factory = cmdtest.FailingFileSvcFactory(fileSvcErr)
	}
	return dockerInitCmdWithConfig(
		configLoader,
		factory,
		authcmd.PanickingClientFactory(),
		func(*config.Config) error { return nil },
		authcmd.PanickingEnrollerFactory(),
		authcmd.PanickingAppEnrollerFactory(),
	)
}

func TestDockerInit_PreflightFailuresStopBeforeAnyStackChange(t *testing.T) {
	t.Run("docker not installed", func(t *testing.T) {
		tmp := writeRootCompose(t)
		writeInitEnv(t, tmp)
		withoutDocker(t)

		_, err := runDockerCommand(t, initCmdWithStubs(t, nil, nil))

		require.ErrorIs(t, err, constants.ErrServiceUnavailable)
	})

	t.Run("evaluation environment missing", func(t *testing.T) {
		writeRootCompose(t)
		callLog := installFakeDocker(t, "exit 0")

		_, err := runDockerCommand(t, initCmdWithStubs(t, nil, nil))

		require.ErrorIs(t, err, constants.ErrDockerInitEnvRequired)
		assert.NotContains(t, strings.Join(fakeDockerCalls(t, callLog), "\n"), "compose", "no compose command may run before the environment is validated")
	})

	t.Run("a declined --clean confirmation aborts without touching the stack", func(t *testing.T) {
		tmp := writeRootCompose(t)
		writeInitEnv(t, tmp)
		callLog := installFakeDocker(t, "exit 0")
		cmd := initCmdWithStubs(t, nil, nil)
		require.NoError(t, cmd.Flags().Set("clean", "true"))
		cmd.SetIn(strings.NewReader("n\n"))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)

		err := cmd.RunE(cmd, nil)

		require.NoError(t, err)
		assert.Contains(t, buf.String(), "Aborted")
		assert.NotContains(t, strings.Join(fakeDockerCalls(t, callLog), "\n"), "compose", "a declined wipe must not run compose")
	})

	t.Run("config loader failure", func(t *testing.T) {
		tmp := writeRootCompose(t)
		writeInitEnv(t, tmp)
		callLog := installFakeDocker(t, "exit 0")
		loadErr := errors.New("config unreadable")

		_, err := runDockerCommand(t, initCmdWithStubs(t, func(string) (*config.Config, error) { return nil, loadErr }, nil))

		require.ErrorIs(t, err, loadErr)
		assert.NotContains(t, strings.Join(fakeDockerCalls(t, callLog), "\n"), "compose")
	})

	t.Run("file service failure", func(t *testing.T) {
		tmp := writeRootCompose(t)
		writeInitEnv(t, tmp)
		installFakeDocker(t, "exit 0")

		_, err := runDockerCommand(t, initCmdWithStubs(t, nil, errFactory))

		require.ErrorIs(t, err, constants.ErrFileServiceInit)
		require.ErrorIs(t, err, errFactory)
	})
}

func TestDockerInit_ImageBuildAndGatewayStartFailuresAreReportedBeforeHealthWaiting(t *testing.T) {
	t.Run("image build failure", func(t *testing.T) {
		tmp := writeRootCompose(t)
		writeInitEnv(t, tmp)
		callLog := installFakeDocker(t, `case "$*" in *" build "*) exit 9;; esac; exit 0`)

		out, err := runDockerCommand(t, initCmdWithStubs(t, nil, nil))

		require.ErrorIs(t, err, constants.ErrProcessStartFailed)
		assert.Contains(t, out, "Building Docker images for the unified stack...")
		assert.Zero(t, countCalls(fakeDockerCalls(t, callLog), composeLine(t, nil, "up -d")), "the stack must not start after a failed build")
	})

	t.Run("gateway start failure when the build is skipped", func(t *testing.T) {
		tmp := writeRootCompose(t)
		writeInitEnv(t, tmp)
		installFakeDocker(t, `case "$*" in *" up "*) exit 8;; esac; exit 0`)
		cmd := initCmdWithStubs(t, nil, nil)
		require.NoError(t, cmd.Flags().Set("skip-build", "true"))

		out, err := runDockerCommand(t, cmd)

		require.ErrorIs(t, err, constants.ErrProcessStartFailed)
		assert.Contains(t, out, "Starting gateway...")
		assert.NotContains(t, out, "Gateway is healthy")
	})
}

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

//go:build linux
// +build linux

package platform

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRunner is a test commandRunner that records calls and returns scripted
// outputs. It never invokes real system commands.
type fakeRunner struct {
	mu        sync.Mutex
	calls     []fakeCall
	responses map[string]fakeResponse
}

type fakeCall struct {
	env  map[string]string
	name string
	args []string
}

type fakeResponse struct {
	output []byte
	err    error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{responses: map[string]fakeResponse{}}
}

// setResponse maps a key (name + first arg, or just name) to a response.
func (f *fakeRunner) setResponse(key string, output []byte, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[key] = fakeResponse{output: output, err: err}
}

func (f *fakeRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{env: env, name: name, args: args})
	resp, ok := f.responses[name]
	f.mu.Unlock()
	if !ok {
		// Try name + first arg as key.
		key := name
		if len(args) > 0 {
			key = name + " " + args[0]
		}
		f.mu.Lock()
		resp, ok = f.responses[key]
		f.mu.Unlock()
	}
	if !ok {
		return nil, &fakeErr{msg: "no scripted response for " + name}
	}
	return resp.output, resp.err
}

func (f *fakeRunner) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeRunner) call(i int) fakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[i]
}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }

func TestLinux_DetectFamily_Debian(t *testing.T) {
	t.Parallel()
	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	family := detectLinuxFamily(r, context.Background())
	assert.Equal(t, linuxFamilyDebian, family)
}

func TestLinux_DetectFamily_RHEL(t *testing.T) {
	t.Parallel()
	r := newFakeRunner()
	r.setResponse("trust", []byte("list output"), nil)
	family := detectLinuxFamily(r, context.Background())
	assert.Equal(t, linuxFamilyRHEL, family)
}

func TestLinux_DetectFamily_Unknown(t *testing.T) {
	t.Parallel()
	r := newFakeRunner()
	family := detectLinuxFamily(r, context.Background())
	assert.Equal(t, linuxFamilyUnknown, family)
}

func TestLinux_IsTrustedDebian_AlreadyTrusted(t *testing.T) {
	t.Parallel()
	rootPEM, _ := testCA(t, "debian-root")
	rootCert := mustParse(t, rootPEM)
	fp := certFingerprint(rootCert)

	r := newFakeRunner()
	r.setResponse("cat", []byte(rootPEM), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	trusted, err := installer.isTrustedDebian(context.Background(), fp)
	require.NoError(t, err)
	assert.True(t, trusted)
}

func TestLinux_IsTrustedDebian_NotPresent(t *testing.T) {
	t.Parallel()
	rootPEM, _ := testCA(t, "debian-root-not-present")
	rootCert := mustParse(t, rootPEM)
	fp := certFingerprint(rootCert)

	r := newFakeRunner()
	r.setResponse("cat", nil, &fakeErr{msg: "no such file"})

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	trusted, err := installer.isTrustedDebian(context.Background(), fp)
	require.NoError(t, err)
	assert.False(t, trusted)
}

func TestLinux_IsTrustedRHEL_AlreadyTrusted(t *testing.T) {
	t.Parallel()
	rootPEM, _ := testCA(t, "rhel-root")
	rootCert := mustParse(t, rootPEM)
	fp := certFingerprint(rootCert)

	r := newFakeRunner()
	r.setResponse("trust", []byte(rootPEM), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	trusted, err := installer.isTrustedRHEL(context.Background(), fp)
	require.NoError(t, err)
	assert.True(t, trusted)
}

func TestLinux_IsTrustedRHEL_DifferentCert(t *testing.T) {
	t.Parallel()
	rootPEM, _ := testCA(t, "rhel-root-a")
	otherPEM, _ := testCA(t, "rhel-root-b")
	rootCert := mustParse(t, rootPEM)
	fp := certFingerprint(rootCert)

	r := newFakeRunner()
	r.setResponse("trust", []byte(otherPEM), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	trusted, err := installer.isTrustedRHEL(context.Background(), fp)
	require.NoError(t, err)
	assert.False(t, trusted)
}

func TestLinux_EnsureSystemTrust_AlreadyTrusted_Debian(t *testing.T) {
	t.Parallel()
	bundle, rootCert := testBundle(t)
	fp := certFingerprint(rootCert)

	r := newFakeRunner()
	// detectLinuxFamily: update-ca-certificates --help succeeds
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	// isTrustedDebian: cat the managed file returns the root PEM
	r.setResponse("cat", []byte(bundle[:strings.Index(bundle, "-----END CERTIFICATE-----\n")+len("-----END CERTIFICATE-----\n")]), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	result, err := installer.EnsureSystemTrust(context.Background(), []byte(bundle))
	require.NoError(t, err)
	assert.Equal(t, SystemTrustAlreadyTrusted, result.Status)
	assert.Equal(t, fp, result.Fingerprint)
	// No sudo calls should have been made
	for i := 0; i < r.callCount(); i++ {
		c := r.call(i)
		assert.NotEqual(t, "sudo", c.name, "no sudo should be invoked when already trusted")
	}
}

func TestLinux_EnsureSystemTrust_Install_Debian(t *testing.T) {
	t.Parallel()
	bundle, rootCert := testBundle(t)
	fp := certFingerprint(rootCert)

	r := newFakeRunner()
	// detectLinuxFamily: update-ca-certificates --help succeeds
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	// isTrustedDebian: cat fails (not present)
	r.setResponse("cat", nil, &fakeErr{msg: "no such file"})
	// sudo cp succeeds
	r.setResponse("sudo", []byte(""), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	result, err := installer.EnsureSystemTrust(context.Background(), []byte(bundle))
	require.NoError(t, err)
	assert.Equal(t, SystemTrustInstalled, result.Status)
	assert.Equal(t, fp, result.Fingerprint)

	// Verify sudo was called with cp and update-ca-certificates (argument arrays, no shell)
	var sudoCalls []fakeCall
	for i := 0; i < r.callCount(); i++ {
		c := r.call(i)
		if c.name == "sudo" {
			sudoCalls = append(sudoCalls, c)
		}
	}
	require.Len(t, sudoCalls, 2, "expected cp + update-ca-certificates")
	// First sudo call: cp <temp> <managed>
	assert.Equal(t, "cp", sudoCalls[0].args[0])
	assert.Contains(t, sudoCalls[0].args[2], "g8e-root-"+fp+".crt")
	// Second sudo call: update-ca-certificates
	assert.Equal(t, "update-ca-certificates", sudoCalls[1].args[0])
}

func TestLinux_EnsureSystemTrust_Install_RHEL(t *testing.T) {
	t.Parallel()
	bundle, rootCert := testBundle(t)
	fp := certFingerprint(rootCert)

	r := newFakeRunner()
	// detectLinuxFamily: update-ca-certificates fails, trust list succeeds
	r.setResponse("update-ca-certificates", nil, &fakeErr{msg: "not found"})
	r.setResponse("trust", []byte(""), nil)
	// isTrustedRHEL: trust list returns empty (not present)
	// Already set above with empty output — no root match
	// sudo cp + sudo update-ca-trust succeed
	r.setResponse("sudo", []byte(""), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	result, err := installer.EnsureSystemTrust(context.Background(), []byte(bundle))
	require.NoError(t, err)
	assert.Equal(t, SystemTrustInstalled, result.Status)
	assert.Equal(t, fp, result.Fingerprint)

	var sudoCalls []fakeCall
	for i := 0; i < r.callCount(); i++ {
		c := r.call(i)
		if c.name == "sudo" {
			sudoCalls = append(sudoCalls, c)
		}
	}
	require.Len(t, sudoCalls, 2)
	assert.Equal(t, "cp", sudoCalls[0].args[0])
	assert.Contains(t, sudoCalls[0].args[2], "g8e-root-"+fp+".crt")
	assert.Equal(t, "update-ca-trust", sudoCalls[1].args[0])
	assert.Equal(t, "extract", sudoCalls[1].args[1])
}

func TestLinux_EnsureSystemTrust_MissingTool(t *testing.T) {
	t.Parallel()
	bundle, _ := testBundle(t)

	r := newFakeRunner()
	// Neither tool found
	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	_, err := installer.EnsureSystemTrust(context.Background(), []byte(bundle))
	require.Error(t, err)
}

func TestLinux_EnsureSystemTrust_SudoFails(t *testing.T) {
	t.Parallel()
	bundle, _ := testBundle(t)

	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	r.setResponse("cat", nil, &fakeErr{msg: "no such file"})
	r.setResponse("sudo", nil, &fakeErr{msg: "sudo: command not found"})

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	_, err := installer.EnsureSystemTrust(context.Background(), []byte(bundle))
	require.Error(t, err)
}

func TestLinux_EnsureSystemTrust_NoShellInjection(t *testing.T) {
	t.Parallel()
	bundle, _ := testBundle(t)

	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	r.setResponse("cat", nil, &fakeErr{msg: "no such file"})
	r.setResponse("sudo", []byte(""), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	_, err := installer.EnsureSystemTrust(context.Background(), []byte(bundle))
	require.NoError(t, err)

	// Verify no call uses sh -c or shell interpolation
	for i := 0; i < r.callCount(); i++ {
		c := r.call(i)
		assert.NotEqual(t, "sh", c.name, "no shell should be used")
		for _, arg := range c.args {
			assert.NotContains(t, arg, "$(", "no command substitution in args")
			assert.NotContains(t, arg, "`", "no backtick substitution in args")
		}
	}
}

func TestLinux_EnsureSystemTrust_ConcurrentCalls(t *testing.T) {
	t.Parallel()
	bundle, _ := testBundle(t)

	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	r.setResponse("cat", nil, &fakeErr{msg: "no such file"})
	r.setResponse("sudo", []byte(""), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := installer.EnsureSystemTrust(context.Background(), []byte(bundle))
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	// All calls should succeed; the test verifies no panic/deadlock under concurrency
}

// --- Stale anchor enumeration/removal tests ---

func TestLinux_ListStaleAnchors_NoStaleAnchors(t *testing.T) {
	t.Parallel()
	bundle, rootCert := testBundle(t)
	fp := certFingerprint(rootCert)

	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	// ls returns only the current anchor file
	r.setResponse("ls", []byte("g8e-root-"+fp+".crt"), nil)
	// cat returns the current root PEM
	currentRootPEM := bundle[:strings.Index(bundle, "-----END CERTIFICATE-----\n")+len("-----END CERTIFICATE-----\n")]
	r.setResponse("cat", []byte(currentRootPEM), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	stale, err := installer.ListStaleAnchors(context.Background(), fp)
	require.NoError(t, err)
	assert.Empty(t, stale, "current anchor should not be listed as stale")
}

func TestLinux_ListStaleAnchors_FindsStale(t *testing.T) {
	t.Parallel()
	bundle, rootCert := testBundle(t)
	currentFP := certFingerprint(rootCert)

	// Generate a stale CA with a different fingerprint.
	stalePEM, _ := testCA(t, "g8e Root CA")
	staleCert := mustParse(t, stalePEM)
	staleFP := certFingerprint(staleCert)

	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	// ls returns both the current and stale anchor files
	r.setResponse("ls", []byte("g8e-root-"+currentFP+".crt\ng8e-root-"+staleFP+".crt"), nil)
	// cat returns the stale PEM for the stale file, current PEM for the current file.
	// The fakeRunner returns the same response for all "cat" calls, so we need
	// a different approach: use a custom runner that returns different output
	// based on the file path argument.
	r2 := &staleListRunner{
		fakeRunner: *newFakeRunner(),
		currentFP:  currentFP,
		currentPEM: bundle[:strings.Index(bundle, "-----END CERTIFICATE-----\n")+len("-----END CERTIFICATE-----\n")],
		staleFP:    staleFP,
		stalePEM:   stalePEM,
	}
	r2.setResponse("update-ca-certificates", []byte("usage"), nil)

	installer := &SystemTrustInstaller{runner: r2, now: time.Now}
	stale, err := installer.ListStaleAnchors(context.Background(), currentFP)
	require.NoError(t, err)
	require.Len(t, stale, 1)
	assert.Equal(t, staleFP, stale[0].Fingerprint)
	assert.Equal(t, "g8e Root CA", stale[0].CommonName)
	assert.Contains(t, stale[0].Handle, staleFP)
}

func TestLinux_ListStaleAnchors_EmptyDir(t *testing.T) {
	t.Parallel()
	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	r.setResponse("ls", []byte(""), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	stale, err := installer.ListStaleAnchors(context.Background(), "any-fp")
	require.NoError(t, err)
	assert.Empty(t, stale)
}

func TestLinux_ListStaleAnchors_DirNotFound(t *testing.T) {
	t.Parallel()
	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	r.setResponse("ls", nil, &fakeErr{msg: "no such directory"})

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	stale, err := installer.ListStaleAnchors(context.Background(), "any-fp")
	require.NoError(t, err)
	assert.Empty(t, stale, "missing directory should yield no stale anchors")
}

func TestLinux_RemoveStaleAnchors_Debian(t *testing.T) {
	t.Parallel()
	anchors := []StaleAnchor{
		{Fingerprint: "stale-1", CommonName: "g8e Root CA", Handle: "/usr/local/share/ca-certificates/g8e-root-stale-1.crt"},
		{Fingerprint: "stale-2", CommonName: "g8e Root CA", Handle: "/usr/local/share/ca-certificates/g8e-root-stale-2.crt"},
	}

	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	r.setResponse("sudo", []byte(""), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	err := installer.RemoveStaleAnchors(context.Background(), anchors)
	require.NoError(t, err)

	// Verify sudo rm was called for each anchor + update-ca-certificates --fresh
	var sudoCalls []fakeCall
	for i := 0; i < r.callCount(); i++ {
		c := r.call(i)
		if c.name == "sudo" {
			sudoCalls = append(sudoCalls, c)
		}
	}
	require.Len(t, sudoCalls, 3, "expected 2 rm + 1 update-ca-certificates")
	assert.Equal(t, "rm", sudoCalls[0].args[0])
	assert.Equal(t, "-f", sudoCalls[0].args[1])
	assert.Contains(t, sudoCalls[0].args[2], "stale-1")
	assert.Equal(t, "rm", sudoCalls[1].args[0])
	assert.Equal(t, "-f", sudoCalls[1].args[1])
	assert.Contains(t, sudoCalls[1].args[2], "stale-2")
	assert.Equal(t, "update-ca-certificates", sudoCalls[2].args[0])
	assert.Equal(t, "--fresh", sudoCalls[2].args[1])
}

func TestLinux_RemoveStaleAnchors_RHEL(t *testing.T) {
	t.Parallel()
	anchors := []StaleAnchor{
		{Fingerprint: "stale-1", CommonName: "g8e Root CA", Handle: "/etc/pki/ca-trust/source/anchors/g8e-root-stale-1.crt"},
	}

	r := newFakeRunner()
	r.setResponse("update-ca-certificates", nil, &fakeErr{msg: "not found"})
	r.setResponse("trust", []byte(""), nil)
	r.setResponse("sudo", []byte(""), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	err := installer.RemoveStaleAnchors(context.Background(), anchors)
	require.NoError(t, err)

	var sudoCalls []fakeCall
	for i := 0; i < r.callCount(); i++ {
		c := r.call(i)
		if c.name == "sudo" {
			sudoCalls = append(sudoCalls, c)
		}
	}
	require.Len(t, sudoCalls, 2, "expected 1 rm + 1 update-ca-trust")
	assert.Equal(t, "rm", sudoCalls[0].args[0])
	assert.Equal(t, "update-ca-trust", sudoCalls[1].args[0])
	assert.Equal(t, "extract", sudoCalls[1].args[1])
}

func TestLinux_RemoveStaleAnchors_Empty_NoOp(t *testing.T) {
	t.Parallel()
	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	err := installer.RemoveStaleAnchors(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 0, r.callCount(), "no commands should run for empty anchor list")
}

func TestLinux_RemoveStaleAnchors_SudoFails(t *testing.T) {
	t.Parallel()
	anchors := []StaleAnchor{
		{Fingerprint: "stale-1", CommonName: "g8e Root CA", Handle: "/path/stale-1.crt"},
	}

	r := newFakeRunner()
	r.setResponse("update-ca-certificates", []byte("usage"), nil)
	r.setResponse("sudo", nil, &fakeErr{msg: "sudo: command not found"})

	installer := &SystemTrustInstaller{runner: r, now: time.Now}
	err := installer.RemoveStaleAnchors(context.Background(), anchors)
	require.Error(t, err)
}

// staleListRunner is a fakeRunner that returns different cat output depending
// on the file path argument. This lets us simulate multiple g8e-root-*.crt
// files with different PEM contents.
type staleListRunner struct {
	fakeRunner
	currentFP  string
	currentPEM string
	staleFP    string
	stalePEM   string
}

func (r *staleListRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) ([]byte, error) {
	if name == "cat" && len(args) > 0 {
		switch {
		case strings.Contains(args[0], r.staleFP):
			return []byte(r.stalePEM), nil
		case strings.Contains(args[0], r.currentFP):
			return []byte(r.currentPEM), nil
		}
	}
	if name == "ls" {
		return []byte("g8e-root-" + r.currentFP + ".crt\ng8e-root-" + r.staleFP + ".crt"), nil
	}
	return r.fakeRunner.Run(ctx, env, name, args...)
}

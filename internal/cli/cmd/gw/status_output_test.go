// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/cmdtest"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/stretchr/testify/require"
)

func TestGatewayStatusDistinguishesUnknownAndEmptyRegistry(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operators []byte
		want      string
	}{
		{"unavailable", nil, "Operators  unavailable (registry query failed)"},
		{"malformed", []byte("not JSON"), "Operators  unavailable (registry query failed)"},
		{"rejected", []byte(`{"success":false}`), "Operators  unavailable (registry query failed)"},
		{"empty", []byte(`{"success":true,"operators":[]}`), "Operators  none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
			mock := &statusMockClient{responses: map[string][]byte{"/api/v1/health": []byte(`{"status":"ok"}`)}}
			if tc.operators != nil {
				mock.responses[constants.APIPaths.Operators] = tc.operators
			}
			factory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) { return mock, nil }
			cmd := gatewayStatusCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), factory, cmdtest.FileSvcFactoryFor(fileSvc))
			cmd.SetArgs(nil)
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			require.NoError(t, cmd.Execute())
			require.Contains(t, buf.String(), "Gateway  online")
			require.Contains(t, buf.String(), tc.want)
			require.NotContains(t, buf.String(), "Endpoints:")
		})
	}
}

func TestGatewayStatusOnlyShowsConnectedOperators(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{
		{OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference}, CurrentHostname: "gpu-host", Status: constants.OperatorStatusActive},
		{CurrentHostname: "stopped-host", Status: constants.OperatorStatusStopped},
		{CurrentHostname: "unclaimed-host", Status: constants.OperatorStatusActive, IsSlot: true},
	}})
	require.NoError(t, err)
	mock := &statusMockClient{responses: map[string][]byte{"/api/v1/health": []byte(`{"status":"ok"}`), constants.APIPaths.Operators: body}}
	factory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) { return mock, nil }
	cmd, buf := newOutputCmd()
	require.NoError(t, printGatewayStatus(cmd, cfg, factory, cmdtest.FileSvcFactoryFor(fileSvc)))
	require.Contains(t, buf.String(), "CAPABILITIES")
	require.Contains(t, buf.String(), "inference")
	require.Contains(t, buf.String(), "gpu-host")
	require.NotContains(t, buf.String(), "stopped-host")
	require.NotContains(t, buf.String(), "unclaimed-host")
}

func TestQuietStartKeepsRunningGatewayAndSuppressesGuidance(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	pidPath := filepath.Join(constants.PidDirname, constants.OperatorPIDFilename)
	pid := []byte(strconv.Itoa(os.Getpid()))
	require.NoError(t, fileSvc.WriteFile(context.Background(), pidPath, pid, constants.PermFilePrivate))
	cmd := gatewayStartCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), cmdtest.FileSvcFactoryFor(fileSvc), nil)
	cmd.SetArgs([]string{"--quiet", "--cert-mode", "localhost"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	require.NoError(t, cmd.Execute())
	require.Empty(t, buf.String())
	saved, err := fileSvc.ReadFile(context.Background(), pidPath)
	require.NoError(t, err)
	require.Equal(t, pid, saved)
	cmd.Println("output restored")
	require.Contains(t, buf.String(), "output restored")
}

func TestStatusShowsLocalEmbeddedCapabilitiesWithoutCLIEnrollment(t *testing.T) {
	for _, roles := range []constants.OperatorRoles{
		{constants.OperatorRoleEmbedded},
		{constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver},
	} {
		t.Run(roles.String(), func(t *testing.T) {
			fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
			require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PidDirname, constants.OperatorPIDFilename), []byte(strconv.Itoa(os.Getpid())), constants.PermFilePrivate))
			require.NoError(t, serve.WriteLaunchProfile(fileSvc, serve.GatewayConfig{OperatorRoles: roles, Posture: "doctrine", LogLevel: constants.LogLevelInfo}))
			expected := append(roles, constants.OperatorRoleEmbedded).String()
			var full bytes.Buffer
			printOperatorTables(&full, collectOperatorInventory(nil, fileSvc, cfg))
			require.Contains(t, full.String(), string(constants.DocIDEmbeddedOperator))
			require.Contains(t, full.String(), expected)
			require.Contains(t, full.String(), "Remote Operators  unavailable (CLI not enrolled)")
			require.NotContains(t, full.String(), "No connected operators")
			factory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) {
				return nil, constants.ErrNotFound
			}
			cmd, status := newOutputCmd()
			require.NoError(t, printGatewayStatus(cmd, cfg, factory, cmdtest.FileSvcFactoryFor(fileSvc)))
			require.Contains(t, status.String(), string(constants.DocIDEmbeddedOperator))
			require.Contains(t, status.String(), expected)
		})
	}
}

func TestLocalEmbeddedOperatorOmitsStoppedRuntimeAndMarksInvalidProfileUnknown(t *testing.T) {
	for _, tc := range []struct {
		name    string
		running bool
		valid   bool
		want    bool
	}{
		{"stopped", false, true, false}, {"missing profile", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
			if tc.running {
				require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PidDirname, constants.OperatorPIDFilename), []byte(strconv.Itoa(os.Getpid())), constants.PermFilePrivate))
			}
			if tc.valid {
				require.NoError(t, serve.WriteLaunchProfile(fileSvc, serve.GatewayConfig{OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference}, Posture: "doctrine", LogLevel: constants.LogLevelInfo}))
			}
			var buf bytes.Buffer
			inv := collectOperatorInventory(nil, fileSvc, cfg)
			require.Equal(t, tc.want, len(inv.Operators) == 1)
			printOperatorTables(&buf, inv)
			require.NotContains(t, buf.String(), "inference")
			if tc.want {
				require.Equal(t, []string{string(constants.DocIDEmbeddedOperator), "embedded", "running", "local", "-", "unknown", filepath.Dir(fileSvc.Resolve(""))}, tableRow(t, buf.String(), string(constants.DocIDEmbeddedOperator)))
			}
		})
	}
}

func TestStatusDoesNotDuplicateEnrolledEmbeddedOperator(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PidDirname, constants.OperatorPIDFilename), []byte(strconv.Itoa(os.Getpid())), constants.PermFilePrivate))
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, serve.GatewayConfig{OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference}, Posture: "doctrine", LogLevel: constants.LogLevelInfo}))
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{
		{ID: string(constants.DocIDEmbeddedOperator), OperatorType: constants.OperatorTypeEmbedded, Status: constants.OperatorStatusActive, OperatorRoles: constants.OperatorRoles{constants.OperatorRoleInference}},
	}})
	require.NoError(t, err)
	mock := &statusMockClient{responses: map[string][]byte{constants.APIPaths.Operators: body}}
	var buf bytes.Buffer
	printOperatorTables(&buf, collectOperatorInventory(mock, fileSvc, cfg))
	require.Contains(t, buf.String(), "embedded,data,inference")
	require.Equal(t, 1, operatorTableRows(buf.String(), string(constants.DocIDEmbeddedOperator)))
	require.NotContains(t, buf.String(), "launch profile")
}

func TestGatewayStatusShowsLocalOperatorTablesWithoutEnrollment(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PidDirname, constants.OperatorPIDFilename), []byte(strconv.Itoa(os.Getpid())), constants.PermFilePrivate))
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, serve.GatewayConfig{OperatorRoles: constants.OperatorRoles{constants.OperatorRoleData, constants.OperatorRoleInference, constants.OperatorRoleProvenance, constants.OperatorRoleObserver}, Posture: "doctrine", LogLevel: constants.LogLevelInfo}))
	factory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) {
		return nil, constants.ErrNotFound
	}
	cmd := gatewayStatusCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), factory, cmdtest.FileSvcFactoryFor(fileSvc))
	cmd.SetArgs(nil)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	require.NoError(t, cmd.Execute())
	require.Contains(t, buf.String(), "embedded-operator")
	require.Contains(t, buf.String(), "embedded,data,inference,provenance,observer")
	require.Contains(t, buf.String(), "CLI not enrolled")
	require.Contains(t, buf.String(), "Remote Operators  unavailable (CLI not enrolled)")
	require.Contains(t, buf.String(), "Enrollments  unavailable (CLI not enrolled)")
	require.Equal(t, 1, strings.Count(buf.String(), "Enrollments"))
	require.NotContains(t, buf.String(), "launch profile")
	require.NotContains(t, buf.String(), "check enrollment or Gateway logs")
}

func TestGatewayStatusHasOneOutputFormat(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	factory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) {
		return nil, constants.ErrNotFound
	}
	cmd := gatewayStatusCmdWithConfig(cmdtest.ConfigLoaderFor(cfg), factory, cmdtest.FileSvcFactoryFor(fileSvc))
	require.Nil(t, cmd.Flags().Lookup("brief"))
	require.Nil(t, cmd.Flags().Lookup("details"))
}

// tableRow returns the whitespace-separated cells of the first row of out that
// starts with id. Test values contain no spaces.
func tableRow(t *testing.T, out, id string) []string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == id {
			return fields
		}
	}
	t.Fatalf("no row for %q in:\n%s", id, out)
	return nil
}

// operatorTableRows counts the Operators-table rows for id, excluding the
// Operator flags table.
func operatorTableRows(out, id string) int {
	table, _, _ := strings.Cut(out, "Operator flags")
	count := 0
	for _, line := range strings.Split(table, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == id {
			count++
		}
	}
	return count
}

func lineStartingWith(out, prefix string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

// flagRowsFor returns the (capability, flag, value) rows listed for id.
func flagRowsFor(out, id string) [][]string {
	var rows [][]string
	inFlags := false
	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "Operator flags":
			inFlags = true
		case inFlags && strings.HasPrefix(strings.TrimSpace(line), id+" "):
			rows = append(rows, strings.Fields(line)[1:])
		}
	}
	return rows
}

func TestGatewayStatusListsEveryOperatorWithTypeCapabilitiesAndStartValues(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, serve.GatewayConfig{
		OperatorRoles:           constants.OperatorRoles{constants.OperatorRoleInference, constants.OperatorRoleProvenance},
		InferenceOllamaEndpoint: "http://192.168.1.2:11434",
		InferenceKeepAlive:      "30m",
		Posture:                 "doctrine",
		LogLevel:                constants.LogLevelInfo,
	}))
	body, err := json.Marshal(models.OperatorSlotResponse{Success: true, Operators: []models.OperatorDocumentGo{
		{
			ID: "op-prov", OperatorType: constants.OperatorTypeRemote, Status: constants.OperatorStatusActive, CurrentHostname: "store-host",
			RuntimeConfig: &models.RuntimeConfig{
				Roles: constants.OperatorRoles{constants.OperatorRoleProvenance}, ProvenanceOperatorEnabled: true,
				ProvenanceOperatorModelStorageRoot: "/srv/models", LocalDir: "/srv/g8e/provenance", HTTPPort: 8080,
				// Reported by every Operator, but only meaningful with the inference role.
				InferenceOllamaEndpoint: "http://ignored:11434",
			},
		},
		{
			ID: "op-inf", OperatorType: constants.OperatorTypeRemote, Status: constants.OperatorStatusStale, CurrentHostname: "gpu-host",
			RuntimeConfig: &models.RuntimeConfig{
				Roles: constants.OperatorRoles{constants.OperatorRoleInference}, InferenceEnabled: true,
				InferenceOllamaEndpoint: "http://gpu-host:11434", LocalDir: "/srv/g8e/inference", HTTPPort: 8080,
				LogLevel: "debug", NoGit: true,
			},
		},
		{
			ID: string(constants.DocIDEmbeddedOperator), OperatorType: constants.OperatorTypeEmbedded, Status: constants.OperatorStatusActive,
			RuntimeConfig: &models.RuntimeConfig{
				Roles: constants.OperatorRoles{constants.OperatorRoleInference, constants.OperatorRoleProvenance}, InferenceEnabled: true,
				InferenceOllamaEndpoint: "http://192.168.1.2:11434", ProvenanceOperatorEnabled: true,
				ProvenanceOperatorModelStorageRoot: "/mnt/models", LocalDir: "/home/me/g8e", HTTPPort: 8081,
			},
		},
	}})
	require.NoError(t, err)
	mock := &statusMockClient{responses: map[string][]byte{"/api/v1/health": []byte(`{"status":"ok"}`), constants.APIPaths.Operators: body}}
	factory := func(fs.RuntimeFileService, *config.Config) (authcmd.APIClient, error) { return mock, nil }
	cmd, buf := newOutputCmd()
	require.NoError(t, printGatewayStatus(cmd, cfg, factory, cmdtest.FileSvcFactoryFor(fileSvc)))
	out := buf.String()

	require.Contains(t, out, "\n  OPERATOR ID ")
	require.Equal(t, []string{"OPERATOR", "ID", "TYPE", "STATUS", "HOST", "HTTP", "PORT", "CAPABILITIES", "DIRECTORY"}, strings.Fields(lineStartingWith(out, "  OPERATOR ID")))
	require.Equal(t, []string{"embedded-operator", "embedded", "active", "local", "8081", "embedded,data,inference,provenance", "/home/me/g8e"}, tableRow(t, out, "embedded-operator"))
	require.Equal(t, []string{"op-inf", "remote", "stale", "gpu-host", "8080", "inference", "/srv/g8e/inference"}, tableRow(t, out, "op-inf"))
	require.Equal(t, []string{"op-prov", "remote", "active", "store-host", "8080", "provenance", "/srv/g8e/provenance"}, tableRow(t, out, "op-prov"))
	require.Less(t, strings.Index(out, "embedded-operator"), strings.Index(out, "op-inf"), "embedded Operator is listed first, then by ID")

	// The embedded Operator's own launch profile adds values the registry lacks.
	require.Equal(t, [][]string{
		{"inference", "--inference-ollama-endpoint", "http://192.168.1.2:11434"},
		{"inference", "--inference-keep-alive", "30m"},
		{"provenance", "--model-storage-root", "/mnt/models"},
		{"provenance", "--provenance-operator-id", "(default)"},
	}, flagRowsFor(out, "embedded-operator"))
	// A remote Operator lists only what it reported: common flags only when customised,
	// capability flags only for held capabilities, and nothing it never sent the Gateway.
	require.Equal(t, [][]string{
		{"all", "--log", "debug"},
		{"all", "--no-git", "true"},
		{"inference", "--inference-ollama-endpoint", "http://gpu-host:11434"},
	}, flagRowsFor(out, "op-inf"))
	require.Equal(t, [][]string{
		{"provenance", "--model-storage-root", "/srv/models"},
	}, flagRowsFor(out, "op-prov"))
	require.NotContains(t, out, "http://ignored:11434")
	require.NotContains(t, out, "Remote Operators  unavailable")
}

func TestGatewayStatusLocalFlagsComeFromLaunchProfileWithoutEnrollment(t *testing.T) {
	fileSvc, cfg := cmdtest.NewCmdTestEnv(t)
	require.NoError(t, fileSvc.WriteFile(context.Background(), filepath.Join(constants.PidDirname, constants.OperatorPIDFilename), []byte(strconv.Itoa(os.Getpid())), constants.PermFilePrivate))
	require.NoError(t, serve.WriteLaunchProfile(fileSvc, serve.GatewayConfig{
		OperatorRoles:                      constants.OperatorRoles{constants.OperatorRoleData, constants.OperatorRoleProvenance, constants.OperatorRoleObserver},
		ProvenanceOperatorModelStorageRoot: "/mnt/d/ai/Ollama/models",
		ProviderBoundaryObserverID:         "g8e-observer",
		HTTPPort:                           8080,
		Posture:                            "doctrine",
		LogLevel:                           constants.LogLevelInfo,
	}))
	var buf bytes.Buffer
	printOperatorTables(&buf, collectOperatorInventory(nil, fileSvc, cfg))
	require.Equal(t, []string{"embedded-operator", "embedded", "running", "local", "8080", "embedded,data,provenance,observer", filepath.Dir(fileSvc.Resolve(""))}, tableRow(t, buf.String(), "embedded-operator"))
	require.Equal(t, [][]string{
		{"provenance", "--model-storage-root", "/mnt/d/ai/Ollama/models"},
		{"provenance", "--provenance-operator-id", "(default)"},
		{"observer", "--provider-boundary-observer-id", "g8e-observer"},
	}, flagRowsFor(buf.String(), "embedded-operator"))
}

func TestGatewayStatusOmitsFlagsTableWhenNoOperatorHasFlags(t *testing.T) {
	var buf bytes.Buffer
	printOperatorTables(&buf, operatorInventory{Operators: []operatorStatus{{ID: "op-data", Type: "remote", Status: "active", Host: "h", Roles: constants.OperatorRoles{constants.OperatorRoleData}}}})
	require.Contains(t, buf.String(), "op-data")
	require.NotContains(t, buf.String(), "Operator flags")
}

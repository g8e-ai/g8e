// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gw

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	clioperator "github.com/g8e-ai/g8e/v2/internal/cli/operator"
	"github.com/g8e-ai/g8e/v2/internal/cli/platform"
	"github.com/g8e-ai/g8e/v2/internal/cli/serve"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

// Start flags `gw status` can attribute to an Operator. The Gateway only knows
// the values an Operator reported at registration (models.RuntimeConfig) plus,
// for the embedded Operator, the Gateway's own launch profile.
const (
	flagLog                = "--log"
	flagCloud              = "--cloud"
	flagProvider           = "--provider"
	flagNoGit              = "--no-git"
	flagInferenceEndpoint  = "--inference-ollama-endpoint"
	flagInferenceKeepAlive = "--inference-keep-alive"
	flagModelStorageRoot   = "--model-storage-root"
	flagProvenanceID       = "--provenance-operator-id"
	flagObserverID         = "--provider-boundary-observer-id"

	flagScopeAll   = "all"
	flagUnsetValue = "(default)"
	cellUnknown    = "-"
)

// capabilityFlags lists, per capability, the start flags that tune it. They are
// shown for every Operator holding the capability and in this order.
var capabilityFlags = []struct {
	role  constants.OperatorRole
	flags []string
}{
	{constants.OperatorRoleInference, []string{flagInferenceEndpoint, flagInferenceKeepAlive}},
	{constants.OperatorRoleProvenance, []string{flagModelStorageRoot, flagProvenanceID}},
	{constants.OperatorRoleObserver, []string{flagObserverID}},
}

// operatorStatus is one Operator row of `gw status`. Registry documents and the
// local launch profile both map onto it so every Operator renders identically.
type operatorStatus struct {
	ID     string
	Type   string
	Status string
	Host   string
	Dir    string
	Port   int
	Roles  constants.OperatorRoles

	// reported maps a start flag to its value. A missing key means the source
	// did not report the flag; an empty value means it was reported unset.
	reported map[string]string
}

type operatorInventory struct {
	Operators []operatorStatus
	// Unavailable is why the Gateway registry could not be listed ("" when it
	// could). Remote Operators are only known through the registry.
	Unavailable string
}

func operatorStatusFromDocument(op models.OperatorDocumentGo) operatorStatus {
	row := operatorStatus{
		ID:       op.ID,
		Type:     operatorTypeDisplay(op.OperatorType),
		Status:   string(op.Status),
		Host:     operatorHostnameDisplay(op),
		Dir:      op.LocalDir,
		Port:     op.Port,
		Roles:    operatorcapability.GetOperatorRoles(op),
		reported: map[string]string{},
	}
	rc := op.RuntimeConfig
	if rc == nil {
		return row
	}
	if row.Dir == "" {
		row.Dir = rc.LocalDir
	}
	if row.Port == 0 {
		row.Port = rc.HTTPPort
	}
	row.reportCustom(rc.LogLevel, rc.CloudMode, rc.CloudProvider, rc.NoGit)
	row.reported[flagInferenceEndpoint] = rc.InferenceOllamaEndpoint
	row.reported[flagModelStorageRoot] = rc.ProvenanceOperatorModelStorageRoot
	return row
}

// reportCustom records only the common flags that differ from their defaults.
func (o *operatorStatus) reportCustom(logLevel string, cloud bool, provider string, noGit bool) {
	if logLevel != "" && logLevel != constants.LogLevelInfo {
		o.reported[flagLog] = logLevel
	}
	if cloud {
		o.reported[flagCloud] = "true"
		o.reported[flagProvider] = provider
	}
	if noGit {
		o.reported[flagNoGit] = "true"
	}
}

// fillFromProfile adds the values the Gateway launched its embedded Operator
// with. Values the registry already reported are kept.
func (o *operatorStatus) fillFromProfile(cfg serve.GatewayConfig) {
	if o.Dir == "" {
		o.Dir = cfg.WorkingDir
	}
	if o.Port == 0 {
		o.Port = cfg.HTTPPort
	}
	if _, ok := o.reported[flagLog]; !ok {
		o.reportCustom(cfg.LogLevel, false, "", false)
	}
	for flag, value := range map[string]string{
		flagInferenceEndpoint:  cfg.InferenceOllamaEndpoint,
		flagInferenceKeepAlive: cfg.InferenceKeepAlive,
		flagModelStorageRoot:   cfg.ProvenanceOperatorModelStorageRoot,
		flagProvenanceID:       cfg.ProvenanceOperatorID,
		flagObserverID:         cfg.ProviderBoundaryObserverID,
	} {
		if _, ok := o.reported[flag]; !ok {
			o.reported[flag] = value
		}
	}
}

func operatorTypeDisplay(t constants.OperatorType) string {
	if t == "" {
		return cellUnknown
	}
	return string(t)
}

// flagRows returns the (capability, flag, value) rows for this Operator: common
// flags that were customised, then each held capability's flags.
func (o *operatorStatus) flagRows() [][3]string {
	var rows [][3]string
	for _, flag := range []string{flagLog, flagCloud, flagProvider, flagNoGit} {
		if value, ok := o.reported[flag]; ok {
			rows = append(rows, [3]string{flagScopeAll, flag, flagValueDisplay(value)})
		}
	}
	for _, capability := range capabilityFlags {
		if !o.Roles.Has(capability.role) {
			continue
		}
		for _, flag := range capability.flags {
			if value, ok := o.reported[flag]; ok {
				rows = append(rows, [3]string{string(capability.role), flag, flagValueDisplay(value)})
			}
		}
	}
	return rows
}

func flagValueDisplay(value string) string {
	if value == "" {
		return flagUnsetValue
	}
	return value
}

// collectOperatorInventory lists every connected Operator as one row each. The
// registry is authoritative; the local embedded Operator is added from the
// launch profile when the registry cannot list it (for example before the CLI
// is enrolled). This is local process status, not a fabricated enrollment,
// owner binding, or Operator session.
func collectOperatorInventory(client authcmd.APIClient, fileSvc fs.RuntimeFileService, cfg *config.Config) operatorInventory {
	var inv operatorInventory
	profile, profileErr := serve.ReadLaunchProfile(fileSvc)
	hasEmbedded := false
	if client == nil {
		inv.Unavailable = "CLI not enrolled"
	} else {
		reqPath := constants.APIPaths.Operators
		if creds, _ := auth.LoadCredentials(fileSvc, cfg); creds != nil && creds.UserID != "" {
			reqPath += "?user_id=" + creds.UserID
		}
		resp, err := client.Get(reqPath)
		var response models.OperatorSlotResponse
		if err != nil || json.Unmarshal(resp, &response) != nil || !response.Success {
			inv.Unavailable = "registry query failed"
		} else {
			for _, op := range response.Operators {
				if !clioperator.IsConnected(op) {
					continue
				}
				row := operatorStatusFromDocument(op)
				if op.OperatorType == constants.OperatorTypeEmbedded {
					hasEmbedded = true
					if profileErr == nil {
						row.fillFromProfile(profile.Config)
					}
				}
				inv.Operators = append(inv.Operators, row)
			}
		}
	}
	if !hasEmbedded {
		if row, ok := localEmbeddedOperator(fileSvc, profile, profileErr); ok {
			inv.Operators = append(inv.Operators, row)
		}
	}
	sort.SliceStable(inv.Operators, func(i, j int) bool {
		a, b := inv.Operators[i], inv.Operators[j]
		if (a.Type == string(constants.OperatorTypeEmbedded)) != (b.Type == string(constants.OperatorTypeEmbedded)) {
			return a.Type == string(constants.OperatorTypeEmbedded)
		}
		return a.ID < b.ID
	})
	return inv
}

// localEmbeddedOperator describes the Gateway's in-process Operator from the
// managed process and its validated launch profile. A running process with an
// unreadable profile still has an Operator; its capabilities are unknown.
func localEmbeddedOperator(fileSvc fs.RuntimeFileService, profile serve.GatewayLaunchProfile, profileErr error) (operatorStatus, bool) {
	exists, err := fileSvc.FileExists(context.Background(), filepath.Join(constants.PidDirname, constants.OperatorPIDFilename))
	if err != nil || !exists {
		return operatorStatus{}, false
	}
	pm, err := platform.NewProcessManager(fileSvc)
	if err != nil {
		return operatorStatus{}, false
	}
	running, _, err := pm.OperatorStatus()
	if err != nil || !running {
		return operatorStatus{}, false
	}
	row := operatorStatus{
		ID:       string(constants.DocIDEmbeddedOperator),
		Type:     string(constants.OperatorTypeEmbedded),
		Status:   "running",
		Host:     "local",
		reported: map[string]string{},
	}
	if profileErr == nil {
		row.Roles = append(append(constants.OperatorRoles{}, profile.Config.OperatorRoles...), constants.OperatorRoleEmbedded).Effective()
		row.fillFromProfile(profile.Config)
	}
	// The launch profile does not record the Gateway's working directory; its
	// runtime state lives in <working directory>/.g8e.
	if runtimeDir := fileSvc.Resolve(""); row.Dir == "" && filepath.Base(runtimeDir) == constants.RuntimeDirname {
		row.Dir = filepath.Dir(runtimeDir)
	}
	return row, true
}

// printOperatorTables renders the Operators table (one row per Operator ID) and
// the Operator flags table (one row per start flag value).
func printOperatorTables(w io.Writer, inv operatorInventory) {
	if len(inv.Operators) == 0 {
		if inv.Unavailable != "" {
			fmt.Fprintf(w, "Operators  unavailable (%s)\n", inv.Unavailable)
		} else {
			fmt.Fprintln(w, "Operators  none")
		}
		return
	}
	fmt.Fprintln(w, "Operators")
	table := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(table, "  OPERATOR ID\tTYPE\tSTATUS\tHOST\tHTTP PORT\tCAPABILITIES\tDIRECTORY")
	for _, op := range inv.Operators {
		capabilities := op.Roles.String()
		if len(op.Roles) == 0 {
			capabilities = "unknown"
		}
		fmt.Fprintf(table, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			op.ID, op.Type, op.Status, op.Host, portDisplay(op.Port), capabilities, valueOrUnknown(op.Dir))
	}
	_ = table.Flush()

	flags := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	count := 0
	for _, op := range inv.Operators {
		for _, row := range op.flagRows() {
			if count == 0 {
				fmt.Fprintln(w)
				fmt.Fprintln(w, "Operator flags")
				fmt.Fprintln(flags, "  OPERATOR ID\tCAPABILITY\tFLAG\tVALUE")
			}
			count++
			fmt.Fprintf(flags, "  %s\t%s\t%s\t%s\n", op.ID, row[0], row[1], row[2])
		}
	}
	_ = flags.Flush()
	if inv.Unavailable != "" {
		fmt.Fprintf(w, "\nRemote Operators  unavailable (%s)\n", inv.Unavailable)
	}
}

func portDisplay(port int) string {
	if port <= 0 {
		return cellUnknown
	}
	return strconv.Itoa(port)
}

func valueOrUnknown(value string) string {
	if value == "" {
		return cellUnknown
	}
	return value
}

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package operatorcmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/g8e-ai/g8e/v2/internal/cli/cmd/shared"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/cli/auth"
	authcmd "github.com/g8e-ai/g8e/v2/internal/cli/cmd/auth"
	"github.com/g8e-ai/g8e/v2/internal/cli/config"
	"github.com/g8e-ai/g8e/v2/internal/cli/output"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/fs"
	"github.com/g8e-ai/g8e/v2/internal/services/operatorcapability"
)

type operatorBindClient interface {
	Bind(ctx context.Context, fileSvc fs.RuntimeFileService, operatorSessionIDs []string) (auth.CLISessionBind, error)
	Unbind(ctx context.Context, fileSvc fs.RuntimeFileService) (auth.CLISessionUnbind, error)
	SessionInfo(ctx context.Context, fileSvc fs.RuntimeFileService) (auth.CLISessionInfo, error)
}

type operatorBindClientFactory func(cfg *config.Config) operatorBindClient

type operatorBindOutput struct {
	Success           bool                      `json:"success"`
	CLISessionID      string                    `json:"cli_session_id"`
	UserID            string                    `json:"user_id"`
	OperatorID        string                    `json:"operator_id"`
	OperatorSessionID string                    `json:"operator_session_id"`
	AlreadyBound      bool                      `json:"already_bound"`
	BoundCount        int                       `json:"bound_count"`
	Bound             []models.CLIBoundOperator `json:"bound"`
}

type operatorBindingEntry struct {
	CLISessionID      string                   `json:"cli_session_id"`
	OperatorID        string                   `json:"operator_id"`
	OperatorSessionID string                   `json:"operator_session_id"`
	OperatorType      constants.OperatorType   `json:"operator_type"`
	OperatorRoles     constants.OperatorRoles  `json:"operator_roles"`
	Status            constants.OperatorStatus `json:"status"`
	Hostname          string                   `json:"hostname,omitempty"`
	Name              string                   `json:"name,omitempty"`
}

type operatorBindListOutput struct {
	CLISessionID string                 `json:"cli_session_id"`
	UserID       string                 `json:"user_id"`
	Bindings     []operatorBindingEntry `json:"bindings"`
}

type operatorUnbindOutput struct {
	Success        bool   `json:"success"`
	CLISessionID   string `json:"cli_session_id"`
	UserID         string `json:"user_id"`
	AlreadyUnbound bool   `json:"already_unbound"`
}

func defaultOperatorBindClientFactory(cfg *config.Config) operatorBindClient {
	return auth.NewEnrollmentClient(cfg, nil)
}

func operatorBindCmd() *cobra.Command {
	return operatorBindCmdWithConfig(shared.LoadConfig, authcmd.DefaultAPIClientFactory, defaultOperatorBindClientFactory, shared.NewFileSvc)
}

func operatorBindCmdWithConfig(
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	bindClientFactory operatorBindClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "bind [operator-session-id...|list|unbind]",
		Short: "Manage CLI session operator bindings",
		Long: `Manage the authenticated CLI session's operator binding.

  bind <operator-session-id>...   Bind the CLI session to one or more operator sessions in a single call
  bind list                       Show operators bound to the current CLI session
  bind unbind                     Clear the operator binding from the CLI session

Every operator session is validated before any binding changes; if one is
rejected, nothing is bound. The first session is the primary binding. Binding
changes issue a replacement CLI session server-side and update local
credentials. Use './g8e operator list' to discover operator session IDs.
Confirmation can be skipped with --yes for non-interactive automation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if len(args) == 1 {
				switch args[0] {
				case "list":
					return runOperatorBindList(cmd, configLoader, clientFactory, bindClientFactory, fileSvcFactory)
				case "unbind":
					return runOperatorBindUnbind(cmd, yes, configLoader, bindClientFactory, fileSvcFactory)
				}
			}
			return runOperatorBind(cmd, yes, args, configLoader, clientFactory, bindClientFactory, fileSvcFactory)
		},
	}

	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation prompt (non-interactive automation).")
	return cmd
}

func runOperatorBind(
	cmd *cobra.Command,
	yes bool,
	operatorSessionIDs []string,
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	bindClientFactory operatorBindClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) error {
	targets := make([]string, 0, len(operatorSessionIDs))
	seen := make(map[string]struct{}, len(operatorSessionIDs))
	for _, id := range operatorSessionIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return fmt.Errorf("%w: operator session id is required", constants.ErrGatewayOperatorSessionIDRequired)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return fmt.Errorf("%w: operator session id is required", constants.ErrGatewayOperatorSessionIDRequired)
	}

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

	client, err := clientFactory(fileSvc, cfg)
	if err != nil {
		return fmt.Errorf("operator bind: create API client: %w", err)
	}

	resp, err := client.Get(constants.APIPaths.Operators + "?user_id=" + creds.UserID)
	if err != nil {
		return fmt.Errorf("operator bind: list operators: %w", err)
	}
	var slotResp models.OperatorSlotResponse
	if err := json.Unmarshal(resp, &slotResp); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
	}

	resolved := make([]*models.OperatorDocumentGo, 0, len(targets))
	for _, id := range targets {
		target := findOperatorBySessionID(slotResp.Operators, id)
		if target == nil {
			return fmt.Errorf("operator bind: no operator found with session id %s for user %s", id, creds.UserID)
		}
		resolved = append(resolved, target)
	}

	if !output.JSONEnabled(cmd) {
		printOperatorBindTargets(cmd, resolved, creds)
	}

	if !yes && !output.JSONEnabled(cmd) {
		reader := bufio.NewReader(os.Stdin)
		if len(targets) == 1 {
			fmt.Printf("\nBind CLI session to operator session %s? (y/N): ", targets[0])
		} else {
			fmt.Printf("\nBind CLI session to %d operator sessions? (y/N): ", len(targets))
		}
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			cmd.Println("Aborted.")
			return nil
		}
	}

	bind, err := bindClientFactory(cfg).Bind(cmd.Context(), fileSvc, targets)
	if err != nil {
		return fmt.Errorf("operator bind: %w", err)
	}

	creds.CLISessionID = bind.CLISessionID
	creds.OperatorSessionID = bind.OperatorSessionID
	creds.OperatorID = bind.OperatorID
	if err := auth.SaveCredentials(fileSvc, cfg, creds); err != nil {
		return fmt.Errorf("operator bind: save credentials: %w", err)
	}

	if output.JSONEnabled(cmd) {
		return output.WriteJSON(cmd.OutOrStdout(), operatorBindOutput{
			Success:           true,
			CLISessionID:      bind.CLISessionID,
			UserID:            bind.UserID,
			OperatorID:        bind.OperatorID,
			OperatorSessionID: bind.OperatorSessionID,
			AlreadyBound:      bind.AlreadyBound,
			BoundCount:        len(bind.Bound),
			Bound:             bind.Bound,
		})
	}

	if bind.AlreadyBound {
		cmd.Printf("CLI session already bound to %d operator session(s); primary %s\n", len(bind.Bound), bind.OperatorSessionID)
	} else {
		cmd.Printf("CLI session bound to %d operator session(s); primary %s\n", len(bind.Bound), bind.OperatorSessionID)
		cmd.Printf("New CLI session ID: %s\n", bind.CLISessionID)
	}
	cmd.Printf("Operator ID: %s\n", bind.OperatorID)
	return nil
}

// operatorBindDetailLimit is the largest target list printed operator by
// operator; bigger binds print a summary instead of thousands of lines.
const operatorBindDetailLimit = 10

func printOperatorBindTargets(cmd *cobra.Command, targets []*models.OperatorDocumentGo, creds *auth.Credentials) {
	if len(targets) == 1 {
		target := targets[0]
		cmd.Printf("Operator bind target\n")
		cmd.Println(strings.Repeat("=", 72))
		cmd.Printf("  Operator ID:         %s\n", target.ID)
		cmd.Printf("  Operator session ID: %s\n", target.OperatorSessionID)
		cmd.Printf("  Type:                %s\n", target.OperatorType)
		cmd.Printf("  Status:              %s\n", target.Status)
		if target.Name != "" {
			cmd.Printf("  Name:                %s\n", target.Name)
		}
		cmd.Printf("  Current CLI session: %s\n", creds.CLISessionID)
		if creds.OperatorSessionID == target.OperatorSessionID {
			cmd.Printf("  Current binding:     already bound to this operator session\n")
		} else if creds.OperatorSessionID != "" {
			cmd.Printf("  Current binding:     %s\n", creds.OperatorSessionID)
		}
		return
	}

	cmd.Printf("Operator bind targets (%d)\n", len(targets))
	cmd.Println(strings.Repeat("=", 72))
	cmd.Printf("  Primary (first):     %s\n", targets[0].OperatorSessionID)
	cmd.Printf("  Current CLI session: %s\n", creds.CLISessionID)
	if len(targets) > operatorBindDetailLimit {
		return
	}
	for _, target := range targets {
		cmd.Printf("  %-36s  %-12s  %s\n", target.OperatorSessionID, target.OperatorType, target.Status)
	}
}

func runOperatorBindList(
	cmd *cobra.Command,
	configLoader func(string) (*config.Config, error),
	clientFactory authcmd.APIClientFactory,
	bindClientFactory operatorBindClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) error {
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

	sessionInfo, err := bindClientFactory(cfg).SessionInfo(cmd.Context(), fileSvc)
	if err != nil {
		return fmt.Errorf("operator bind list: %w", err)
	}

	boundSessionIDs := sessionInfo.BoundOperatorSessionIDs
	if len(boundSessionIDs) == 0 && sessionInfo.OperatorSessionID != "" {
		boundSessionIDs = []string{sessionInfo.OperatorSessionID}
	}

	var operators []models.OperatorDocumentGo
	if len(boundSessionIDs) > 0 {
		client, err := clientFactory(fileSvc, cfg)
		if err != nil {
			return fmt.Errorf("operator bind list: create API client: %w", err)
		}

		resp, err := client.Get(constants.APIPaths.Operators + "?user_id=" + creds.UserID)
		if err != nil {
			return fmt.Errorf("operator bind list: list operators: %w", err)
		}
		var slotResp models.OperatorSlotResponse
		if err := json.Unmarshal(resp, &slotResp); err != nil {
			return fmt.Errorf("%w: %w", constants.ErrInvalidJSONResponse, err)
		}

		for _, sessionID := range boundSessionIDs {
			if target := findOperatorBySessionID(slotResp.Operators, sessionID); target != nil {
				operators = append(operators, *target)
				continue
			}
			unknown := models.OperatorDocumentGo{OperatorSessionID: sessionID, Status: "unknown"}
			if sessionID == sessionInfo.OperatorSessionID {
				unknown.ID = sessionInfo.OperatorID
			}
			operators = append(operators, unknown)
		}
	}

	if output.JSONEnabled(cmd) {
		entries := make([]operatorBindingEntry, 0, len(operators))
		for _, op := range operators {
			entries = append(entries, operatorBindingEntry{
				CLISessionID:      sessionInfo.CLISessionID,
				OperatorID:        op.ID,
				OperatorSessionID: op.OperatorSessionID,
				OperatorType:      op.OperatorType,
				OperatorRoles:     operatorcapability.GetOperatorRoles(op),
				Status:            op.Status,
				Hostname:          operatorHostnameValue(op),
				Name:              op.Name,
			})
		}
		return output.WriteJSON(cmd.OutOrStdout(), operatorBindListOutput{
			CLISessionID: sessionInfo.CLISessionID,
			UserID:       sessionInfo.UserID,
			Bindings:     entries,
		})
	}

	cmd.Printf("CLI session bindings\n")
	cmd.Println(strings.Repeat("=", 120))
	cmd.Printf("  CLI session ID: %s\n", sessionInfo.CLISessionID)
	cmd.Printf("  User ID:        %s\n", sessionInfo.UserID)
	if len(operators) == 0 {
		cmd.Println("\nNo operators bound to this CLI session.")
		return nil
	}

	cmd.Printf("\nBound operators (%d)\n", len(operators))
	cmd.Println(strings.Repeat("-", 120))
	cmd.Printf("  %-36s  %-12s  %-24s  %-36s  %-15s\n", "ID", "Type", "Hostname", "Session ID", "Status")
	for _, op := range operators {
		cmd.Printf("  %-36s  %-12s  %-24s  %-36s  %-15s\n", op.ID, op.OperatorType, operatorHostnameDisplay(op), op.OperatorSessionID, op.Status)
	}
	return nil
}

func runOperatorBindUnbind(
	cmd *cobra.Command,
	yes bool,
	configLoader func(string) (*config.Config, error),
	bindClientFactory operatorBindClientFactory,
	fileSvcFactory func(string, *slog.Logger) (fs.RuntimeFileService, error),
) error {
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

	if !output.JSONEnabled(cmd) {
		cmd.Printf("Operator unbind\n")
		cmd.Println(strings.Repeat("=", 72))
		cmd.Printf("  Current CLI session: %s\n", creds.CLISessionID)
		if creds.OperatorSessionID != "" {
			cmd.Printf("  Current binding:     %s\n", creds.OperatorSessionID)
			if creds.OperatorID != "" {
				cmd.Printf("  Operator ID:         %s\n", creds.OperatorID)
			}
		} else {
			cmd.Printf("  Current binding:     none\n")
		}
	}

	if !yes && !output.JSONEnabled(cmd) {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nUnbind operator from CLI session? (y/N): ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			cmd.Println("Aborted.")
			return nil
		}
	}

	unbind, err := bindClientFactory(cfg).Unbind(cmd.Context(), fileSvc)
	if err != nil {
		return fmt.Errorf("operator unbind: %w", err)
	}

	creds.CLISessionID = unbind.CLISessionID
	creds.OperatorSessionID = ""
	creds.OperatorID = ""
	if err := auth.SaveCredentials(fileSvc, cfg, creds); err != nil {
		return fmt.Errorf("operator unbind: save credentials: %w", err)
	}

	if output.JSONEnabled(cmd) {
		return output.WriteJSON(cmd.OutOrStdout(), operatorUnbindOutput{
			Success:        true,
			CLISessionID:   unbind.CLISessionID,
			UserID:         unbind.UserID,
			AlreadyUnbound: unbind.AlreadyUnbound,
		})
	}

	if unbind.AlreadyUnbound {
		cmd.Println("CLI session already has no operator binding.")
	} else {
		cmd.Println("CLI session unbound from operator.")
		cmd.Printf("New CLI session ID: %s\n", unbind.CLISessionID)
	}
	return nil
}

func findOperatorBySessionID(operators []models.OperatorDocumentGo, operatorSessionID string) *models.OperatorDocumentGo {
	for i := range operators {
		if operators[i].OperatorSessionID == operatorSessionID {
			return &operators[i]
		}
	}
	return nil
}

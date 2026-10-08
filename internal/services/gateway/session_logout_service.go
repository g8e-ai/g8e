// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
)

// cliCertificateRevoker is the slice of the PKI authority logout needs: it
// must skip serials that are already revoked so a retried logout never
// overwrites the reason and time of an earlier revocation.
type cliCertificateRevoker interface {
	IsRevoked(ctx context.Context, serial string) (bool, error)
	RevokeCertificate(ctx context.Context, serial string, reason string) error
}

// SessionLogoutServiceDeps groups all dependencies for SessionLogoutService.
type SessionLogoutServiceDeps struct {
	Logger      *slog.Logger
	CLISessions *CLISessionService
	WebSessions *WebSessionService
	Reg         *RegistrationService
	PKI         cliCertificateRevoker
}

// SessionLogoutService terminates a user's web and CLI sessions together with
// the operator bindings and CLI certificates behind them.
//
// Terminating CLI sessions alone does not log a CLI out: the refresh endpoint
// re-issues a session for any unrevoked certificate, whatever state its old
// session is in. A CLI logout is therefore only complete once the
// certificates are revoked, and that revocation is the last step.
type SessionLogoutService struct {
	logger      *slog.Logger
	cliSessions *CLISessionService
	webSessions *WebSessionService
	reg         *RegistrationService
	pki         cliCertificateRevoker
}

// NewSessionLogoutService creates a SessionLogoutService.
func NewSessionLogoutService(deps SessionLogoutServiceDeps) *SessionLogoutService {
	return &SessionLogoutService{
		logger:      deps.Logger,
		cliSessions: deps.CLISessions,
		webSessions: deps.WebSessions,
		reg:         deps.Reg,
		pki:         deps.PKI,
	}
}

// SessionLogoutResult reports what a logout terminated. Counts cover live
// sessions only; UnboundOperatorSessionIDs is the sorted, distinct set of
// operator sessions unbound from a live web or CLI session.
type SessionLogoutResult struct {
	WebSessionsTerminated     int
	CLISessionsTerminated     int
	CertificatesRevoked       int
	UnboundOperatorSessionIDs []string
}

// Logout terminates the sessions of userID selected by scope.
//
// Order, chosen so a failure leaves the caller able to retry with the session
// it authenticated with:
//  1. Web scope: unbind the operators bound to web sessions, then delete the
//     web sessions.
//  2. CLI scope: terminate every active CLI session, the caller's own
//     (currentCLISessionID) last, clearing each session's operator binding.
//  3. CLI scope: revoke the certificate of every CLI session the user ever
//     held, plus presentedCertSerial (the certificate that authenticated this
//     request, which may have no session document left).
//
// Each step is idempotent. An error aborts the remaining steps and is
// returned with the result accumulated so far.
func (s *SessionLogoutService) Logout(ctx context.Context, userID, currentCLISessionID, presentedCertSerial string, scope constants.LogoutScope) (*SessionLogoutResult, error) {
	if userID == "" {
		return nil, constants.ErrRegistrationUserIDRequired
	}
	if !scope.Valid() {
		return nil, fmt.Errorf("%w: %q", constants.ErrLogoutScopeInvalid, scope)
	}

	result := &SessionLogoutResult{}
	unbound := map[string]struct{}{}
	finish := func() *SessionLogoutResult {
		result.UnboundOperatorSessionIDs = sortedKeys(unbound)
		return result
	}

	if scope.IncludesWeb() {
		sessionIDs, err := s.reg.UnbindUserWebOperators(ctx, userID)
		for _, id := range sessionIDs {
			unbound[id] = struct{}{}
		}
		if err != nil {
			return finish(), fmt.Errorf("logout web sessions: %w", err)
		}
		terminated, err := s.webSessions.TerminateUserWebSessions(ctx, userID)
		result.WebSessionsTerminated = terminated
		if err != nil {
			return finish(), fmt.Errorf("logout web sessions: %w", err)
		}
	}

	if scope.IncludesCLI() {
		sessions, err := s.cliSessions.ListUserCLISessions(ctx, userID)
		if err != nil {
			return finish(), fmt.Errorf("logout CLI sessions: %w", err)
		}
		if err := s.terminateCLISessions(ctx, sessions, currentCLISessionID, result, unbound); err != nil {
			return finish(), fmt.Errorf("logout CLI sessions: %w", err)
		}
		revoked, err := s.revokeCLICertificates(ctx, sessions, presentedCertSerial)
		result.CertificatesRevoked = revoked
		if err != nil {
			return finish(), fmt.Errorf("logout CLI certificates: %w", err)
		}
	}

	finish()
	s.logger.Info("User logged out",
		"user_id", userID,
		"scope", string(scope),
		"web_sessions_terminated", result.WebSessionsTerminated,
		"cli_sessions_terminated", result.CLISessionsTerminated,
		"cli_certificates_revoked", result.CertificatesRevoked,
		"operator_sessions_unbound", len(result.UnboundOperatorSessionIDs),
	)
	return result, nil
}

// terminateCLISessions terminates every active session, the caller's own
// last. A session only counts, and only reports its operator bindings, when it
// was live: a session that is active but past its expiry is cleaned up
// without being reported as a logout.
func (s *SessionLogoutService) terminateCLISessions(ctx context.Context, sessions []*models.CLISession, currentCLISessionID string, result *SessionLogoutResult, unbound map[string]struct{}) error {
	ordered := make([]*models.CLISession, 0, len(sessions))
	var current *models.CLISession
	for _, session := range sessions {
		if !session.IsActive {
			continue
		}
		if session.ID == currentCLISessionID {
			current = session
			continue
		}
		ordered = append(ordered, session)
	}
	if current != nil {
		ordered = append(ordered, current)
	}

	now := time.Now().UTC()
	for _, session := range ordered {
		terminated, err := s.cliSessions.TerminateCLISession(ctx, session.ID)
		if err != nil {
			return err
		}
		if !terminated || (!session.ExpiresAt.IsZero() && now.After(session.ExpiresAt)) {
			continue
		}
		result.CLISessionsTerminated++
		for _, id := range boundSessionIDsOf(session) {
			unbound[id] = struct{}{}
		}
	}
	return nil
}

// revokeCLICertificates revokes the certificate serial of every CLI session
// the user ever held, active or not, plus presentedCertSerial. Sessions that
// refresh or bind replaced share their certificate, so the serial set is
// small; an expired-but-active session's certificate is the very thing
// refresh would otherwise revive. It attempts every serial and returns the
// number it revoked together with the joined failures.
func (s *SessionLogoutService) revokeCLICertificates(ctx context.Context, sessions []*models.CLISession, presentedCertSerial string) (int, error) {
	if s.pki == nil {
		return 0, constants.ErrPKIDatabaseNotAvailable
	}
	serials := map[string]struct{}{}
	if presentedCertSerial != "" {
		serials[presentedCertSerial] = struct{}{}
	}
	for _, session := range sessions {
		if session.CertSerial != "" {
			serials[session.CertSerial] = struct{}{}
		}
	}

	revoked := 0
	var errs []error
	for _, serial := range sortedKeys(serials) {
		already, err := s.pki.IsRevoked(ctx, serial)
		if err != nil {
			errs = append(errs, fmt.Errorf("check revocation of %s: %w", serial, err))
			continue
		}
		if already {
			continue
		}
		if err := s.pki.RevokeCertificate(ctx, serial, constants.CLILogoutCertRevocationReason); err != nil {
			errs = append(errs, fmt.Errorf("revoke %s: %w", serial, err))
			continue
		}
		revoked++
	}
	return revoked, errors.Join(errs...)
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

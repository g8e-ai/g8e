// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
	"github.com/g8e-ai/g8e/internal/timesvc"
)

// SSEEventService provides server-sent event storage and retrieval.
type SSEEventService struct {
	db     *sqliteutil.DB
	logger *slog.Logger
}

// NewSSEEventService creates a new SSE event service.
func NewSSEEventService(db *sqliteutil.DB, logger *slog.Logger) *SSEEventService {
	return &SSEEventService{
		db:     db,
		logger: logger,
	}
}

// SSERoute is the routing target for an SSE event row. UserID is always
// required (ownership/identity dimension). Exactly one of WebSessionID or
// CLISessionID MUST be set (delivery/routing dimension). The Gateway refuses
// to talk about a bare session id - every routing key is tagged at the type
// level so a web_session_id can never be mis-delivered as a cli_session_id
// (or vice versa). user_id alone is not a valid route.
type SSERoute struct {
	UserID       string
	WebSessionID string
	CLISessionID string
}

// validate ensures user_id is set and exactly one session id is set.
func (r SSERoute) validate() error {
	if r.UserID == "" {
		return constants.ErrGatewaySSERouteUserIDRequired
	}
	n := 0
	if r.WebSessionID != "" {
		n++
	}
	if r.CLISessionID != "" {
		n++
	}
	switch n {
	case 0:
		return constants.ErrGatewaySSERouteSessionRequired
	case 1:
		return nil
	default:
		return constants.ErrGatewaySSERouteSessionMutuallyExclusive
	}
}

// SSEEventsAppend inserts a row into the sse_events table and returns the
// assigned row ID. The route MUST set UserID and exactly one of WebSessionID
// or CLISessionID. The producer_id is the app identity (SPIFFE ID) that
// produced the event for attribution. The returned ID is stamped into the
// pub/sub envelope (models.SSEPublishedEvent) so the stream handler can
// deduplicate live events against replayed rows and emit an `id:` field on
// the live path.
func (s *SSEEventService) SSEEventsAppend(route SSERoute, eventType, payload, producerID string) (int64, error) {
	if err := route.validate(); err != nil {
		return 0, err
	}
	now := timesvc.NowTimestamp()
	result, err := s.db.ExecWithRetry(
		"INSERT INTO sse_events (user_id, web_session_id, cli_session_id, event_type, payload, producer_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		route.UserID, nullIfEmpty(route.WebSessionID), nullIfEmpty(route.CLISessionID), eventType, payload, nullIfEmpty(producerID), now,
	)
	if err != nil {
		return 0, fmt.Errorf("sse_event_service: append: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("sse_event_service: append: last insert id: %w", err)
	}
	return id, nil
}

// nullIfEmpty returns sql.NullString for empty strings so the
// CHECK constraint on sse_events sees a NULL rather than an empty string.
func nullIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// SSEEventsCleanup deletes events older than the given max age. This prevents
// the ring buffer from growing unboundedly between reconnections.
func (s *SSEEventService) SSEEventsCleanup(maxAge time.Duration) (int64, error) {
	cutoff := timesvc.FormatTimestamp(time.Now().UTC().Add(-maxAge))
	result, err := s.db.ExecWithRetry("DELETE FROM sse_events WHERE created_at < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("sse_event_service: cleanup: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sse_event_service: cleanup: rows affected: %w", err)
	}
	return count, nil
}

// SSEEventsWipe deletes all rows from the sse_events table. Returns the number of rows deleted.
func (s *SSEEventService) SSEEventsWipe() (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM sse_events")
	if err != nil {
		return 0, fmt.Errorf("sse_event_service: wipe: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("sse_event_service: wipe: rows affected: %w", err)
	}
	return count, nil
}

// SSEEventsCount returns the total number of rows in the sse_events table.
func (s *SSEEventService) SSEEventsCount() (int64, error) {
	var count int64
	err := s.db.QueryRowWithRetry("SELECT COUNT(*) FROM sse_events").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("sse_event_service: count: %w", err)
	}
	return count, nil
}

// SSEEventsListSince returns up to `limit` events with id > sinceID, ordered by
// id ascending. The route MUST set UserID and exactly one of WebSessionID or
// CLISessionID. SSEEventsListAllSince is the admin-only "all routes" variant.
func (s *SSEEventService) SSEEventsListSince(route SSERoute, sinceID int64, limit int) ([]models.SSEEventRow, error) {
	if err := route.validate(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var query string
	var args []interface{}
	switch {
	case route.WebSessionID != "":
		query = "SELECT id, user_id, web_session_id, cli_session_id, event_type, payload, created_at FROM sse_events WHERE web_session_id = ? AND user_id = ? AND id > ? ORDER BY id ASC LIMIT ?"
		args = []interface{}{route.WebSessionID, route.UserID, sinceID, limit}
	case route.CLISessionID != "":
		query = "SELECT id, user_id, web_session_id, cli_session_id, event_type, payload, created_at FROM sse_events WHERE cli_session_id = ? AND user_id = ? AND id > ? ORDER BY id ASC LIMIT ?"
		args = []interface{}{route.CLISessionID, route.UserID, sinceID, limit}
	default:
		return nil, constants.ErrGatewaySSERouteSessionRequired
	}

	return sqliteutil.MaterializeRows(s.db, query, args, func(r *sql.Rows) (models.SSEEventRow, error) {
		var row models.SSEEventRow
		var web, cli sql.NullString
		if err := r.Scan(&row.ID, &row.UserID, &web, &cli, &row.EventType, &row.Payload, &row.CreatedAt); err != nil {
			return models.SSEEventRow{}, fmt.Errorf("sse_event_service: list_since: scan: %w", err)
		}
		row.WebSessionID = web.String
		row.CLISessionID = cli.String
		return row, nil
	})
}

// SSEEventsListAllSince is an admin/debug helper that returns events across
// every routing target with id > sinceID. Production paths MUST use
// SSEEventsListSince with a typed route.
func (s *SSEEventService) SSEEventsListAllSince(sinceID int64, limit int) ([]models.SSEEventRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	return sqliteutil.MaterializeRows(s.db,
		"SELECT id, user_id, web_session_id, cli_session_id, event_type, payload, created_at FROM sse_events WHERE id > ? ORDER BY id ASC LIMIT ?",
		[]any{sinceID, limit},
		func(r *sql.Rows) (models.SSEEventRow, error) {
			var row models.SSEEventRow
			var web, cli sql.NullString
			if err := r.Scan(&row.ID, &row.UserID, &web, &cli, &row.EventType, &row.Payload, &row.CreatedAt); err != nil {
				return models.SSEEventRow{}, fmt.Errorf("sse_event_service: list_all_since: scan: %w", err)
			}
			row.WebSessionID = web.String
			row.CLISessionID = cli.String
			return row, nil
		})
}

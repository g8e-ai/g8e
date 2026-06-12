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

package gateway

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/sqliteutil"
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

// SSERoute is the routing target for an SSE event row. Exactly one of the
// three id fields MUST be non-empty. The Gateway refuses to talk about a
// bare session id - every routing key is tagged at the type level so a
// web_session_id can never be mis-delivered as a cli_session_id (or vice
// versa) and a user_id (background fan-out) can never be mistaken for a
// per-session id.
type SSERoute struct {
	WebSessionID string
	CLISessionID string
	UserID       string
}

// validate ensures exactly one routing id is set.
func (r SSERoute) validate() error {
	n := 0
	if r.WebSessionID != "" {
		n++
	}
	if r.CLISessionID != "" {
		n++
	}
	if r.UserID != "" {
		n++
	}
	switch n {
	case 0:
		return fmt.Errorf("sse route requires exactly one of web_session_id, cli_session_id, user_id")
	case 1:
		return nil
	default:
		return fmt.Errorf("sse route is mutually-exclusive: set exactly one of web_session_id, cli_session_id, user_id")
	}
}

// SSEEventsAppend inserts a row into the sse_events table. The route MUST set
// exactly one of WebSessionID, CLISessionID, UserID. The producer_id is the
// app identity (SPIFFE ID) that produced the event for attribution.
func (s *SSEEventService) SSEEventsAppend(route SSERoute, eventType, payload, producerID string) error {
	if err := route.validate(); err != nil {
		return err
	}
	now := sqliteutil.NowTimestamp()
	_, err := s.db.ExecWithRetry(
		"INSERT INTO sse_events (web_session_id, cli_session_id, user_id, event_type, payload, producer_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		nullIfEmpty(route.WebSessionID), nullIfEmpty(route.CLISessionID), nullIfEmpty(route.UserID), eventType, payload, nullIfEmpty(producerID), now,
	)
	return err
}

// nullIfEmpty returns sql.NullString{Valid: false} for empty strings so the
// CHECK constraint on sse_events sees a NULL rather than an empty string.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// SSEEventsWipe deletes all rows from the sse_events table. Returns the number of rows deleted.
func (s *SSEEventService) SSEEventsWipe() (int64, error) {
	result, err := s.db.ExecWithRetry("DELETE FROM sse_events")
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// SSEEventsCount returns the total number of rows in the sse_events table.
func (s *SSEEventService) SSEEventsCount() (int64, error) {
	var count int64
	err := s.db.QueryRowWithRetry("SELECT COUNT(*) FROM sse_events").Scan(&count)
	return count, err
}

// SSEEventsListSince returns up to `limit` events with id > sinceID, ordered by
// id ascending. The route MUST set exactly one of WebSessionID, CLISessionID,
// UserID. SSEEventsListAllSince is the admin-only "all routes" variant.
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
		query = "SELECT id, web_session_id, cli_session_id, user_id, event_type, payload, created_at FROM sse_events WHERE web_session_id = ? AND id > ? ORDER BY id ASC LIMIT ?"
		args = []interface{}{route.WebSessionID, sinceID, limit}
	case route.CLISessionID != "":
		query = "SELECT id, web_session_id, cli_session_id, user_id, event_type, payload, created_at FROM sse_events WHERE cli_session_id = ? AND id > ? ORDER BY id ASC LIMIT ?"
		args = []interface{}{route.CLISessionID, sinceID, limit}
	default:
		query = "SELECT id, web_session_id, cli_session_id, user_id, event_type, payload, created_at FROM sse_events WHERE user_id = ? AND id > ? ORDER BY id ASC LIMIT ?"
		args = []interface{}{route.UserID, sinceID, limit}
	}

	return sqliteutil.MaterializeRows(s.db, query, args, func(r *sql.Rows) (models.SSEEventRow, error) {
		var row models.SSEEventRow
		var web, cli, user sql.NullString
		if err := r.Scan(&row.ID, &web, &cli, &user, &row.EventType, &row.Payload, &row.CreatedAt); err != nil {
			return models.SSEEventRow{}, err
		}
		row.WebSessionID = web.String
		row.CLISessionID = cli.String
		row.UserID = user.String
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
		"SELECT id, web_session_id, cli_session_id, user_id, event_type, payload, created_at FROM sse_events WHERE id > ? ORDER BY id ASC LIMIT ?",
		[]interface{}{sinceID, limit},
		func(r *sql.Rows) (models.SSEEventRow, error) {
			var row models.SSEEventRow
			var web, cli, user sql.NullString
			if err := r.Scan(&row.ID, &web, &cli, &user, &row.EventType, &row.Payload, &row.CreatedAt); err != nil {
				return models.SSEEventRow{}, err
			}
			row.WebSessionID = web.String
			row.CLISessionID = cli.String
			row.UserID = user.String
			return row, nil
		})
}

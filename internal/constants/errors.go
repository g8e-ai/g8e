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

package constants

import "errors"

// Standard platform errors
var (
	ErrUserNotFound         = errors.New("user not found")
	ErrNoPasskeysRegistered = errors.New("no passkeys registered")
	ErrInvalidJSONBody      = errors.New("invalid JSON body")
	ErrUserIDRequired       = errors.New("user_id required")
	ErrMethodNotAllowed     = errors.New("method not allowed")
	ErrForbidden            = errors.New("forbidden")
	ErrInternal             = errors.New("internal error")
	ErrNotFound             = errors.New("not found")
	ErrAlreadyExists        = errors.New("already exists")
	ErrConstraintViolation  = errors.New("constraint violation")
	ErrDatabaseLocked       = errors.New("database is locked")
	ErrServiceUnavailable   = errors.New("service unavailable")
	ErrDatabaseReplay       = errors.New("database replay detected")
	ErrDuplicateColumn      = errors.New("duplicate column name")
	ErrProcessKilled        = errors.New("process killed")
	ErrTrustBundleStale     = errors.New("trust bundle stale")
	ErrKeyNotFound          = errors.New("key not found")
	ErrExpired              = errors.New("expired")
)

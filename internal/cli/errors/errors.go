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

package errors

import "errors"

var (
	// ErrNotAuthenticated is returned when credentials are missing or invalid
	ErrNotAuthenticated = errors.New("not authenticated")

	// ErrFailedToLoadCredentials is returned when loading credentials fails
	ErrFailedToLoadCredentials = errors.New("failed to load credentials")

	// ErrFailedToLoadClientCertificate is returned when loading client certificate fails
	ErrFailedToLoadClientCertificate = errors.New("failed to load client certificate")

	// ErrFailedToReadTrustBundle is returned when reading trust bundle fails
	ErrFailedToReadTrustBundle = errors.New("failed to read trust bundle")

	// ErrFailedToParseTrustBundle is returned when parsing trust bundle fails
	ErrFailedToParseTrustBundle = errors.New("failed to parse trust bundle")

	// ErrFailedToParsePaths is returned when parsing paths.json fails
	ErrFailedToParsePaths = errors.New("failed to parse paths.json")
)

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// GatewayHTTP2MaxConcurrentStreams is a Go runtime transport limit, not a
// wire-contract constant. It accommodates the supported 5,000-Operator fleet
// plus the embedded Operator and concurrent control-plane requests on one
// connection, avoiding a TCP dial burst at the standard library stream limit.
const GatewayHTTP2MaxConcurrentStreams = 8192

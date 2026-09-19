// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package docs embeds the pre-generated Swagger/OpenAPI specification so it
// can be served without depending on the swaggo/swag runtime library.
package docs

import _ "embed"

//go:embed swagger.json
var SwaggerJSON []byte

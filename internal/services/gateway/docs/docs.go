// Package docs embeds the pre-generated Swagger/OpenAPI specification so it
// can be served without depending on the swaggo/swag runtime library.
package docs

import _ "embed"

//go:embed swagger.json
var SwaggerJSON []byte

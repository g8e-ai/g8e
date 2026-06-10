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

//	@title			g8e Gateway API
//	@version		1.0
//	@description	API documentation for the g8e Gateway public endpoints
//	@termsOfService	https://github.com/g8e-ai/g8e

//	@contact.name	g8e Team
//	@contact.url	https://github.com/g8e-ai/g8e
//	@contact.email	support@g8e.ai

//	@license.name	Apache 2.0
//	@license.url	http://www.apache.org/licenses/LICENSE-2.0.html

//	@host		localhost:8443
//	@BasePath	/api/v1

//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Bearer token authentication (JWT or mTLS certificate)

//	@Summary		Health check
//	@Description	Returns the current health status of the gateway
//	@Tags			health
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	models.HealthResponse
//	@Failure		503	{object}	response.ErrorResponse
//	@Router			/health [get]

//	@Summary		Bootstrap health check
//	@Description	Returns the current health status during bootstrap (no state root check)
//	@Tags			health
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	models.HealthResponse
//	@Failure		503	{object}	response.ErrorResponse
//	@Router			/health [get]

//	@Summary		Landing page
//	@Description	Returns the public landing page for the gateway
//	@Tags			public
//	@Accept			html
//	@Produce		html
//	@Success		200	{string}	string
//	@Router			/ [get]

//	@Summary		Get CA bundle
//	@Description	Returns the platform's root CA certificate bundle for trust establishment
//	@Tags			pki
//	@Accept			json
//	@Produce		pem
//	@Success		200	{string}	string
//	@Router			/.well-known/g8e/pki/ca-bundle [get]

//	@Summary		Get PKI fingerprint
//	@Description	Returns the platform's PKI fingerprint for verification
//	@Tags			pki
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/.well-known/g8e/pki/fingerprint [get]

//	@Summary		Get CRL
//	@Description	Returns the certificate revocation list
//	@Tags			pki
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/.well-known/g8e/pki/crl [get]

//	@Summary		Bootstrap auth
//	@Description	Initiates local bootstrap authentication flow
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/auth/bootstrap [post]

//	@Summary		Bootstrap status
//	@Description	Returns the current bootstrap status
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/auth/bootstrap/status [get]

//	@Summary		CLI enrollment
//	@Description	Enrolls a CLI client with the gateway
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/auth/cli/enroll [post]

//	@Summary		Device enrollment
//	@Description	Enrolls a device with the gateway
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/auth/device/enroll [post]

//	@Summary		PKI device enrollment
//	@Description	Enrolls a device via PKI endpoint
//	@Tags			pki
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/pki/devices/enroll [post]

//	@Summary		MCP endpoint
//	@Description	Unified MCP JSON-RPC endpoint for AI IDE integration
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/mcp [post]

//	@Summary		List MCP tools
//	@Description	Returns the list of available MCP tools
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/api/v1/mcp/tools/list [get]

//	@Summary		Call MCP tool
//	@Description	Calls an MCP tool with the provided arguments
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/api/v1/mcp/tools/call [post]

//	@Summary		Call MCP tool (SSE)
//	@Description	Calls an MCP tool with SSE streaming response
//	@Tags			mcp
//	@Accept			json
//	@Produce		text/event-stream
//	@Success		200	{string}	string
//	@Router			/api/v1/mcp/tools/call/sse [post]

//	@Summary		List MCP resources
//	@Description	Returns the list of available MCP resources
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/api/v1/mcp/resources/list [get]

//	@Summary		Read MCP resource
//	@Description	Reads a specific MCP resource
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/api/v1/mcp/resources/read [post]

//	@Summary		List MCP prompts
//	@Description	Returns the list of available MCP prompts
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/api/v1/mcp/prompts/list [get]

//	@Summary		Get MCP prompt
//	@Description	Returns a specific MCP prompt template
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/api/v1/mcp/prompts/get [post]

//	@Summary		A2A call
//	@Description	Calls an A2A agent endpoint
//	@Tags			a2a
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/api/v1/a2a/call [post]

//	@Summary		Passkey CLI register challenge
//	@Description	Initiates passkey registration for CLI bootstrap
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/auth/passkeys/cli-register/challenge [post]

//	@Summary		Passkey CLI register verify
//	@Description	Verifies passkey registration for CLI bootstrap
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/auth/passkeys/cli-register/verify [post]

//	@Summary		Passkey CLI authenticate challenge
//	@Description	Initiates passkey authentication for CLI bootstrap
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/auth/passkeys/cli/authenticate/challenge [post]

//	@Summary		Passkey CLI authenticate verify
//	@Description	Verifies passkey authentication for CLI bootstrap
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/api/v1/auth/passkeys/cli/authenticate/verify [post]

//	@Summary		Bootstrap CA Linux script
//	@Description	Returns the Linux CA trust bootstrap script
//	@Tags			bootstrap
//	@Accept			*
//	@Produce		text/plain
//	@Success		200	{string}	string
//	@Router			/bootstrap-ca [get]

//	@Summary		Bootstrap CA Windows script
//	@Description	Returns the Windows CA trust bootstrap script
//	@Tags			bootstrap
//	@Accept			*
//	@Produce		text/plain
//	@Success		200	{string}	string
//	@Router			/bootstrap-ca.ps1 [get]

//	@Summary		Deploy script Linux
//	@Description	Returns the Linux operator deployment script
//	@Tags			deploy
//	@Accept			*
//	@Produce		text/plain
//	@Success		200	{string}	string
//	@Router			/g8e-operator.sh [get]

//	@Summary		Deploy script Windows
//	@Description	Returns the Windows operator deployment script
//	@Tags			deploy
//	@Accept			*
//	@Produce		text/plain
//	@Success		200	{string}	string
//	@Router			/g8e-operator.ps1 [get]

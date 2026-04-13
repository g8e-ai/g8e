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

import express from 'express';
import { now } from '../../models/base.js';
import { logger } from '../../utils/logger.js';
import { MCPPaths } from '../../constants/api_paths.js';
import { getVersionInfo } from '../../utils/version.js';

const MCP_SERVER_INFO = {
    protocolVersion: '2025-03-26',
    serverInfo: {
        name: 'g8e',
        version: getVersionInfo().version,
    },
    capabilities: {
        tools: {},
    },
};

function jsonrpcError(id, code, message) {
    return { jsonrpc: '2.0', id: id ?? null, error: { code, message } };
}

function jsonrpcResult(id, result) {
    return { jsonrpc: '2.0', id, result };
}

/**
 * Authenticate using OAuth Client ID (treated as a G8eKey).
 * This enables Claude Code connector to use G8eKeys for authentication.
 *
 * @param {string} oauthClientId - The OAuth Client ID (G8eKey)
 * @param {Object} apiKeyService - ApiKeyService instance
 * @param {Object} userService - UserService instance
 * @returns {Promise<{success: boolean, user_id?: string, organization_id?: string, error?: string}>}
 */
async function authenticateViaOAuthClientId(oauthClientId, apiKeyService, userService) {
    try {
        if (!oauthClientId) {
            return { success: false, error: 'OAuth Client ID is required' };
        }

        // Validate as a G8eKey (API key)
        const validation = await apiKeyService.validateKey(oauthClientId);
        if (!validation.success) {
            logger.warn('[MCP] OAuth Client ID validation failed', { error: validation.error });
            return { success: false, error: validation.error };
        }

        const keyData = validation.data;
        const userId = keyData.user_id;

        if (!userId) {
            logger.error('[MCP] OAuth Client ID missing user_id');
            return { success: false, error: 'Invalid OAuth Client ID' };
        }

        const user = await userService.getUser(userId);
        if (!user) {
            logger.error('[MCP] User not found for OAuth Client ID', { user_id: userId });
            return { success: false, error: 'User not found' };
        }

        logger.info('[MCP] OAuth Client ID authenticated successfully', { user_id: userId });

        // Record usage
        apiKeyService.recordUsage(oauthClientId).catch(err => {
            logger.warn('[MCP] Failed to record OAuth Client ID usage', { error: err.message });
        });

        return {
            success: true,
            user_id: userId,
            organization_id: user.organization_id || keyData.organization_id,
        };
    } catch (error) {
        logger.error('[MCP] OAuth Client ID authentication error', { error: error.message });
        return { success: false, error: 'Internal authentication error' };
    }
}

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 * @param {Object} options.rateLimiters - Rate limiter objects
 */
export function createMCPRouter({
    services,
    authMiddleware,
    rateLimiters,
}) {
    const { internalHttpClient, apiKeyService, userService, bindingService } = services;
    const { requireAuth, requireOperatorBinding } = authMiddleware;
    const { apiRateLimiter } = rateLimiters;
    const router = express.Router();

    router.post(MCPPaths.ROOT, apiRateLimiter, async (req, res, next) => {
        // Try OAuth Client ID authentication first (for Claude Code connector)
        const oauthClientId = req.headers['x-oauth-client-id'] || req.query.oauth_client_id;
        if (oauthClientId) {
            const authResult = await authenticateViaOAuthClientId(oauthClientId, apiKeyService, userService);
            if (authResult.success) {
                req.userId = authResult.user_id;
                req.organizationId = authResult.organization_id;
                req.webSessionId = null; // No session for API key auth
                req.session = null;
                // Apply requireOperatorBinding middleware for OAuth Client ID auth
                return requireOperatorBinding(req, res, (err) => {
                    if (err) return next(err);
                    return handleMCPRequest(req, res, { internalHttpClient });
                });
            } else {
                return res.status(401).json(
                    jsonrpcError(null, -32000, `OAuth Client ID authentication failed: ${authResult.error}`)
                );
            }
        }

        // Fall back to standard session authentication
        return requireAuth(req, res, (err) => {
            if (err) return next(err);
            return requireOperatorBinding(req, res, (bindErr) => {
                if (bindErr) return next(bindErr);
                return handleMCPRequest(req, res, { internalHttpClient });
            });
        });
    });

    async function handleMCPRequest(req, res, { internalHttpClient }) {
        const body = req.body;

        if (!body || body.jsonrpc !== '2.0') {
            return res.status(400).json(
                jsonrpcError(body?.id, -32600, 'Invalid JSON-RPC request: missing jsonrpc "2.0" field')
            );
        }

        const { method, id, params } = body;

        if (!method) {
            return res.status(400).json(
                jsonrpcError(id, -32600, 'Invalid JSON-RPC request: missing method field')
            );
        }

        switch (method) {
            case 'initialize':
                return res.json(jsonrpcResult(id, MCP_SERVER_INFO));

            case 'notifications/initialized':
                return res.status(204).end();

            case 'ping':
                return res.json(jsonrpcResult(id, {}));

            case 'tools/list':
                return await handleToolsList(req, res, { id, internalHttpClient });

            case 'tools/call':
                return await handleToolsCall(req, res, { id, params, internalHttpClient });

            default:
                return res.status(400).json(
                    jsonrpcError(id, -32601, `Method not found: ${method}`)
                );
        }
    }

    async function handleToolsList(req, res, { id, internalHttpClient }) {
        try {
            const g8eeResponse = await internalHttpClient.mcpToolsList(req.g8eContext);

            return res.json(jsonrpcResult(id, { tools: g8eeResponse.tools || [] }));
        } catch (error) {
            logger.error('[MCP] tools/list failed', { error: error.message });
            return res.status(500).json(
                jsonrpcError(id, -32603, `Internal error: ${error.message}`)
            );
        }
    }

    async function handleToolsCall(req, res, { id, params, internalHttpClient }) {
        if (!params?.name) {
            return res.status(400).json(
                jsonrpcError(id, -32602, 'Invalid params: missing tool name')
            );
        }

        try {
            const g8eeResponse = await internalHttpClient.mcpToolsCall({
                tool_name: params.name,
                arguments: params.arguments || {},
                request_id: id,
            }, req.g8eContext);

            if (g8eeResponse.error) {
                return res.json({
                    jsonrpc: '2.0',
                    id,
                    error: g8eeResponse.error,
                });
            }

            return res.json(jsonrpcResult(id, g8eeResponse.result || {}));
        } catch (error) {
            logger.error('[MCP] tools/call failed', { error: error.message, tool: params.name });
            return res.status(500).json(
                jsonrpcError(id, -32603, `Internal error: ${error.message}`)
            );
        }
    }

    return router;
}

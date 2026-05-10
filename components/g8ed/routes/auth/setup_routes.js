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
import { logger } from '../../utils/logger.js';
import { SetupPaths } from '../../constants/api_paths.js';
import { LLMProvider, PROVIDER_MODELS, PROVIDER_DEFAULT_MODELS } from '../../constants/ai.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 */
export function createSetupRouter({ services }) {
    const { setupService } = services;
    const router = express.Router();

    router.get(SetupPaths.WIZARD, async (req, res, next) => {
        try {
            const firstRun = await setupService.isFirstRun();
            if (!firstRun) {
                return res.redirect('/');
            }
        } catch (error) {
            logger.warn('[SETUP] User check failed', { error: error.message });
            return res.redirect('/');
        }

        res.render('setup', {
            llmCatalog: {
                providers: LLMProvider,
                providerModels: PROVIDER_MODELS,
                providerDefaultModels: PROVIDER_DEFAULT_MODELS,
            },
        });
    });

    router.post(SetupPaths.FETCH_MODELS, async (req, res) => {
        const { url } = req.body;
        if (!url) {
            return res.status(400).json({ error: 'URL is required' });
        }

        let parsedUrl;
        try {
            // Prepend protocol if missing for robust parsing
            const urlWithProtocol = url.trim().match(/^https?:\/\//i) ? url.trim() : `http://${url.trim()}`;
            parsedUrl = new URL(urlWithProtocol);
        } catch (error) {
            return res.status(400).json({ error: 'Invalid URL format.' });
        }

        // 1. Strict protocol check
        if (parsedUrl.protocol !== 'http:' && parsedUrl.protocol !== 'https:') {
            return res.status(400).json({ error: 'Only HTTP and HTTPS are supported.' });
        }

        // 2. Prevent SSRF to cloud metadata endpoints
        const host = parsedUrl.hostname.toLowerCase();
        const forbiddenHosts = ['169.254.169.254', 'metadata.google.internal', '100.100.100.200'];
        if (forbiddenHosts.some(forbidden => host === forbidden || host.endsWith(`.${forbidden}`))) {
            return res.status(403).json({ error: 'Access to the specified host is forbidden.' });
        }

        // 3. Construct final URL securely using the URL object properties
        const finalUrl = new URL('/api/tags', parsedUrl.origin);

        try {
            const response = await fetch(finalUrl.toString(), {
                method: 'GET',
                headers: { 'Accept': 'application/json' },
                signal: AbortSignal.timeout(5000) // 5 second timeout
            });

            if (!response.ok) {
                return res.status(response.status).json({ 
                    error: `Ollama returned ${response.status}: ${response.statusText}` 
                });
            }

            const data = await response.json();
            res.json(data);
        } catch (error) {
            logger.error('[SETUP] Ollama fetch failed', { error: error.message, url: ollamaUrl });
            res.status(500).json({ error: `Failed to connect to Ollama: ${error.message}` });
        }
    });

    return router;
}

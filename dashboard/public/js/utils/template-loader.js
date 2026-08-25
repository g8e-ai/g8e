// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

import { devLogger } from './dev-logger.js';
import { ServiceName } from '../constants/service-client-constants.js';

const TEMPLATES_BASE_PATH = '/js/components/templates/';

/**
 * TemplateLoader - Loads and caches HTML templates
 * 
 * Features:
 * - Asynchronous template loading from HTML files
 * - In-memory caching for performance
 * - Simple variable replacement using {{variable}} syntax
 * - Clean separation of HTML from JavaScript
 */
export class TemplateLoader {
    constructor(basePath = TEMPLATES_BASE_PATH, transport = null) {
        this.basePath = basePath;
        this.cache = new Map();
        this.loading = new Map(); // Track in-flight requests
        this._transport = transport;
    }

    seed(templateName, html) {
        this.cache.set(templateName, html);
    }

    /**
     * Load a template from file (with caching)
     * @param {string} templateName - Name of template file (without .html extension)
     * @returns {Promise<string>} Template HTML content
     */
    async load(templateName) {
        // Return cached template if available
        if (this.cache.has(templateName)) {
            return this.cache.get(templateName);
        }

        // If already loading, wait for existing request
        if (this.loading.has(templateName)) {
            return this.loading.get(templateName);
        }

        // Create new loading promise
        const loadingPromise = this._fetchTemplate(templateName);
        this.loading.set(templateName, loadingPromise);

        try {
            const template = await loadingPromise;
            this.cache.set(templateName, template);
            return template;
        } finally {
            this.loading.delete(templateName);
        }
    }

    /**
     * Fetch template from server
     * @private
     */
    async _fetchTemplate(templateName) {
        const path = `${this.basePath}${templateName}.html`;

        try {
            const client = this._transport ?? window.serviceClient;
            const response = await client.get(ServiceName.g8ed, path);
            return await response.text();
        } catch (error) {
            devLogger.error(`[TemplateLoader] Error loading template '${templateName}':`, error);
            throw error;
        }
    }

    /**
     * Load template and apply variable replacements
     * @param {string} templateName - Name of template file
     * @param {Object} variables - Key-value pairs for replacements
     * @returns {Promise<string>} Rendered template HTML
     */
    async render(templateName, variables = {}) {
        const template = await this.load(templateName);
        return this.replace(template, variables);
    }

    /**
     * Replace variables in template string
     * Uses {{variable}} for HTML-escaped values (safe for user content)
     * Uses {{{variable}}} for raw/unescaped values (only for trusted content)
     * @param {string} template - Template HTML content
     * @param {Object} variables - Key-value pairs for replacements
     * @returns {string} Rendered template HTML
     */
    replace(template, variables = {}) {
        let result = template;
        
        for (const [key, value] of Object.entries(variables)) {
            const rawPlaceholder = new RegExp(`\\{\\{\\{${key}\\}\\}\\}`, 'g');
            result = result.replace(rawPlaceholder, value ?? '');
            
            const escapedPlaceholder = new RegExp(`\\{\\{${key}\\}\\}`, 'g');
            result = result.replace(escapedPlaceholder, this.escapeHtml(value ?? ''));
        }
        
        return result;
    }

    /**
     * Escape HTML special characters to prevent XSS
     * @param {string} text - Text to escape
     * @returns {string} Escaped text safe for HTML insertion
     */
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = String(text);
        return div.innerHTML;
    }

    /**
     * Preload multiple templates
     * @param {string[]} templateNames - Array of template names to preload
     * @returns {Promise<void>}
     */
    async preload(templateNames) {
        await Promise.all(templateNames.map(name => this.load(name)));
    }

    /**
     * Clear cache for a specific template or all templates
     * @param {string} [templateName] - Optional template name to clear
     */
    clearCache(templateName) {
        if (templateName) {
            this.cache.delete(templateName);
        } else {
            this.cache.clear();
        }
    }

    /**
     * Get cache statistics
     * @returns {Object} Cache stats
     */
    getCacheStats() {
        return {
            size: this.cache.size,
            templates: Array.from(this.cache.keys()),
            loading: Array.from(this.loading.keys())
        };
    }
}

// Create and export singleton instance
export const templateLoader = new TemplateLoader();

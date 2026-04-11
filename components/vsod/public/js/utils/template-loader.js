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

import { devLogger } from './dev-logger.js';
import { ServiceName } from '../constants/service-client-constants.js';
import { escapeHtml, escapeHtmlAttribute } from './html.js';

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
            const response = await client.get(ServiceName.VSOD, path);
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
     * Uses {{variable}} for HTML-escaped values (safe for text content)
     * Uses {{!variable}} for attribute-escaped values (safe for HTML attributes)
     * Uses {{{variable}}} for raw/unescaped values (only for trusted content)
     * 
     * SECURITY WARNING:
     * - NEVER use {{{variable}}} in HTML attributes - this allows attribute injection
     * - Use {{!variable}} for data attributes (data-*, id, class, etc.)
     * - Use {{variable}} for text content between HTML tags
     * - Only use {{{variable}}} for trusted HTML markup (e.g., pre-rendered icons)
     * 
     * @param {string} template - Template HTML content
     * @param {Object} variables - Key-value pairs for replacements
     * @returns {string} Rendered template HTML
     */
    replace(template, variables = {}) {
        const keys = Object.keys(variables);
        if (keys.length === 0) {
            return template;
        }

        // Escape keys for use in a regex
        const escapedKeys = keys.map(key => key.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
        const keyPattern = escapedKeys.join('|');
        
        // Single regex to match all three types of variables in one pass
        // 1. Triple braces: {{{key}}} (unescaped)
        // 2. Attribute escaping: {{!key}}
        // 3. Double braces: {{key}} (HTML escaped)
        const combinedPattern = new RegExp(`\\{\\{(?:(\\{)|(!))?(${keyPattern})\\}\\}\\}?`, 'g');
        
        return template.replace(combinedPattern, (match, isTriple, isAttr, key) => {
            const value = variables[key] ?? '';
            
            // Check if it's a triple brace match (isTriple is '{' and match ends with '}')
            // Since our regex uses \\}\\}\\}? for the end, we should check if the match actually has 3 braces
            if (isTriple && match.endsWith('}}}')) {
                return value;
            }
            
            // Attribute escaping
            if (isAttr) {
                return escapeHtmlAttribute(value);
            }
            
            // Default: Double braces (HTML escaped)
            return escapeHtml(value);
        });
    }

    /**
     * Create a DocumentFragment from a template
     * @param {string} templateName - Name of template file
     * @param {Object} variables - Key-value pairs for replacements
     * @returns {Promise<DocumentFragment>} Rendered DocumentFragment
     */
    async createFragment(templateName, variables = {}) {
        const html = await this.render(templateName, variables);
        const templateEl = document.createElement('template');
        templateEl.innerHTML = html;
        return templateEl.content;
    }

    /**
     * Render a template directly into a container element
     * Clears existing content and appends the fragment for performance
     * @param {HTMLElement} container - Target container element
     * @param {string} templateName - Name of template file
     * @param {Object} variables - Key-value pairs for replacements
     * @returns {Promise<void>}
     */
    async renderTo(container, templateName, variables = {}) {
        const fragment = await this.createFragment(templateName, variables);
        container.textContent = '';
        container.appendChild(fragment);
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

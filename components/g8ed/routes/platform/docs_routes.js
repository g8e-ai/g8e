import express from 'express';
import fs from 'fs';
import path from 'path';
import { logger } from '../../utils/logger.js';
import { DEFAULT_DOCS_DIR } from '../../constants/service_config.js';
import { DocsPaths } from '../../constants/api_paths.js';
import { DocsTreeResponse, DocsFileResponse, ErrorResponse } from '../../models/response_models.js';

/**
 * @param {Object} options
 * @param {Object} options.services - Services object containing all platform services
 * @param {Object} options.authMiddleware - Auth middleware object
 */
export function createDocsRouter({ services, authMiddleware }) {
    const { optionalAuth } = authMiddleware;
    const router = express.Router();

    const { settingsService } = services;
    const docsDir = settingsService.docs_dir || DEFAULT_DOCS_DIR;
    const readmePath = settingsService.readme_path || null;

    function buildTree(dirPath, relativePath = '') {
        const entries = fs.readdirSync(dirPath, { withFileTypes: true });
        const nodes = [];

        for (const entry of entries) {
            const entryRelative = relativePath ? `${relativePath}/${entry.name}` : entry.name;
            if (entry.isDirectory()) {
                const children = buildTree(path.join(dirPath, entry.name), entryRelative);
                if (children.length > 0) {
                    nodes.push({ type: 'dir', name: entry.name, path: entryRelative, children });
                }
            } else if (entry.isFile() && entry.name.endsWith('.md')) {
                nodes.push({ type: 'file', name: entry.name, path: entryRelative });
            }
        }

        nodes.sort((a, b) => {
            if (a.name === 'README.md') return -1;
            if (b.name === 'README.md') return 1;
            if (a.type !== b.type) return a.type === 'dir' ? -1 : 1;
            return a.name.localeCompare(b.name);
        });

        return nodes;
    }

    router.get(DocsPaths.TREE, (req, res, next) => {
        try {
            if (!fs.existsSync(docsDir)) {
                return res.status(503).json(new ErrorResponse({ error: 'Docs directory not available' }).forClient());
            }
            const tree = buildTree(docsDir);
            if (readmePath && fs.existsSync(readmePath)) {
                tree.unshift({ type: 'file', name: 'README.md', path: '__root__/README.md' });
            }
            res.json(new DocsTreeResponse({ success: true, tree }).forClient());
        } catch (err) {
            logger.error('[DOCS] Failed to build docs tree', { error: err.message });
            res.status(500).json(new ErrorResponse({ error: 'Failed to read docs directory' }).forClient());
        }
    });

    router.get(DocsPaths.FILE, optionalAuth, (req, res, next) => {
        const filePath = req.query.path;
        if (!filePath) {
            return res.status(400).json(new ErrorResponse({ error: 'Missing path parameter' }).forClient());
        }

        if (filePath === '__root__/README.md') {
            if (!readmePath || !fs.existsSync(readmePath)) {
                return res.status(404).json(new ErrorResponse({ error: 'File not found' }).forClient());
            }
            try {
                const content = fs.readFileSync(readmePath, 'utf8');
                return res.json(new DocsFileResponse({ success: true, content, path: filePath }).forClient());
            } catch (err) {
                logger.error('[DOCS] Failed to read README', { error: err.message });
                return res.status(500).json(new ErrorResponse({ error: 'Failed to read file' }).forClient());
            }
        }

        const resolved = path.resolve(docsDir, filePath);
        if (!resolved.startsWith(path.resolve(docsDir) + path.sep) && resolved !== path.resolve(docsDir)) {
            logger.warn('[DOCS] Path traversal attempt blocked', { filePath, userId: req.userId });
            return res.status(403).json(new ErrorResponse({ error: 'Access denied' }).forClient());
        }

        if (!resolved.endsWith('.md')) {
            return res.status(400).json(new ErrorResponse({ error: 'Only .md files are accessible' }).forClient());
        }

        try {
            if (!fs.existsSync(resolved)) {
                return res.status(404).json(new ErrorResponse({ error: 'File not found' }).forClient());
            }
            const content = fs.readFileSync(resolved, 'utf8');
            res.json(new DocsFileResponse({ success: true, content, path: filePath }).forClient());
        } catch (err) {
            logger.error('[DOCS] Failed to read docs file', { filePath, error: err.message });
            res.status(500).json(new ErrorResponse({ error: 'Failed to read file' }).forClient());
        }
    });

    return router;
}

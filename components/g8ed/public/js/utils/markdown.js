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

const COPY_FEEDBACK_DURATION_MS = 2000;
const DEFAULT_MERMAID_THEME = 'dark';

import { escapeHtml as escapeHtmlUtil } from './html.js';

class MarkdownRenderer {
    constructor({ markdownitFactory, domPurify, highlightJs } = {}) {
        /* eslint-disable no-undef */
        this._markdownit = markdownitFactory ?? (typeof markdownit !== 'undefined' ? markdownit : null);
        this._DOMPurify = domPurify ?? (typeof DOMPurify !== 'undefined' ? DOMPurify : null);
        this._hljs = highlightJs ?? (typeof hljs !== 'undefined' ? hljs : null);
        /* eslint-enable no-undef */
        this.md = null;
        this.initializeMarkdownIt();
    }

    initializeMarkdownIt() {
        if (!this._markdownit) {
            throw new Error('markdown-it is required but not loaded');
        }

        this.md = this._markdownit({
            html: false,
            breaks: true,
            linkify: true,
            typographer: false
        });

        this.md.enable('table');

        this.md.renderer.rules.fence = (tokens, idx) => {
            const token = tokens[idx];
            const lang = token.info?.trim();
            const content = token.content;
            
            if (lang === 'mermaid') {
                const codeForAttr = this.escapeAttr(content);
                
                return `<div class="mermaid-wrapper">
                    <div class="code-block-header">
                        <span class="code-lang-label">DIAGRAM</span>
                        <button class="code-copy-btn" data-code="${codeForAttr}" title="Copy diagram code">
                            <span class="material-symbols-outlined">content_copy</span>
                        </button>
                    </div>
                    <pre class="mermaid">${content}</pre>
                </div>`;
            }
            
            let highlightedCode;
            if (lang && this._hljs && this._hljs.getLanguage(lang)) {
                try {
                    highlightedCode = this._hljs.highlight(content, { language: lang }).value;
                } catch (e) {
                    highlightedCode = escapeHtmlUtil(content);
                }
            } else {
                highlightedCode = escapeHtmlUtil(content);
            }
            
            const langLabel = lang ? `<span class="code-lang-label">${lang}</span>` : '';
            const codeForAttr = this.escapeAttr(content);
            
            return `<div class="code-block-wrapper">
                <div class="code-block-header">
                    ${langLabel}
                    <button class="code-copy-btn" data-code="${codeForAttr}" title="Copy code">
                        <span class="material-symbols-outlined">content_copy</span>
                    </button>
                </div>
                <pre><code class="hljs${lang ? ` language-${lang}` : ''}">${highlightedCode}</code></pre>
            </div>`;
        };

        this.md.renderer.rules.code_block = (tokens, idx) => this.md.renderer.rules.fence(tokens, idx);

        this.md.renderer.rules.code_inline = (tokens, idx) => {
            const content = tokens[idx].content;
            return `<code class="inline-code">${escapeHtmlUtil(content)}</code>`;
        };

        this.md.renderer.rules.bullet_list_open = (tokens, idx, options, env, slf) => {
            const token = tokens[idx];
            token.attrJoin('class', 'markdown-list');
            return slf.renderToken(tokens, idx, options);
        };

        this.md.renderer.rules.ordered_list_open = (tokens, idx, options, env, slf) => {
            const token = tokens[idx];
            token.attrJoin('class', 'markdown-list');
            return slf.renderToken(tokens, idx, options);
        };

        this.md.renderer.rules.list_item_open = (tokens, idx, options, env, slf) => {
            const token = tokens[idx];
            token.attrJoin('class', 'markdown-list-item');
            return slf.renderToken(tokens, idx, options);
        };

        this.md.renderer.rules.paragraph_open = () => '<p class="markdown-paragraph">';
        this.md.renderer.rules.paragraph_close = () => '</p>';

        this.md.renderer.rules.heading_open = (tokens, idx, options, env, slf) => {
            const token = tokens[idx];
            const tag = token.tag || 'h1';
            const level = Number.parseInt(tag.replace('h', ''), 10) || 1;
            token.attrJoin('class', `markdown-heading markdown-h${level}`);
            const renderedAttrs = slf.renderAttrs(token);
            return `<${tag}${renderedAttrs}>`;
        };

        this.md.renderer.rules.heading_close = (tokens, idx) => {
            const token = tokens[idx];
            const tag = token.tag || 'h1';
            return `</${tag}>`;
        };

        this.md.renderer.rules.link_open = (tokens, idx, options, env, slf) => {
            const token = tokens[idx];
            const href = token.attrGet('href');
            if (!href) {
                return '';
            }

            const safeHref = escapeHtmlUtil(href);
            const title = token.attrGet('title');
            const safeTitle = title ? ` title="${escapeHtmlUtil(title)}"` : '';
            token.attrSet('href', safeHref);
            if (title) {
                token.attrSet('title', escapeHtmlUtil(title));
            }
            token.attrJoin('class', 'markdown-link');
            token.attrSet('target', '_blank');
            token.attrSet('rel', 'noopener noreferrer');
            return slf.renderToken(tokens, idx, options);
        };

        this.md.renderer.rules.link_close = (tokens, idx, options, env, slf) => slf.renderToken(tokens, idx, options);

        this.md.renderer.rules.strong_open = () => '<strong class="markdown-bold">';
        this.md.renderer.rules.strong_close = () => '</strong>';

        this.md.renderer.rules.em_open = () => '<em class="markdown-italic">';
        this.md.renderer.rules.em_close = () => '</em>';

        this.md.renderer.rules.table_open = () => '<div class="markdown-table-wrapper"><table class="markdown-table">';
        this.md.renderer.rules.table_close = () => '</table></div>';
        
        this.md.renderer.rules.thead_open = () => '<thead class="markdown-table-head">';
        this.md.renderer.rules.thead_close = () => '</thead>';
        
        this.md.renderer.rules.tbody_open = () => '<tbody class="markdown-table-body">';
        this.md.renderer.rules.tbody_close = () => '</tbody>';
        
        this.md.renderer.rules.tr_open = (tokens, idx, options, env, slf) => {
            const token = tokens[idx];
            token.attrJoin('class', 'markdown-table-row');
            return slf.renderToken(tokens, idx, options);
        };
        
        this.md.renderer.rules.th_open = (tokens, idx, options, env, slf) => {
            const token = tokens[idx];
            token.attrJoin('class', 'markdown-table-header-cell');
            const style = token.attrGet('style');
            if (style) {
                const alignMatch = style.match(/text-align:\s*(left|center|right)/);
                if (alignMatch) {
                    token.attrJoin('class', `text-${alignMatch[1]}`);
                }
                token.attrSet('style', null);
            }
            return slf.renderToken(tokens, idx, options);
        };
        
        this.md.renderer.rules.td_open = (tokens, idx, options, env, slf) => {
            const token = tokens[idx];
            token.attrJoin('class', 'markdown-table-cell');
            const style = token.attrGet('style');
            if (style) {
                const alignMatch = style.match(/text-align:\s*(left|center|right)/);
                if (alignMatch) {
                    token.attrJoin('class', `text-${alignMatch[1]}`);
                }
                token.attrSet('style', null);
            }
            return slf.renderToken(tokens, idx, options);
        };
    }

    parseMarkdown(content, streaming = false) {
        if (!content) return '';

        try {
            let html = this.md.render(content);

            if (this._DOMPurify) {
                html = this._DOMPurify.sanitize(html, {
                    ALLOWED_TAGS: [
                        'p', 'br', 'strong', 'em', 'code', 'pre', 'div', 'span', 'sup',
                        'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
                        'ul', 'ol', 'li', 'blockquote', 'a', 'i', 'button', 'img',
                        'table', 'thead', 'tbody', 'tr', 'th', 'td',
                        'svg', 'g', 'path', 'rect', 'circle', 'ellipse', 'line', 'polyline', 'polygon', 'text', 'tspan', 'defs', 'marker', 'use'
                    ],
                    ALLOWED_ATTR: [
                        'class', 'data-code', 'href', 'target', 'rel', 'title', 
                        'aria-label', 'data-source-index', 'data-domain', 'data-citation',
                        'src', 'alt', 'align', 'id',
                        'viewBox', 'width', 'height', 'x', 'y', 'd', 'fill', 'stroke', 'stroke-width',
                        'transform', 'cx', 'cy', 'r', 'rx', 'ry', 'x1', 'y1', 'x2', 'y2', 'points',
                        'text-anchor', 'font-family', 'font-size', 'font-weight', 'dominant-baseline'
                    ],
                    KEEP_CONTENT: true
                });
            }

            if (streaming) {
                html += '<span class="streaming-cursor">▊</span>';
            }

            return html;
        } catch (error) {
            console.error('Markdown parsing error:', error);
            throw error;
        }
    }

    escapeHtml(text) {
        return escapeHtmlUtil(text);
    }

    escapeAttr(text) {
        return text
            .replace(/&/g, '&amp;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#39;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;');
    }

    async renderMermaidDiagrams(container) {
        if (typeof mermaid === 'undefined') {
            console.error('[Mermaid] Mermaid library not loaded');
            return;
        }
        
        const diagrams = container.querySelectorAll('.mermaid:not([data-processed])');
        
        if (diagrams.length === 0) {
            return;
        }
        
        for (const element of diagrams) {
            element.setAttribute('data-processed', 'true');
            
            const content = element.textContent.trim();
            if (!content) {
                continue;
            }

            try {
                const currentTheme = window.ThemeManager ? window.ThemeManager.getTheme() : (document.body.getAttribute('data-theme') || DEFAULT_MERMAID_THEME);
                const themeConfig = currentTheme === 'light' ? {
                    theme: 'base',
                    themeVariables: {
                        primaryTextColor: '#1f2937',
                        textColor: '#374151',
                        primaryColor: '#3b82f6',
                        primaryBorderColor: '#2563eb',
                        lineColor: '#6b7280',
                        background: '#ffffff',
                        mainBkg: '#f3f4f6',
                        secondBkg: '#e5e7eb',
                        tertiaryColor: '#f9fafb',
                        nodeBorder: '#9ca3af',
                        nodeTextColor: '#1f2937',
                        clusterBkg: '#f3f4f6',
                        clusterBorder: '#d1d5db',
                        actorTextColor: '#1f2937',
                        actorBkg: '#f3f4f6',
                        actorBorder: '#9ca3af',
                        signalColor: '#374151',
                        signalTextColor: '#1f2937',
                        labelBoxBkgColor: '#f9fafb',
                        labelBoxBorderColor: '#d1d5db',
                        labelTextColor: '#1f2937',
                        loopTextColor: '#374151',
                        noteBkgColor: '#fef3c7',
                        noteBorderColor: '#f59e0b',
                        noteTextColor: '#92400e',
                        sectionBkgColor: '#f3f4f6',
                        altSectionBkgColor: '#e5e7eb',
                        sectionBkgColor2: '#dbeafe',
                        taskBorderColor: '#3b82f6',
                        taskBkgColor: '#dbeafe',
                        taskTextColor: '#1e40af',
                        taskTextLightColor: '#1f2937',
                        taskTextDarkColor: '#1f2937',
                        edgeLabelBackground: '#ffffff',
                        classText: '#1f2937'
                    }
                } : {
                    theme: 'dark',
                    themeVariables: {
                        primaryTextColor: '#f0f6fc',
                        textColor: '#e5e7eb',
                        primaryColor: '#3b82f6',
                        primaryBorderColor: '#60a5fa',
                        lineColor: '#9ca3af',
                        background: '#0d1117',
                        mainBkg: '#1f2937',
                        secondBkg: '#374151',
                        tertiaryColor: '#161b22',
                        nodeBorder: '#6b7280',
                        nodeTextColor: '#f0f6fc',
                        clusterBkg: '#1f2937',
                        clusterBorder: '#4b5563',
                        actorTextColor: '#f0f6fc',
                        actorBkg: '#1f2937',
                        actorBorder: '#6b7280',
                        signalColor: '#e5e7eb',
                        signalTextColor: '#f0f6fc',
                        labelBoxBkgColor: '#161b22',
                        labelBoxBorderColor: '#4b5563',
                        labelTextColor: '#f0f6fc',
                        loopTextColor: '#e5e7eb',
                        noteBkgColor: '#422006',
                        noteBorderColor: '#f59e0b',
                        noteTextColor: '#fef3c7',
                        sectionBkgColor: '#1f2937',
                        altSectionBkgColor: '#374151',
                        sectionBkgColor2: '#1e3a5f',
                        taskBorderColor: '#60a5fa',
                        taskBkgColor: '#1e3a5f',
                        taskTextColor: '#93c5fd',
                        taskTextLightColor: '#f0f6fc',
                        taskTextDarkColor: '#f0f6fc',
                        edgeLabelBackground: '#1f2937',
                        classText: '#f0f6fc'
                    }
                };
                
                mermaid.initialize({ startOnLoad: false, ...themeConfig });
                
                const id = `mermaid-${crypto.randomUUID()}`;
                const { svg } = await mermaid.render(id, content);
                element.innerHTML = svg;
                element.classList.remove('mermaid');
                element.classList.add('mermaid-rendered');
            } catch (err) {
                console.error('[Mermaid] Render error:', err);
                console.error('[Mermaid] Failed content:', content);
                element.innerHTML = `<div class="mermaid-error">Failed to render diagram: ${escapeHtmlUtil(err.message)}</div>`;
            }
        }
    }

}

document.addEventListener('click', (e) => {
    const copyBtn = e.target.closest('.code-copy-btn');
    if (!copyBtn) return;
    
    const code = copyBtn.getAttribute('data-code');
    if (!code) return;
    
    navigator.clipboard.writeText(code).then(() => {
        const icon = copyBtn.querySelector('.material-symbols-outlined');
        if (icon) {
            const originalIcon = icon.textContent;
            icon.textContent = 'check';
            copyBtn.classList.add('copy-success');
            
            setTimeout(() => {
                icon.textContent = originalIcon;
                copyBtn.classList.remove('copy-success');
            }, COPY_FEEDBACK_DURATION_MS);
        }
    }).catch(err => {
        console.error('Failed to copy code:', err);
        const icon = copyBtn.querySelector('.material-symbols-outlined');
        if (icon) {
            const originalIcon = icon.textContent;
            icon.textContent = 'error';
            copyBtn.classList.add('copy-error');
            
            setTimeout(() => {
                icon.textContent = originalIcon;
                copyBtn.classList.remove('copy-error');
            }, COPY_FEEDBACK_DURATION_MS);
        }
    });
});

export { MarkdownRenderer };

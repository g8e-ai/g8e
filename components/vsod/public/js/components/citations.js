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

import { CitationLayout } from '../constants/events.js';
import { CitationSource, CitationItem } from '../models/ai-event-models.js';
import { escapeHtml } from '../utils/html.js';

export class CitationsHandler {
    constructor() {
        this.globalCard = null;
        this._createGlobalHoverCard();
        this._setupHoverListeners();
    }

    _createGlobalHoverCard() {
        this.globalCard = document.createElement('div');
        this.globalCard.className = 'citation-hover-card citation-card-hidden';
        document.body.appendChild(this.globalCard);
    }

    _setupHoverListeners() {
        document.addEventListener('mouseover', (e) => {
            const wrapper = e.target.closest('.citation-wrapper');
            if (wrapper) {
                const trigger = wrapper.querySelector('.citation-hover-trigger');
                if (trigger) {
                    const citationData = Array.from(trigger.querySelectorAll('.citation-data-item')).map(el =>
                        new CitationItem({
                            uri:         el.getAttribute('data-uri'),
                            displayName: el.getAttribute('data-display-name'),
                            domain:      el.getAttribute('data-domain'),
                            fullTitle:   el.getAttribute('data-full-title') || null,
                            citationNum: Number(el.getAttribute('data-citation-num')),
                        })
                    );
                    this._showHoverCard(wrapper, citationData);
                }
            }
        });

        document.addEventListener('mouseout', (e) => {
            const wrapper = e.target.closest('.citation-wrapper');
            if (wrapper && !wrapper.contains(e.relatedTarget) && !this.globalCard.contains(e.relatedTarget)) {
                this._hideHoverCard();
            }
        });

        this.globalCard.addEventListener('mouseenter', () => {
            this.globalCard.classList.remove('citation-card-hidden');
            this.globalCard.classList.add('citation-card-visible');
        });

        this.globalCard.addEventListener('mouseleave', () => {
            this._hideHoverCard();
        });
    }

    _showHoverCard(wrapper, citationData) {
        if (!citationData.length) return;

        const citationItems = citationData.map(cite => {
            if (!this._isSafeUrl(cite.uri)) {
                console.warn('[Citations] Rejected unsafe URL:', cite.uri);
                return '';
            }

            const displayTitle = cite.fullTitle && cite.fullTitle.trim() && cite.fullTitle !== cite.domain
                ? cite.fullTitle
                : cite.displayName;

            const safeUri = escapeHtml(cite.uri);
            const safeTitle = escapeHtml(displayTitle);
            const safeDomain = escapeHtml(cite.domain);
            const safeCitationNum = escapeHtml(String(cite.citationNum));
            const faviconUrl = `https://www.google.com/s2/favicons?domain=${encodeURIComponent(cite.domain)}&sz=${CitationLayout.FAVICON_SIZE_PX}`;

            return `
                <span class="citation-hover-item-wrapper">
                    <a href="${safeUri}" class="citation-hover-item" target="_blank" rel="noopener noreferrer">
                        <img src="${faviconUrl}" class="citation-hover-favicon" alt="" />
                        <span class="citation-hover-content">
                            <span class="citation-hover-number">${safeCitationNum}</span>
                            <span class="citation-hover-title">${safeTitle}</span>
                            <span class="citation-hover-domain">${safeDomain}</span>
                        </span>
                    </a>
                </span>
            `;
        }).filter(item => item !== '').join('');

        this.globalCard.innerHTML = citationItems;

        const rect = wrapper.getBoundingClientRect();
        const cardWidth = CitationLayout.HOVER_CARD_WIDTH_PX;
        let left = rect.left + rect.width / 2;

        const viewportWidth = window.innerWidth;
        const minLeft = CitationLayout.HOVER_CARD_VIEWPORT_MARGIN_PX;
        const maxLeft = viewportWidth - cardWidth - CitationLayout.HOVER_CARD_VIEWPORT_MARGIN_PX;

        if (left < minLeft + cardWidth / 2) {
            left = minLeft;
            this.globalCard.style.setProperty('--card-transform', 'translateX(0)');
        } else if (left > maxLeft + cardWidth / 2) {
            left = maxLeft;
            this.globalCard.style.setProperty('--card-transform', 'translateX(0)');
        } else {
            this.globalCard.style.setProperty('--card-transform', 'translateX(-50%)');
        }

        this.globalCard.style.setProperty('left', `${left}px`);
        this.globalCard.style.setProperty('top', `${rect.bottom + 4}px`);
        this.globalCard.classList.remove('citation-card-hidden');
        this.globalCard.classList.add('citation-card-visible');
    }

    _hideHoverCard() {
        this.globalCard.classList.remove('citation-card-visible');
        this.globalCard.classList.add('citation-card-hidden');
    }

    addInlineCitations(htmlContent, groundingMetadata) {
        try {
            if (!groundingMetadata || !groundingMetadata.grounding_used) {
                return htmlContent;
            }

            const supports = groundingMetadata.grounding_supports;
            const sources = groundingMetadata.sources;

            if (!supports?.length || !sources?.length) {
                return htmlContent;
            }

            const tempDiv = document.createElement('div');
            tempDiv.innerHTML = htmlContent;
            const plainText = tempDiv.textContent;

            const codeBlockRanges = this._getCodeBlockRanges(tempDiv, plainText);

            const positionToCitations = new Map();

            supports.forEach(support => {
                const startIndex = support.start_index || 0;
                const endIndex = support.end_index;
                const chunkIndices = support.grounding_chunk_indices;

                if (!chunkIndices.length || endIndex > plainText.length) return;

                if (this._isHeadingOnlySegment(tempDiv, plainText, startIndex, endIndex)) {
                    return;
                }

                const adjustedIndex = this._findSmartCitationPoint(plainText, tempDiv, endIndex);

                if (this._isPositionInCodeBlock(adjustedIndex, codeBlockRanges)) {
                    return;
                }

                if (!positionToCitations.has(adjustedIndex)) {
                    positionToCitations.set(adjustedIndex, []);
                }
                positionToCitations.get(adjustedIndex).push(...chunkIndices);
            });

            const sortedPositions = Array.from(positionToCitations.keys()).sort((a, b) => b - a);

            sortedPositions.forEach(position => {
                const chunkIndices = positionToCitations.get(position);
                const uniqueChunks = [...new Set(chunkIndices)];

                const citationNumbers = [];
                const citationData = [];
                uniqueChunks.forEach(chunkIdx => {
                    if (chunkIdx < sources.length) {
                        const source = CitationSource.parse(sources[chunkIdx]);
                        citationNumbers.push(source.citation_num);
                        citationData.push(new CitationItem({
                            uri: source.uri,
                            displayName: source.display_name,
                            domain: source.domain,
                            fullTitle: source.full_title,
                            citationNum: source.citation_num,
                        }));
                    }
                });

                if (citationNumbers.length > 0) {
                    const numbersDisplay = citationNumbers.join(',');
                    const hoverCardContent = this._buildCitationHoverCard(citationData);
                    const citationGroup = `<span class="citation-wrapper"><sup class="citation-superscript" data-citations="${numbersDisplay}">[${numbersDisplay}]</sup>${hoverCardContent}</span>`;
                    this._insertCitationAtTextIndex(tempDiv, position, citationGroup);
                }
            });

            return tempDiv.innerHTML;

        } catch (error) {
            console.warn('[Citations] Failed to add inline citations:', error);
            return htmlContent;
        }
    }

    _buildCitationHoverCard(citationData) {
        const itemElements = citationData.map(item => {
            const uri         = escapeHtml(item.uri);
            const displayName = escapeHtml(item.displayName);
            const domain      = escapeHtml(item.domain);
            const fullTitle   = item.fullTitle ? escapeHtml(item.fullTitle) : '';
            const citationNum = escapeHtml(String(item.citationNum));
            return `<span class="citation-data-item" data-uri="${uri}" data-display-name="${displayName}" data-domain="${domain}" data-full-title="${fullTitle}" data-citation-num="${citationNum}"></span>`;
        }).join('');
        return `<span class="citation-hover-trigger">${itemElements}</span>`;
    }

    renderSourcesPanel(sources) {
        const panel = document.createElement('div');
        panel.className = 'sources-panel';

        const header = document.createElement('div');
        header.className = 'sources-panel-header';

        const title = document.createElement('div');
        title.className = 'sources-panel-title';
        title.innerHTML = `
            <span class="material-symbols-outlined icon-18">menu_book</span> Sources
            <span class="sources-panel-count">${sources.length}</span>
        `;

        const toggle = document.createElement('div');
        toggle.className = 'sources-panel-toggle';
        toggle.textContent = '▼';

        header.appendChild(title);
        header.appendChild(toggle);

        const list = document.createElement('div');
        list.className = 'sources-list initially-hidden';

        sources.forEach((source, index) => {
            const src = CitationSource.parse(source);
            const item = document.createElement('a');
            item.className = 'source-item source-link';
            item.href = src.uri;
            item.target = '_blank';
            item.rel = 'noopener noreferrer';

            const faviconUrl = src.favicon_url || `https://www.google.com/s2/favicons?domain=${encodeURIComponent(src.domain)}&sz=${CitationLayout.FAVICON_SIZE_PX}`;
            const pageTitle = src.full_title && src.full_title.trim() && src.full_title !== src.domain
                ? src.full_title
                : src.display_name;
            const citationNumber = src.citation_num || (index + 1);

            let segmentsHtml = '';
            if (src.segments && src.segments.length > 0) {
                const snippets = src.segments.map(seg =>
                    `<div class="source-snippet">"${escapeHtml(seg)}"</div>`
                ).join('');
                segmentsHtml = `<div class="source-snippets">${snippets}</div>`;
            }

            item.innerHTML = `
                <img src="${faviconUrl}" class="source-favicon" alt="${escapeHtml(src.display_name)}" />
                <div class="source-content">
                    <div class="source-header">
                        <span class="source-citation-number">${citationNumber}</span>
                        <div class="source-title">${escapeHtml(pageTitle)}</div>
                    </div>
                    ${segmentsHtml}
                    <div class="source-domain">
                        <span class="material-symbols-outlined source-domain-icon icon-14">link</span>
                        ${escapeHtml(src.domain)}
                    </div>
                </div>
            `;

            const favicon = item.querySelector('.source-favicon');
            if (favicon) {
                favicon.addEventListener('error', () => favicon.classList.add('initially-hidden'));
            }

            list.appendChild(item);
        });

        header.addEventListener('click', () => {
            const isExpanded = !list.classList.contains('initially-hidden');
            list.classList.toggle('initially-hidden', isExpanded);
            toggle.classList.toggle('expanded', !isExpanded);
        });

        panel.appendChild(header);
        panel.appendChild(list);

        return panel;
    }

    _isHeadingOnlySegment(containerDiv, plainText, startIndex, endIndex) {
        const segmentText = plainText.substring(startIndex, endIndex).trim();
        if (!segmentText) return true;

        const headings = containerDiv.querySelectorAll('h1, h2, h3, h4, h5, h6');
        for (const heading of headings) {
            const headingText = heading.textContent.trim();
            if (segmentText === headingText || headingText.startsWith(segmentText)) {
                return true;
            }
        }

        if (segmentText.endsWith(':') && segmentText.length < CitationLayout.HEADING_SEGMENT_MAX_LENGTH && !segmentText.includes('.')) {
            return true;
        }

        return false;
    }

    _findSmartCitationPoint(plainText, containerDiv, endIndex) {
        const sentenceEnders = ['.', '!', '?'];
        for (let i = endIndex; i < Math.min(endIndex + CitationLayout.SENTENCE_LOOKAHEAD_CHARS, plainText.length); i++) {
            if (sentenceEnders.includes(plainText[i])) {
                const candidatePos = i + 1;
                if (!this._isPositionInHeading(containerDiv, plainText, candidatePos)) {
                    return candidatePos;
                }
            }
        }

        if (plainText[endIndex] === ':') {
            const candidatePos = endIndex + 1;
            if (!this._isPositionInHeading(containerDiv, plainText, candidatePos)) {
                return candidatePos;
            }
        }

        if (!this._isPositionInHeading(containerDiv, plainText, endIndex)) {
            return endIndex;
        }

        for (let i = endIndex; i < Math.min(endIndex + CitationLayout.PARAGRAPH_LOOKAHEAD_CHARS, plainText.length); i++) {
            if (plainText[i] === '\n' && plainText[i + 1] && plainText[i + 1] !== '\n') {
                return i + 1;
            }
        }

        return endIndex;
    }

    _isPositionInHeading(containerDiv, plainText, targetPos) {
        const headings = containerDiv.querySelectorAll('h1, h2, h3, h4, h5, h6');
        let currentPos = 0;

        const walker = document.createTreeWalker(
            containerDiv,
            NodeFilter.SHOW_TEXT,
            null,
            false
        );

        let textNode;
        while (textNode = walker.nextNode()) {
            const nodeLength = textNode.textContent.length;
            const nodeStart = currentPos;
            const nodeEnd = currentPos + nodeLength;

            if (targetPos >= nodeStart && targetPos <= nodeEnd) {
                let parent = textNode.parentElement;
                while (parent && parent !== containerDiv) {
                    if (/^H[1-6]$/i.test(parent.tagName)) {
                        return true;
                    }
                    parent = parent.parentElement;
                }
                return false;
            }

            currentPos += nodeLength;
        }

        return false;
    }

    _isSafeUrl(url) {
        if (!url || typeof url !== 'string') return false;
        try {
            const parsed = new URL(url, window.location.origin);
            return parsed.protocol === 'http:' || parsed.protocol === 'https:';
        } catch {
            return false;
        }
    }

    _insertCitationAtTextIndex(containerElement, targetIndex, citationHTML) {
        let currentIndex = 0;
        const walker = document.createTreeWalker(
            containerElement,
            NodeFilter.SHOW_TEXT,
            null,
            false
        );

        let textNode;
        while (textNode = walker.nextNode()) {
            const nodeLength = textNode.textContent.length;

            if (currentIndex + nodeLength >= targetIndex) {
                let parent = textNode.parentElement;
                let inCodeBlock = false;
                while (parent && parent !== containerElement) {
                    if (parent.tagName === 'PRE' || parent.tagName === 'CODE') {
                        inCodeBlock = true;
                        break;
                    }
                    parent = parent.parentElement;
                }

                if (inCodeBlock) {
                    currentIndex += nodeLength;
                    continue;
                }

                const offsetInNode = targetIndex - currentIndex;
                const range = document.createRange();
                range.setStart(textNode, offsetInNode);
                range.setEnd(textNode, offsetInNode);

                const tempDiv = document.createElement('div');
                tempDiv.innerHTML = citationHTML;
                const citationNode = tempDiv.firstChild;

                range.insertNode(citationNode);
                return;
            }

            currentIndex += nodeLength;
        }
    }

    _getCodeBlockRanges(container, plainText) {
        const ranges = [];
        const codeElements = container.querySelectorAll('pre, code');

        codeElements.forEach(codeEl => {
            const codeText = codeEl.textContent;
            if (!codeText) return;

            let currentIndex = 0;
            const walker = document.createTreeWalker(
                container,
                NodeFilter.SHOW_TEXT,
                null,
                false
            );

            let textNode;
            while (textNode = walker.nextNode()) {
                if (codeEl.contains(textNode)) {
                    const startPos = currentIndex;
                    const endPos = currentIndex + textNode.textContent.length;
                    ranges.push({ start: startPos, end: endPos });
                }
                currentIndex += textNode.textContent.length;
            }
        });

        ranges.sort((a, b) => a.start - b.start);
        const merged = [];
        for (const range of ranges) {
            if (merged.length === 0 || merged[merged.length - 1].end < range.start) {
                merged.push(range);
            } else {
                merged[merged.length - 1].end = Math.max(merged[merged.length - 1].end, range.end);
            }
        }

        return merged;
    }

    _isPositionInCodeBlock(position, ranges) {
        return ranges.some(range => position >= range.start && position <= range.end);
    }
}

window.CitationsHandler = CitationsHandler;

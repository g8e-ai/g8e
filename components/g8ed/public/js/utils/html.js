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

let _decoder = null;
let _escaper = null;

function _getDecoder() {
    if (!_decoder) {
        _decoder = document.createElement('textarea');
    }
    return _decoder;
}

function _getEscaper() {
    if (!_escaper) {
        _escaper = document.createElement('div');
    }
    return _escaper;
}

export function escapeHtml(text) {
    if (text === null || text === undefined) return '';
    const el = _getEscaper();
    el.textContent = String(text);
    return el.innerHTML;
}

export function decodeHtmlEntities(text) {
    if (text === null || text === undefined) {
        return '';
    }
    const decoder = _getDecoder();
    decoder.innerHTML = text;
    return decoder.value.replace(/\u00a0/g, ' ');
}

/**
 * Escapes text for safe use in HTML attributes.
 * Prevents attribute injection by escaping quotes and other special characters.
 * @param {string} text - Text to escape
 * @returns {string} Escaped text safe for HTML attributes
 */
export function escapeHtmlAttribute(text) {
    if (!text) return '';
    const escaped = escapeHtml(text);
    return escaped
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

/**
 * Escapes text for safe use in URL attributes (href, src).
 * Combines URL encoding with HTML escaping for defense in depth.
 * @param {string} text - Text to escape
 * @returns {string} Escaped text safe for URL attributes
 */
export function escapeUrl(text) {
    if (!text) return '';
    try {
        return encodeURIComponent(text).replace(/'/g, '%27').replace(/"/g, '%22');
    } catch (e) {
        return '';
    }
}

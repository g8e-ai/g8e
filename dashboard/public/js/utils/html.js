// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

let _decoder = null;

function _getDecoder() {
    if (!_decoder) {
        _decoder = document.createElement('textarea');
    }
    return _decoder;
}

export function decodeHtmlEntities(text) {
    if (text === null || text === undefined) {
        return '';
    }
    const decoder = _getDecoder();
    decoder.innerHTML = text;
    return decoder.value.replace(/\u00a0/g, ' ');
}

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

/**
 * OperatorLayoutMixin - Panel resize.
 *
 * Covers: drag-to-resize divider between operator and chat panels.
 *
 * Mixed into OperatorPanel via Object.assign(OperatorPanel.prototype, OperatorLayoutMixin).
 */
export const OperatorLayoutMixin = {

    _initPanelResize() {
        const divider = document.getElementById('panel-divider');
        const panelContainer = document.getElementById('operator-panel-container');

        if (!divider || !panelContainer) return;

        // Cleanup previous listeners if any
        if (this._panelResizeAbortController) {
            this._panelResizeAbortController.abort();
        }
        this._panelResizeAbortController = new AbortController();
        const { signal } = this._panelResizeAbortController;

        const MIN_PX = 240;
        const MAX_FRACTION = 0.80;
        const DRAG_THRESHOLD_PX = 4;
        const DEFAULT_EXPANDED_PX = 440;

        let pointerDown = false;
        let dragging = false;
        let startX = 0;
        let startWidth = 0;
        // Remembers the last expanded width so re-opening restores the user's size.
        this._lastExpandedWidth = this._lastExpandedWidth || DEFAULT_EXPANDED_PX;

        const getParentWidth = () => panelContainer.parentElement.getBoundingClientRect().width;

        const clamp = (width) => {
            const maxPx = getParentWidth() * MAX_FRACTION;
            return Math.max(MIN_PX, Math.min(maxPx, width));
        };

        const isCollapsed = () => panelContainer.classList.contains('collapsed');

        const setCollapsed = (collapsed) => {
            if (collapsed) {
                const current = panelContainer.getBoundingClientRect().width;
                if (current > 0) this._lastExpandedWidth = current;
                panelContainer.classList.add('collapsed');
                divider.classList.add('collapsed');
                divider.style.cursor = 'pointer';
            } else {
                panelContainer.classList.remove('collapsed');
                divider.classList.remove('collapsed');
                divider.style.cursor = '';
                panelContainer.style.width = `${clamp(this._lastExpandedWidth || DEFAULT_EXPANDED_PX)}px`;
            }
        };

        const toggleCollapsed = () => setCollapsed(!isCollapsed());

        const beginPointer = (clientX) => {
            if (panelContainer.classList.contains('mobile-drawer-mode')) return false;
            pointerDown = true;
            dragging = false;
            startX = clientX;
            startWidth = panelContainer.getBoundingClientRect().width;
            return true;
        };

        const movePointer = (clientX) => {
            if (!pointerDown) return;
            const delta = clientX - startX;
            if (!dragging) {
                if (Math.abs(delta) < DRAG_THRESHOLD_PX) return;
                // Promote to drag. Drag cancels any collapsed state so the user
                // can size the panel by pulling the handle outward.
                dragging = true;
                if (isCollapsed()) {
                    panelContainer.classList.remove('collapsed');
                    divider.classList.remove('collapsed');
                    startWidth = 0;
                }
                divider.classList.add('dragging');
                document.body.style.cursor = 'col-resize';
                document.body.style.userSelect = 'none';
            }
            panelContainer.style.width = `${clamp(startWidth + delta)}px`;
        };

        const endPointer = () => {
            if (!pointerDown) return;
            const wasDragging = dragging;
            pointerDown = false;
            dragging = false;
            divider.classList.remove('dragging');
            document.body.style.cursor = '';
            document.body.style.userSelect = '';
            if (!wasDragging) {
                // Treat as click -> toggle collapse.
                toggleCollapsed();
            } else {
                // Persist last expanded width after a drag finishes.
                this._lastExpandedWidth = panelContainer.getBoundingClientRect().width;
                divider.style.cursor = '';
            }
        };

        const onMouseDown = (e) => {
            if (!beginPointer(e.clientX)) return;
            e.preventDefault();
        };
        const onMouseMove = (e) => movePointer(e.clientX);
        const onMouseUp = () => endPointer();

        const onTouchStart = (e) => {
            const touch = e.touches[0];
            if (!beginPointer(touch.clientX)) return;
            e.preventDefault();
        };
        const onTouchMove = (e) => {
            if (!pointerDown) return;
            const touch = e.touches[0];
            movePointer(touch.clientX);
        };
        const onTouchEnd = () => endPointer();

        divider.addEventListener('mousedown', onMouseDown, { signal });
        document.addEventListener('mousemove', onMouseMove, { signal });
        document.addEventListener('mouseup', onMouseUp, { signal });
        divider.addEventListener('touchstart', onTouchStart, { passive: false, signal });
        document.addEventListener('touchmove', onTouchMove, { passive: false, signal });
        document.addEventListener('touchend', onTouchEnd, { signal });
    },

};


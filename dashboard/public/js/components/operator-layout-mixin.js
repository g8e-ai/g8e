// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

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

        const MIN_PX = 240;
        const MAX_FRACTION = 0.80;

        let dragging = false;
        let startX = 0;
        let startWidth = 0;

        const getParentWidth = () => panelContainer.parentElement.getBoundingClientRect().width;

        const clamp = (width) => {
            const maxPx = getParentWidth() * MAX_FRACTION;
            return Math.max(MIN_PX, Math.min(maxPx, width));
        };

        const onMouseDown = (e) => {
            if (panelContainer.classList.contains('mobile-drawer-mode')) return;
            dragging = true;
            startX = e.clientX;
            startWidth = panelContainer.getBoundingClientRect().width;
            divider.classList.add('dragging');
            document.body.style.cursor = 'col-resize';
            document.body.style.userSelect = 'none';
            e.preventDefault();
        };

        const onMouseMove = (e) => {
            if (!dragging) return;
            const delta = e.clientX - startX;
            panelContainer.style.width = `${clamp(startWidth + delta)}px`;
        };

        const onMouseUp = () => {
            if (!dragging) return;
            dragging = false;
            divider.classList.remove('dragging');
            document.body.style.cursor = '';
            document.body.style.userSelect = '';
        };

        const onTouchStart = (e) => {
            if (panelContainer.classList.contains('mobile-drawer-mode')) return;
            const touch = e.touches[0];
            dragging = true;
            startX = touch.clientX;
            startWidth = panelContainer.getBoundingClientRect().width;
            divider.classList.add('dragging');
            e.preventDefault();
        };

        const onTouchMove = (e) => {
            if (!dragging) return;
            const touch = e.touches[0];
            const delta = touch.clientX - startX;
            panelContainer.style.width = `${clamp(startWidth + delta)}px`;
        };

        const onTouchEnd = () => {
            dragging = false;
            divider.classList.remove('dragging');
        };

        divider.addEventListener('mousedown', onMouseDown);
        document.addEventListener('mousemove', onMouseMove);
        document.addEventListener('mouseup', onMouseUp);
        divider.addEventListener('touchstart', onTouchStart, { passive: false });
        document.addEventListener('touchmove', onTouchMove, { passive: false });
        document.addEventListener('touchend', onTouchEnd);
    },

};


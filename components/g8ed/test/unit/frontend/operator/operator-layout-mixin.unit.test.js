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

// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { OperatorLayoutMixin } from '@g8ed/public/js/components/operator-layout-mixin.js';

describe('OperatorLayoutMixin [UNIT - jsdom]', () => {
    let divider;
    let panelContainer;
    let parentElement;
    let ctx;

    beforeEach(() => {
        // Setup DOM
        document.body.innerHTML = `
            <div id="parent" style="width: 1000px;">
                <div id="operator-panel-container" style="width: 300px;">
                    <div id="panel-divider"></div>
                </div>
            </div>
        `;

        divider = document.getElementById('panel-divider');
        panelContainer = document.getElementById('operator-panel-container');
        parentElement = document.getElementById('parent');

        // Mock getBoundingClientRect for parent (used for max width calculation)
        vi.spyOn(parentElement, 'getBoundingClientRect').mockReturnValue({ width: 1000 });
        // Mock getBoundingClientRect for panelContainer (used for initial width)
        vi.spyOn(panelContainer, 'getBoundingClientRect').mockReturnValue({ width: 300 });

        // Create mixin context
        ctx = Object.create(null);
        Object.assign(ctx, OperatorLayoutMixin);
    });

    afterEach(() => {
        if (ctx._panelResizeAbortController) {
            ctx._panelResizeAbortController.abort();
        }
        vi.restoreAllMocks();
        document.body.innerHTML = '';
    });

    describe('_initPanelResize', () => {
        it('returns early if elements are missing', () => {
            document.body.innerHTML = '';
            expect(() => ctx._initPanelResize()).not.toThrow();
        });

        it('attaches event listeners to divider and document', () => {
            const dividerSpy = vi.spyOn(divider, 'addEventListener');
            const docSpy = vi.spyOn(document, 'addEventListener');

            ctx._initPanelResize();

            expect(dividerSpy).toHaveBeenCalledWith('mousedown', expect.any(Function), expect.objectContaining({ signal: expect.any(AbortSignal) }));
            expect(dividerSpy).toHaveBeenCalledWith('touchstart', expect.any(Function), expect.objectContaining({ passive: false, signal: expect.any(AbortSignal) }));
            expect(docSpy).toHaveBeenCalledWith('mousemove', expect.any(Function), expect.objectContaining({ signal: expect.any(AbortSignal) }));
            expect(docSpy).toHaveBeenCalledWith('mouseup', expect.any(Function), expect.objectContaining({ signal: expect.any(AbortSignal) }));
            expect(docSpy).toHaveBeenCalledWith('touchmove', expect.any(Function), expect.objectContaining({ passive: false, signal: expect.any(AbortSignal) }));
            expect(docSpy).toHaveBeenCalledWith('touchend', expect.any(Function), expect.objectContaining({ signal: expect.any(AbortSignal) }));
        });

        describe('Mouse resizing', () => {
            it('starts dragging on mousedown + mousemove past threshold and updates width', () => {
                ctx._initPanelResize();

                // Mousedown alone should NOT enter dragging state (click vs drag discrimination).
                divider.dispatchEvent(new MouseEvent('mousedown', { clientX: 100 }));
                expect(divider.classList.contains('dragging')).toBe(false);

                // Small movement under threshold (4px) still should not promote to drag.
                document.dispatchEvent(new MouseEvent('mousemove', { clientX: 102 }));
                expect(divider.classList.contains('dragging')).toBe(false);

                // Movement beyond threshold promotes to drag and applies the resize.
                document.dispatchEvent(new MouseEvent('mousemove', { clientX: 150 }));
                expect(divider.classList.contains('dragging')).toBe(true);
                expect(document.body.style.cursor).toBe('col-resize');
                expect(panelContainer.style.width).toBe('350px');
            });

            it('stops dragging on mouseup', () => {
                ctx._initPanelResize();

                // Start dragging (requires movement past threshold).
                divider.dispatchEvent(new MouseEvent('mousedown', { clientX: 100 }));
                document.dispatchEvent(new MouseEvent('mousemove', { clientX: 150 }));
                expect(divider.classList.contains('dragging')).toBe(true);

                // Stop dragging
                document.dispatchEvent(new MouseEvent('mouseup'));

                expect(divider.classList.contains('dragging')).toBe(false);
                expect(document.body.style.cursor).toBe('');

                // Further moves should not change width
                const widthAfterDrag = panelContainer.style.width;
                document.dispatchEvent(new MouseEvent('mousemove', { clientX: 400 }));
                expect(panelContainer.style.width).toBe(widthAfterDrag);
            });

            it('treats mousedown + mouseup with no drag as a click that toggles collapsed state', () => {
                ctx._initPanelResize();

                // Click (no movement) -> collapse.
                divider.dispatchEvent(new MouseEvent('mousedown', { clientX: 100 }));
                document.dispatchEvent(new MouseEvent('mouseup'));
                expect(panelContainer.classList.contains('collapsed')).toBe(true);
                expect(divider.classList.contains('collapsed')).toBe(true);
                expect(divider.classList.contains('dragging')).toBe(false);

                // Click again -> expand (restored width written inline).
                divider.dispatchEvent(new MouseEvent('mousedown', { clientX: 100 }));
                document.dispatchEvent(new MouseEvent('mouseup'));
                expect(panelContainer.classList.contains('collapsed')).toBe(false);
                expect(divider.classList.contains('collapsed')).toBe(false);
                expect(panelContainer.style.width).toMatch(/px$/);
            });

            it('does not collapse when pointer moves past threshold (drag, not click)', () => {
                ctx._initPanelResize();

                divider.dispatchEvent(new MouseEvent('mousedown', { clientX: 100 }));
                document.dispatchEvent(new MouseEvent('mousemove', { clientX: 160 }));
                document.dispatchEvent(new MouseEvent('mouseup'));

                expect(panelContainer.classList.contains('collapsed')).toBe(false);
            });

            it('does not start dragging if mobile-drawer-mode class is present', () => {
                panelContainer.classList.add('mobile-drawer-mode');
                ctx._initPanelResize();

                divider.dispatchEvent(new MouseEvent('mousedown', { clientX: 100 }));

                expect(divider.classList.contains('dragging')).toBe(false);
            });

            it('clamps width to MIN_PX (240px)', () => {
                ctx._initPanelResize();

                // Start at 300px, drag left by 100px (target 200px)
                divider.dispatchEvent(new MouseEvent('mousedown', { clientX: 300 }));
                document.dispatchEvent(new MouseEvent('mousemove', { clientX: 200 }));

                expect(panelContainer.style.width).toBe('240px');
            });

            it('clamps width to MAX_FRACTION (80% of parent)', () => {
                ctx._initPanelResize();

                // Parent is 1000px, max should be 800px
                // Start at 300px, drag right by 600px (target 900px)
                divider.dispatchEvent(new MouseEvent('mousedown', { clientX: 300 }));
                document.dispatchEvent(new MouseEvent('mousemove', { clientX: 900 }));

                expect(panelContainer.style.width).toBe('800px');
            });
        });

        describe('Touch resizing', () => {
            it('starts dragging on touchstart + touchmove past threshold and updates width', () => {
                ctx._initPanelResize();

                // Touchstart alone should not enter dragging state.
                divider.dispatchEvent(new TouchEvent('touchstart', {
                    touches: [{ clientX: 100 }]
                }));
                expect(divider.classList.contains('dragging')).toBe(false);

                // Touchmove past threshold promotes to drag.
                document.dispatchEvent(new TouchEvent('touchmove', {
                    touches: [{ clientX: 150 }]
                }));

                expect(divider.classList.contains('dragging')).toBe(true);
                expect(panelContainer.style.width).toBe('350px');
            });

            it('stops dragging on touchend', () => {
                ctx._initPanelResize();

                // Start dragging (touchstart + touchmove past threshold).
                divider.dispatchEvent(new TouchEvent('touchstart', {
                    touches: [{ clientX: 100 }]
                }));
                document.dispatchEvent(new TouchEvent('touchmove', {
                    touches: [{ clientX: 150 }]
                }));
                expect(divider.classList.contains('dragging')).toBe(true);

                // Stop dragging
                document.dispatchEvent(new TouchEvent('touchend'));

                expect(divider.classList.contains('dragging')).toBe(false);
            });

            it('treats tap (touchstart + touchend with no movement) as a click toggling collapsed', () => {
                ctx._initPanelResize();

                divider.dispatchEvent(new TouchEvent('touchstart', {
                    touches: [{ clientX: 100 }]
                }));
                document.dispatchEvent(new TouchEvent('touchend'));

                expect(panelContainer.classList.contains('collapsed')).toBe(true);
                expect(divider.classList.contains('collapsed')).toBe(true);
            });

            it('does not start dragging on touchstart if mobile-drawer-mode class is present', () => {
                panelContainer.classList.add('mobile-drawer-mode');
                ctx._initPanelResize();

                const touchStartEvent = new TouchEvent('touchstart', {
                    touches: [{ clientX: 100 }]
                });
                divider.dispatchEvent(touchStartEvent);

                expect(divider.classList.contains('dragging')).toBe(false);
            });
        });
    });
});

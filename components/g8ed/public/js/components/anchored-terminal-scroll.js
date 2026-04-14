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

export class TerminalScrollMixin {
    initScrollState() {
        this.userHasScrolled = true;
        this.scrollThreshold = 100;
        this._scrollRafId = null;
        this.isResizing = false;
        this.resizeStartY = 0;
        this.resizeStartTerminalHeight = 0;
        this.resizeStartMessagesHeight = 0;
    }

    bindScrollListener() {
        if (!this.scrollContainer) return;

        this.scrollContainer.addEventListener('scroll', () => {
            const isNearBottom = this.isNearBottom(this.scrollContainer);

            if (!isNearBottom) {
                this.userHasScrolled = true;
            } else {
                this.userHasScrolled = false;
            }
        }, { passive: true });
    }

    isNearBottom(container) {
        if (!container) return true;
        const { scrollTop, scrollHeight, clientHeight } = container;
        return scrollHeight - scrollTop - clientHeight < this.scrollThreshold;
    }

    resetAutoScroll() {
        this.userHasScrolled = false;
    }

    scrollToBottom(options = {}) {
        const { force = false, smooth = false } = options;

        if (this.userHasScrolled && !force) {
            return;
        }

        if (this._scrollRafId !== null) {
            cancelAnimationFrame(this._scrollRafId);
        }

        this._scrollRafId = requestAnimationFrame(() => {
            this._scrollRafId = null;
            if (!this.scrollContainer) return;
            if (smooth) {
                this.scrollContainer.scrollTo({ top: this.scrollContainer.scrollHeight, behavior: 'smooth' });
            } else {
                this.scrollContainer.scrollTop = this.scrollContainer.scrollHeight;
            }
        });
    }

    startResize(e) {
        const chatMessagesPanel = document.getElementById('chat-messages');
        if (!this.terminal || !chatMessagesPanel) return;

        e.preventDefault();
        this.isResizing = true;
        this.resizeStartY = e.clientY;
        this.resizeStartTerminalHeight = this.terminal.offsetHeight;
        this.resizeStartMessagesHeight = chatMessagesPanel.offsetHeight;

        this.resizeHandle.classList.add('dragging');
        document.body.style.cursor = 'ns-resize';
        document.body.style.userSelect = 'none';

        this._boundHandleResize = (e) => this.handleResize(e);
        this._boundStopResize = () => this.stopResize();
        document.addEventListener('mousemove', this._boundHandleResize);
        document.addEventListener('mouseup', this._boundStopResize);
    }

    handleResize(e) {
        if (!this.isResizing) return;

        const chatMessagesPanel = document.getElementById('chat-messages');
        if (!chatMessagesPanel) return;

        const deltaY = this.resizeStartY - e.clientY;
        const newTerminalHeight = this.resizeStartTerminalHeight + deltaY;
        const newMessagesHeight = this.resizeStartMessagesHeight - deltaY;

        const minTerminalHeight = 180;
        const maxTerminalHeight = 600;
        const minMessagesHeight = 150;

        if (newTerminalHeight < minTerminalHeight || newTerminalHeight > maxTerminalHeight) return;
        if (newMessagesHeight < minMessagesHeight) return;

        this.terminal.style.maxHeight = `${newTerminalHeight}px`;
        this.terminal.style.minHeight = `${newTerminalHeight}px`;
        chatMessagesPanel.style.flex = 'none';
        chatMessagesPanel.style.height = `${newMessagesHeight}px`;
    }

    stopResize() {
        if (!this.isResizing) return;

        this.isResizing = false;
        this.resizeHandle.classList.remove('dragging');
        document.body.style.cursor = '';
        document.body.style.userSelect = '';

        document.removeEventListener('mousemove', this._boundHandleResize);
        document.removeEventListener('mouseup', this._boundStopResize);
    }
}

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

import { vi } from 'vitest';

/**
 * Creates a comprehensive spy for AnchoredTerminal with all known methods.
 * This unified spy prevents test breakage when new terminal methods are added.
 * 
 * @returns {Object} A vitest spy object with all AnchoredTerminal methods mocked
 */
export function makeAnchoredTerminalSpy() {
    return {
        finalizeAIResponseChunk: vi.fn(),
        clearActivityIndicators: vi.fn(),
        appendDirectHtmlResponse: vi.fn(() => document.createElement('div')),
        replaceStreamingHtml: vi.fn(),
        appendUserMessage: vi.fn(() => document.createElement('div')),
        appendSystemMessage: vi.fn(() => document.createElement('div')),
        appendErrorMessage: vi.fn(),
        applyCitations: vi.fn(),
        completeActivityIndicator: vi.fn(),
        sealStreamingResponse: vi.fn(),
        resetAutoScroll: vi.fn(),
        showWaitingIndicator: vi.fn(),
        hideWaitingIndicator: vi.fn(),
        clear: vi.fn(),
        focus: vi.fn(),
        enable: vi.fn(),
        disable: vi.fn(),
        setUser: vi.fn(),
        clearOutput: vi.fn(),
        scrollToBottom: vi.fn(),
        restoreCommandExecution: vi.fn(),
        restoreCommandResult: vi.fn(),
        restoreApprovalRequest: vi.fn(),
        appendStreamingTextChunk: vi.fn(),
        appendThinkingContent: vi.fn(),
        completeThinkingEntry: vi.fn(),
        appendActivityIndicator: vi.fn(),
        showTribunal: vi.fn(),
        updateTribunalPass: vi.fn(),
        updateTribunalStatus: vi.fn(),
        completeTribunal: vi.fn(),
        failTribunal: vi.fn(),
        pendingApprovals: new Map(),
        activeExecutions: new Map(),
        executionResultsContainers: new Map(),
    };
}

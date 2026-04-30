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
 * TutorialManager handles the interactive onboarding tutorial using driver.js
 */
export class TutorialManager {
    constructor() {
        this.driver = null;
        this.isInitialized = false;
    }

    init() {
        if (typeof window.driver === 'undefined') {
            console.error('[TutorialManager] driver.js not loaded');
            return;
        }

        this.driver = window.driver.js.driver({
            showProgress: true,
            animate: true,
            allowClose: true,
            stagePadding: 5,
            nextBtnText: 'Next',
            prevBtnText: 'Previous',
            doneBtnText: 'Got it!',
            theme: {
                popover: {
                    className: 'g8e-tutorial-popover',
                    titleStyle: {
                        color: 'var(--text-bright)',
                        fontSize: '18px',
                        fontWeight: '600'
                    },
                    descriptionStyle: {
                        color: 'var(--text-primary)',
                        fontSize: '14px'
                    }
                }
            },
            steps: [
                {
                    element: '#operator-download-collapsible',
                    popover: {
                        title: 'Operator Download & Authentication',
                        description: 'Deploy the operator binary to your target host. Use <strong>Manual</strong> download with an API key, or generate a <strong>Device Link</strong> for a one-line curl/wget deployment.',
                        side: "right",
                        align: 'start'
                    }
                },
                {
                    element: '#operator-panel-container',
                    popover: {
                        title: 'Operator Management',
                        description: 'Authenticated operators appear here as <strong>Active</strong>. An operator is already running in your <strong>g8ep</strong> sidecar container, ready to rock. You must manually <strong>Bind</strong> it to your session to start issuing commands.',
                        side: "right",
                        align: 'start'
                    }
                },
                {
                    element: '#anchored-terminal-container',
                    popover: {
                        title: 'Issue Intent',
                        description: 'State your goal in natural language. The platform is designed to efficiently transform your intent into safe and effective execution across your fleet.',
                        side: "left",
                        align: 'start'
                    }
                },
                {
                    element: '#anchored-terminal-input',
                    popover: {
                        title: 'Chat Input',
                        description: 'Start a conversation with the AI or use a direct command for execution on bound operators:<pre style="background: var(--bg-deep); color: var(--text-bright); padding: 8px; border-radius: 4px; border: 1px solid var(--border-color); font-family: monospace;">/run &lt;command&gt;</pre>',
                        side: "top",
                        align: 'center'
                    }
                },
                {
                    element: '#case-dropdown',
                    popover: {
                        title: 'Past Conversations',
                        description: 'Access your previous chats and execution history from this dropdown.',
                        side: "bottom",
                        align: 'center'
                    }
                },
                {
                    element: '#llm-model-drawer',
                    popover: {
                        title: 'Provider & Model',
                        description: 'Dynamically change the AI provider and model during your chat. Permanent defaults can be set in the <strong>Settings</strong> page.',
                        side: "bottom",
                        align: 'center'
                    }
                },
                {
                    element: '#ai-stop-btn',
                    popover: {
                        title: 'Stop Response',
                        description: 'Stop the AI at any time by clicking the stop button if you want to interrupt or change course.',
                        side: "bottom",
                        align: 'center'
                    }
                }
            ]
        });

        this.isInitialized = true;
    }

    start() {
        if (!this.isInitialized) {
            this.init();
        }
        
        if (this.driver) {
            this.driver.drive();
        }
    }
}

export const tutorialManager = new TutorialManager();

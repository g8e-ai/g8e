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

import { EventType } from '../constants/events.js';
import { ApiPaths } from '../constants/api-paths.js';
import { ComponentName } from '../constants/service-client-constants.js';
import { notificationService } from '../utils/notification-service.js';

export const TRIAGE_DEFAULT_TIMEOUT_MS = 30000;
export const TRIAGE_DISMISS_ANIMATION_MS = 220;

export class TriageQuestionsPopup {
    constructor(eventBus, container) {
        this.eventBus = eventBus;
        this.container = container;
        this.timer = null;
    }

    get serviceClient() {
        return (typeof window !== 'undefined' && window.serviceClient) || null;
    }

    async show(payload) {
        if (!payload || !Array.isArray(payload.questions) || payload.questions.length === 0) {
            return null;
        }
        const id = Date.now();
        const timeoutDuration = Number(payload.timeout_ms) > 0
            ? Number(payload.timeout_ms)
            : TRIAGE_DEFAULT_TIMEOUT_MS;

        const popupElement = this._buildElement(id, payload.questions);
        this.container.appendChild(popupElement);

        this._bindEvents(popupElement, payload);
        this._startTimeout(popupElement, payload, timeoutDuration);

        requestAnimationFrame(() => popupElement.classList.add('is-visible'));
        return popupElement;
    }

    _buildElement(id, questions) {
        const root = document.createElement('div');
        root.className = 'triage-questions-popup';
        root.id = `triage-questions-${id}`;
        root.setAttribute('role', 'dialog');
        root.setAttribute('aria-label', 'Clarifying questions from Dash');

        const header = document.createElement('div');
        header.className = 'triage-questions-header';

        const title = document.createElement('span');
        title.className = 'triage-questions-title';
        title.textContent = 'Dash needs clarification';
        header.appendChild(title);

        const skipBtn = document.createElement('button');
        skipBtn.type = 'button';
        skipBtn.className = 'triage-questions-skip';
        skipBtn.title = 'Skip questions';
        skipBtn.setAttribute('aria-label', 'Skip clarifying questions');
        skipBtn.dataset.action = 'skip';
        const skipIcon = document.createElement('span');
        skipIcon.className = 'material-symbols-outlined';
        skipIcon.textContent = 'close';
        skipBtn.appendChild(skipIcon);
        header.appendChild(skipBtn);

        root.appendChild(header);

        const list = document.createElement('div');
        list.className = 'triage-questions-list';
        questions.forEach((question, index) => {
            list.appendChild(this._buildQuestionItem(question, index));
        });
        root.appendChild(list);

        const footer = document.createElement('div');
        footer.className = 'triage-questions-footer';
        const bar = document.createElement('div');
        bar.className = 'triage-timeout-bar';
        const progress = document.createElement('div');
        progress.className = 'triage-timeout-progress';
        progress.id = `triage-timeout-${id}`;
        bar.appendChild(progress);
        footer.appendChild(bar);
        root.appendChild(footer);

        return root;
    }

    _buildQuestionItem(question, index) {
        const item = document.createElement('div');
        item.className = 'triage-question-item';
        item.dataset.index = String(index);

        const text = document.createElement('p');
        text.className = 'triage-question-text';
        text.textContent = typeof question === 'string'
            ? question
            : (question?.text ?? '');
        item.appendChild(text);

        const actions = document.createElement('div');
        actions.className = 'triage-question-actions';
        for (const value of ['yes', 'no']) {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = `triage-btn triage-btn-${value}`;
            btn.dataset.answer = value;
            btn.textContent = value === 'yes' ? 'Yes' : 'No';
            actions.appendChild(btn);
        }
        item.appendChild(actions);
        return item;
    }

    _bindEvents(element, payload) {
        element.addEventListener('click', (event) => {
            const target = event.target instanceof Element ? event.target : null;
            if (!target) return;

            if (target.closest('[data-action="skip"]')) {
                this._handleSkip(element, payload);
                return;
            }

            const btn = target.closest('.triage-btn');
            if (btn) {
                const item = btn.closest('.triage-question-item');
                if (!item) return;
                const index = parseInt(item.dataset.index, 10);
                const answer = btn.dataset.answer === 'yes';
                this._handleAnswer(element, payload, index, answer);
            }
        });
    }

    _startTimeout(element, payload, durationMs) {
        const startTime = Date.now();
        const progressBar = element.querySelector('.triage-timeout-progress');

        const tick = () => {
            const elapsed = Date.now() - startTime;
            const remaining = Math.max(0, durationMs - elapsed);
            if (progressBar) {
                progressBar.style.width = `${(remaining / durationMs) * 100}%`;
            }
            if (remaining <= 0) {
                this._handleTimeout(element, payload);
                return;
            }
            this.timer = requestAnimationFrame(tick);
        };
        this.timer = requestAnimationFrame(tick);
    }

    async _handleAnswer(element, payload, questionIndex, answer) {
        this._stopTimer();
        const ok = await this._postSafely(
            ApiPaths.triage.answer(),
            {
                investigation_id: payload.investigation_id,
                question_index: questionIndex,
                answer,
            },
            'Failed to record answer',
        );
        if (ok) {
            this.eventBus.emit(EventType.AI_TRIAGE_CLARIFICATION_ANSWERED, {
                investigation_id: payload.investigation_id,
                question_index: questionIndex,
                answer,
            });
        }
        this.dismiss(element);
    }

    async _handleSkip(element, payload) {
        this._stopTimer();
        const ok = await this._postSafely(
            ApiPaths.triage.skip(),
            { investigation_id: payload.investigation_id },
            'Failed to skip clarifying questions',
        );
        if (ok) {
            this.eventBus.emit(EventType.AI_TRIAGE_CLARIFICATION_SKIPPED, {
                investigation_id: payload.investigation_id,
            });
        }
        this.dismiss(element);
    }

    async _handleTimeout(element, payload) {
        this._stopTimer();
        const ok = await this._postSafely(
            ApiPaths.triage.timeout(),
            { investigation_id: payload.investigation_id },
            'Failed to record clarification timeout',
        );
        if (ok) {
            this.eventBus.emit(EventType.AI_TRIAGE_CLARIFICATION_TIMEOUT, {
                investigation_id: payload.investigation_id,
            });
        }
        this.dismiss(element);
    }

    async _postSafely(path, body, userMessage) {
        const client = this.serviceClient;
        if (!client) {
            notificationService.error(userMessage, { detail: 'Service client unavailable' });
            return false;
        }
        try {
            await client.post(ComponentName.G8EE, path, body);
            return true;
        } catch (error) {
            const detail = error instanceof Error ? error.message : String(error);
            console.error('[TRIAGE]', userMessage, error);
            notificationService.error(userMessage, { detail });
            return false;
        }
    }

    _stopTimer() {
        if (this.timer) {
            cancelAnimationFrame(this.timer);
            this.timer = null;
        }
    }

    dismiss(element) {
        if (!element) return;
        element.classList.add('is-exiting');
        setTimeout(() => {
            if (element.parentNode) {
                element.parentNode.removeChild(element);
            }
        }, TRIAGE_DISMISS_ANIMATION_MS);
    }
}

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
 * CasesManager - Handles case management functionality
 * 
 * Features:
 * - Case creation and loading
 * - Case dropdown management
 * - Integration with g8ee backend
 * - Event-driven architecture
 * - URL-based session persistence (investigation ID in URL query param)
 * 
 * URL State:
 * - Investigation ID stored in URL: /chat?investigation=<case_id>
 * - Page refresh restores the last investigation from URL
 * - Browser back/forward navigation switches between investigations
 * - Shareable/bookmarkable conversation links
 */

import { EventType } from '../constants/events.js';
import { InvestigationStatus } from '../constants/investigation-constants.js';
import { ApiPaths } from '../constants/api-paths.js';



export class CasesManager {
    constructor(eventBus) {
        this.eventBus = eventBus;
        this.userCases = [];
        this.currentCaseId = null;
        this.currentTaskId = null;
        this.currentInvestigationId = null;
        this.wasJustCreated = false;
        this.casesLoaded = false;
        this._pendingCaseCreation = null;
        this._casesRefreshPromise = null;
        this._isHandlingAuth = false;
        this._restoringFromUrl = false;

        // Bound handlers for cleanup
        this.boundHandlers = {
            dropdownToggle: null,
            dropdownKeydown: null,
            documentClick: null,
            newCaseClick: null,
            windowResize: null,
            popstate: null
        };
        this.authStateUnsubscribe = null;
        this._dropdownValue = '';
    }

    init() {
        this.setupDOMElements();
        this.setupEventListeners();
        this.setupUrlStateListener();
        this.setupAuthStateListener();
    }

    /**
     * Setup DOM element references for custom dropdown
     */
    setupDOMElements() {
        this.caseDropdown = document.getElementById('case-dropdown');
        this.dropdownSelected = document.getElementById('case-dropdown-selected');
        this.dropdownText = this.dropdownSelected?.querySelector('.case-dropdown__text');
        this.dropdownMenu = document.getElementById('case-dropdown-menu');
        this.dropdownValueInput = document.getElementById('case-dropdown-value');
        this.newCaseBtn = document.getElementById('new-case-btn');
    }

    /**
     * Setup event listeners for custom dropdown
     */
    setupEventListeners() {
        if (this._eventListenersRegistered) {
            return;
        }
        this._eventListenersRegistered = true;

        // Toggle dropdown on click
        this.boundHandlers.dropdownToggle = (e) => {
            e.stopPropagation();
            this.toggleDropdown();
        };
        this.dropdownSelected?.addEventListener('click', this.boundHandlers.dropdownToggle);

        // Keyboard navigation
        this.boundHandlers.dropdownKeydown = (e) => {
            this.handleDropdownKeydown(e);
        };
        this.caseDropdown?.addEventListener('keydown', this.boundHandlers.dropdownKeydown);

        // Close on click outside
        this.boundHandlers.documentClick = (e) => {
            if (!this.caseDropdown?.contains(e.target)) {
                this.closeDropdown();
            }
        };
        document.addEventListener('click', this.boundHandlers.documentClick);

        // New case button click - reset session for new case (don't create actual case yet)
        this.boundHandlers.newCaseClick = () => {
            this.resetForNewCase();
        };
        this.newCaseBtn?.addEventListener('click', this.boundHandlers.newCaseClick);

        // Window resize event to update dropdown titles responsively
        this.boundHandlers.windowResize = () => {
            this.updateDropdownTitlesOnResize();
        };
        window.addEventListener('resize', this.boundHandlers.windowResize);

        // Browser back/forward navigation
        this.boundHandlers.popstate = (event) => {
            this.handlePopState(event);
        };
        window.addEventListener('popstate', this.boundHandlers.popstate);

        // EventBus listeners for investigation events from backend
        this.eventBus.on(EventType.INVESTIGATION_LIST_COMPLETED, (data) => {
            this.handleInvestigationQuerySuccess(data);
        });

        this.eventBus.on(EventType.CASE_CREATED, (data) => {
            this._applyCaseCreationResult(data);
        });

        this.eventBus.on(EventType.CASE_UPDATED, (data) => {
            this.handleCaseUpdated(data);
        });
    }

    /**
     * Toggle dropdown open/closed
     */
    toggleDropdown() {
        if (this.caseDropdown?.classList.contains('open')) {
            this.closeDropdown();
        } else {
            this.openDropdown();
        }
    }

    /**
     * Open the dropdown menu
     */
    openDropdown() {
        this.caseDropdown?.classList.add('open');
    }

    /**
     * Close the dropdown menu
     */
    closeDropdown() {
        this.caseDropdown?.classList.remove('open');
    }

    /**
     * Handle keyboard navigation in dropdown
     */
    handleDropdownKeydown(e) {
        const isOpen = this.caseDropdown?.classList.contains('open');

        switch (e.key) {
            case 'Enter':
            case ' ':
                e.preventDefault();
                if (!isOpen) {
                    this.openDropdown();
                }
                break;
            case 'Escape':
                e.preventDefault();
                this.closeDropdown();
                break;
            case 'ArrowDown':
                e.preventDefault();
                if (!isOpen) {
                    this.openDropdown();
                } else {
                    this.focusNextOption(1);
                }
                break;
            case 'ArrowUp':
                e.preventDefault();
                if (isOpen) {
                    this.focusNextOption(-1);
                }
                break;
        }
    }

    /**
     * Focus next/previous option in dropdown
     */
    focusNextOption(direction) {
        const options = this.dropdownMenu?.querySelectorAll('.case-dropdown__option');
        if (!options?.length) return;

        const currentIndex = Array.from(options).findIndex(opt => opt.classList.contains('focused'));
        let nextIndex = currentIndex + direction;

        if (nextIndex < 0) nextIndex = options.length - 1;
        if (nextIndex >= options.length) nextIndex = 0;

        options.forEach(opt => opt.classList.remove('focused'));
        options[nextIndex].classList.add('focused');
        options[nextIndex].scrollIntoView({ block: 'nearest' });
    }

    /**
     * Select an option in the dropdown
     */
    selectOption(value, text, isPlaceholder = false) {
        this._dropdownValue = value;
        if (this.dropdownValueInput) {
            this.dropdownValueInput.value = value;
        }

        if (this.dropdownText) {
            this.dropdownText.textContent = text;
            this.dropdownText.classList.toggle('placeholder', isPlaceholder);
        }

        // Update selected state on options
        this.dropdownMenu?.querySelectorAll('.case-dropdown__option').forEach(opt => {
            opt.classList.toggle('selected', opt.getAttribute('data-value') === value);
        });

        this.closeDropdown();
    }

    /**
     * Get current dropdown value
     */
    getDropdownValue() {
        return this._dropdownValue;
    }

    /**
     * Set dropdown value programmatically
     */
    setDropdownValue(value) {
        const option = this.dropdownMenu?.querySelector(`[data-value="${value}"]`);
        if (option) {
            const text = option.textContent;
            const isPlaceholder = option.classList.contains('placeholder');
            this.selectOption(value, text, isPlaceholder);
        } else if (!value) {
            const placeholder = this.dropdownMenu?.querySelector('.case-dropdown__option.placeholder');
            const text = placeholder ? placeholder.textContent : '';
            this.selectOption('', text, true);
        }
    }

    /**
     * Load user investigations from g8ee via HTTP - SINGLE CALL ONLY
     */
    async loadUserCases() {
        // Prevent duplicate loading
        if (this.casesLoaded) {
            return;
        }

        // Check if authState is available
        if (!window.authState) {
            console.warn('[CasesManager] authState not yet initialized, skipping loadUserCases');
            return;
        }

        const authState = window.authState.getState();
        if (!authState.isAuthenticated || !authState.webSessionModel) {
            console.warn('[CasesManager] User not authenticated, skipping loadUserCases');
            return;
        }

        const webSessionModel = authState.webSessionModel;

        try{
            // Mark as loading to prevent duplicates
            this.casesLoaded = true;

            if (!window.serviceClient) {
                throw new Error('Service client not initialized');
            }

            const response = await window.serviceClient.get('vsod', ApiPaths.chat.investigations());

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            const data = await response.json();

            if (data.success && data.investigations) {
                this.handleInvestigationQuerySuccess({ investigations: data.investigations });
            } else {
                throw new Error('Invalid response format');
            }

        } catch (error) {
            console.warn('[CasesManager] Error loading investigations:', error.message);
            this.userCases = [];
            this.populateCaseDropdown();
            // Reset flag on error so retry is possible
            this.casesLoaded = false;
        }
    }

    async refreshUserCases() {
        const authState = window.authState?.getState?.();
        if (!authState?.isAuthenticated || !authState.webSessionModel) {
            console.warn('[CasesManager] Skipping investigation refresh - user not authenticated');
            return;
        }

        if (this._casesRefreshPromise) {
            return this._casesRefreshPromise;
        }

        this._casesRefreshPromise = (async () => {
            try {
                this.casesLoaded = false;
                await this.loadUserCases();
            } finally {
                this._casesRefreshPromise = null;
            }
        })();

        return this._casesRefreshPromise;
    }

    /**
     * Populate the case dropdown with user cases
     * Called by SSE event (investigation.list.completed) on connect, or by HTTP refresh
     */
    handleInvestigationQuerySuccess(data) {
        // Mark as loaded to prevent duplicate HTTP calls
        this.casesLoaded = true;
        
        this.userCases = data.investigations;
        this.populateCaseDropdown();

        // Restore investigation from URL if present (page refresh scenario)
        if (!this.currentCaseId) {
            const urlCaseId = this.getInvestigationFromUrl();
            if (urlCaseId && this.userCases.some(inv => inv.case_id === urlCaseId)) {
                this._restoringFromUrl = true;
                this.setDropdownValue(urlCaseId);
                this.switchToCase(urlCaseId).finally(() => {
                    this._restoringFromUrl = false;
                });
            } else if (this.userCases.length > 0) {
                this.resetForNewCase();
            }
        }
    }

    handleInvestigationQueryFailure(data) {
        console.error('[CasesManager] Investigation query failure event received:', data);
        this.userCases = [];
        this.populateCaseDropdown();
    }

    /**
     * Handle case.updated SSE events to update dropdown in real-time
     */
    handleCaseUpdated(data) {
        if (!data.case_id) {
            console.warn('[CasesManager] case.updated event missing case_id');
            return;
        }

        const caseIndex = this.userCases.findIndex(inv => inv.case_id === data.case_id);
        if (caseIndex !== -1) {
            if (data.title) {
                this.userCases[caseIndex].case_title = data.title;
            }

            // Update the dropdown option for this case
            const option = this.dropdownMenu?.querySelector(`[data-value="${data.case_id}"]`);
            if (option) {
                const displayTitle = data.title || 'Untitled Case';
                const fullTitle = `${data.case_id.slice(0, 8)} - ${displayTitle}`;
                option.textContent = this.getResponsiveTitle(fullTitle);

                // Update selected text if this is the current case
                if (this._dropdownValue === data.case_id && this.dropdownText) {
                    this.dropdownText.textContent = this.getResponsiveTitle(fullTitle);
                }
            }
        }
    }

    populateCaseDropdown() {
        if (!this.dropdownMenu) return;

        // Clear existing options and add placeholder
        this.dropdownMenu.innerHTML = '';
        const placeholderOption = document.createElement('div');
        placeholderOption.className = 'case-dropdown__option placeholder';
        placeholderOption.setAttribute('data-value', '');
        placeholderOption.textContent = 'Select a past conversation here, or start a new one below';
        placeholderOption.addEventListener('click', () => {
            this.selectOption('', placeholderOption.textContent, true);
            this.switchToCase('');
        });
        this.dropdownMenu.appendChild(placeholderOption);

        if (!Array.isArray(this.userCases)) {
            console.warn('[CasesManager] userCases is not an array:', typeof this.userCases, this.userCases);
            this.userCases = [];
            return;
        }

        // Sort cases by creation date (newest first)
        this.userCases.sort((a, b) => {
            const dateA = a.created_at ? new Date(a.created_at) : new Date(0);
            const dateB = b.created_at ? new Date(b.created_at) : new Date(0);
            return dateB - dateA;
        });

        // Add case options
        this.userCases.forEach(investigation => {
            if (!investigation.case_id) {
                console.warn('[CasesManager] Skipping investigation without case_id:', investigation);
                return;
            }

            const option = document.createElement('div');
            option.className = 'case-dropdown__option';
            option.setAttribute('data-value', investigation.case_id);

            const displayTitle = investigation.case_title || investigation.title || 'Untitled Case';
            const fullTitle = `${investigation.case_id.slice(0, 8)} - ${displayTitle}`;
            option.textContent = this.getResponsiveTitle(fullTitle);

            if (investigation.case_id === this.currentCaseId) {
                option.classList.add('selected');
            }

            option.addEventListener('click', () => {
                this.selectOption(investigation.case_id, option.textContent, false);
                this.switchToCase(investigation.case_id);
            });

            this.dropdownMenu.appendChild(option);
        });

        // Set current value in display
        if (this.currentCaseId) {
            this.setDropdownValue(this.currentCaseId);
        } else {
            this.setDropdownValue('');
        }
    }

    /**
     * Clear case dropdown
     */
    clearCaseDropdown() {
        if (!this.dropdownMenu) return;

        // Reset to placeholder only
        this.dropdownMenu.innerHTML = '';
        const placeholderOption = document.createElement('div');
        placeholderOption.className = 'case-dropdown__option placeholder';
        placeholderOption.setAttribute('data-value', '');
        placeholderOption.textContent = 'Select a past conversation here, or start a new one below';
        placeholderOption.addEventListener('click', () => {
            this.selectOption('', placeholderOption.textContent, true);
            this.switchToCase('');
        });
        this.dropdownMenu.appendChild(placeholderOption);

        this.setDropdownValue('');
        this.userCases = [];
        this.eventBus.emit(EventType.CASE_CLEARED);
    }

    /**
     * Get responsive title based on screen width
     * Truncates to 100 characters with ellipsis when screen width <= 720px
     * Truncates to 80 characters with ellipsis when screen width <= 480px
     */
    getResponsiveTitle(fullTitle) {
        // Check if screen width is very small (mobile portrait)
        if (window.innerWidth <= 480) {
            // Truncate to 80 characters and add ellipsis if needed
            if (fullTitle.length > 80) {
                return fullTitle.substring(0, 80) + '...';
            }
        }
        // Check if screen width is small (mobile landscape/tablet)
        else if (window.innerWidth <= 720) {
            // Truncate to 100 characters and add ellipsis if needed
            if (fullTitle.length > 100) {
                return fullTitle.substring(0, 100) + '...';
            }
        }
        // For larger screens, allow even longer titles
        else if (fullTitle.length > 120) {
            return fullTitle.substring(0, 120) + '...';
        }
        return fullTitle;
    }

    /**
     * Update dropdown titles when window is resized
     */
    updateDropdownTitlesOnResize() {
        if (!this.dropdownMenu || !Array.isArray(this.userCases)) return;

        const options = this.dropdownMenu.querySelectorAll('.case-dropdown__option:not(.placeholder)');
        options.forEach((option, index) => {
            const investigation = this.userCases[index];
            if (investigation && investigation.case_id) {
                const displayTitle = investigation.case_title || investigation.title || 'Untitled Case';
                const fullTitle = `${investigation.case_id.slice(0, 8)} - ${displayTitle}`;
                option.textContent = this.getResponsiveTitle(fullTitle);
            }
        });

        // Update selected text display
        if (this._dropdownValue && this.dropdownText) {
            const currentCase = this.userCases.find(c => c.case_id === this._dropdownValue);
            if (currentCase) {
                const displayTitle = currentCase.case_title || currentCase.title || 'Untitled Case';
                const fullTitle = `${currentCase.case_id.slice(0, 8)} - ${displayTitle}`;
                this.dropdownText.textContent = this.getResponsiveTitle(fullTitle);
            }
        }
    }

    /**
     * Switch to a different case
     */
    async switchToCase(caseId) {
        if (!caseId) {
            this.currentCaseId = null;
            this.currentInvestigationId = null;
            this.updateUrlState(null);
            this.eventBus.emit(EventType.CASE_CLEARED);
            return;
        }

        const response = await window.serviceClient.get('vsod', ApiPaths.chat.investigations() + `?case_id=${caseId}`);

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        const data = await response.json();
        const investigations = data.success ? data.investigations : data;

        if (!investigations || investigations.length === 0) {
            console.error('[CasesManager] No investigations found for case:', caseId);
            this.updateUrlState(null);
            return;
        }

        const investigationData = investigations[0];

        this.currentCaseId = caseId;
        this.currentInvestigationId = investigationData.id;

        // Update URL to reflect current investigation (enables refresh/bookmarking)
        this.updateUrlState(caseId);

        this.eventBus.emit(EventType.CASE_SELECTED, {
            caseId: this.currentCaseId,
            investigationId: this.currentInvestigationId,
            caseData: investigationData,
            conversationHistory: investigationData.conversation_history
        });
        
        this.eventBus.emit(EventType.CASE_SWITCHED, {
            caseId: this.currentCaseId,
            investigationId: this.currentInvestigationId,
            investigation: investigationData
        });
    }


    /**
     * Reset session for new case (without creating actual case yet)
     */
    resetForNewCase() {
        this.currentCaseId = null;
        this.currentInvestigationId = null;
        this._pendingCaseCreation = null;

        this.setDropdownValue('');
        this.updateUrlState(null);
        this.eventBus.emit(EventType.CASE_CLEARED);
    }

    _applyCaseCreationResult(caseData) {
        const resolvedId = caseData?.id || caseData?.case_id;
        if (!caseData || !resolvedId) {
            console.warn('[CasesManager] Invalid case data received:', caseData);
            return;
        }

        this.currentCaseId = resolvedId;
        this.currentTaskId = caseData.task_id;
        this.currentInvestigationId = caseData.investigation_id;
        this.wasJustCreated = true;

        const normalizedCase = {
            ...caseData,
            id: resolvedId,
            case_id: resolvedId,
            case_title: caseData.title || caseData.case_title
        };

        const existingIndex = this.userCases.findIndex((existingCase) => existingCase.case_id === normalizedCase.case_id);
        if (existingIndex === -1) {
            this.userCases.unshift(normalizedCase);
        } else {
            this.userCases[existingIndex] = { ...this.userCases[existingIndex], ...normalizedCase };
        }

        if (this.dropdownMenu) {
            let option = this.dropdownMenu.querySelector(`[data-value="${normalizedCase.case_id}"]`);

            const displayTitleSource = normalizedCase.case_title || 'Untitled Case';
            const fullTitle = `${normalizedCase.case_id.slice(0, 8)} - ${displayTitleSource}`;
            const responsiveTitle = this.getResponsiveTitle(fullTitle);

            if (!option) {
                option = document.createElement('div');
                option.className = 'case-dropdown__option';
                option.setAttribute('data-value', normalizedCase.case_id);
                option.addEventListener('click', () => {
                    this.selectOption(normalizedCase.case_id, option.textContent, false);
                    this.switchToCase(normalizedCase.case_id);
                });
                // Insert after placeholder
                const placeholder = this.dropdownMenu.querySelector('.case-dropdown__option.placeholder');
                if (placeholder && placeholder.nextSibling) {
                    this.dropdownMenu.insertBefore(option, placeholder.nextSibling);
                } else {
                    this.dropdownMenu.appendChild(option);
                }
            }

            option.textContent = responsiveTitle;
            this.selectOption(normalizedCase.case_id, responsiveTitle, false);
        }

        this.updateUrlState(resolvedId);
    }

    /**
     * Setup authentication state listener
     */
    setupAuthStateListener() {
        if (!window.authState) {
            console.warn('[CasesManager] authState not available, retrying...');
            setTimeout(() => this.setupAuthStateListener(), 100);
            return;
        }

        this.authStateUnsubscribe = window.authState.subscribe((event, data) => {
            switch (event) {
                case EventType.AUTH_USER_AUTHENTICATED:
                    this.handleUserAuthenticated(data);
                    break;
                case EventType.AUTH_USER_UNAUTHENTICATED:
                    this.handleUserUnauthenticated(data);
                    break;
            }
        });

        // Check current state immediately after subscribing (catches already-authenticated)
        const authState = window.authState.getState();
        if (authState.isAuthenticated && authState.webSessionModel) {
            this.handleUserAuthenticated(authState);
        }
    }

    /**
     * Handle user authentication - enable interface
     * Note: Investigation list is now sent via SSE on connection, not HTTP
     * The SSE event (investigation.list.completed) triggers handleInvestigationQuerySuccess()
     */
    handleUserAuthenticated(authData) {
        if (this._isHandlingAuth) {
            return;
        }
        this._isHandlingAuth = true;

        if (!authData?.webSessionModel) {
            console.warn('[CasesManager] No session model in auth data');
            this._isHandlingAuth = false;
            return;
        }

        // Investigation list arrives via SSE on connection - no HTTP call needed
        // SSE sends investigation.list.completed which calls handleInvestigationQuerySuccess()
        this._isHandlingAuth = false;
    }

    /**
     * Handle user unauthentication - disable interface
     */
    handleUserUnauthenticated(data) {
        // Reset auth handling flag to allow re-auth
        this._isHandlingAuth = false;
        // Reset cases loaded flag so investigations can be loaded again on next login
        this.casesLoaded = false;
        this.userCases = [];
        this.currentCaseId = null;
        this.currentInvestigationId = null;
    }


    /**
     * Get current case ID
     */
    getCurrentCaseId() {
        return this.currentCaseId;
    }

    /**
     * Get current task ID
     */
    getCurrentTaskId() {
        return this.currentTaskId;
    }

    /**
     * Get current investigation ID
     */
    getCurrentInvestigationId() {
        return this.currentInvestigationId;
    }

    /**
     * Get current case object
     */
    getCurrentCase() {
        if (!this.currentCaseId) return null;
        return this.userCases.find(c => c.case_id === this.currentCaseId);
    }

    /**
     * Get current case data in creation format
     */
    getCurrentCaseData() {
        const currentCase = this.getCurrentCase();
        if (!currentCase) return null;

        return {
            id: this.currentCaseId,
            task_id: this.currentTaskId,
            investigation_id: this.currentInvestigationId,
            title: currentCase.case_title || currentCase.title || 'Untitled Case',
            description: currentCase.description,
            status: currentCase.status || InvestigationStatus.OPEN,
            created_at: currentCase.created_at
        };
    }

    /**
     * Cleanup event listeners
     */
    cleanup() {
        Object.entries(this.boundHandlers).forEach(([eventName, handler]) => {
            if (handler) {
                switch (eventName) {
                    case 'dropdownToggle':
                        this.dropdownSelected?.removeEventListener('click', handler);
                        break;
                    case 'dropdownKeydown':
                        this.caseDropdown?.removeEventListener('keydown', handler);
                        break;
                    case 'documentClick':
                        document.removeEventListener('click', handler);
                        break;
                    case 'newCaseClick':
                        this.newCaseBtn?.removeEventListener('click', handler);
                        break;
                    case 'windowResize':
                        window.removeEventListener('resize', handler);
                        break;
                    case 'popstate':
                        window.removeEventListener('popstate', handler);
                        break;
                }
            }
        });

        // Unsubscribe from auth state
        if (this.authStateUnsubscribe) {
            this.authStateUnsubscribe();
            this.authStateUnsubscribe = null;
        }
    }

    /**
     * Setup URL state listener for browser navigation
     */
    setupUrlStateListener() {
        // Initial URL state is read in handleInvestigationQuerySuccess after cases load
    }

    /**
     * Get investigation/case ID from URL query parameter
     * @returns {string|null} The case_id from URL or null
     */
    getInvestigationFromUrl() {
        const params = new URLSearchParams(window.location.search);
        return params.get('investigation') || null;
    }

    /**
     * Update URL to reflect current investigation state
     * Uses replaceState to avoid polluting browser history on every switch
     * @param {string|null} caseId - The case ID to set in URL, or null to clear
     */
    updateUrlState(caseId) {
        const url = new URL(window.location.href);
        
        if (caseId) {
            url.searchParams.set('investigation', caseId);
        } else {
            url.searchParams.delete('investigation');
        }

        // Use replaceState for normal navigation, pushState only for explicit user actions
        // This prevents browser history pollution when auto-restoring
        if (this._restoringFromUrl) {
            window.history.replaceState({ caseId }, '', url.toString());
        } else {
            window.history.pushState({ caseId }, '', url.toString());
        }
    }

    /**
     * Handle browser back/forward navigation
     * @param {PopStateEvent} event - The popstate event
     */
    handlePopState(event) {
        const caseId = event.state?.caseId || this.getInvestigationFromUrl();
        
        if (caseId && caseId !== this.currentCaseId) {
            this._restoringFromUrl = true;
            this.switchToCase(caseId).finally(() => {
                this._restoringFromUrl = false;
            });
            
            this.setDropdownValue(caseId);
        } else if (!caseId && this.currentCaseId) {
            this.resetForNewCase();
        }
    }
}

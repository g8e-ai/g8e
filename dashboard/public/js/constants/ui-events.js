// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

/**
 * Dashboard-local event-bus signals. These are not protocol events and must
 * not be published over SSE or registered in the protocol event catalog.
 */
export const UIEventType = Object.freeze({
    AUTH_COMPONENT_INITIALIZED_AUTHSTATE: 'ui.auth.component.initialized.authstate',
    AUTH_COMPONENT_INITIALIZED_CHAT: 'ui.auth.component.initialized.chat',
    AUTH_COMPONENT_INITIALIZED_OPERATOR: 'ui.auth.component.initialized.operator',
    CHAT_STOP_HIDE: 'ui.chat.stop.hide',
    CHAT_STOP_SHOW: 'ui.chat.stop.show',
    TERMINAL_CLOSED: 'ui.terminal.closed',
    TERMINAL_MAXIMIZED: 'ui.terminal.maximized',
    TERMINAL_MINIMIZED: 'ui.terminal.minimized',
    TERMINAL_OPENED: 'ui.terminal.opened',
});

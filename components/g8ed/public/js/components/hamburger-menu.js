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

export class HamburgerMenu {
    constructor() {
        this.btn = null;
        this.dropdown = null;
        this._boundOnDocumentClick = this._onDocumentClick.bind(this);
        this._boundOnKeydown = this._onKeydown.bind(this);
    }

    init() {
        this.btn = document.getElementById('hamburger-btn');
        this.dropdown = document.getElementById('hamburger-dropdown');

        if (!this.btn || !this.dropdown) {
            return;
        }

        this.btn.addEventListener('click', (e) => {
            e.stopPropagation();
            const isOpen = this.dropdown.classList.contains('active');
            this.dropdown.classList.toggle('active', !isOpen);
        });

        document.addEventListener('click', this._boundOnDocumentClick);
        document.addEventListener('keydown', this._boundOnKeydown);

        this._initCollapsibleSections();
        this._highlightActivePage();
        this._initThemeToggle();
    }

    _onDocumentClick(e) {
        if (!this.btn.contains(e.target) && !this.dropdown.contains(e.target)) {
            this.dropdown.classList.remove('active');
        }
    }

    _onKeydown(e) {
        if (e.key === 'Escape') {
            this.dropdown.classList.remove('active');
        }
    }

    _initCollapsibleSections() {
        const categoryLabels = document.querySelectorAll('.hamburger-category-label[data-section]');
        categoryLabels.forEach((label) => {
            const section = label.closest('.hamburger-section');
            if (!section) return;
            section.classList.add('collapsed');
            label.addEventListener('click', (e) => {
                e.stopPropagation();
                section.classList.toggle('collapsed');
            });
        });
    }

    _initThemeToggle() {
        const themeToggle = document.getElementById('hamburger-theme-toggle');
        if (!themeToggle) return;

        this._updateThemeToggleLabel(themeToggle);

        themeToggle.addEventListener('click', (e) => {
            e.stopPropagation();
            if (!window.ThemeManager) return;
            const newTheme = window.ThemeManager.toggle();
            this._updateThemeToggleLabel(themeToggle, newTheme);
        });

        if (window.ThemeManager) {
            window.ThemeManager.onChange((theme) => {
                this._updateThemeToggleLabel(themeToggle, theme);
            });
        }
    }

    _updateThemeToggleLabel(button, theme) {
        const text = button.querySelector('span');
        if (!text) return;
        const active = theme || (window.ThemeManager ? window.ThemeManager.getTheme() : null);
        if (!active) return;
        text.textContent = active === window.ThemeManager.getDefaultTheme() ? 'Go Light' : 'Go Dark';
    }

    _highlightActivePage() {
        const currentPath = window.location.pathname;
        document.querySelectorAll('.hamburger-menu-item').forEach((item) => {
            const href = item.getAttribute('href');
            if (href && (href === currentPath || (currentPath === '/' && href === '/'))) {
                item.classList.add('active');
            }
        });
    }

    destroy() {
        document.removeEventListener('click', this._boundOnDocumentClick);
        document.removeEventListener('keydown', this._boundOnKeydown);
        this.btn = null;
        this.dropdown = null;
    }
}

if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', () => new HamburgerMenu().init());
    } else {
        new HamburgerMenu().init();
    }
}

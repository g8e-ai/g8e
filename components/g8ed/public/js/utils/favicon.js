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
 * Generates a favicon programmatically using a canvas and sets it as the page's favicon.
 * This is used to satisfy the favicon requirement with 'g8e' text without needing a physical file.
 */
export async function initFavicon() {
    try {
        // Wait for Material Symbols to be ready if possible
        if (document.fonts) {
            await document.fonts.ready;
        }

        const canvas = document.createElement('canvas');
        canvas.width = 32;
        canvas.height = 32;
        const ctx = canvas.getContext('2d');

        if (!ctx) return;

        // Background
        ctx.fillStyle = '#0d1117'; // Match theme dark background
        ctx.beginPath();
        ctx.roundRect(0, 0, 32, 32, 6);
        ctx.fill();

        // Material Symbol 'gavel'
        ctx.fillStyle = '#3b82f6'; // Match logo-accent/primaryColor
        ctx.font = '24px "Material Symbols Outlined"';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        
        // Draw the gavel icon using its ligature name
        ctx.fillText('gavel', 16, 16);

        const link = document.createElement('link');
        link.type = 'image/x-icon';
        link.rel = 'shortcut icon';
        link.href = canvas.toDataURL('image/x-icon');
        
        // Remove existing favicons if any
        const existingFavicons = document.querySelectorAll('link[rel*="icon"]');
        existingFavicons.forEach(el => el.remove());
        
        document.head.appendChild(link);
    } catch (error) {
        console.warn('[Favicon] Failed to generate programmatic favicon:', error);
    }
}

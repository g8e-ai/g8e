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

import { UserRole } from '../../constants/auth.js';
import { HTTP_X_FORWARDED_HOST_HEADER, HTTP_X_FORWARDED_PROTO_HEADER } from '../../constants/headers.js';

export class SetupService {
    constructor({ userService, settingsService }) {
        this.userService = userService;
        this.settingsService = settingsService;
        this._setupLock = null;
    }

    async isFirstRun() {
        const platformSettings = await this.settingsService.getPlatformSettings();
        return platformSettings?.setup_complete !== true;
    }

    async completeSetup() {
        return this.settingsService.savePlatformSettings({ 
            setup_complete: true 
        });
    }

    async createAdminUser({ email, name }) {
        return this.userService.createUser({
            email,
            name,
            roles: [UserRole.SUPERADMIN],
        });
    }

    async performFirstRunSetup({ email, name, userSettings, req }) {
        // Serialize concurrent calls to prevent the TOCTOU race between
        // findUserByEmail and createUser that would create duplicate users.
        if (this._setupLock) {
            await this._setupLock;
        }

        let resolve;
        this._setupLock = new Promise(r => { resolve = r; });

        try {
            const { passkey_rp_id, passkey_origin } = this.derivePasskeyFields(req);

            await this.settingsService.savePlatformSettings({
                passkey_rp_id,
                passkey_origin,
                setup_complete: false
            });

            let user = await this.userService.findUserByEmail(email);
            if (!user) {
                user = await this.createAdminUser({ email, name });
            }

            if (userSettings && typeof userSettings === 'object') {
                await this.settingsService.updateUserSettings(user.id, userSettings);
            }

            return user;
        } finally {
            this._setupLock = null;
            resolve();
        }
    }

    derivePasskeyFields(req) {
        if (!req || typeof req.get !== 'function') {
            return { passkey_rp_id: 'localhost', passkey_origin: 'https://localhost' };
        }

        const xForwardedHost  = req.get(HTTP_X_FORWARDED_HOST_HEADER);
        const xForwardedProto = req.get(HTTP_X_FORWARDED_PROTO_HEADER);

        const host  = (xForwardedHost || req.get('host') || req.hostname || 'localhost');
        const proto = (xForwardedProto || req.protocol || 'https');

        const rpId   = host.split(':')[0];
        const origin = `${proto}://${host}`;

        return { passkey_rp_id: rpId, passkey_origin: origin };
    }
}

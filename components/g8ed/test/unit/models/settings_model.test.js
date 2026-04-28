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

import { describe, it, expect } from 'vitest';
import { structureUserSettings, flattenUserSettings } from '@g8ed/models/settings_model.js';

describe('settings_model [UNIT - PURE LOGIC]', () => {
    describe('structureUserSettings', () => {
        it('maps enable_command_whitelisting to command_validation.enable_whitelisting', () => {
            const flat = {
                enable_command_whitelisting: true,
            };
            const nested = structureUserSettings(flat);
            expect(nested.command_validation).toEqual({
                enable_whitelisting: true,
            });
        });

        it('maps whitelisted_commands_csv to command_validation.whitelisted_commands', () => {
            const flat = {
                whitelisted_commands_csv: 'uptime,df,free,ps',
            };
            const nested = structureUserSettings(flat);
            expect(nested.command_validation).toEqual({
                whitelisted_commands: 'uptime,df,free,ps',
            });
        });

        it('maps enable_command_blacklisting to command_validation.enable_blacklisting', () => {
            const flat = {
                enable_command_blacklisting: true,
            };
            const nested = structureUserSettings(flat);
            expect(nested.command_validation).toEqual({
                enable_blacklisting: true,
            });
        });

        it('maps enable_command_auto_approve to command_validation.enable_auto_approve', () => {
            const flat = {
                enable_command_auto_approve: true,
            };
            const nested = structureUserSettings(flat);
            expect(nested.command_validation).toEqual({
                enable_auto_approve: true,
            });
        });

        it('maps auto_approved_commands_csv to command_validation.auto_approved_commands', () => {
            const flat = {
                auto_approved_commands_csv: 'uptime,df,free',
            };
            const nested = structureUserSettings(flat);
            expect(nested.command_validation).toEqual({
                auto_approved_commands: 'uptime,df,free',
            });
        });

        it('maps both command validation settings together', () => {
            const flat = {
                enable_command_whitelisting: true,
                whitelisted_commands_csv: 'uptime,df,free',
                enable_command_blacklisting: false,
            };
            const nested = structureUserSettings(flat);
            expect(nested.command_validation).toEqual({
                enable_whitelisting: true,
                whitelisted_commands: 'uptime,df,free',
                enable_blacklisting: false,
            });
        });

        it('preserves false values for command validation settings', () => {
            const flat = {
                enable_command_whitelisting: false,
                whitelisted_commands_csv: '',
                enable_command_blacklisting: false,
            };
            const nested = structureUserSettings(flat);
            expect(nested.command_validation).toEqual({
                enable_whitelisting: false,
                whitelisted_commands: '',
                enable_blacklisting: false,
            });
        });

        it('includes other settings sections alongside command_validation', () => {
            const flat = {
                llm_primary_provider: 'gemini',
                vertex_search_enabled: true,
                enable_command_whitelisting: true,
                whitelisted_commands_csv: 'uptime,df',
                g8e_api_key: 'test-key',
            };
            const nested = structureUserSettings(flat);
            expect(nested.llm).toEqual({ primary_provider: 'gemini' });
            expect(nested.search).toEqual({ enabled: true });
            expect(nested.command_validation).toEqual({ enable_whitelisting: true, whitelisted_commands: 'uptime,df' });
            expect(nested.security).toEqual({ g8e_api_key: 'test-key' });
        });
    });

    describe('flattenUserSettings', () => {
        it('maps command_validation.enable_whitelisting to enable_command_whitelisting', () => {
            const nested = {
                command_validation: {
                    enable_whitelisting: true,
                },
            };
            const flat = flattenUserSettings(nested);
            expect(flat.enable_command_whitelisting).toBe(true);
        });

        it('maps command_validation.whitelisted_commands to whitelisted_commands_csv', () => {
            const nested = {
                command_validation: {
                    whitelisted_commands: 'uptime,df,free,ps',
                },
            };
            const flat = flattenUserSettings(nested);
            expect(flat.whitelisted_commands_csv).toBe('uptime,df,free,ps');
        });

        it('maps command_validation.enable_blacklisting to enable_command_blacklisting', () => {
            const nested = {
                command_validation: {
                    enable_blacklisting: true,
                },
            };
            const flat = flattenUserSettings(nested);
            expect(flat.enable_command_blacklisting).toBe(true);
        });

        it('maps command_validation.enable_auto_approve to enable_command_auto_approve', () => {
            const nested = {
                command_validation: {
                    enable_auto_approve: true,
                },
            };
            const flat = flattenUserSettings(nested);
            expect(flat.enable_command_auto_approve).toBe(true);
        });

        it('maps command_validation.auto_approved_commands to auto_approved_commands_csv', () => {
            const nested = {
                command_validation: {
                    auto_approved_commands: 'uptime,df',
                },
            };
            const flat = flattenUserSettings(nested);
            expect(flat.auto_approved_commands_csv).toBe('uptime,df');
        });

        it('maps both command validation settings together', () => {
            const nested = {
                command_validation: {
                    enable_whitelisting: true,
                    whitelisted_commands: 'uptime,df,free',
                    enable_blacklisting: false,
                },
            };
            const flat = flattenUserSettings(nested);
            expect(flat.enable_command_whitelisting).toBe(true);
            expect(flat.whitelisted_commands_csv).toBe('uptime,df,free');
            expect(flat.enable_command_blacklisting).toBe(false);
        });

        it('handles missing command_validation object', () => {
            const nested = {
                llm: { primary_provider: 'gemini' },
            };
            const flat = flattenUserSettings(nested);
            expect(flat.enable_command_whitelisting).toBeUndefined();
            expect(flat.whitelisted_commands_csv).toBeUndefined();
            expect(flat.enable_command_blacklisting).toBeUndefined();
        });

        it('includes other settings sections alongside command_validation', () => {
            const nested = {
                llm: { primary_provider: 'gemini' },
                search: { enabled: true },
                command_validation: { enable_whitelisting: true, whitelisted_commands: 'uptime,df' },
                security: { g8e_api_key: 'test-key' },
            };
            const flat = flattenUserSettings(nested);
            expect(flat.llm_primary_provider).toBe('gemini');
            expect(flat.vertex_search_enabled).toBe(true);
            expect(flat.enable_command_whitelisting).toBe(true);
            expect(flat.whitelisted_commands_csv).toBe('uptime,df');
            expect(flat.g8e_api_key).toBe('test-key');
        });
    });

    describe('round-trip mapping', () => {
        it('preserves command validation settings through structure -> flatten', () => {
            const originalFlat = {
                enable_command_whitelisting: true,
                whitelisted_commands_csv: 'uptime,df,free',
                enable_command_blacklisting: false,
            };
            const nested = structureUserSettings(originalFlat);
            const resultFlat = flattenUserSettings(nested);
            expect(resultFlat.enable_command_whitelisting).toBe(true);
            expect(resultFlat.whitelisted_commands_csv).toBe('uptime,df,free');
            expect(resultFlat.enable_command_blacklisting).toBe(false);
        });

        it('preserves all settings through structure -> flatten', () => {
            const originalFlat = {
                llm_primary_provider: 'gemini',
                vertex_search_enabled: true,
                enable_command_whitelisting: true,
                whitelisted_commands_csv: 'uptime,df',
                enable_command_blacklisting: false,
                g8e_api_key: 'test-key',
            };
            const nested = structureUserSettings(originalFlat);
            const resultFlat = flattenUserSettings(nested);
            expect(resultFlat.llm_primary_provider).toBe('gemini');
            expect(resultFlat.vertex_search_enabled).toBe(true);
            expect(resultFlat.enable_command_whitelisting).toBe(true);
            expect(resultFlat.whitelisted_commands_csv).toBe('uptime,df');
            expect(resultFlat.enable_command_blacklisting).toBe(false);
            expect(resultFlat.g8e_api_key).toBe('test-key');
        });

        it('preserves false values through round-trip', () => {
            const originalFlat = {
                enable_command_whitelisting: false,
                whitelisted_commands_csv: '',
                enable_command_blacklisting: false,
            };
            const nested = structureUserSettings(originalFlat);
            const resultFlat = flattenUserSettings(nested);
            expect(resultFlat.enable_command_whitelisting).toBe(false);
            expect(resultFlat.whitelisted_commands_csv).toBe('');
            expect(resultFlat.enable_command_blacklisting).toBe(false);
        });
    });
});

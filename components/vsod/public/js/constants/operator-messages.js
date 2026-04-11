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

export const OperatorDialogs = Object.freeze({
    RESET_SLOT_CONFIRM:
        'RESET OPERATOR SLOT\n\n' +
        'Terminates the operator, generates a new API key, and clears deployment data. ' +
        'Command history and recent system status details are preserved.\n\n' +
        'Continue?',

    STOP_OPERATOR_CONFIRM:
        'WARNING: This will immediately terminate the operator.\n\n' +
        'The Operator process will be shut down and disconnected.\n\n' +
        'Are you sure you want to stop this operator?',

    REVOKE_DEVICE_LINK_CONFIRM:
        'Revoke this device link?\n\n' +
        'Operators that already claimed this link will continue to work, but no new claims will be allowed.\n\n' +
        'Continue?',
});

export const OperatorAlerts = Object.freeze({
    SLOT_RESET_KEY_COPIED: (slotNumber) =>
        'Operator Slot Reset Successfully!\n\n' +
        'The new API key has been copied to your clipboard.\n\n' +
        `Slot ${slotNumber} has been reset:\n` +
        '• Old Operator terminated (preserved for history)\n' +
        '• New Operator created with fresh API key\n' +
        '• Slot is now completely available\n\n' +
        'Use the new API key to connect an Operator to this slot.',

    SLOT_RESET_KEY_DISPLAY: (slotNumber, newApiKey) =>
        'Operator Slot Reset Successfully!\n\n' +
        `New API Key:\n${newApiKey}\n\n` +
        `Slot ${slotNumber} has been reset:\n` +
        '• Old Operator terminated (preserved for history)\n' +
        '• New Operator created with fresh API key\n' +
        '• Slot is now completely available\n\n' +
        'Copy this key and use it to connect an operator.',
});

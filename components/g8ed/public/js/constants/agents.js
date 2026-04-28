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
 * Tribunal Member Icons
 * Icon names for each tribunal member, indexed by pass number.
 * Sourced from shared/constants/agents.json agent metadata.
 */
export const TribunalMemberIcons = Object.freeze({
    0: 'call_merge',
    1: 'verified',
    2: 'account_tree',
    3: 'menu_book',
    4: 'report',
});

/**
 * Auditor Icon
 * Icon for the Tribunal auditor.
 * Sourced from shared/constants/agents.json agent metadata.
 */
export const AuditorIcon = 'fact_check';

/**
 * Tribunal Outcome
 * Terminal outcomes the Tribunal pipeline can produce.
 */
export const TribunalOutcome = Object.freeze({
    CONSENSUS: 'consensus',
    VERIFIED: 'verified',
    VERIFICATION_FAILED: 'verification_failed',
    CONSENSUS_FAILED: 'consensus_failed',
});

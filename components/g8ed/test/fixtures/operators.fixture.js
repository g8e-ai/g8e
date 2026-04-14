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
 * Operator test fixtures
 *
 * Uses proper OperatorDocument and SystemInfo models instead of hand-rolled dicts.
 */

import { OperatorStatus } from '../../constants/operator.js';
import { OperatorDocument, SystemInfo, HeartbeatSnapshot } from '../../models/operator_model.js';
import { now, addSeconds } from '@test/fixtures/base.fixture.js';

const PRIMARY_USER_ID = 'a1b2c3d4-e5f6-4789-a0b1-c2d3e4f56789';
const SECONDARY_USER_ID = 'b2c3d4e5-f6g7-4890-b1c2-d3e4f5678901';

export const mockOperators = {
  unclaimed: new OperatorDocument({
    operator_id: '01234567-89ab-4cde-f012-345678901234',
    user_id: PRIMARY_USER_ID,
    organization_id: PRIMARY_USER_ID,
    name: 'Operator Slot 1',
    slot_number: 1,
    status: OperatorStatus.AVAILABLE,
    is_slot: true,
    claimed: false
  }),

  claimed: new OperatorDocument({
    operator_id: '12345678-9abc-4def-0123-456789012345',
    user_id: PRIMARY_USER_ID,
    organization_id: PRIMARY_USER_ID,
    name: 'Claimed Operator',
    slot_number: 2,
    status: OperatorStatus.OFFLINE,
    claimed: true,
    system_fingerprint: 'fingerprint_abc123',
    operator_session_id: 'session_op_123',
    system_info: new SystemInfo({
      system_fingerprint: 'fingerprint_abc123',
      hostname: 'workstation-1'
    }),
    last_heartbeat: addSeconds(now(), -120),
    created_at: addSeconds(now(), -3600),
    updated_at: addSeconds(now(), -120)
  }),

  stale: new OperatorDocument({
    operator_id: '34567890-12bc-4def-2345-678901234567',
    user_id: PRIMARY_USER_ID,
    organization_id: PRIMARY_USER_ID,
    name: 'Stale Operator',
    slot_number: 3,
    status: OperatorStatus.ACTIVE,
    claimed: true,
    system_fingerprint: 'fingerprint_stale_123',
    operator_session_id: 'session_op_old',
    system_info: new SystemInfo({
      system_fingerprint: 'fingerprint_stale_123',
      hostname: 'old-workstation'
    }),
    last_heartbeat: addSeconds(now(), -120),
    created_at: addSeconds(now(), -7200),
    updated_at: addSeconds(now(), -120)
  }),

  differentUser: new OperatorDocument({
    operator_id: '45678901-23cd-4ef0-3456-789012345678',
    user_id: SECONDARY_USER_ID,
    organization_id: SECONDARY_USER_ID,
    name: 'Other User Operator',
    slot_number: 1,
    status: OperatorStatus.ACTIVE,
    claimed: true,
    system_fingerprint: 'fingerprint_other_123',
    system_info: new SystemInfo({
      system_fingerprint: 'fingerprint_other_123',
      hostname: 'other-workstation'
    })
  }),

  activeOperator: new OperatorDocument({
    operator_id: '56789012-34de-4f01-4567-890123456789',
    user_id: PRIMARY_USER_ID,
    organization_id: PRIMARY_USER_ID,
    name: 'Active Operator',
    slot_number: 4,
    status: OperatorStatus.ACTIVE,
    claimed: true,
    system_fingerprint: 'fingerprint_active_123',
    system_info: new SystemInfo({
      system_fingerprint: 'fingerprint_active_123',
      hostname: 'active-workstation'
    }),
    last_heartbeat: now()
  })
};

export const mockSystemInfo = {
  valid: new SystemInfo({
    system_fingerprint: 'fingerprint_abc123',
    hostname: 'workstation-1',
    os: 'linux',
    architecture: 'x64',
    cpu_count: 8,
    memory_mb: 16384
  }),

  differentFingerprint: new SystemInfo({
    system_fingerprint: 'fingerprint_different_xyz',
    hostname: 'workstation-2',
    os: 'linux',
    architecture: 'x64',
    cpu_count: 4,
    memory_mb: 8192
  })
};

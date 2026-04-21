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
import { OperatorListUpdatedEvent, OperatorSlot, OperatorSlotSystemInfo } from '@g8ed/models/operator_model.js';

describe('OperatorListUpdatedEvent nested model serialization [UNIT - PURE LOGIC]', () => {
    it('should serialize OperatorSlot instances to plain objects in forWire()', () => {
        const systemInfo = new OperatorSlotSystemInfo({
            hostname: 'node-01',
            os: 'linux',
            architecture: 'amd64',
        });

        const operatorSlot = new OperatorSlot({
            operator_id: 'op-1',
            name: 'node-01',
            status: 'ACTIVE',
            status_display: 'ACTIVE',
            status_class: 'active',
            system_info: systemInfo,
        });

        const event = new OperatorListUpdatedEvent({
            type: 'g8e.v1.operator.panel.list.updated',
            operators: [operatorSlot],
            total_count: 1,
            active_count: 1,
            used_slots: 1,
            max_slots: 5,
        });

        const wire = event.forWire();

        // Verify structure
        expect(wire.type).toBe('g8e.v1.operator.panel.list.updated');
        expect(wire.data).toBeDefined();
        expect(Array.isArray(wire.data.operators)).toBe(true);
        expect(wire.data.operators.length).toBe(1);

        // Verify operator is a plain object, not a model instance
        const serializedOperator = wire.data.operators[0];
        expect(serializedOperator instanceof OperatorSlot).toBe(false);
        expect(typeof serializedOperator).toBe('object');

        // Verify fields are present
        expect(serializedOperator.operator_id).toBe('op-1');
        expect(serializedOperator.name).toBe('node-01');
        expect(serializedOperator.status).toBe('ACTIVE');

        // Verify nested system_info is a plain object, not a model instance
        expect(serializedOperator.system_info instanceof OperatorSlotSystemInfo).toBe(false);
        expect(typeof serializedOperator.system_info).toBe('object');
        expect(serializedOperator.system_info.hostname).toBe('node-01');
        expect(serializedOperator.system_info.os).toBe('linux');
        expect(serializedOperator.system_info.architecture).toBe('amd64');
    });

    it('should serialize multiple OperatorSlot instances correctly', () => {
        const slots = [
            new OperatorSlot({ id: 'op-1', status: 'ACTIVE' }),
            new OperatorSlot({ id: 'op-2', status: 'AVAILABLE' }),
        ];

        const event = new OperatorListUpdatedEvent({
            type: 'g8e.v1.operator.panel.list.updated',
            operators: slots,
            total_count: 2,
            active_count: 1,
        });

        const wire = event.forWire();

        expect(wire.data.operators).toHaveLength(2);
        expect(wire.data.operators[0] instanceof OperatorSlot).toBe(false);
        expect(wire.data.operators[1] instanceof OperatorSlot).toBe(false);
        expect(wire.data.operators[0].id).toBe('op-1');
        expect(wire.data.operators[1].id).toBe('op-2');
    });

    it('should handle JSON.stringify roundtrip correctly', () => {
        const systemInfo = new OperatorSlotSystemInfo({
            hostname: 'node-01',
            os: 'linux',
        });

        const operatorSlot = new OperatorSlot({
            id: 'op-1',
            name: 'node-01',
            status: 'ACTIVE',
            system_info: systemInfo,
        });

        const event = new OperatorListUpdatedEvent({
            type: 'g8e.v1.operator.panel.list.updated',
            operators: [operatorSlot],
            total_count: 1,
        });

        const wire = event.forWire();
        const json = JSON.stringify(wire);
        const parsed = JSON.parse(json);

        // Verify parsed structure
        expect(parsed.type).toBe('g8e.v1.operator.panel.list.updated');
        expect(Array.isArray(parsed.data.operators)).toBe(true);
        expect(parsed.data.operators[0].id).toBe('op-1');
        expect(typeof parsed.data.operators[0].system_info).toBe('object');
        expect(parsed.data.operators[0].system_info.hostname).toBe('node-01');
    });

    it('should not stringify nested system_info model', () => {
        const systemInfo = new OperatorSlotSystemInfo({
            hostname: 'node-01',
            os: 'linux',
            architecture: 'amd64',
            cpu_count: 4,
            memory_mb: 8192,
        });

        const operatorSlot = new OperatorSlot({
            operator_id: 'op-1',
            name: 'node-01',
            status: 'ACTIVE',
            system_info: systemInfo,
        });

        const event = new OperatorListUpdatedEvent({
            type: 'g8e.v1.operator.panel.list.updated',
            operators: [operatorSlot],
            total_count: 1,
        });

        const wire = event.forWire();
        const serializedOperator = wire.data.operators[0];

        // Verify system_info is an object, not a JSON string
        expect(typeof serializedOperator.system_info).toBe('object');
        expect(serializedOperator.system_info).not.toBeInstanceOf(OperatorSlotSystemInfo);
        expect(typeof serializedOperator.system_info.hostname).toBe('string');
        expect(typeof serializedOperator.system_info.os).toBe('string');
        expect(typeof serializedOperator.system_info.architecture).toBe('string');
        expect(typeof serializedOperator.system_info.cpu_count).toBe('number');
        expect(typeof serializedOperator.system_info.memory_mb).toBe('number');

        // Verify it's not a stringified JSON
        expect(serializedOperator.system_info.hostname).not.toContain('{');
        expect(serializedOperator.system_info.hostname).not.toContain('"');
    });
});

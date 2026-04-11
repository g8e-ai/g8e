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

import { expect } from 'vitest';
import { G8eBaseModel } from '@g8ed/models/base.js';

/**
 * Custom matchers for g8e Models to decouple tests from specific field implementations.
 */
export const modelMatchers = {
    /**
     * Asserts that an object is a valid instance of a given Model class.
     * @param {Object} obj - The object to test
     * @param {Function} ModelClass - The Expected Model class
     */
    toBeValidModel(obj, ModelClass) {
        expect(obj).toBeInstanceOf(ModelClass);
        // Ensure it passes model validation
        expect(() => ModelClass.parse(obj.forDB())).not.toThrow();
    },

    /**
     * Asserts that a value matches the expected type for a model field.
     * Use this instead of hardcoding 'instanceof Date' or 'typeof === string'.
     * @param {G8eBaseModel} model - The model instance
     * @param {string} fieldName - The field to check
     * @param {any} value - The value to check
     */
    toMatchFieldType(model, fieldName, value) {
        // Handle inheritance by collecting all fields in the hierarchy
        const getAllFields = (cls) => {
            const allFields = {};
            let current = cls;
            while (current && current !== Object) {
                if (Object.prototype.hasOwnProperty.call(current, 'fields')) {
                    Object.assign(allFields, current.fields);
                }
                current = Object.getPrototypeOf(current);
            }
            return allFields;
        };

        const fields = getAllFields(model.constructor);
        const field = fields[fieldName];
        if (!field) {
            throw new Error(`Field ${fieldName} not found on model ${model.constructor.name}`);
        }

        const type = field.type;
        // Basic type checking based on g8e Model field types
        if (type.name === 'string') {
            expect(typeof value).toBe('string');
        } else if (type.name === 'date') {
            expect(value).toBeInstanceOf(Date);
        } else if (type.name === 'boolean') {
            expect(typeof value).toBe('boolean');
        } else if (type.name === 'array') {
            expect(Array.isArray(value)).toBe(true);
        } else if (type.name === 'number') {
            expect(typeof value).toBe('number');
        }
    }
};

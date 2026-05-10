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
 * g8ed Logging Configuration
 * 
 * Centralized logging setup following g8e standards
 */

import { Writable } from 'stream';
import winston from 'winston';
import { SourceComponent } from '../constants/ai.js';
import { DEFAULT_LOG_LEVEL } from '../constants/service_config.js';
import { PII_FIELDS, EMAIL_REGEX, PII_REDACT_MAX_DEPTH } from '../utils/security.js';

/**
 * Redact an email address showing first/last chars with asterisks
 * Example: "myemail@gmail.com" -> "m*****l@g*******m"
 * @param {string} email - The email address to redact
 * @returns {string} - Redacted email
 */
function redactEmail(email) {
    const atIndex = email.indexOf('@');
    if (atIndex === -1) return email;
    
    const localPart = email.substring(0, atIndex);
    const domainPart = email.substring(atIndex + 1);
    
    // Redact local part: show first and last char with asterisks
    let redactedLocal;
    if (localPart.length <= 2) {
        redactedLocal = '*'.repeat(localPart.length);
    } else {
        redactedLocal = localPart[0] + '*'.repeat(localPart.length - 2) + localPart[localPart.length - 1];
    }
    
    // Redact domain: show first and last char with asterisks
    let redactedDomain;
    if (domainPart.length <= 2) {
        redactedDomain = '*'.repeat(domainPart.length);
    } else {
        redactedDomain = domainPart[0] + '*'.repeat(domainPart.length - 2) + domainPart[domainPart.length - 1];
    }
    
    return `${redactedLocal}@${redactedDomain}`;
}

/**
 * Redact a value based on field type
 * @param {string} fieldName - The name of the field
 * @param {*} value - The value to potentially redact
 * @returns {*} - Redacted or original value
 */
function redactValue(fieldName, value) {
    if (value === null || value === undefined) {
        return value;
    }

    const lowerField = fieldName.toLowerCase();
    
    // Check if this is a known PII field
    const isPiiField = PII_FIELDS.some(pii => lowerField === pii.toLowerCase());
    
    if (isPiiField) {
        if (typeof value === 'string') {
            // Redact email addresses
            if (value.includes('@')) {
                return redactEmail(value);
            }
            // Redact names - show first character only
            if (value.length > 0) {
                return `${value[0]}${'*'.repeat(Math.max(0, value.length - 1))}`;
            }
        }
        return '***';
    }

    // For non-PII string fields, still scan for embedded email addresses
    if (typeof value === 'string' && value.includes('@')) {
        return value.replace(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g, (match) => redactEmail(match));
    }

    return value;
}

/**
 * Recursively redact PII from an object
 * @param {Object} obj - Object to sanitize
 * @param {number} depth - Current recursion depth (max 10)
 * @returns {Object} - Sanitized object
 */
function redactPii(obj, depth = 0) {
    if (depth > PII_REDACT_MAX_DEPTH || obj === null || obj === undefined) {
        return obj;
    }

    if (typeof obj !== 'object') {
        return obj;
    }

    if (Array.isArray(obj)) {
        return obj.map(item => redactPii(item, depth + 1));
    }

    if (obj.constructor && obj.constructor !== Object) {
        return obj;
    }

    const sanitized = {};
    for (const [key, value] of Object.entries(obj)) {
        if (typeof value === 'object' && value !== null) {
            sanitized[key] = redactPii(value, depth + 1);
        } else {
            sanitized[key] = redactValue(key, value);
        }
    }
    return sanitized;
}

/**
 * Winston format for PII redaction
 * Automatically redacts email addresses and names from log metadata
 */
const piiRedactionFormat = winston.format((info) => {
    // Redact the message if it contains email addresses
    if (typeof info.message === 'string' && info.message.includes('@')) {
        info.message = info.message.replace(/[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g, (match) => redactEmail(match));
    }

    // Redact metadata fields
    const reservedKeys = ['level', 'message', 'timestamp', 'service', 'stack'];
    for (const [key, value] of Object.entries(info)) {
        if (reservedKeys.includes(key)) {
            continue;
        }
        if (typeof value === 'object' && value !== null) {
            info[key] = redactPii(value);
        } else {
            info[key] = redactValue(key, value);
        }
    }

    return info;
});

const LOG_RING_BUFFER_SIZE = 500;
const logRingBuffer = [];
const logListeners = new Set();

const ringBufferStream = new Writable({
    write(chunk, _encoding, callback) {
        try {
            const entry = JSON.parse(chunk.toString());
            logRingBuffer.push(entry);
            if (logRingBuffer.length > LOG_RING_BUFFER_SIZE) {
                logRingBuffer.shift();
            }
            for (const listener of logListeners) {
                try { listener(entry); } catch (_) {}
            }
        } catch (_) {}
        callback();
    }
});

const ringBufferTransport = new winston.transports.Stream({ stream: ringBufferStream });

export const logger = winston.createLogger({
    level: DEFAULT_LOG_LEVEL,
    format: winston.format.combine(
        winston.format.timestamp(),
        winston.format.errors({ stack: true }),
        piiRedactionFormat(),
        winston.format.json()
    ),
    defaultMeta: { service: SourceComponent.G8ED },
    transports: [
        new winston.transports.Console({
            format: winston.format.combine(
                winston.format.timestamp(),
                winston.format.errors({ stack: true }),
                piiRedactionFormat(),
                winston.format.printf(({ timestamp, level, message, stack, service, ...meta }) => {
                    const logService = service || SourceComponent.G8ED;
                    const baseMessage = stack || message;
                    const extraMeta = Object.keys(meta).length ? ` | extra=${JSON.stringify(meta, (_k, v) => v instanceof Date ? v.toISOString() : v)}` : '';
                    return `${timestamp} - ${logService} - ${level.toUpperCase()} - ${baseMessage}${extraMeta}`;
                })
            )
        }),
        ringBufferTransport
    ]
});

export function getLogRingBuffer() {
    return logRingBuffer;
}

export function addLogListener(fn) {
    logListeners.add(fn);
}

export function removeLogListener(fn) {
    logListeners.delete(fn);
}

export { redactValue, redactPii, redactEmail, PII_FIELDS, EMAIL_REGEX };

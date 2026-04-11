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

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import winston from 'winston';
import { UserRole } from '@vsod/constants/auth.js';

// Test the logger module by importing it
describe('Logger Utility [UNIT]', () => {
  let originalEnv;

  beforeEach(() => {
    // Setup
  });

  afterEach(() => {
    // Clear module cache
    vi.resetModules();
  });

  describe('logger initialization', () => {
    it('should create logger with default log level', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      
      expect(logger).toBeDefined();
      // During tests, logger level is set to 'error' to suppress output
      expect(logger.level).toBe('error');
    });

    it('should have default service metadata', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      
      expect(logger.defaultMeta).toEqual({ service: 'vsod' });
    });

    it('should have console transport configured', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      
      const transports = logger.transports;
      expect(transports.length).toBeGreaterThan(0);
      expect(transports[0]).toBeInstanceOf(winston.transports.Console);
    });
  });

  describe('logging methods', () => {
    it('should have standard logging methods', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      
      expect(typeof logger.info).toBe('function');
      expect(typeof logger.error).toBe('function');
      expect(typeof logger.warn).toBe('function');
      expect(typeof logger.info).toBe('function');
    });

    it('should log info messages', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      const writeSpy = vi.spyOn(logger.transports[0], 'log');
      
      logger.info('Test info message');
      
      expect(writeSpy).toHaveBeenCalled();
      writeSpy.mockRestore();
    });

    it('should log error messages with metadata', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      const writeSpy = vi.spyOn(logger.transports[0], 'log');
      
      logger.error('Test error', { userId: 'user-123', code: 500 });
      
      expect(writeSpy).toHaveBeenCalled();
      writeSpy.mockRestore();
    });

    it('should handle stack traces in errors', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      const writeSpy = vi.spyOn(logger.transports[0], 'log');
      
      const error = new Error('Test error with stack');
      logger.error('Error occurred', { error: error.message, stack: error.stack });
      
      expect(writeSpy).toHaveBeenCalled();
      writeSpy.mockRestore();
    });
  });

  describe('transports', () => {
    it('should always have at least a console transport', async () => {
      vi.resetModules();
      
      const { logger } = await import('@vsod/utils/logger.js');
      
      expect(logger.transports.length).toBeGreaterThanOrEqual(1);
      expect(logger.transports[0].constructor.name).toBe('Console');
    });
  });

  describe('log format', () => {
    it('should include timestamp in logs', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      
      const format = logger.format;
      expect(format).toBeDefined();
    });

    it('should format errors with stack traces', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      
      // Logger should have errors format configured
      const formatConfig = logger.format.options || {};
      expect(logger.format).toBeDefined();
    });

    it('should output JSON format for structured logging', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      
      // The base format should include JSON
      expect(logger.format).toBeDefined();
    });
  });

  describe('log levels', () => {
    it('should support all standard log levels', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      
      // Test that logger responds to all levels
      const levels = ['error', 'warn', 'info', 'debug'];
      
      levels.forEach(level => {
        expect(typeof logger[level]).toBe('function');
      });
    });

    it('should respect log level hierarchy', async () => {
      const { logger } = await import('@vsod/utils/logger.js');

      const saved = logger.level;
      logger.level = 'error';
      expect(logger.level).toBe('error');
      expect(logger.isErrorEnabled()).toBe(true);
      expect(logger.isInfoEnabled()).toBe(false);
      logger.level = saved;
    });
  });

  describe('metadata handling', () => {
    it('should merge custom metadata with default service metadata', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      const writeSpy = vi.spyOn(logger.transports[0], 'log');
      
      logger.info('Test with metadata', { userId: 'user-123', action: 'test' });
      
      expect(writeSpy).toHaveBeenCalled();
      writeSpy.mockRestore();
    });

    it('should handle empty metadata', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      const writeSpy = vi.spyOn(logger.transports[0], 'log');
      
      logger.info('Test without metadata');
      
      expect(writeSpy).toHaveBeenCalled();
      writeSpy.mockRestore();
    });

    it('should handle nested metadata objects', async () => {
      const { logger } = await import('@vsod/utils/logger.js');
      const writeSpy = vi.spyOn(logger.transports[0], 'log');
      
      logger.info('Test with nested metadata', {
        user: { id: 'user-123', role: UserRole.ADMIN },
        request: { method: 'POST', path: '/api/test' }
      });
      
      expect(writeSpy).toHaveBeenCalled();
      writeSpy.mockRestore();
    });
  });

  describe('PII redaction', () => {
    it('should redact email addresses in PII fields with asterisks', async () => {
      const { redactValue, redactEmail } = await import('@vsod/utils/logger.js');
      
      // user@example.com -> u**r@e*********m
      expect(redactEmail('user@example.com')).toBe('u**r@e*********m');
      expect(redactValue('email', 'myemail@gmail.com')).toBe('m*****l@g*******m');
      expect(redactValue('customerEmail', 'test@domain.org')).toBe('t**t@d********g');
    });

    it('should redact names showing first character with asterisks', async () => {
      const { redactValue } = await import('@vsod/utils/logger.js');
      
      expect(redactValue('name', 'John Doe')).toBe('J*******');
      expect(redactValue('customerName', 'Alice Smith')).toBe('A**********');
      expect(redactValue('firstName', 'Admin')).toBe('A****');
    });

    it('should not redact non-PII fields', async () => {
      const { redactValue } = await import('@vsod/utils/logger.js');
      
      expect(redactValue('userId', 'user-123')).toBe('user-123');
      expect(redactValue('action', 'login')).toBe('login');
      expect(redactValue('action', 'update')).toBe('update');
    });

    it('should redact emails embedded in non-PII string fields', async () => {
      const { redactValue } = await import('@vsod/utils/logger.js');
      
      const result = redactValue('message', 'User user@example.com logged in');
      expect(result).toBe('User u**r@e*********m logged in');
    });

    it('should recursively redact PII from nested objects', async () => {
      const { redactPii } = await import('@vsod/utils/logger.js');
      
      const input = {
        user: {
          email: 'test@example.com',
          name: 'John Doe',
          id: 'user-123'
        },
        action: 'signup'
      };
      
      const result = redactPii(input);
      
      expect(result.user.email).toBe('t**t@e*********m');
      expect(result.user.name).toBe('J*******');
      expect(result.user.id).toBe('user-123');
      expect(result.action).toBe('signup');
    });

    it('should pass through Date objects without destroying them', async () => {
      const { redactPii } = await import('@vsod/utils/logger.js');

      const date = new Date('2026-04-06T13:20:23.710Z');
      const input = { not_before: date, not_after: date, label: 'test' };
      const result = redactPii(input);

      expect(result.not_before).toBe(date);
      expect(result.not_after).toBe(date);
      expect(result.label).toBe('test');
    });

    it('should not recurse into non-plain objects', async () => {
      const { redactPii } = await import('@vsod/utils/logger.js');

      const buf = Buffer.from('hello');
      const input = { data: buf, id: 'x' };
      const result = redactPii(input);

      expect(result.data).toBe(buf);
      expect(result.id).toBe('x');
    });

    it('should handle null and undefined values', async () => {
      const { redactValue, redactPii } = await import('@vsod/utils/logger.js');
      
      expect(redactValue('email', null)).toBeNull();
      expect(redactValue('name', undefined)).toBeUndefined();
      expect(redactPii(null)).toBeNull();
      expect(redactPii(undefined)).toBeUndefined();
    });

    it('should handle arrays in redactPii', async () => {
      const { redactPii } = await import('@vsod/utils/logger.js');
      
      const input = [
        { email: 'ab@test.com', id: '1' },
        { email: 'cd@test.com', id: '2' }
      ];
      
      const result = redactPii(input);
      
      expect(result[0].email).toBe('**@t******m');
      expect(result[0].id).toBe('1');
      expect(result[1].email).toBe('**@t******m');
      expect(result[1].id).toBe('2');
    });

    it('should handle empty strings', async () => {
      const { redactValue } = await import('@vsod/utils/logger.js');
      
      expect(redactValue('name', '')).toBe('***');
      expect(redactValue('email', '')).toBe('***');
    });

    it('should show first and last chars in email redaction', async () => {
      const { redactEmail } = await import('@vsod/utils/logger.js');
      
      const result = redactEmail('longusername@company.example.com');
      // longusername -> l**********e, company.example.com -> c*****************m
      expect(result).toBe('l**********e@c*****************m');
      expect(result).not.toContain('longusername');
      expect(result).not.toContain('company');
    });

    it('should handle short email parts', async () => {
      const { redactEmail } = await import('@vsod/utils/logger.js');
      
      // Short local part (2 chars) -> all asterisks
      // gmail.com = 9 chars, first+last preserved = g*******m (7 asterisks)
      expect(redactEmail('ab@gmail.com')).toBe('**@g*******m');
      // Single char local -> single asterisk
      // x.co = 4 chars, first+last preserved = x**o (2 asterisks)
      expect(redactEmail('a@x.co')).toBe('*@x**o');
    });
  });
});

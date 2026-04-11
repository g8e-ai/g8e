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
 * Cache-Aside Service
 *
 * All writes go to the VSODB document store first (authoritative), then the
 * cache is updated in place with the new value.
 *
 * WRITE:  DB write  →  setex (cache updated in place)
 *                   →  del   (if DB returns no data — eviction is intentional;
 *                             the next read will re-populate from the DB)
 * READ:   KV get   →  miss: DB read + setex
 * DELETE: DB delete →  del
 */

import { VSOBaseModel } from '../../models/base.js';
import { logger } from '../../utils/logger.js';
import crypto from 'crypto';
import { SourceComponent } from '../../constants/ai.js';
import { Collections } from '../../constants/collections.js';
import { SESSION_TTL_SECONDS } from '../../constants/session.js';
import { CacheTTL } from '../../constants/service_config.js';
import { KVKey } from '../../constants/kv_keys.js';


const TTL_STRATEGIES = {
    [Collections.WEB_SESSIONS]: SESSION_TTL_SECONDS,
    [Collections.OPERATOR_SESSIONS]: SESSION_TTL_SECONDS,
    [Collections.USERS]: CacheTTL.USER,
    [Collections.API_KEYS]: CacheTTL.API_KEY,
    [Collections.CASES]: CacheTTL.CASE,
    [Collections.INVESTIGATIONS]: CacheTTL.INVESTIGATION,
    [Collections.ORGANIZATIONS]: CacheTTL.ORGANIZATION,
    [Collections.SETTINGS]: CacheTTL.SETTINGS,
    [Collections.OPERATORS]: CacheTTL.OPERATOR,
};

class CacheAsideService {
    /**
     * Initialize cache-aside service.
     * 
     * @param {Object} kvClient - KV client for caching
     * @param {Object} dbClient - VSODB document client (authoritative data store)
     * @param {string} componentName - Component name for key prefixing
     * @param {number} defaultTTL - Default TTL for cached items (seconds)
     */
    constructor(kvClient, dbClient, componentName = SourceComponent.VSOD, defaultTTL = CacheTTL.DEFAULT) {
        this.kvClient = kvClient;
        this.db = dbClient;
        this.componentName = componentName;
        this.defaultTTL = defaultTTL;
        
        logger.info(`[${componentName.toUpperCase()}-CACHE-ASIDE] Service initialized (VSODB document store writes, VSODB KV cache)`);
    }

    /**
     * Generate cache key.
     * 
     * For sessions: KVKey.webSessionKey(id) or KVKey.operatorSessionKey(id)
     * For other collections: KVKey.doc(collection, documentId)
     * 
     * @param {string} collection - Collection name
     * @param {string} documentId - Document ID
     * @returns {string} Cache key
     */
    _makeKey(collection, documentId) {
        if (collection === Collections.WEB_SESSIONS)      return KVKey.webSessionKey(documentId);
        if (collection === Collections.OPERATOR_SESSIONS) return KVKey.operatorSessionKey(documentId);
        return KVKey.doc(collection, documentId);
    }

    /**
     * Get TTL based on collection type.
     * @param {string} collection - Collection name
     * @returns {number} TTL in seconds
     */
    _getTTLForCollection(collection) {
        return TTL_STRATEGIES[collection] || this.defaultTTL;
    }

    // ===== WRITE OPERATIONS (VSODB document store) =====

    /**
     * Create document using cache-aside pattern.
     *
     * Flow:
     * 1. Write to VSODB document store (authoritative)
     * 2. Update VSODB KV cache synchronously
     * 3. Return success
     *
     * @param {string} collection - Collection name
     * @param {string} documentId - Document ID
     * @param {VSOBaseModel|Object} data - Model instance or plain object
     * @param {number} ttl - Cache TTL (uses collection-specific TTL if null)
     * @returns {Promise<{success: boolean, documentId?: string, cached?: boolean, error?: string}>}
     */
    async createDocument(collection, documentId, data, ttl = null) {
        const flat = data instanceof VSOBaseModel ? data.forDB() : data;
        try {
            const result = await this.db.setDocument(collection, documentId, flat);

            if (!result.success) {
                logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] DB write failed`, {
                    collection,
                    documentId: documentId.substring(0, 12) + '...',
                    error: result.error
                });
                return {
                    success: false,
                    error: result.error || 'DB write failed'
                };
            }

            logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Document created in VSODB`, {
                collection,
                documentId: documentId.substring(0, 12) + '...'
            });

            await this._invalidateQueryCache(collection);

            const key = this._makeKey(collection, documentId);
            const cacheTTL = ttl !== null ? ttl : this._getTTLForCollection(collection);

            let cacheSuccess = false;
            try {
                await this.kvClient.set_json(key, flat, cacheTTL);
                cacheSuccess = true;
                logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Document cached in VSODB KV`, {
                    collection,
                    documentId: documentId.substring(0, 12) + '...',
                    ttl: cacheTTL
                });
            } catch (cacheError) {
                logger.warn(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] VSODB KV cache update failed (non-critical)`, {
                    collection,
                    documentId: documentId.substring(0, 12) + '...',
                    error: cacheError.message
                });
            }

            return {
                success: true,
                documentId,
                cached: cacheSuccess
            };

        } catch (error) {
            logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Create failed`, {
                collection,
                documentId: documentId.substring(0, 12) + '...',
                error: error.message
            });
            return {
                success: false,
                error: error.message
            };
        }
    }

    /**
     * Update document using cache-aside pattern.
     *
     * DB-first write, then invalidate the cache key. The next read re-populates
     * from the DB. This guarantees the cache is always populated from a full
     * canonical DB read and eliminates stale-partial-data bugs.
     *
     * @param {string} collection - Collection name
     * @param {string} documentId - Document ID
     * @param {VSOBaseModel|Object} data - Model instance or partial patch (plain object)
     * @param {boolean} merge - Whether the DB should merge with existing fields (default true)
     * @returns {Promise<{success: boolean, documentId?: string, error?: string}>}
     */
    async updateDocument(collection, documentId, data, merge = true) {
        try {
            const result = await this.db.updateDocument(collection, documentId, data, merge);

            if (!result.success) {
                return { success: false, error: result.error || 'DB update failed' };
            }

            const key = this._makeKey(collection, documentId);
            try {
                await this.kvClient.del(key);
            } catch (cacheError) {
                logger.warn(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Cache invalidation after write failed (non-critical)`, {
                    collection,
                    documentId: documentId.substring(0, 12) + '...',
                    error: cacheError.message
                });
            }

            await this._invalidateQueryCache(collection);

            logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Document updated, cache invalidated`, {
                collection,
                documentId: documentId.substring(0, 12) + '...'
            });

            return { success: true, documentId };

        } catch (error) {
            logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Update failed`, {
                collection,
                documentId: documentId.substring(0, 12) + '...',
                error: error.message
            });
            return { success: false, error: error.message };
        }
    }

    /**
     * Delete document using cache-aside pattern.
     *
     * Deletes from DB then removes the cache key.  This is the only place a
     * cache key is deleted — because the document no longer exists.
     *
     * @param {string} collection - Collection name
     * @param {string} documentId - Document ID
     * @returns {Promise<{success: boolean, notFound: boolean, documentId?: string, error?: string}>}
     */
    async deleteDocument(collection, documentId) {
        try {
            const result = await this.db.deleteDocument(collection, documentId);

            const key = this._makeKey(collection, documentId);
            await this.kvClient.del(key);
            await this._invalidateQueryCache(collection);

            if (result.notFound) {
                logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Document not found on delete (cache cleared)`, {
                    collection,
                    documentId: documentId.substring(0, 12) + '...'
                });
                return { success: false, notFound: true, error: result.error };
            }

            if (!result.success) {
                return { success: false, notFound: false, error: result.error };
            }

            logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Document deleted`, {
                collection,
                documentId: documentId.substring(0, 12) + '...'
            });

            return { success: true, notFound: false, documentId };

        } catch (error) {
            logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Delete failed`, {
                collection,
                documentId: documentId.substring(0, 12) + '...',
                error: error.message
            });
            return { success: false, notFound: false, error: error.message };
        }
    }

    // ===== READ OPERATIONS (Cache-aside: VSODB KV → VSODB document store fallback) =====

    /**
     * Get document using cache-aside pattern.
     * 
     * Flow:
     * 1. Check VSODB KV cache (~1-5ms)
     * 2. If miss, read from VSODB document store
     * 3. Populate VSODB KV cache
     * 4. Return data
     * 
     * @param {string} collection - Collection name
     * @param {string} documentId - Document ID
     * @returns {Promise<Object|null>} Flat plain object or null if not found
     */
    async getDocument(collection, documentId) {
        const key = this._makeKey(collection, documentId);
        
        try {
            // 1. Check VSODB KV cache first
            const cached = await this.kvClient.get_json(key);
            if (cached !== null) {
                logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Cache HIT (VSODB KV)`, {
                    collection,
                    documentId: documentId.substring(0, 12) + '...'
                });
                return cached;
            }

            // 2. Cache MISS - read from VSODB document store (authoritative)
            logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Cache MISS - reading from VSODB`, {
                collection,
                documentId: documentId.substring(0, 12) + '...',
                dbClientComponent: this.db._http?.component || 'unknown'
            });

            const dbResponse = await this.db.getDocument(collection, documentId);

            if (!dbResponse.success || !dbResponse.data) {
                // Enhanced logging for VSODB connectivity issues
                if (dbResponse.error && dbResponse.error.includes('fetch failed')) {
                    logger.warn(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] VSODB connectivity issue during read`, {
                        collection,
                        documentId: documentId.substring(0, 12) + '...',
                        error: dbResponse.error,
                        listenUrl: this.db._http?.listenUrl || 'unknown',
                        fallbackBehavior: 'treating as document not found'
                    });
                } else {
                    logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Document not found in VSODB`, {
                        collection,
                        documentId: documentId.substring(0, 12) + '...',
                        dbError: dbResponse.error
                    });
                }
                return null;
            }

            const data = dbResponse.data;

            // 3. Populate VSODB KV cache for next read
            const ttl = this._getTTLForCollection(collection);
            try {
                await this.kvClient.set_json(key, data, ttl);
                logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Cache warmed from VSODB`, {
                    collection,
                    documentId: documentId.substring(0, 12) + '...',
                    ttl
                });
            } catch (cacheError) {
                logger.warn(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Failed to warm cache (non-critical)`, {
                    collection,
                    documentId: documentId.substring(0, 12) + '...',
                    error: cacheError.message
                });
            }

            return data;
            
        } catch (error) {
            logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Get failed`, {
                collection,
                documentId: documentId.substring(0, 12) + '...',
                error: error.message
            });
            return null;
        }
    }

    /**
     * Evict a single document's cache key.
     *
     * Use when an external writer (e.g. g8ee) has updated the document directly in the
     * DB and VSOD needs the next read to re-populate from the authoritative source.
     * Does not touch the DB.
     *
     * @param {string} collection
     * @param {string} documentId
     */
    async evictDocument(collection, documentId) {
        const key = this._makeKey(collection, documentId);
        try {
            await this.kvClient.del(key);
        } catch (error) {
            logger.warn(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] evictDocument failed (non-critical)`, {
                collection,
                documentId: documentId.substring(0, 12) + '...',
                error: error.message
            });
        }
    }

    // ===== QUERY CACHE =====

    /**
     * Query documents using cache-aside pattern.
     *
     * Flow:
     * 1. Check query cache (KV)
     * 2. On miss, query DB directly
     * 3. Populate query cache with result
     * 4. Return results
     *
     * @param {string} collection - Collection name
     * @param {Array} filters - Query filter array
     * @param {number|null} limit - Optional result limit
     * @returns {Promise<Array>} Array of plain objects
     */
    async queryDocuments(collection, filters = [], limit = null) {
        const queryParams = { filters, limit };
        try {
            const cached = await this.getQueryResult(collection, queryParams);
            if (cached !== null) {
                return cached;
            }

            const result = await this.db.queryDocuments(collection, filters, limit);
            const data = result.success ? result.data : [];

            await this.setQueryResult(collection, queryParams, data);
            return data;
        } catch (error) {
            logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] queryDocuments failed`, {
                collection,
                error: error.message
            });
            return [];
        }
    }

    /**
     * Invalidate all query cache entries for a collection.
     *
     * Called after any write so the next queryDocuments call re-fetches from DB.
     *
     * @param {string} collection - Collection name
     */
    async _invalidateQueryCache(collection) {
        const pattern = KVKey.query(collection, '*');
        try {
            const keys = await this.kvClient.keys(pattern);
            if (keys && keys.length > 0) {
                await Promise.all(keys.map(k => this.kvClient.del(k)));
            }
        } catch (error) {
            logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Query cache invalidation failed — stale results may be served`, {
                collection,
                error: error.message
            });
        }
    }

    /**
     * Get cached query results.
     *
     * @param {string} collection - Collection name
     * @param {Object} queryParams - Query parameters
     * @returns {Promise<Array|null>} Cached results or null
     */
    async getQueryResult(collection, queryParams) {
        const queryStr = JSON.stringify(queryParams, Object.keys(queryParams).sort());
        const filterHash = crypto.createHash('md5').update(queryStr).digest('hex');
        const key = KVKey.query(collection, filterHash);

        try {
            const cached = await this.kvClient.get_json(key);
            if (cached !== null) {
                logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Query cache HIT`, {
                    collection
                });
                return cached;
            }
            return null;
        } catch (error) {
            logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Query get failed`, {
                collection,
                error: error.message
            });
            return null;
        }
    }

    /**
     * Cache query results.
     *
     * @param {string} collection - Collection name
     * @param {Object} queryParams - Query parameters
     * @param {Object[]} results - Plain objects — caller must call .forKV() on each before passing
     * @param {number} ttl - Cache TTL (default 5 minutes)
     * @returns {Promise<boolean>} True if successfully cached
     */
    async setQueryResult(collection, queryParams, results, ttl = CacheTTL.QUERY) {
        if (!Array.isArray(results)) {
            throw new Error('CacheAsideService.setQueryResult requires an array of results');
        }
        
        // Don't cache empty results - this causes stale "not found" errors
        // when data is added after an initial empty query
        if (results.length === 0) {
            return false;
        }
        
        const queryStr = JSON.stringify(queryParams, Object.keys(queryParams).sort());
        const filterHash = crypto.createHash('md5').update(queryStr).digest('hex');
        const key = KVKey.query(collection, filterHash);

        try {
            await this.kvClient.set_json(key, results, ttl);
            logger.info(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Query results cached`, {
                collection,
                resultCount: results.length,
                ttl
            });
            return true;
        } catch (error) {
            logger.error(`[${this.componentName.toUpperCase()}-CACHE-ASIDE] Query cache set failed`, {
                collection,
                error: error.message
            });
            return false;
        }
    }

    // ===== KV PASSTHROUGH =====
    // Every KV operation in the application must go through these methods.
    // No service holds or calls the raw kvClient directly.

    async kvGet(key) {
        return this.kvClient.get(key);
    }

    async kvSet(key, value, ...args) {
        return this.kvClient.set(key, value, ...args);
    }

    async kvDel(...keys) {
        return this.kvClient.del(...keys);
    }

    async kvSetex(key, ttl, value) {
        return this.kvClient.setex(key, ttl, value);
    }

    async kvGetJson(key) {
        return this.kvClient.get_json(key);
    }

    async kvSetJson(key, value, ttl = null) {
        return this.kvClient.set_json(key, value, ttl);
    }

    async kvTtl(key) {
        return this.kvClient.ttl(key);
    }

    async kvExpire(key, seconds) {
        return this.kvClient.expire(key, seconds);
    }

    async kvExists(key) {
        return this.kvClient.exists(key);
    }

    async kvIncr(key) {
        return this.kvClient.incr(key);
    }

    async kvDecr(key) {
        return this.kvClient.decr(key);
    }

    async kvSadd(key, ...members) {
        return this.kvClient.sadd(key, ...members);
    }

    async kvSrem(key, ...members) {
        return this.kvClient.srem(key, ...members);
    }

    async kvSmembers(key) {
        return this.kvClient.smembers(key);
    }

    async kvScard(key) {
        return this.kvClient.scard(key);
    }

    async kvRpush(key, ...values) {
        return this.kvClient.rpush(key, ...values);
    }

    async kvLrange(key, start, stop) {
        return this.kvClient.lrange(key, start, stop);
    }

    async kvZadd(key, score, member) {
        return this.kvClient.zadd(key, score, member);
    }

    async kvZrem(key, ...members) {
        return this.kvClient.zrem(key, ...members);
    }

    async kvZrange(key, start, stop) {
        return this.kvClient.zrange(key, start, stop);
    }

    async kvZrevrange(key, start, stop) {
        return this.kvClient.zrevrange(key, start, stop);
    }

    async kvKeys(pattern) {
        return this.kvClient.keys(pattern);
    }

}

export { CacheAsideService };

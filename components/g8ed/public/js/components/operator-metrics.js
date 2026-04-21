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
 * OperatorMetrics - Extracts normalized Operator metrics from a raw operator payload.
 *
 * Canonical sources (each field reads from exactly one place — no unions):
 *   operator.system_info                 - persisted OperatorSystemInfo; static identity
 *                                          (hostname, os, architecture, current_user,
 *                                          public_ip, internal_ip, interfaces, cpu_count,
 *                                          memory_mb, *_details, environment).
 *   operator.latest_heartbeat_snapshot   - persisted OperatorHeartbeat (nested).
 *     .performance.{cpu_percent, memory_percent, disk_percent, network_latency,
 *                   memory_used_mb, memory_total_mb, disk_used_gb, disk_total_gb}
 *     .uptime.{uptime_display, uptime_seconds}
 *     .version_info.{operator_version, status}
 *
 * Both the persisted document and the SSE envelope metrics share this one shape
 * (shared/models/wire/heartbeat.json#operator_heartbeat). No flat projection exists.
 */
export class OperatorMetrics {
    constructor(rawData) {
        this.rawData = rawData || {};
        this._extractData();
    }

    _extractData() {
        const actualData = this.rawData.data || this.rawData;

        const systemInfo = actualData.system_info || {};
        const snapshot = actualData.latest_heartbeat_snapshot || {};
        const perf = snapshot.performance || {};
        const uptime = snapshot.uptime || {};
        const version = snapshot.version_info || {};

        this.hostname = systemInfo.hostname ?? null;
        this.os = systemInfo.os ?? null;
        this.architecture = systemInfo.architecture ?? null;
        this.currentUser = systemInfo.current_user ?? null;
        this.publicIp = systemInfo.public_ip ?? null;
        this.internalIp = systemInfo.internal_ip ?? null;
        this.interfaces = systemInfo.interfaces ?? null;
        this.cpuCount = systemInfo.cpu_count ?? null;

        this.cpu = perf.cpu_percent ?? null;
        this.memory = perf.memory_percent ?? null;
        this.disk = perf.disk_percent ?? null;
        this.networkLatency = perf.network_latency ?? null;
        this.memoryUsedMb = perf.memory_used_mb ?? null;
        this.memoryTotalMb = perf.memory_total_mb ?? null;
        this.diskUsedGb = perf.disk_used_gb ?? null;
        this.diskTotalGb = perf.disk_total_gb ?? null;

        this.uptime = uptime.uptime_display ?? null;
        this.uptimeSeconds = uptime.uptime_seconds ?? null;

        this.version = version.operator_version ?? null;
        this.status = version.status ?? null;

        this.osDetails = systemInfo.os_details ?? null;
        this.userDetails = systemInfo.user_details ?? null;
        this.diskDetails = systemInfo.disk_details ?? null;
        this.memoryDetails = systemInfo.memory_details ?? null;
        this.environment = systemInfo.environment ?? null;
    }

    _findValue(getters) {
        for (const getter of getters) {
            try {
                const value = getter();
                if (value !== undefined && value !== null) return value;
            } catch (e) {
                // continue
            }
        }
        return undefined;
    }

    _findInObject(obj, paths) {
        if (!obj || typeof obj !== 'object') return undefined;
        for (const path of paths) {
            if (path.includes('.')) {
                const parts = path.split('.');
                let current = obj;
                let found = true;
                for (const part of parts) {
                    if (current && typeof current === 'object' && part in current) {
                        current = current[part];
                    } else {
                        found = false;
                        break;
                    }
                }
                if (found && current !== undefined) return current;
            } else {
                if (path in obj && obj[path] !== undefined) return obj[path];
            }
        }
        return undefined;
    }

    getCpuDisplay() {
        return (this.cpu !== undefined && this.cpu !== null) ? `${this.cpu.toFixed(1)}%` : 'N/A';
    }

    getMemoryDisplay() {
        return (this.memory !== undefined && this.memory !== null) ? `${this.memory.toFixed(1)}%` : 'N/A';
    }

    getDiskDisplay() {
        return (this.disk !== undefined && this.disk !== null) ? `${this.disk.toFixed(1)}%` : 'N/A';
    }

    getNetworkDisplay() {
        return (this.networkLatency !== undefined && this.networkLatency !== null) ? `${this.networkLatency}ms` : 'N/A';
    }

    getUptimeDisplay() {
        return this.uptime || 'N/A';
    }

    getHostnameDisplay() {
        return this.hostname || 'Unknown';
    }

    getCurrentUserDisplay() {
        return this.currentUser || 'Unknown';
    }

    isValid() {
        return !!(this.hostname || this.cpu !== undefined || this.memory !== undefined);
    }

    toLogObject() {
        return {
            hostname: this.hostname,
            current_user: this.currentUser,
            cpu: this.cpu,
            memory: this.memory,
            disk: this.disk,
            network_latency: this.networkLatency,
            uptime: this.uptime,
            os: this.os,
            architecture: this.architecture,
            version: this.version,
            status: this.status,
            public_ip: this.publicIp,
            uptime_seconds: this.uptimeSeconds
        };
    }

    toObject() {
        return {
            hostname: this.hostname,
            currentUser: this.currentUser,
            os: this.os,
            architecture: this.architecture,
            cpu: this.cpu,
            memory: this.memory,
            disk: this.disk,
            networkLatency: this.networkLatency,
            uptime: this.uptime,
            uptimeSeconds: this.uptimeSeconds,
            version: this.version,
            status: this.status,
            publicIp: this.publicIp,
            interfaces: this.interfaces
        };
    }
}

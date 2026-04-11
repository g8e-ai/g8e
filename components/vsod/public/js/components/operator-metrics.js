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
 * OperatorMetrics - Extracts and normalizes Operator metrics from raw heartbeat payloads.
 *
 * Data sources (in priority order):
 *   system_info              - single source of truth for static system data
 *   latest_heartbeat_snapshot - current performance metrics
 *   Heartbeat payload fields  - real-time SSE data (system_identity, performance_metrics, etc.)
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
        const systemIdentity = actualData.system_identity || {};
        const metrics = actualData.performance_metrics || {};
        const uptime = actualData.uptime_info || {};
        const network = actualData.network_info || {};
        const version = actualData.version_info || {};

        this.hostname = systemInfo.hostname || systemIdentity.hostname || null;
        this.os = systemInfo.os || systemIdentity.os || null;
        this.architecture = systemInfo.architecture || systemIdentity.architecture || null;
        this.currentUser = systemInfo.current_user || systemIdentity.current_user || null;

        this.cpu = snapshot.cpu_percent ?? metrics.cpu_percent ?? null;
        this.memory = snapshot.memory_percent ?? metrics.memory_percent ?? systemInfo.memory_details?.percent ?? null;
        this.disk = snapshot.disk_percent ?? metrics.disk_percent ?? systemInfo.disk_details?.percent ?? null;
        this.networkLatency = snapshot.network_latency ?? metrics.network_latency ?? null;

        this.uptime = snapshot.uptime ?? uptime.uptime ?? uptime.uptime_string ?? null;
        this.uptimeSeconds = snapshot.uptime_seconds ?? uptime.uptime_seconds ?? null;

        this.version = version.vsa_version || actualData.vsa_version || null;
        this.status = version.status || actualData.status || null;

        this.publicIp = systemInfo.public_ip || network.public_ip || null;
        this.interfaces = systemInfo.interfaces || network.interfaces;

        this.osDetails = systemInfo.os_details || null;
        this.userDetails = systemInfo.user_details || null;
        this.diskDetails = systemInfo.disk_details || null;
        this.memoryDetails = systemInfo.memory_details || null;
        this.environment = systemInfo.environment || null;

        this.memoryUsedMb = systemInfo.memory_details?.used_mb ?? metrics.memory_used_mb ?? null;
        this.memoryTotalMb = systemInfo.memory_mb ?? systemInfo.memory_details?.total_mb ?? metrics.memory_total_mb ?? null;
        this.diskUsedGb = systemInfo.disk_details?.used_gb ?? metrics.disk_used_gb ?? null;
        this.diskTotalGb = systemInfo.disk_details?.total_gb ?? metrics.disk_total_gb ?? null;
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

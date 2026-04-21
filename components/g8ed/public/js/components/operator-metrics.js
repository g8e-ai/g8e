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
 * Single source of truth: operator.latest_heartbeat_snapshot (OperatorHeartbeat,
 * shared/models/wire/heartbeat.json#operator_heartbeat). Same shape whether read
 * from the persisted operator document or the SSE envelope. All identity,
 * performance, network, and detail fields are read from this one nested object —
 * never from operator.system_info, which is a stale, redundant projection that
 * does not update on every heartbeat and causes identity fields to blank out.
 */
export class OperatorMetrics {
    constructor(rawData) {
        this.rawData = rawData || {};
        this._extractData();
    }

    _extractData() {
        const actualData = this.rawData.data || this.rawData;

        const snapshot = actualData.latest_heartbeat_snapshot || {};
        const identity = snapshot.system_identity || {};
        const network = snapshot.network || {};
        const perf = snapshot.performance || {};
        const uptime = snapshot.uptime || {};
        const version = snapshot.version_info || {};

        this.hostname = identity.hostname ?? null;
        this.os = identity.os ?? null;
        this.architecture = identity.architecture ?? null;
        this.currentUser = identity.current_user ?? null;
        this.cpuCount = identity.cpu_count ?? null;
        this.memoryMb = identity.memory_mb ?? null;

        this.publicIp = network.public_ip ?? null;
        this.internalIp = network.internal_ip ?? null;
        this.interfaces = network.interfaces ?? null;

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

        this.osDetails = snapshot.os_details ?? null;
        this.userDetails = snapshot.user_details ?? null;
        this.diskDetails = snapshot.disk_details ?? null;
        this.memoryDetails = snapshot.memory_details ?? null;
        this.environment = snapshot.environment ?? null;
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

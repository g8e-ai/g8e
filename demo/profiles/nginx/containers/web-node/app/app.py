#!/usr/bin/env python3
# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Simple Flask web app that nginx proxies to."""

import os
import json
import random
import time
from datetime import UTC, datetime
from flask import Flask, jsonify, request

app = Flask(__name__)

NODE_ID = os.environ.get('NODE_ID', 'node-01')
START_TIME = time.time()
REQUEST_COUNT = 0


@app.route('/')
def index():
    """Main page served through nginx proxy."""
    global REQUEST_COUNT
    REQUEST_COUNT += 1
    return jsonify({
        'node': NODE_ID,
        'status': 'ok',
        'version': '2.1.0',
        'timestamp': datetime.now(UTC).isoformat() + 'Z'
    })


@app.route('/health')
def health():
    """Health check endpoint."""
    uptime = int(time.time() - START_TIME)
    return jsonify({
        'status': 'healthy',
        'node': NODE_ID,
        'uptime_seconds': uptime,
        'requests_served': REQUEST_COUNT
    })


@app.route('/api/v1/data')
def api_data():
    """Simulated API endpoint."""
    global REQUEST_COUNT
    REQUEST_COUNT += 1
    delay = random.uniform(0.01, 0.15)
    time.sleep(delay)
    return jsonify({
        'node': NODE_ID,
        'records': random.randint(10, 500),
        'response_time_ms': round(delay * 1000, 2)
    })


@app.route('/api/v1/status')
def api_status():
    """Status endpoint with system info."""
    return jsonify({
        'node': NODE_ID,
        'services': {
            'nginx': 'running',
            'app': 'running',
            'watchdog': 'running',
            'data_sync': 'running'
        },
        'load': round(random.uniform(0.1, 2.5), 2),
        'memory_percent': round(random.uniform(30, 75), 1)
    })

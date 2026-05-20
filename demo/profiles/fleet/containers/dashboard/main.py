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

import os
import json
import docker
from fastapi import FastAPI
from fastapi.responses import HTMLResponse
from fastapi.staticfiles import StaticFiles

app = FastAPI()
client = docker.from_env()

@app.get("/api/nodes")
async def get_nodes():
    nodes = []
    try:
        # Filter for containers belonging to this demo
        containers = client.containers.list(all=True, filters={"label": "demo.service=operator-node"})
        
        for c in containers:
            node_data = {
                "id": c.short_id,
                "name": c.name,
                "status": c.status, # Docker status (running, exited, etc.)
                "operator_online": False,
                "metrics": {},
                "last_error": None
            }
            
            if c.status == "running":
                try:
                    # Check if operator process is running
                    exec_op = c.exec_run("pgrep -f g8e.operator")
                    node_data["operator_online"] = (exec_op.exit_code == 0)
                    
                    # Read metrics
                    exec_metrics = c.exec_run("cat /var/log/edge-service/metrics.json")
                    if exec_metrics.exit_code == 0:
                        node_data["metrics"] = json.loads(exec_metrics.output.decode())
                except Exception as e:
                    node_data["last_error"] = str(e)
            
            nodes.append(node_data)
            
    except Exception as e:
        # CodeQL: Don't return raw exception messages to the client
        print(f"Error listing nodes: {e}")
        return {"error": "Internal server error", "nodes": []}
    
    # Sort nodes by name
    nodes.sort(key=lambda x: x["name"])
    return nodes

@app.get("/", response_class=HTMLResponse)
async def read_index():
    with open("index.html", "r") as f:
        return f.read()

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8080)

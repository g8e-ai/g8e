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
        return {"error": str(e), "nodes": []}
    
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

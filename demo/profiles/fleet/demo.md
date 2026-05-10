# Fleet Profile

This profile demonstrates running an actual fleet of 20 to 1000 devices natively on your machine, leveraging the system's hardware capabilities.

To accomplish this efficiently, we use a featherweight approach:
- Nodes are run via Docker Compose, built on top of a minimal Alpine Linux image (~5MB).
- Each node runs a lightweight edge device microservice that generates realistic logs and metrics.
- The `g8e` Operator binary is executed on each node.
- Memory usage is heavily optimized, preventing typical OOM (Out Of Memory) issues when spinning up 1000 nodes on a single host.

### Node Operations
Each node runs the `g8e` Operator and a background process that simulates edge device activity:
- **Metrics collection**: CPU, memory, disk usage generated in JSON format every 5 seconds.
- **Log generation**: Standard operator logs for troubleshooting demos.
- **Data storage**: Metrics are written to `/var/log/edge-service/metrics.json`.

### Why Docker Compose?
The `g8e` platform relies on a **system fingerprint** to deduplicate devices upon registration. The fingerprint logic combines hardware info and the OS hostname to generate a unique hash for each physical device.

If we simply used a `while` loop running the binary in the background directly on your host machine, all instances would share the same hostname and machine ID, thus generating the **same exact fingerprint**. The backend would reject them as duplicate registrations.

By using Docker Compose, every single container automatically gets assigned a unique hostname, ensuring the system fingerprints are completely unique across all 1000 nodes without spoofing or breaking platform logic.

### Fleet Dashboard
A real-time visual dashboard is included to monitor the fleet:
- **URL**: `http://localhost:8080`
- **Visualization**: Shows a grid of all nodes with their operator status (Online/Offline) and edge service metrics (CPU, Memory).
- **Polling**: Automatically refreshes every 5 seconds.

### Usage
Start the demo with any desired number of nodes (`N` specifies the node count):

```bash
cd demo/profiles/fleet
make up N=20 -d dlk_YOUR_DEVICE_LINK_TOKEN
```

After starting, visit `http://localhost:8080` to see the fleet in action.

### Resource Requirements
Because each node only runs the lightweight operator binary, a basic microservice, and a minimal alpine container, CPU overhead is near zero, and RAM overhead is just a few megabytes per node. 1000 nodes can easily run on ~5GB of RAM.

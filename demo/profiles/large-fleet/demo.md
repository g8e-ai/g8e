# Large Fleet Profile

This profile demonstrates running an actual fleet of 20 to 1000 devices natively on your machine, leveraging the system's hardware capabilities. 

To accomplish this efficiently, we use a featherweight approach:
- Nodes are run via Docker Compose, built on top of a minimal Alpine Linux image (~5MB).
- Only the `g8e` Operator binary is executed on each node.
- Memory usage is heavily optimized, preventing typical OOM (Out Of Memory) issues when spinning up 1000 nodes on a single host.

### Why Docker Compose?
The `g8e` platform relies on a **system fingerprint** to deduplicate devices upon registration. The fingerprint logic combines hardware info and the OS hostname to generate a unique hash for each physical device.

If we simply used a `while` loop running the binary in the background directly on your host machine, all instances would share the same hostname and machine ID, thus generating the **same exact fingerprint**. The backend would reject them as duplicate registrations. 

By using Docker Compose, every single container automatically gets assigned a unique hostname, ensuring the system fingerprints are completely unique across all 1000 nodes without spoofing or breaking platform logic.

### Usage
Start the demo with any desired number of nodes (`N` specifies the node count):

```bash
cd demo/profiles/large-fleet
make up N=1000 -d dlk_YOUR_DEVICE_LINK_TOKEN
```

### Resource Requirements
Because each node only runs the lightweight operator binary and a basic alpine container, CPU overhead is near zero, and RAM overhead is just a few megabytes per node. 1000 nodes can easily run on ~5GB of RAM.

import time
import json
import random
import os
import logging
from datetime import datetime

# Configuration
LOG_DIR = "/var/log/edge-service"
LOG_FILE = os.path.join(LOG_DIR, "service.log")
METRICS_FILE = os.path.join(LOG_DIR, "metrics.json")

# Ensure log directory exists
os.makedirs(LOG_DIR, exist_ok=True)

# Setup logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    handlers=[
        logging.FileHandler(LOG_FILE),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger("edge-service")

def get_metrics():
    """Simulate CPU, Memory, and Disk metrics."""
    return {
        "timestamp": datetime.utcnow().isoformat(),
        "hostname": os.uname().nodename,
        "cpu_usage_percent": round(random.uniform(5.0, 45.0), 2),
        "memory_usage_mb": round(random.uniform(150.0, 800.0), 2),
        "disk_usage_percent": round(random.uniform(20.0, 70.0), 2),
        "status": "healthy"
    }

def main():
    logger.info("Edge device microservice started.")
    
    while True:
        try:
            # 1. Health check & Request processing simulation
            logger.info("Performing health check...")
            time.sleep(random.uniform(0.1, 0.5)) # Simulated workload latency
            
            # 2. Metrics collection
            metrics = get_metrics()
            with open(METRICS_FILE, "w") as f:
                json.dump(metrics, f)
            
            # 3. Log generation
            chance = random.random()
            if chance < 0.05:
                logger.error("Failed to synchronize data to central storage: connection timeout")
            elif chance < 0.15:
                logger.warning(f"High latency detected in request processing: {random.uniform(500, 1500):.2f}ms")
            else:
                logger.info("Request processed successfully")

            # 4. Data sync simulation
            if random.random() < 0.3:
                logger.info("Data synchronization completed")

            time.sleep(10) # Wait before next cycle
            
        except Exception as e:
            logger.error(f"Error in edge-service loop: {str(e)}")
            time.sleep(5)

if __name__ == "__main__":
    main()

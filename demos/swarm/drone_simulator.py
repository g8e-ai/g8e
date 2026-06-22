#!/usr/bin/env python3
"""
Drone Simulator for Battlefield Swarm Demo
Simulates individual drone behavior with battlefield telemetry data
"""

import json
import random
import time
import sys
from datetime import datetime
from pathlib import Path

class DroneSimulator:
    def __init__(self, drone_id, drone_type):
        self.drone_id = drone_id
        self.drone_type = drone_type
        self.battery = random.randint(60, 100)
        self.altitude = 0
        self.coordinates = [0.0, 0.0]
        self.status = "standby"
        self.mission_time = 0
        self.sensor_data = {
            "thermal": [],
            "lidar": [],
            "camera": []
        }
        
    def launch(self):
        """Launch drone and begin mission"""
        self.status = "active"
        self.altitude = random.randint(100, 250)
        self.coordinates = [round(random.uniform(34.0, 36.0), 2), 
                            round(random.uniform(45.0, 47.0), 2)]
        print(f"[{self.drone_id}] LAUNCHED - Alt: {self.altitude}m, Pos: {self.coordinates}")
        
    def update_telemetry(self):
        """Update drone telemetry with battlefield data"""
        if self.status != "active":
            return
            
        # Simulate movement
        self.coordinates[0] += round(random.uniform(-0.1, 0.1), 2)
        self.coordinates[1] += round(random.uniform(-0.1, 0.1), 2)
        self.altitude += random.randint(-10, 10)
        self.altitude = max(50, min(300, self.altitude))
        
        # Simulate battery drain
        self.battery -= random.randint(1, 3)
        if self.battery <= 20:
            self.status = "returning"
            print(f"[{self.drone_id}] LOW BATTERY - Returning to base")
        elif self.battery <= 0:
            self.status = "landed"
            print(f"[{self.drone_id}] LANDED - Battery depleted")
            
        # Simulate sensor data collection
        self.sensor_data["thermal"].append({
            "timestamp": datetime.utcnow().isoformat(),
            "hotspots": random.randint(0, 5),
            "ambient_temp": random.randint(15, 35)
        })
        self.sensor_data["lidar"].append({
            "timestamp": datetime.utcnow().isoformat(),
            "obstacles": random.randint(0, 3),
            "terrain_height": random.randint(0, 100)
        })
        self.sensor_data["camera"].append({
            "timestamp": datetime.utcnow().isoformat(),
            "objects_detected": random.choice(["none", "vehicle", "person", "structure"]),
            "confidence": round(random.uniform(0.7, 0.99), 2)
        })
        
        # Keep only last 10 sensor readings
        for sensor in self.sensor_data:
            if len(self.sensor_data[sensor]) > 10:
                self.sensor_data[sensor] = self.sensor_data[sensor][-10:]
                
    def get_status(self):
        """Return current drone status"""
        return {
            "drone_id": self.drone_id,
            "type": self.drone_type,
            "status": self.status,
            "battery": self.battery,
            "altitude": self.altitude,
            "coordinates": self.coordinates,
            "mission_time": self.mission_time,
            "sensor_data": self.sensor_data
        }
        
    def execute_mission(self, duration_seconds=60):
        """Execute mission for specified duration"""
        self.launch()
        start_time = time.time()
        
        while time.time() - start_time < duration_seconds and self.status == "active":
            self.mission_time += 1
            self.update_telemetry()
            
            # Periodic status report
            if self.mission_time % 10 == 0:
                status = self.get_status()
                print(f"[{self.drone_id}] STATUS - Alt: {status['altitude']}m, "
                      f"Batt: {status['battery']}%, Pos: {status['coordinates']}")
                
            time.sleep(1)
            
        return self.get_status()

def load_battlefield_data():
    """Load battlefield intelligence from target-data"""
    data_path = Path("/var/g8e/target/battlefield_intel.json")
    if data_path.exists():
        with open(data_path) as f:
            return json.load(f)
    return None

def load_fleet_manifest():
    """Load drone fleet manifest"""
    data_path = Path("/var/g8e/target/drone_fleet_manifest.json")
    if data_path.exists():
        with open(data_path) as f:
            return json.load(f)
    return None

def main():
    # Get drone ID from environment or use default
    drone_id = sys.argv[1] if len(sys.argv) > 1 else "DRONE-001"
    drone_type = sys.argv[2] if len(sys.argv) > 2 else "recon"
    
    print(f"=== DRONE SIMULATOR: {drone_id} ({drone_type}) ===")
    
    # Load battlefield context
    battlefield = load_battlefield_data()
    if battlefield:
        print(f"Mission: {battlefield.get('mission_id', 'UNKNOWN')}")
        print(f"Theater: {battlefield.get('theater', 'UNKNOWN')}")
        print(f"Sectors: {len(battlefield.get('sectors', []))}")
    
    fleet = load_fleet_manifest()
    if fleet:
        print(f"Fleet: {fleet.get('fleet_id', 'UNKNOWN')}")
        print(f"Total Drones: {fleet.get('total_drones', 0)}")
    
    # Initialize and run drone
    drone = DroneSimulator(drone_id, drone_type)
    
    try:
        # Run mission loop
        while True:
            final_status = drone.execute_mission(duration_seconds=30)
            
            # If landed, recharge and relaunch
            if final_status["status"] == "landed":
                print(f"[{drone_id}] RECHARGING...")
                time.sleep(5)
                drone.battery = 100
                drone.status = "standby"
                print(f"[{drone_id}] RECHARGED - Ready for next mission")
                
            # Brief pause between missions
            time.sleep(2)
            
    except KeyboardInterrupt:
        print(f"\n[{drone_id}] MISSION ABORTED - Landing safely")
        drone.status = "landed"
        print(json.dumps(drone.get_status(), indent=2))

if __name__ == "__main__":
    main()

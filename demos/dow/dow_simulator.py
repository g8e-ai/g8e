#!/usr/bin/env python3
"""
DoW Tactical Edge Sensor Simulator
Simulates SIGINT, EO/IR, and PNT Fusion sensor agents for the
Department of War (DoW) Challenge Areas 5 & 8 demonstration.

Each sensor type produces A2A-protocol-compatible JSON payloads
that can be submitted to the g8e gateway for governed cross-cueing.
"""

import json
import random
import time
import sys
from datetime import datetime, timezone
from pathlib import Path


class TacticalSensor:
    """Base class for tactical edge sensor agents."""

    def __init__(self, sensor_id, sensor_type):
        self.sensor_id = sensor_id
        self.sensor_type = sensor_type
        self.status = "standby"
        self.mission_time = 0

    def get_status(self):
        return {
            "sensor_id": self.sensor_id,
            "type": self.sensor_type,
            "status": self.status,
            "mission_time": self.mission_time,
            "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        }

    def load_tactical_env(self):
        path = Path("/var/g8e/target/tactical_environment.json")
        if path.exists():
            with open(path) as f:
                return json.load(f)
        return None

    def load_payload_manifest(self):
        path = Path("/var/g8e/target/payload_manifest.json")
        if path.exists():
            with open(path) as f:
                return json.load(f)
        return None


class SigintSensor(TacticalSensor):
    """Broadband SIGINT receiver agent (Challenge Area 5)."""

    def __init__(self, sensor_id):
        super().__init__(sensor_id, "sigint")
        self.detections = []

    def scan(self, env):
        """Scan RF environment and emit detection events."""
        self.status = "active"
        rf_env = env.get("rf_environment", {})
        signals = rf_env.get("signals", [])

        for signal in signals:
            detection = {
                "event_id": f"SIGINT-DET-{self.mission_time:04d}-{signal['signal_id']}",
                "sensor_id": self.sensor_id,
                "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
                "signal_id": signal["signal_id"],
                "type": signal["type"],
                "frequency_mhz": signal["frequency_mhz"],
                "bearing_deg": signal["bearing_deg"],
                "confidence": signal["confidence"],
                "classification": signal["classification"],
                "coordinates": signal["coordinates"],
                "a2a_action": "cross_cue_request",
                "a2a_target_agent": "agent-eoir",
                "a2a_skill": "slew_to_coordinates",
                "a2a_payload": {
                    "target_coordinates": signal["coordinates"],
                    "reason": f"SIGINT detection: {signal['classification']}",
                    "priority": "high" if signal["confidence"] > 0.85 else "medium",
                },
            }
            self.detections.append(detection)
            print(f"[{self.sensor_id}] DETECTION - {signal['type']} at {signal['frequency_mhz']} MHz "
                  f"(conf: {signal['confidence']}, class: {signal['classification']})")
            print(f"  → A2A cross-cue request to {detection['a2a_target_agent']}: "
                  f"slew to {signal['coordinates']}")
            print(f"  JSON: {json.dumps(detection)}")

    def run(self, iterations=0):
        env = self.load_tactical_env()
        if not env:
            print(f"[{self.sensor_id}] ERROR - tactical_environment.json not found")
            return

        print(f"[{self.sensor_id}] SIGINT sensor online. Mission: {env.get('mission_id', 'UNKNOWN')}")
        print(f"[{self.sensor_id}] Scanning RF environment...")

        while iterations == 0 or self.mission_time < iterations:
            self.mission_time += 1
            self.scan(env)
            if iterations == 0 or self.mission_time < iterations:
                time.sleep(5)

        self.status = "standby"
        print(f"[{self.sensor_id}] Scan complete. {len(self.detections)} detections emitted.")


class EoirSensor(TacticalSensor):
    """EO/IR gimbal camera agent (Challenge Area 5)."""

    def __init__(self, sensor_id):
        super().__init__(sensor_id, "eoir")
        self.azimuth = 0
        self.elevation = 0
        self.tracking = False

    def slew_to(self, coordinates, env):
        """Slew camera to target coordinates (simulated actuator)."""
        self.status = "slewing"
        target_az = random.randint(0, 360)
        target_el = random.randint(0, 90)

        print(f"[{self.sensor_id}] SLEWING to {coordinates} "
              f"(az: {target_az}°, el: {target_el}°)")

        self.azimuth = target_az
        self.elevation = target_el
        self.status = "tracking"
        self.tracking = True

        result = {
            "event_id": f"EOIR-SLEW-{self.mission_time:04d}",
            "sensor_id": self.sensor_id,
            "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
            "action": "slew_complete",
            "coordinates": coordinates,
            "azimuth_deg": self.azimuth,
            "elevation_deg": self.elevation,
            "tracking": True,
            "governance_note": "Slew authorized via L2 consensus GovernanceEnvelope",
        }
        print(f"  → Slew complete. Tracking target at {coordinates}")
        print(f"  JSON: {json.dumps(result)}")

    def run(self, iterations=0):
        env = self.load_tactical_env()
        if not env:
            print(f"[{self.sensor_id}] ERROR - tactical_environment.json not found")
            return

        eoir = env.get("eoir_payload", {})
        print(f"[{self.sensor_id}] EO/IR camera online. Camera: {eoir.get('camera_id', 'UNKNOWN')}")
        print(f"[{self.sensor_id}] Status: {eoir.get('status', 'standby')}")

        while iterations == 0 or self.mission_time < iterations:
            self.mission_time += 1

            # Simulate receiving a cross-cue from SIGINT
            if self.mission_time % 10 == 0:
                signals = env.get("rf_environment", {}).get("signals", [])
                if signals:
                    target = signals[0]["coordinates"]
                    self.slew_to(target, env)

            if iterations == 0 or self.mission_time < iterations:
                time.sleep(5)

        self.status = "standby"
        print(f"[{self.sensor_id}] Mission complete. Returning to standby.")


class PNTFusionSensor(TacticalSensor):
    """PNT Fusion engine agent (Challenge Area 8 - Alternative PNT)."""

    def __init__(self, sensor_id):
        super().__init__(sensor_id, "pnt_fusion")
        self.sources = []
        self.consensus_position = None
        self.spoofing_detected = False

    def fuse(self, env):
        """Fuse PNT sources and run BFT consensus voting."""
        self.status = "active"
        pnt_sources = env.get("pnt_sources", [])
        self.sources = pnt_sources

        trusted_positions = []
        spoofed_sources = []

        for source in pnt_sources:
            is_spoofed = source.get("spoofed", False)
            tag = " [SPOOFED]" if is_spoofed else ""
            print(f"[{self.sensor_id}] Source {source['source_id']} ({source['type']}): "
                  f"{source['coordinates']} (uncertainty: {source['uncertainty_m']}m){tag}")

            if is_spoofed:
                spoofed_sources.append(source)
            else:
                trusted_positions.append(source)

        if trusted_positions:
            # Simple consensus: average trusted coordinates
            lats = [float(p["coordinates"].split(",")[0]) for p in trusted_positions]
            lons = [float(p["coordinates"].split(",")[1]) for p in trusted_positions]
            self.consensus_position = f"{sum(lats)/len(lats):.4f}, {sum(lons)/len(lons):.4f}"

            print(f"[{self.sensor_id}] CONSENSUS POSITION: {self.consensus_position} "
                  f"(from {len(trusted_positions)} trusted sources)")

        if spoofed_sources:
            self.spoofing_detected = True
            for spoof in spoofed_sources:
                print(f"[{self.sensor_id}] ⚠ BFT SPOOFING DETECTED: {spoof['source_id']} "
                      f"diverges from consensus!")
                print(f"  Spoofed coords: {spoof['coordinates']} vs consensus: {self.consensus_position}")
                print(f"  Note: {spoof.get('spoof_note', 'GNSS spoofing detected')}")

            result = {
                "event_id": f"PNT-BFT-ALERT-{self.mission_time:04d}",
                "sensor_id": self.sensor_id,
                "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
                "action": "bft_spoofing_detected",
                "spoofed_sources": [s["source_id"] for s in spoofed_sources],
                "consensus_position": self.consensus_position,
                "trusted_sources": [s["source_id"] for s in trusted_positions],
                "governance_action": "reject_gnss_input",
                "doctrine_rule": "pnt_diversion_detected",
                "confidence": 0.90,
            }
            print(f"  → BFT consensus rejects spoofed source. Operator fails closed.")
            print(f"  JSON: {json.dumps(result)}")
        else:
            self.spoofing_detected = False
            print(f"[{self.sensor_id}] All PNT sources in consensus. No spoofing detected.")

    def run(self, iterations=0):
        env = self.load_tactical_env()
        if not env:
            print(f"[{self.sensor_id}] ERROR - tactical_environment.json not found")
            return

        print(f"[{self.sensor_id}] PNT Fusion engine online.")
        print(f"[{self.sensor_id}] Fusing {len(env.get('pnt_sources', []))} PNT sources...")

        while iterations == 0 or self.mission_time < iterations:
            self.mission_time += 1
            self.fuse(env)
            if iterations == 0 or self.mission_time < iterations:
                time.sleep(5)

        self.status = "standby"
        print(f"[{self.sensor_id}] PNT fusion complete.")


def main():
    sensor_id = sys.argv[1] if len(sys.argv) > 1 else "SIGINT-01"
    sensor_type = sys.argv[2] if len(sys.argv) > 2 else "sigint"
    iterations = int(sys.argv[3]) if len(sys.argv) > 3 else 0

    print(f"=== DoW TACTICAL EDGE SENSOR: {sensor_id} ({sensor_type}) ===")

    if sensor_type == "sigint":
        sensor = SigintSensor(sensor_id)
    elif sensor_type == "eoir":
        sensor = EoirSensor(sensor_id)
    elif sensor_type == "pnt_fusion":
        sensor = PNTFusionSensor(sensor_id)
    else:
        print(f"Unknown sensor type: {sensor_type}")
        print("Valid types: sigint, eoir, pnt_fusion")
        sys.exit(1)

    try:
        sensor.run(iterations=iterations)
    except KeyboardInterrupt:
        print(f"\n[{sensor_id}] MISSION ABORTED - Returning to standby")
        sensor.status = "standby"
        print(json.dumps(sensor.get_status(), indent=2))


if __name__ == "__main__":
    main()

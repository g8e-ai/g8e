#!/usr/bin/env python3
"""MAVSDK CLI bridge for the g8e live demo. Usage:
  drone_cmd.py takeoff <alt_m> | set_altitude <alt_m> | goto <lat> <lon> <alt_m> | land | status
"""
import asyncio, sys
from mavsdk import System

CONNECT_TIMEOUT = 10  # seconds to wait for SITL connection


def parse_float(value, label):
    try:
        return float(value)
    except ValueError:
        print(f"Error: {label} must be a number, got '{value}'")
        sys.exit(1)


async def connect_drone(drone):
    await drone.connect(system_address="udpin://0.0.0.0:14540")
    try:
        async def _wait_connect():
            async for state in drone.core.connection_state():
                if state.is_connected:
                    return
        await asyncio.wait_for(_wait_connect(), timeout=CONNECT_TIMEOUT)
    except asyncio.TimeoutError:
        print(f"Error: could not connect to drone within {CONNECT_TIMEOUT}s. Is SITL running on udpin://0.0.0.0:14540?")
        sys.exit(1)


async def main():
    if len(sys.argv) < 2:
        print("Usage: drone_cmd.py <command> [args...]")
        print("Commands: takeoff <alt_m> | set_altitude <alt_m> | goto <lat> <lon> <alt_m> | land | status")
        sys.exit(1)

    cmd = sys.argv[1]
    drone = System()
    await connect_drone(drone)

    if cmd == "takeoff":
        if len(sys.argv) != 3:
            print("Usage: drone_cmd.py takeoff <alt_m>"); sys.exit(1)
        alt = parse_float(sys.argv[2], "altitude")
        await drone.action.set_takeoff_altitude(alt)
        await drone.action.arm()
        await drone.action.takeoff()
        print(f"Takeoff to {alt}m initiated.")
    elif cmd == "set_altitude":
        if len(sys.argv) != 3:
            print("Usage: drone_cmd.py set_altitude <alt_m>"); sys.exit(1)
        alt = parse_float(sys.argv[2], "altitude")
        async for pos in drone.telemetry.position():
            await drone.action.goto_location(pos.latitude_deg, pos.longitude_deg,
                                             pos.absolute_altitude_m - pos.relative_altitude_m + alt, 0)
            break
        print(f"Altitude change to {alt}m initiated.")
    elif cmd == "goto":
        if len(sys.argv) != 5:
            print("Usage: drone_cmd.py goto <lat> <lon> <alt_m>"); sys.exit(1)
        lat = parse_float(sys.argv[2], "latitude")
        lon = parse_float(sys.argv[3], "longitude")
        alt = parse_float(sys.argv[4], "altitude")
        await drone.action.goto_location(lat, lon, alt, 0)
        print(f"Going to {lat},{lon} at {alt}m.")
    elif cmd == "land":
        if len(sys.argv) != 2:
            print("Usage: drone_cmd.py land"); sys.exit(1)
        await drone.action.land()
        print("Landing initiated.")
    elif cmd == "status":
        if len(sys.argv) != 2:
            print("Usage: drone_cmd.py status"); sys.exit(1)
        async for pos in drone.telemetry.position():
            print(f"Lat: {pos.latitude_deg}, Lon: {pos.longitude_deg}, Alt(rel): {pos.relative_altitude_m}m")
            break
    else:
        print(f"Unknown command: {cmd}")
        print("Commands: takeoff <alt_m> | set_altitude <alt_m> | goto <lat> <lon> <alt_m> | land | status")
        sys.exit(1)

    await asyncio.sleep(2)  # let the command propagate before the process exits

asyncio.run(main())

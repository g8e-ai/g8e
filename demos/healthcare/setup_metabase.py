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

"""
Auto-setup script for Metabase compliance dashboard.
Runs once on first startup to configure the admin account, database connection,
and pre-load the two required DCBS/OHA compliance queries.
"""
import json
import time
import urllib.error
import urllib.request
import sys
import os

METABASE_URL = os.environ.get("METABASE_URL", "http://compliance-dashboard:3000")
ADMIN_EMAIL = os.environ.get("MB_ADMIN_EMAIL", "admin@g8e.local")
ADMIN_PASSWORD = os.environ.get("MB_ADMIN_PASSWORD", "Metabase1!")
DB_NAME = "oregon_pa_metrics"
DB_HOST = os.environ.get("DB_HOST", "reporting-db")
DB_USER = os.environ.get("DB_USER", "compliance_admin")
DB_PASS = os.environ.get("DB_PASS", "secure_hipaa_password")


def _http_get(url, headers=None, timeout=10):
    req = urllib.request.Request(url, headers=headers or {})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        body = resp.read().decode("utf-8")
        return resp.status, body


def _http_post(url, payload, headers=None, timeout=10):
    data = json.dumps(payload).encode("utf-8")
    hdrs = {"Content-Type": "application/json"}
    if headers:
        hdrs.update(headers)
    req = urllib.request.Request(url, data=data, headers=hdrs, method="POST")
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        body = resp.read().decode("utf-8")
        return resp.status, body


def wait_for_metabase(timeout=180):
    print("Waiting for Metabase to be ready...")
    for i in range(timeout):
        try:
            status, body = _http_get(f"{METABASE_URL}/api/health", timeout=5)
            if status == 200 and json.loads(body).get("status") == "ok":
                print("Metabase is ready.")
                return True
        except (urllib.error.URLError, OSError):
            pass
        if i % 10 == 0 and i > 0:
            print(f"  still waiting... ({i}s)")
        time.sleep(1)
    print("Timed out waiting for Metabase.")
    return False


def get_setup_token():
    try:
        status, body = _http_get(f"{METABASE_URL}/api/session/properties", timeout=5)
        if status == 200:
            return json.loads(body).get("setup-token")
    except Exception:
        pass
    return None


def complete_setup(token):
    print("Completing initial Metabase setup...")
    payload = {
        "token": token,
        "user": {
            "first_name": "Admin",
            "last_name": "User",
            "email": ADMIN_EMAIL,
            "password": ADMIN_PASSWORD,
            "site_name": "Healthcare Compliance Dashboard",
        },
        "prefs": {
            "site_name": "Healthcare Compliance Dashboard",
            "site_locale": "en",
            "allow_tracking": False,
        },
    }
    try:
        status, body = _http_post(f"{METABASE_URL}/api/setup", payload, timeout=30)
    except urllib.error.HTTPError as e:
        print(f"Setup failed ({e.code}): {e.read().decode('utf-8', errors='replace')}")
        return False
    if status == 200:
        print("Initial setup complete.")
        return True
    print(f"Setup failed ({status}): {body}")
    return False


def authenticate():
    try:
        status, body = _http_post(
            f"{METABASE_URL}/api/session",
            {"username": ADMIN_EMAIL, "password": ADMIN_PASSWORD},
            timeout=10,
        )
    except urllib.error.HTTPError as e:
        print(f"Authentication failed ({e.code}): {e.read().decode('utf-8', errors='replace')}")
        return None
    if status == 200:
        token = json.loads(body).get("id")
        print(f"Authenticated as {ADMIN_EMAIL}.")
        return token
    print(f"Authentication failed ({status}): {body}")
    return None


def find_database(token):
    try:
        status, body = _http_get(
            f"{METABASE_URL}/api/database",
            headers={"X-Metabase-Session": token},
            timeout=10,
        )
    except urllib.error.HTTPError:
        return None
    if status != 200:
        return None
    parsed = json.loads(body)
    dbs = parsed.get("data", parsed) if isinstance(parsed, dict) else parsed
    for db in dbs:
        if db.get("name") == DB_NAME:
            return db["id"]
    return None


def add_database(token):
    print(f"Adding {DB_NAME} database connection...")
    try:
        status, body = _http_post(
            f"{METABASE_URL}/api/database",
            {
                "engine": "postgres",
                "name": DB_NAME,
                "details": {
                    "host": DB_HOST,
                    "port": 5432,
                    "user": DB_USER,
                    "password": DB_PASS,
                    "dbname": DB_NAME,
                },
            },
            headers={"X-Metabase-Session": token},
            timeout=30,
        )
    except urllib.error.HTTPError as e:
        print(f"Failed to add database ({e.code}): {e.read().decode('utf-8', errors='replace')}")
        return None
    if status == 200:
        db_id = json.loads(body)["id"]
        print(f"Database added (id={db_id}).")
        return db_id
    print(f"Failed to add database ({status}): {body}")
    return None


def wait_for_database(token, retries=12, delay=5):
    for attempt in range(retries):
        db_id = find_database(token)
        if db_id:
            print(f"Database ready (id={db_id}).")
            return db_id
        if attempt < retries - 1:
            print(f"  waiting for database sync... ({(attempt + 1) * delay}s)")
            time.sleep(delay)
    print("Database never became visible in Metabase.")
    return None


def questions_exist(token):
    try:
        status, body = _http_get(
            f"{METABASE_URL}/api/card",
            headers={"X-Metabase-Session": token},
            timeout=10,
        )
    except urllib.error.HTTPError:
        return False
    if status == 200:
        parsed = json.loads(body)
        cards = parsed if isinstance(parsed, list) else []
        names = {c["name"] for c in cards}
        return (
            "DCBS March 1 Filing - Denial Rates by Request Type" in names
            and "OHA March 31 Filing - Median Decision Time" in names
        )
    return False


def create_question(token, db_id, name, sql, description):
    print(f"Creating question: {name}")
    try:
        status, body = _http_post(
            f"{METABASE_URL}/api/card",
            {
                "name": name,
                "description": description,
                "dataset_query": {
                    "type": "native",
                    "native": {"query": sql},
                    "database": db_id,
                },
                "display": "table",
                "visualization_settings": {},
            },
            headers={"X-Metabase-Session": token},
            timeout=10,
        )
    except urllib.error.HTTPError as e:
        print(f"  Failed ({e.code}): {e.read().decode('utf-8', errors='replace')}")
        return False
    if status == 200:
        print(f"  Created (id={json.loads(body)['id']}).")
        return True
    print(f"  Failed ({status}): {body}")
    return False


def main():
    if not wait_for_metabase():
        sys.exit(1)

    setup_token = get_setup_token()
    if setup_token:
        if not complete_setup(setup_token):
            sys.exit(1)
        time.sleep(3)
    else:
        print("Setup already complete, skipping initial setup.")

    session_token = authenticate()
    if not session_token:
        sys.exit(1)

    if questions_exist(session_token):
        print("Compliance questions already exist. Nothing to do.")
        print("Dashboard: http://localhost:3001")
        print(f"Login: {ADMIN_EMAIL} (use the configured MB_ADMIN_PASSWORD)")
        return

    db_id = find_database(session_token)
    if not db_id:
        db_id = add_database(session_token)
        if not db_id:
            sys.exit(1)

    db_id = wait_for_database(session_token)
    if not db_id:
        sys.exit(1)

    create_question(
        session_token,
        db_id,
        "DCBS March 1 Filing - Denial Rates by Request Type",
        """SELECT
  request_type,
  COUNT(*) AS total,
  SUM(CASE WHEN status = 'DENIED' THEN 1 ELSE 0 END) AS denied,
  ROUND(100.0 * SUM(CASE WHEN status = 'DENIED' THEN 1 ELSE 0 END) / COUNT(*), 2) AS denial_pct
FROM pa_requests
GROUP BY request_type;""",
        "Annual compliance report: percent of standard vs expedited requests denied",
    )

    create_question(
        session_token,
        db_id,
        "OHA March 31 Filing - Median Decision Time",
        """SELECT
  PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY days_elapsed) AS median_days,
  MAX(days_elapsed) AS max_days,
  SUM(CASE WHEN status = 'SLA_BREACHED' THEN 1 ELSE 0 END) AS breached_count
FROM pa_requests;""",
        "Annual compliance report: median time elapsed for decisions and SLA breaches",
    )

    print("\n=== Metabase setup complete ===")
    print("Dashboard: http://localhost:3001")
    print(f"Login: {ADMIN_EMAIL} / {ADMIN_PASSWORD}")
    print("Questions are in the 'Questions' section.")


if __name__ == "__main__":
    main()

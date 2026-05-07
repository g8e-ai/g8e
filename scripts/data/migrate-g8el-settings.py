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
G8EL Migration Script

Scans platform and user settings for the deprecated 'g8el' LLM provider 
and resets it to 'llamacpp' or 'ollama'.
"""

import sys
import json
from datetime import datetime, timezone
from typing import Dict, Any

from _lib import (
    g8es_request,
    query_collection,
)

G8ES_SETTINGS_COLLECTION = 'settings'

def migrate_doc(doc: Dict[str, Any]) -> bool:
    """Migrate a single settings document. Returns True if modified."""
    modified = False
    settings = doc.get('settings', {})
    
    # Check primary, assistant, and lite providers
    fields_to_check = ['llm_primary_provider', 'llm_assistant_provider', 'llm_lite_provider']
    
    for field in fields_to_check:
        if settings.get(field) == 'g8el':
            print(f"  [FIX] Resetting {field} from 'g8el' to 'llamacpp'")
            settings[field] = 'llamacpp'
            modified = True
            
    if modified:
        doc['updated_at'] = datetime.now(timezone.utc).isoformat()
        
    return modified

def main():
    print("━" * 52)
    print("  G8EL Settings Migration")
    print("━" * 52)
    
    # 1. Get all documents from settings collection
    print("[migrate] Fetching all settings documents...")
    docs = query_collection(G8ES_SETTINGS_COLLECTION)
    
    if not docs:
        print("[migrate] No settings documents found.")
        return 0
        
    print(f"[migrate] Found {len(docs)} documents. Scanning for 'g8el'...")
    
    modified_count = 0
    for doc in docs:
        doc_id = doc.get('id')
        if not doc_id:
            continue
            
        if migrate_doc(doc):
            print(f"[migrate] Updating document: {doc_id}")
            try:
                g8es_request('PUT', f'/db/{G8ES_SETTINGS_COLLECTION}/{doc_id}', doc)
                modified_count += 1
            except Exception as e:
                print(f"[migrate] Error updating {doc_id}: {e}", file=sys.stderr)
                
    print("━" * 52)
    print(f"  Migration complete. Updated {modified_count} document(s).")
    print("━" * 52)
    return 0

if __name__ == '__main__':
    sys.exit(main())

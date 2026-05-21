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

import sys
import json
import argparse

def main():
    parser = argparse.ArgumentParser(description="Standardized SSE Stream parser for CLI")
    parser.add_argument("--cursor-file", help="Path to write the latest event ID to")
    args = parser.parse_args()

    current_id = None
    current_event_type = None
    current_data = []

    # Stream from stdin line-by-line (unbuffered)
    for line in sys.stdin:
        stripped = line.strip()
        
        if not stripped:
            # End of event block. Process if we have accumulated data.
            if current_data:
                data_str = "".join(current_data)
                try:
                    payload = json.loads(data_str)
                    etype = current_event_type or payload.get("event", {}).get("type")
                    
                    if current_id and args.cursor_file:
                        try:
                            with open(args.cursor_file, "w") as f:
                                f.write(str(current_id))
                        except Exception as e:
                            print(f"[SSE-STREAM] Error writing cursor file: {e}", file=sys.stderr)

                    # Event handling mirroring chat.sh structure
                    if etype == 'g8e.v1.ai.llm.chat.iteration.text.chunk.received':
                        content = payload.get('event', {}).get('data', {}).get('content', '')
                        sys.stdout.write(content)
                    elif etype in ('g8e.v1.ai.llm.chat.iteration.failed', 'g8e.v1.ai.llm.chat.iteration.stopped'):
                        err = payload.get('event', {}).get('data', {}).get('error', 'Unknown error')
                        sys.stdout.write(f'\n\033[1;31m[{etype}]\033[0m {err}\n')
                    elif etype == 'g8e.v1.ai.llm.chat.iteration.thinking.started':
                        thinking = payload.get('event', {}).get('data', {}).get('thinking', '')
                        action = payload.get('event', {}).get('data', {}).get('action_type', 'UPDATE')
                        if action == 'START':
                            sys.stdout.write('\n\033[1;30mThinking...\033[0m ')
                        elif thinking and thinking != 'null':
                            sys.stdout.write('\033[1;30m.\033[0m')
                    elif etype == 'g8e.v1.ai.llm.chat.iteration.text.completed':
                        sys.stdout.write('\n')
                    elif etype and 'tool' in etype:
                        tool_name = payload.get('event', {}).get('data', {}).get('tool_name', 'unknown')
                        status = payload.get('event', {}).get('data', {}).get('status', '')
                        if status == 'STARTED':
                            sys.stdout.write(f'\n\033[1;34m[Tool: {tool_name}]\033[0m ')

                    sys.stdout.flush()

                    # Terminal event check
                    if etype in ('g8e.v1.ai.llm.chat.iteration.text.completed', 
                                 'g8e.v1.ai.llm.chat.iteration.failed', 
                                 'g8e.v1.ai.llm.chat.iteration.stopped'):
                        sys.exit(0)

                except Exception as e:
                    print(f"[SSE-STREAM] Error parsing SSE payload: {e}", file=sys.stderr)
                
                # Reset event-specific accumulators
                current_id = None
                current_event_type = None
                current_data = []
            continue

        if stripped.startswith(":"):
            # Comment line (like : heartbeat), ignore
            continue

        if line.startswith("id:"):
            current_id = line[3:].strip()
        elif line.startswith("event:"):
            current_event_type = line[6:].strip()
        elif line.startswith("data:"):
            current_data.append(line[5:].strip())

if __name__ == "__main__":
    main()

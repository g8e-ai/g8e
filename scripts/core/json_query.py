#!/usr/bin/env python3
import sys
import json
import argparse

def query_path(data, path_str):
    if not path_str or path_str == ".":
        return data
    
    parts = path_str.strip(".").split(".")
    current = data
    for part in parts:
        if isinstance(current, dict) and part in current:
            current = current[part]
        else:
            return None
    return current

def main():
    parser = argparse.ArgumentParser(description="Clean, modern JSON query utility.")
    parser.add_argument("file", help="Path to JSON file or '-' for stdin")
    parser.add_argument("path", nargs="?", default=".", help="Dot-separated query path")
    parser.add_argument("--default", default=None, help="Fallback default value if result is missing/null")
    parser.add_argument("--length", action="store_true", help="Output the length of the matched array or object")
    parser.add_argument("--exists", action="store_true", help="Exit with 0 if key exists and is truthy, else 1")
    parser.add_argument("--array", action="store_true", help="Iterate and print array items line-by-line")
    args = parser.parse_args()

    try:
        if args.file == "-":
            data = json.load(sys.stdin)
        else:
            with open(args.file, "r", encoding="utf-8") as f:
                data = json.load(f)
    except Exception as e:
        print(f"Error loading JSON: {e}", file=sys.stderr)
        sys.exit(1)

    result = query_path(data, args.path)

    if args.exists:
        if result is not None and result is not False:
            sys.exit(0)
        sys.exit(1)

    if result is None:
        if args.default is not None:
            result = args.default
        else:
            result = ""

    if args.length:
        if isinstance(result, (list, dict, str)):
            print(len(result))
        else:
            print(0)
        sys.exit(0)

    if args.array:
        if isinstance(result, list):
            for item in result:
                print(item)
        elif isinstance(result, dict):
            for k, v in result.items():
                print(f"{k}: {v}")
        elif result:
            print(result)
        sys.exit(0)

    if isinstance(result, bool):
        print(str(result).lower())
    elif isinstance(result, (list, dict)):
        print(json.dumps(result))
    else:
        print(result)

if __name__ == "__main__":
    main()

#!/bin/sh
# Shared path utilities for g8e shell scripts

# resolve_g8e_root returns the absolute path to the project root.
# Priority:
# 1. G8E_PROJECT_ROOT environment variable
# 2. Fallback: walks up from the script's directory until it detects the repository root.
resolve_g8e_root() {
    if [ -n "$G8E_PROJECT_ROOT" ]; then
        echo "$G8E_PROJECT_ROOT"
        return
    fi

    # Start from the directory of the script that sourced this file
    # Or fallback to current directory
    current_dir="$(pwd)"
    
    # Try to find root by looking for the 'g8e' binary or 'components' directory
    while [ "$current_dir" != "/" ]; do
        if [ -d "$current_dir/components" ] && [ -f "$current_dir/g8e" ]; then
            echo "$current_dir"
            return
        fi
        current_dir="$(dirname "$current_dir")"
    done

    # Final fallback: if we can't find it, use relative path from components if we are likely there
    # This is a bit risky but better than nothing
    case "$(pwd)" in
        */components/*)
            echo "$(pwd | sed 's/\/components\/.*//')"
            ;;
        *)
            pwd
            ;;
    esac
}

# Export it for use in the current shell
export G8E_PROJECT_ROOT="$(resolve_g8e_root)"

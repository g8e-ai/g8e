#!/bin/sh
# Shared path utilities for g8e shell scripts

# resolve_g8e_root returns the absolute path to the project root.
# This is the canonical root detection heuristic - all languages must match this logic.
# Priority:
# 1. G8E_PROJECT_ROOT environment variable
# 2. Walk up from current directory looking for marker: services/ directory AND g8e file
# 3. If in services/g8eo, walk up 2 levels to root
# 4. If in services/g8ee, walk up 2 levels to root
# 5. Fallback to current directory
resolve_g8e_root() {
    if [ -n "$G8E_PROJECT_ROOT" ]; then
        echo "$G8E_PROJECT_ROOT"
        return
    fi

    current_dir="$(pwd)"
    
    # Try to find root by looking for the marker: services/ directory AND g8e file
    while [ "$current_dir" != "/" ]; do
        if [ -d "$current_dir/services" ] && [ -f "$current_dir/g8e" ]; then
            echo "$current_dir"
            return
        fi
        current_dir="$(dirname "$current_dir")"
    done

    # If in services/g8eo, walk up 2 levels to root
    case "$(pwd)" in
        */services/g8eo/*)
            echo "$(pwd | sed 's/\/services\/g8eo\/.*//')"
            return
            ;;
    esac

    # If in services/g8ee, walk up 2 levels to root
    case "$(pwd)" in
        */services/g8ee/*)
            echo "$(pwd | sed 's/\/services\/g8ee\/.*//')"
            return
            ;;
    esac

    # Final fallback to current directory
    pwd
}

# Export it for use in the current shell
# Note: We don't auto-export here because this function is sourced by different scripts
# that may need to call it at different times. Callers should do:
#   G8E_PROJECT_ROOT="$(resolve_g8e_root)"
#   export G8E_PROJECT_ROOT

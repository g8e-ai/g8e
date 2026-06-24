# Windows Path Handling Guide

## Overview

This document describes the standardized approach for handling file paths in the g8e codebase, with special attention to Windows compatibility.

## The Problem

Windows path handling has several unique challenges:

1. **Drive Letters**: Absolute paths start with drive letters (e.g., `C:\`, `D:\`)
2. **UNC Paths**: Network paths use `\\server\share` format
3. **Mixed Separators**: Both `/` and `\` can appear in paths
4. **Double Path Joining**: Joining two absolute paths creates invalid paths like `C:\temp\C:\temp\file.db`

### Example of the Issue

```go
// WRONG: This creates C:\temp\C:\temp\data.db on Windows
dataDir := "C:\\temp"
dbPath := "C:\\temp\\data.db"
result := filepath.Join(dataDir, dbPath)  // C:\temp\C:\temp\data.db (incorrect)

// CORRECT: Use pathutil.SafeJoin
result := pathutil.SafeJoin(dataDir, dbPath)  // C:\temp\data.db (correct)
```

## Solution: pathutil Package

The `internal/pathutil` package provides Windows-safe path utilities.

### Core Functions

#### SafeJoin

Safely joins path elements, handling absolute paths correctly. If the first element in `elem` is absolute (as determined by `filepath.IsAbs`, which is OS-dependent), that element is used as-is instead of being concatenated with `base`. On Windows, drive-letter paths and UNC paths are recognized as absolute. On Unix, only paths starting with `/` are recognized as absolute.

```go
import "github.com/g8e-ai/g8e/internal/pathutil"

// Relative path - works like filepath.Join
path := pathutil.SafeJoin("/tmp", "data.db")  // /tmp/data.db

// Absolute second path on Windows - uses the absolute path
path := pathutil.SafeJoin("C:\\temp", "C:\\data\\file.db")  // C:\data\file.db

// Absolute second path on Unix - uses the absolute path
path := pathutil.SafeJoin("/tmp", "/var/lib/data.db")  // /var/lib/data.db

// Multiple elements
path := pathutil.SafeJoin("/tmp", "subdir", "file.txt")  // /tmp/subdir/file.txt

// Empty elements - returns base
path := pathutil.SafeJoin("/tmp")  // /tmp
```

#### ResolveDBPath

Specialized function for database paths (common pattern in our codebase):

```go
// If dbPath is relative, join with dataDir
dbPath := pathutil.ResolveDBPath("/var/lib/g8e", "g8e.db")  
// Result: /var/lib/g8e/g8e.db

// If dbPath is absolute, use it as-is
dbPath := pathutil.ResolveDBPath("/var/lib/g8e", "/opt/databases/g8e.db")
// Result: /opt/databases/g8e.db
```

#### NormalizePath

Cleans the path to remove redundant separators and resolves `.` and `..` segments. On Windows, also converts forward slashes to backslashes via `filepath.FromSlash`. On Unix, backslashes are treated as valid filename characters and are not converted.

```go
// On Windows: converts / to \ and cleans redundant separators
path := pathutil.NormalizePath("C:/temp//data")  // C:\temp\data

// On Unix: cleans redundant separators only
path := pathutil.NormalizePath("/tmp//data")  // /tmp/data
```

#### IsWindowsAbsPath

Checks if a path is an absolute Windows path, recognizing drive letters (case-insensitive) and UNC paths. This function is OS-independent and returns the same result on any platform.

```go
pathutil.IsWindowsAbsPath("C:\\temp")      // true
pathutil.IsWindowsAbsPath("D:/data")       // true
pathutil.IsWindowsAbsPath("c:\\temp")      // true (case-insensitive)
pathutil.IsWindowsAbsPath("\\\\server\\share")  // true (UNC, backslash)
pathutil.IsWindowsAbsPath("//server/share")     // true (UNC, forward slash)
pathutil.IsWindowsAbsPath("temp\\data")    // false
pathutil.IsWindowsAbsPath("/tmp/data")     // false
pathutil.IsWindowsAbsPath("")              // false
```

#### ToSlash

Converts OS-specific path separators to forward slashes. Useful for logging and display where forward slashes are universally readable:

```go
path := pathutil.ToSlash("C:\\temp\\data")  // C:/temp/data
```

#### FromSlash

Converts forward slashes to OS-specific separators. Useful when reading paths from configuration files that use forward slashes:

```go
path := pathutil.FromSlash("C:/temp/data")  // C:\temp\data (on Windows)
```

#### EnsureTrailingSeparator

Appends an OS-specific separator to the path if one is not already present. Useful for directory paths that need to be clearly distinguished from file paths:

```go
path := pathutil.EnsureTrailingSeparator("/tmp/data")  // /tmp/data/
path := pathutil.EnsureTrailingSeparator("/tmp/data/") // /tmp/data/
path := pathutil.EnsureTrailingSeparator("")           // ""
```

#### RemoveTrailingSeparator

Removes any trailing OS-specific separator from the path:

```go
path := pathutil.RemoveTrailingSeparator("/tmp/data/") // /tmp/data
path := pathutil.RemoveTrailingSeparator("/tmp/data")  // /tmp/data
```

#### MakeRelative

Attempts to make `targetPath` relative to `basePath`. If the conversion fails (for example, when the paths are on different Windows drives), the original `targetPath` is returned unchanged:

```go
rel := pathutil.MakeRelative("/var/lib/g8e", "/var/lib/g8e/data/file.db")
// Result: data/file.db

rel := pathutil.MakeRelative("/var/lib/g8e", "/opt/other")
// Result: /opt/other (fallback to original)
```

## Usage Guidelines

### 1. Configuration Path Resolution

When resolving paths from configuration:

```go
// BEFORE (problematic)
dbPath := filepath.Join(config.DataDir, config.DBPath)

// AFTER (correct)
dbPath := pathutil.ResolveDBPath(config.DataDir, config.DBPath)
```

### 2. Building Paths from Base Directory

When constructing paths relative to a base:

```go
// BEFORE (problematic)
if !filepath.IsAbs(vaultDir) {
    vaultDir = filepath.Join(projectRoot, vaultDir)
}

// AFTER (correct)
if !filepath.IsAbs(vaultDir) {
    vaultDir = pathutil.SafeJoin(projectRoot, vaultDir)
}
```

### 3. Multiple Path Segments

When joining multiple segments:

```go
// BEFORE (problematic)
ledgerPath := filepath.Join(config.DataDir, config.LedgerDir)
filesPath := filepath.Join(config.DataDir, config.LedgerDir, "files")

// AFTER (correct)
ledgerPath := pathutil.SafeJoin(config.DataDir, config.LedgerDir)
filesPath := pathutil.SafeJoin(config.DataDir, config.LedgerDir, "files")
```

### 4. Display and Logging

For cross-platform display in logs:

```go
// Convert to forward slashes for logging (more readable)
logger.Info("Database path", "path", pathutil.ToSlash(dbPath))
```

## Testing

### Writing Path Tests

Always test both relative and absolute path scenarios:

```go
func TestPathResolution(t *testing.T) {
    tests := []struct {
        name     string
        base     string
        elem     string
        expected string
        osCheck  string  // "windows", "unix", or ""
    }{
        {
            name:     "relative path",
            base:     "C:\\temp",
            elem:     "data.db",
            expected: "C:\\temp\\data.db",
            osCheck:  "windows",
        },
        {
            name:     "absolute path should not double-join",
            base:     "C:\\temp",
            elem:     "C:\\data\\file.db",
            expected: "C:\\data\\file.db",
            osCheck:  "windows",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if tt.osCheck == "windows" && runtime.GOOS != "windows" {
                t.Skip("Windows-only test")
            }
            result := pathutil.SafeJoin(tt.base, tt.elem)
            assert.Equal(t, filepath.Clean(tt.expected), filepath.Clean(result))
        })
    }
}
```

### Test Path Setup

Use `t.TempDir()` for test isolation:

```go
func TestWithPaths(t *testing.T) {
    tmpDir := t.TempDir()  // Automatically cleaned up
    
    // Build paths relative to tmpDir
    dataDir := pathutil.SafeJoin(tmpDir, "data")
    dbPath := pathutil.SafeJoin(dataDir, "test.db")
    
    // Create directories
    require.NoError(t, os.MkdirAll(dataDir, 0755))
}
```

## Common Patterns

### Pattern 1: Config-Based Path Resolution

```go
type Config struct {
    DataDir string
    DBPath  string  // Can be relative or absolute
}

func (c *Config) GetDBPath() string {
    return pathutil.ResolveDBPath(c.DataDir, c.DBPath)
}
```

### Pattern 2: Optional Absolute Override

```go
func resolvePath(baseDir, path string) string {
    if filepath.IsAbs(path) {
        return path  // Use absolute path as-is
    }
    return pathutil.SafeJoin(baseDir, path)
}
```

### Pattern 3: Multi-Level Directory Structure

```go
type Paths struct {
    Base    string
    Data    string
    Ledger  string
    Files   string
}

func NewPaths(base string) *Paths {
    return &Paths{
        Base:   base,
        Data:   pathutil.SafeJoin(base, "data"),
        Ledger: pathutil.SafeJoin(base, "data", "ledger"),
        Files:  pathutil.SafeJoin(base, "data", "ledger", "files"),
    }
}
```

## Migration Checklist

When updating existing code:

- [ ] Replace `filepath.Join` with `pathutil.SafeJoin` for path construction
- [ ] Use `pathutil.ResolveDBPath` for database path resolution
- [ ] Add OS-specific test cases for Windows
- [ ] Verify paths work with both relative and absolute inputs
- [ ] Test with `t.TempDir()` on Windows

## References

- `internal/pathutil/pathutil.go` - Implementation
- `internal/pathutil/pathutil_test.go` - Test examples
- `internal/paths/paths.go` - Primary consumer; constructs all infrastructure paths via `SafeJoin`
- `internal/cli/config/config.go` - Configuration path resolution via `SafeJoin`
- `internal/services/storage/audit_store.go` - Database path resolution via `ResolveDBPath`
- `internal/cli/cmd/vault.go` - CLI vault path handling via `SafeJoin`

## Related Issues

This approach fixes several categories of Windows path issues:

1. **Double path joining**: `C:\temp\C:\temp\file.db`
2. **Mixed separators**: Inconsistent `/` and `\` usage
3. **Test failures**: Paths constructed incorrectly in test environments
4. **Configuration errors**: Absolute paths in config not respected

## Best Practices

1. **Always use pathutil functions** for path construction
2. **Test on Windows** if you modify path-related code
3. **Use `t.TempDir()`** for test isolation
4. **Document path expectations** in configuration structs
5. **Normalize paths** before logging for consistency
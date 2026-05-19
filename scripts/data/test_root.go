package main
import (
"fmt"
"os/exec"
"strings"
)
func main() {
cmd := exec.Command("git", "rev-parse", "--show-toplevel")
out, err := cmd.Output()
fmt.Printf("Root: %q, err: %v\n", strings.TrimSpace(string(out)), err)
}

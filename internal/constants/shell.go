// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package constants provides shell command execution constants.
package constants

// Terminal control character constants
const (
	// CtrlC is the ETX (End of Text) control character, sent by Ctrl+C.
	CtrlC = 3

	// Backspace is the BS (Backspace) control character.
	Backspace = 8

	// Delete is the DEL (Delete) control character.
	Delete = 127

	// PrintableASCIIStart is the first printable ASCII character (space).
	PrintableASCIIStart = 32

	// PrintableASCIIEnd is the last printable ASCII character (tilde).
	PrintableASCIIEnd = 126
)

// Shell command execution constants
const (
	// DefaultShellCommandTimeout is the default timeout for shell command execution in seconds
	DefaultShellCommandTimeout = 30

	// MaxShellCommandTimeout is the maximum allowed timeout for shell command execution in seconds
	MaxShellCommandTimeout = 300

	// ShutdownTimeout is the timeout for graceful shutdown in seconds
	ShutdownTimeout = 15

	// LocalhostHostname is the hostname for local execution
	LocalhostHostname = "localhost"

	// LocalhostIP is the IP address for local execution
	LocalhostIP = "127.0.0.1"

	// RemoteEphemeralScriptTemplate is the bash script injected into remote hosts for Operator deployment.
	// It handles graceful cleanup on session termination.
	RemoteEphemeralScriptTemplate = `set -e
B=$(mktemp)
cat > "$B"
chmod +x "$B"
cleanup() {
  sig=${1:-TERM}
  if [ -n "${PID:-}" ] && kill -0 "$PID" 2>/dev/null; then
    kill -"$sig" "-$PID" 2>/dev/null || kill -"$sig" "$PID" 2>/dev/null || true
    for _ in 1 2 3 4 5 6 7 8 9 10; do
      kill -0 "$PID" 2>/dev/null || break
      sleep 0.2
    done
    kill -0 "$PID" 2>/dev/null && { kill -KILL "-$PID" 2>/dev/null || kill -KILL "$PID" 2>/dev/null || true; }
  fi
  rm -f "$B"
}
trap 'cleanup TERM; exit 143' HUP INT TERM
trap 'rm -f "$B"' EXIT
setsid "$B" %s < /dev/null &
PID=$!
wait "$PID"`

	// RemoteInjectedBinaryMessage is shown when only the binary is injected without execution.
	RemoteInjectedBinaryMessage = "[g8e]Node Binary injected into %s -- run it manually: %s operator run -e <endpoint> [options]"

	// RemoteInjectedScriptMinimal is the minimal script for binary injection.
	RemoteInjectedScriptMinimal = `set -e; B=$(mktemp); cat > "$B"; chmod +x "$B"; trap 'rm -f "$B"' EXIT; echo "%s"`
)

// DangerousCommands is the list of commands that are blocked by safety policy
var DangerousCommands = []string{
	"rm",
	"dd",
	"mkfs",
	"fdisk",
	"format",
	"del",
	"erase",
	"shred",
	"wipe",
	"killall",
	"pkill",
	"reboot",
	"shutdown",
	"halt",
	"poweroff",
	"init",
	"systemctl",
	"service",
	"iptables",
	"ip6tables",
	"nft",
	"ufw",
	"firewall-cmd",
	"route",
	"ifconfig",
	"ip",
	"brctl",
	"tc",
	"modprobe",
	"insmod",
	"rmmod",
	"depmod",
	"mount",
	"umount",
	"swapon",
	"swapoff",
	"mkswap",
	"lvcreate",
	"lvremove",
	"lvchange",
	"vgcreate",
	"vgremove",
	"pvcreate",
	"pvremove",
	"cryptsetup",
	"passwd",
	"chpasswd",
	"usermod",
	"userdel",
	"groupmod",
	"crontab",
	"at",
	"batch",
	"sudo",
	"su",
	"doas",
	"runuser",
	"curl",
	"wget",
}

// DangerousPatterns is the list of patterns that are blocked by safety policy
var DangerousPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	":(){:|:&};:",
	"dd if=/dev/zero",
	"mkfs",
	"> /dev/sda",
	"> /dev/vda",
	"chmod 777 /",
	"chown -R",
	"nc -l",
	"ncat -l",
	"ssh",
	"scp",
	"rsync",
}

// ShellInjectionPatterns is the list of shell injection patterns that are blocked
var ShellInjectionPatterns = []string{
	"$(",
	"`",
	"|",
}

// ShellMetacharacters is the list of shell metacharacters that are not allowed for SSH execution
var ShellMetacharacters = []string{
	"$",
	"`",
	"\\",
	";",
	"&",
	"|",
	">",
	"<",
	"\n",
	"\r",
}

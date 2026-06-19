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

// Shell command execution constants
const (
	// DefaultShellCommandTimeout is the default timeout for shell command execution in seconds
	DefaultShellCommandTimeout = 30

	// MaxShellCommandTimeout is the maximum allowed timeout for shell command execution in seconds
	MaxShellCommandTimeout = 300

	// LocalhostHostname is the hostname for local execution
	LocalhostHostname = "localhost"

	// LocalhostIP is the IP address for local execution
	LocalhostIP = "127.0.0.1"
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
	"wget",
	"curl",
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

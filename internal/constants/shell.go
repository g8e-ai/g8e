// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

// Package constants provides shell command execution constants.
package constants

import "time"

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

	// ExecutionWaitDelay is the duration Wait will delay after the process exits before canceling still-running I/O copies
	ExecutionWaitDelay = 5 * time.Second

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

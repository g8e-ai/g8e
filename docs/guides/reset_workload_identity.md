# Recover workloads after a gateway identity reset

`g8e gw clean` archives the gateway runtime, including its CA. When the gateway
starts again it has a new identity. Existing workload credentials and trust still
belong to the previous gateway.

`make clean` only removes build artifacts and Go caches; it preserves Gateway
state and workload identities. Older versions also called `gw clean`, which
changed the Gateway CA on the next start and left local workloads trusting the
archived Gateway. If that happened, use the recovery procedure below once.

Run `make full-setup` and select the workload directories you intend to use. Before
launching, the launcher compares their saved trust with the local gateway CA and
asks once whether to reset stale identities. Declining launches no workloads.
Unattended `make full` refuses stale identities without prompting. To confirm this recovery in a script, use `make full RESET_IDENTITIES=1` with your `.env` endpoints and any explicit directory flags.
This option only resets stale identities in the selected local directories.

You can also reset one workload explicitly:

```sh
./g8e operator reset-identity --working-dir ~/.ollama/g8e/provenance
./g8e ensemble reset-identity
```

Use `--yes` to confirm a scripted reset. Operator reset stops workers belonging
to that directory on Linux; ensemble reset stops the local Python service.
The reset removes installed enrollment certificates and private keys, cached
gateway trust, and pending enrollment state. It preserves databases, execution
vault keys, model files, configuration, and logs. Gateway runtimes and symlinked
identity paths are refused. An explicit reset clears local identity only; it
does not revoke an identity on a gateway that still exists.

`make full` installs the local gateway's on-disk CA bundle before requesting
fresh enrollment. Workloads retain that trust during enrollment; a TLS failure
does not authorize replacing a CA. Remote launch instructions require a CA bundle
copied through a trusted channel and independently verified, supplied with
`--trust-bundle`. Remote directories are never reset by the local launcher.

Enroll your CLI identity if necessary, then review and approve intended requests:

```sh
./g8e auth enroll user -e localhost
./g8e auth enroll pending
./g8e auth enroll approve <id> --yes
```

The launcher reports connected operators, ready g8ee, pending enrollment or
approval, and failed workers with log paths. Workers still starting when the
startup observation period ends are labeled `starting`. A failed worker makes
the launcher exit unsuccessfully. `gw clean` reports known local workload
identities using the launcher registry and its fixed default directories; it
does not modify those directories or search the home directory recursively.

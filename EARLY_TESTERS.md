# Early testers — how this works

g8e is a solo-built, experimental zero-trust execution platform for AI agents. Parts of this codebase were AI-assisted, with tests and structure maintained by the author. Finding rough edges is the point of this stage — a clear report of where you got stuck is a real contribution, not a complaint.

## The deal

- **Small cohorts.** Early testing runs in capped groups (about 10 at a time) so feedback gets read carefully. If a cohort is full, you're on the next one.
- **Async only.** No Discord, Slack, or calls. Feedback lives in GitHub Issues and Discussions.
- **Batch replies.** The maintainer reads and replies in batches, roughly twice a week. There is no support SLA.
- **No judgment, either way.** Assume good faith about code that looks strange; there was usually a reason. Reports that stick to "I did X, expected Y, got Z" are gold.

## What helps most right now

1. **First Run Reports.** Pick a door in the README (explorer, protocol, binary, or full stack), note where it broke or confused you, file the report. Include OS, Docker version if relevant, `./g8e version`, and the exact step.
2. **Second-machine reports.** Run an Operator on a different OS/arch than the maintainer's and report what enrollment actually felt like.
3. **Docs vs reality.** If a guide says one thing and the binary does another, that's a bug. File it.

## License in one sentence

g8e is Business Source License 1.1 (so it can't be repackaged as a competing commercial service today) and converts to Apache 2.0 on 2030-08-18. By contributing, you grant Lateralus Labs a license to use your work under those terms — see CONTRIBUTING.

Thank you for trying rough software carefully. That's the whole ask.

# g8e CLI Help

## Usage
```bash
./g8e <command> <subcommand> [options]
```

## Commands

### g8e
**Usage:** `./g8e`

**Description:** Rebuild images and restart services (alias: 'platform build')

---

### platform
**Usage:** `./g8e platform <subcommand> [options]`

**Description:** Manage Docker platform services (runs on host)

**Subcommands:**
- `setup` - Full first-time setup: no-cache build of all images, start platform (recommended)
- `settings` - Show effective platform settings (requires platform running)
- `update` - Pull latest changes (with confirmation) and rebuild
- `start` - Start all platform services
- `stop` - Stop all platform services
- `restart` - Restart all platform services (no rebuild)
- `status` - Show service status
- `rebuild|build` - Rebuild images and restart services (data volumes preserved)
- `reset` - Wipe ALL data volumes and rebuild from scratch (destructive)
- `wipe` - Clear app data from the database (preserves platform settings, SSL, LLM)
- `clean` - Remove all managed Docker resources (containers, images, volumes, cache)
- `logs [service]` - Tail service logs (optional: specify service name)

---

### operator
**Usage:** `./g8e operator <subcommand> [options]`

**Description:** Build and deploy the operator binary

**Subcommands:**

#### init
Build the operator binary inside g8ep (first time)

#### build
Rebuild the operator binary inside g8ep

#### build-all
Build all operator architectures with compression (for distribution)

#### deploy `<user@host>`
Copy the operator binary to a remote host via scp

**Options:**
- `--arch amd64|arm64|386` - Architecture (default: amd64)
- `--dest /path` - Remote destination path (default: ./g8e.operator)
- `--endpoint <host>` - Platform endpoint — if set, starts operator on remote host
- `--device-token <tok>` - Device link token
- `--key <apikey>` - API key auth (fallback)
- `--wss-port <port>` - WSS port for pub/sub (default: 443)
- `--http-port <port>` - HTTPS port for auth (default: 443)
- `--no-git` - Disable ledger

#### stream `<host...>`
Stream-inject operator to one or more remote hosts concurrently

**Description:** Uses Go crypto/ssh — no system ssh binary, 1,000+ node capable. Binary piped directly from g8ep — never written to local disk. Remote binary is volatile: deleted automatically on session close.

**Options:**
- `--arch amd64|arm64|386` - Architecture (default: amd64)
- `--hosts <file|->` - File of hosts (one per line), or - for stdin
- `--concurrency <N>` - Max parallel SSH sessions (default: 50)
- `--timeout <secs>` - Per-host dial+inject timeout in seconds (default: 60)
- `--endpoint <host>` - Platform endpoint — if set, starts operator on each remote host
- `--device-token <tok>` - Device link token
- `--key <apikey>` - API key auth (fallback)
- `--no-git` - Disable ledger
- `--ssh-config <path>` - Custom SSH config path (default: ~/.ssh/config)

#### ssh-config
Configure ~/.ssh/config for high-concurrency operator streaming

**Description:** Creates ~/.ssh/sockets/ and appends g8e multiplexing stanza

**Options:**
- `--print` - Print the stanza without writing anything
- `--force` - Replace stanza even if g8e block already present

#### reauth
Kill and relaunch the g8ep operator for a user

**Options:**
- `--user-id <id>` - User ID (required unless --email provided)
- `--email <email>` - Resolve user by email instead of ID

**Examples:**
```bash
./g8e operator deploy user@host.example.com --arch arm64 --endpoint g8e.example.com
./g8e operator stream host1 host2 host3 --concurrency 100
./g8e operator reauth --email user@example.com
```

---

### test
**Usage:** `./g8e test <suite> [options] [-- EXTRA_ARGS]`

**Description:** Run component tests in dedicated test-runner containers

**Suites:**
- `g8ee` - Python/pytest tests (g8ee-test-runner)
- `g8ed` - TypeScript/vitest tests (g8ed-test-runner)
- `g8eo` - Go tests (g8eo-test-runner)

**Options:**
- `-j, --parallel [N|auto]` - Run pytest in parallel via pytest-xdist (g8ee only)

**LLM flags (enables ai_integration tests):**
- `-p, --llm-provider` - gemini | openai | anthropic | ollama
- `-m, --primary-model` - Primary model name
- `-a, --assistant-model` - Assistant model name
- `-e, --llm-endpoint-url` - API endpoint URL
- `-k, --llm-api-key` - API key

**Examples:**
```bash
./g8e test g8ee tests/unit
./g8e test g8ed -- tests/unit/services
./g8e test g8ee -j auto -- -k "test_function"
```

---

### security
**Usage:** `./g8e security <subcommand> [options]`

**Description:** Security tools (runs inside g8ep)

**Subcommands:**
- `validate` - Validate platform security (volumes, env vars, tokens)
- `certs <action>` - Manage SSL certificates
  - `generate` - Generate new SSL certificates
  - `rotate` - Rotate existing SSL certificates
  - `status` - Show SSL certificate status
  - `trust` - Trust the CA certificate on the host system
- `mtls-test` - Test mTLS connectivity
- `scan-licenses` - Scan dependency licenses
- `passkeys` - Manage user passkey credentials
- `rotate-internal-token` - Rotate the internal auth token across all components

---

### data
**Usage:** `./g8e data <subcommand> [options]`

**Description:** Data management tools (runs inside g8ep)

**Subcommands:**
- `users` - Manage platform users (list, create, delete, get)
- `operators` - Manage operator documents (list, create, delete, get)
- `store` - Manage g8es data store (backup, restore, stats)
- `settings` - Read/write platform settings (get, set, show)
- `audit` - Manage audit log (LFAA) (query, export)
- `device-links` - Manage device link tokens (generate, list, revoke, delete)

---

### search
**Usage:** `./g8e search <subcommand> [options]`

**Description:** Manage web search (runs on host)

**Subcommands:**
- `setup` - Configure Vertex AI Search for the search_web tool
- `disable` - Remove web search configuration

---

### mcp
**Usage:** `./g8e mcp <subcommand> [options]`

**Description:** MCP client integration (Claude Code, Windsurf, Cursor, etc.)

**Subcommands:**

#### config
Generate MCP config for a specific AI tool

**Options:**
- `--client <name>` - claude-code | windsurf | cursor | generic
- `--email <email>` - User email (to resolve G8eKey)

#### test
Test MCP endpoint connectivity

**Options:**
- `--email <email>` - User email (to resolve G8eKey)

#### status
Show MCP endpoint info and supported clients

**Examples:**
```bash
./g8e mcp config --client claude-code --email user@example.com
./g8e mcp test --email user@example.com
./g8e mcp status
```

---

### llm
**Usage:** `./g8e llm <subcommand> [options]`

**Description:** Manage LLM tooling (runs on host)

**Subcommands:**
- `setup` - Install and configure LLM provider (interactive or --flags)
- `restart` - Restart the LLM container
- `show` - Show current LLM settings
- `get <key>` - Read a single LLM setting (e.g. llm_model)
- `set <key=value> [...]` - Write one or more LLM settings (e.g. llm_model=gpt-4o)

**Examples:**
```bash
./g8e llm setup --provider gemini --model gemini-2.0-flash
./g8e llm get llm_model
./g8e llm set llm_model=gpt-4o llm_api_key=sk-...
```

---

### ssh
**Usage:** `./g8e ssh <subcommand> [options]`

**Description:** Configure SSH credentials for operator streaming (runs on host)

**Subcommands:**
- `setup` - Mount an SSH directory into g8ep

---

### aws
**Usage:** `./g8e aws <subcommand> [options]`

**Description:** Configure AWS credentials for AI tools (runs on host)

**Subcommands:**
- `setup` - Mount an AWS credentials directory into g8ep

---

### demo
**Usage:** `./g8e demo <subcommand> [options]`

**Description:** Manage the broken-fleet demo (runs on host)

**Subcommands:**
- `up` - Build and start all 10 web nodes
- `down` - Stop all nodes
- `status` - Show container status
- `clean` - Remove everything (containers, images, volumes)
- `health` - Check Flask backend health on all nodes
- `nginx-check` - Check nginx status on all nodes
- `operators` - Show operator status on all nodes
- `logs` - Follow all container logs
- `shell N=<nn>` - Shell into a specific node (e.g. N=01)
- `deploy DEVICE_TOKEN=<tok>` - Deploy operators via API download
- `stream DEVICE_TOKEN=<tok>` - Deploy operators via SSH streaming (auto-configures SSH)
- `discover-hosts` - List discovered demo fleet hosts
- `vanish` - Remove all operators (zero trace)
- `dashboard` - Print dashboard URL

# Quickstart

**Prerequisites:** Go 1.26+ (required) · Python 3.14+ (optional, only for the reference g8ee Ensemble)

```bash
git clone https://github.com/g8e-ai/g8e.git && cd g8e

# Start the mandatory Operator gateway
./g8e platform start

# (Optional) Start the reference g8e-Compliant Agentic Ensemble
./g8e apps start g8ee
```

1. **Bootstrap** — follow the CLI to initialize the Operator and generate a device-link token.
2. **Login** — `./g8e login` authenticates the CLI over mTLS.
3. **Audit** — watch live transaction logs in `.g8e/logs/operator-listen.log`.

<!-- ============================================================= -->
<!-- INSERT: SCREENSHOT — `./g8e platform start` running, with the -->
<!-- live audit log streaming a couple of transactions. Proves     -->
<!-- it's real and self-hosted. -->
<!-- ============================================================= -->

> *Insert screenshot of the running Operator + live audit log here.*

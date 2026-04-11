#!/bin/bash
# Lazy-installs security scanning tools on first use.
# Called by individual scan scripts before invoking their tool.
# Safe to call multiple times — skips tools already present.

set -e

NUCLEI_VERSION="3.7.0"
TRIVY_VERSION="0.58.2"

_install_nuclei() {
    command -v nuclei >/dev/null 2>&1 && return
    echo "[scan-tools] Installing nuclei ${NUCLEI_VERSION}..."
    wget -q "https://github.com/projectdiscovery/nuclei/releases/download/v${NUCLEI_VERSION}/nuclei_${NUCLEI_VERSION}_linux_amd64.zip" \
        -O /tmp/nuclei.zip
    unzip -o -q /tmp/nuclei.zip -d /usr/local/bin
    chmod +x /usr/local/bin/nuclei
    rm /tmp/nuclei.zip
    nuclei -update-templates -silent || true
}

_install_testssl() {
    command -v testssl.sh >/dev/null 2>&1 && return
    echo "[scan-tools] Installing testssl.sh..."
    git clone --depth 1 https://github.com/drwetter/testssl.sh.git /opt/testssl
    ln -sf /opt/testssl/testssl.sh /usr/local/bin/testssl.sh
    chmod +x /opt/testssl/testssl.sh
}

_install_trivy() {
    command -v trivy >/dev/null 2>&1 && return
    echo "[scan-tools] Installing trivy ${TRIVY_VERSION}..."
    wget -q "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz" \
        -O /tmp/trivy.tar.gz
    tar -xzf /tmp/trivy.tar.gz -C /usr/local/bin trivy
    chmod +x /usr/local/bin/trivy
    rm /tmp/trivy.tar.gz
}

_install_grype() {
    command -v grype >/dev/null 2>&1 && return
    echo "[scan-tools] Installing grype..."
    curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin
}

TOOL="${1:-all}"

case "$TOOL" in
    nuclei)   _install_nuclei ;;
    testssl)  _install_testssl ;;
    trivy)    _install_trivy ;;
    grype)    _install_grype ;;
    all)
        _install_nuclei
        _install_testssl
        _install_trivy
        _install_grype
        ;;
    *)
        echo "Usage: $(basename "$0") [nuclei|testssl|trivy|grype|all]" >&2
        exit 1
        ;;
esac

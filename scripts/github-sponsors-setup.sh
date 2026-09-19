# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

#!/usr/bin/env bash
# Enable the GitHub Sponsors button for g8e-ai/g8e after your sponsor profile is live.
#
# Prerequisites (GitHub web UI only — no gh/API equivalent):
#   1. Join GitHub Sponsors: https://github.com/sponsors
#   2. Complete profile (short bio + introduction), tiers, payout/tax info, 2FA
#   3. Submit for approval and wait for github.com/sponsors/<account> to go live
#
# Repo-side setup (this script):
#   - .github/FUNDING.yml must exist on the default branch (already in repo)
#   - hasSponsorshipsEnabled must be true on the repository
#
# Usage:
#   ./scripts/github-sponsors-setup.sh status
#   ./scripts/github-sponsors-setup.sh enable
#   ./scripts/github-sponsors-setup.sh disable

set -euo pipefail

REPO="${GITHUB_REPO:-g8e-ai/g8e}"
REPO_ID="${GITHUB_REPO_ID:-R_kgDOR-ehng}"

query_repo() {
  gh api graphql -f query='
    query($owner: String!, $name: String!) {
      repository(owner: $owner, name: $name) {
        id
        hasSponsorshipsEnabled
        fundingLinks { platform url }
      }
    }
  ' -f owner="${REPO%%/*}" -f name="${REPO##*/}"
}

set_sponsorships_enabled() {
  local enabled="$1"
  gh api graphql -f query='
    mutation($input: UpdateRepositoryInput!) {
      updateRepository(input: $input) {
        repository {
          name
          hasSponsorshipsEnabled
          fundingLinks { platform url }
        }
      }
    }
  ' -F "input[repositoryId]=${REPO_ID}" -F "input[hasSponsorshipsEnabled]=${enabled}"
}

check_sponsor_profile() {
  local account="$1"
  gh api "/users/${account}" --jq '.sponsors_listing_url // empty'
}

cmd="${1:-status}"

case "${cmd}" in
  status)
    echo "Repository: ${REPO}"
    query_repo
    echo
    echo "Sponsor profiles:"
    for account in Badoot opendevops g8e-ai; do
      url="$(check_sponsor_profile "${account}" || true)"
      if [[ -n "${url}" ]]; then
        echo "  ${account}: ${url}"
      else
        echo "  ${account}: not live yet (sponsors_listing_url is null)"
      fi
    done
    ;;
  enable)
    set_sponsorships_enabled true
    ;;
  disable)
    set_sponsorships_enabled false
    ;;
  *)
    echo "Usage: $0 {status|enable|disable}" >&2
    exit 1
    ;;
esac

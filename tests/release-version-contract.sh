#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VALIDATOR="${ROOT_DIR}/.github/scripts/validate-agent-version.sh"

expect_valid() {
  local version="$1"
  local expected_prerelease="$2"
  local actual
  actual="$(bash "${VALIDATOR}" "${version}")"
  [[ "${actual}" == "${expected_prerelease}" ]]
}

expect_invalid() {
  local version="$1"
  if bash "${VALIDATOR}" "${version}" >/dev/null 2>&1; then
    echo "expected invalid Agent version: ${version}" >&2
    exit 1
  fi
}

expect_valid "1.4.0" "false"
expect_valid "1.5.0-rc.2" "true"
expect_valid "1.6.0-nightly.20260710" "true"

expect_invalid "v1.4.0"
expect_invalid "1.4.0-01"
expect_invalid "1.4.0-nightly"
expect_invalid "1.4.0-nightly.01"
expect_invalid "1.4"

echo "Release workflow version contract passed"

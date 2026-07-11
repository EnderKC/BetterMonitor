#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

grep -q '^DASHBOARD_VERSION=' versions.env
grep -q '^AGENT_VERSION=' versions.env
grep -q 'paths:' .github/workflows/release.yml
grep -q -- '- "versions.env"' .github/workflows/release.yml
grep -q 'Read versions from versions.env' .github/workflows/release.yml
grep -q 'concurrency:' .github/workflows/release.yml
grep -q 'node-version: "20"' .github/workflows/release.yml
grep -q 'name: Release Contract Gate' .github/workflows/release.yml
grep -q 'prepare-all-in-one-build.sh' start-all-in-one.sh
grep -q 'BUILD_DATE: ${BUILD_DATE}' docker-compose.all-in-one.yml
if grep -q '\$(date ' docker-compose.all-in-one.yml; then
  echo 'Compose must not contain shell command substitution' >&2
  exit 1
fi

echo 'CI/CD contract passed'

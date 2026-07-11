#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

dashboard_version="$(awk -F= '$1 == "DASHBOARD_VERSION" { print $2 }' versions.env)"
agent_version="$(awk -F= '$1 == "AGENT_VERSION" { print $2 }' versions.env)"

grep -q "Dashboard \*\*${dashboard_version}\*\*" README.md
grep -q "Agent \*\*${agent_version}\*\*" README.md
grep -q -- '--agent-type full' README.md
if grep -q -- '--type full\|admin123' README.md; then
  echo 'README contains a removed installer flag or obsolete default password' >&2
  exit 1
fi

for path in .env.example backend/.env.example versions.env doc/life-probe-technical-guide.md reports/BetterMonitor-2026-07-11.md; do
  test -e "$path"
done

echo 'README contract passed'

#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
bash "${ROOT_DIR}/install-agent.sh" --contract-test "${ROOT_DIR}/testdata/agent-release-contract.json"

grep -q "Get-ScheduledTaskInfo" "${ROOT_DIR}/install-agent.ps1"
grep -q "State -eq 'Running'" "${ROOT_DIR}/install-agent.ps1"

#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT_DIR"

case "$(uname -m)" in
  x86_64|amd64) target_arch=amd64 ;;
  aarch64|arm64) target_arch=arm64 ;;
  *) echo "Unsupported local architecture: $(uname -m)" >&2; exit 1 ;;
esac

dashboard_version="$(awk -F= '$1 == "DASHBOARD_VERSION" { print $2 }' versions.env)"
if [[ -z "$dashboard_version" ]]; then
  echo "DASHBOARD_VERSION is missing from versions.env" >&2
  exit 1
fi

(cd frontend && npm ci && npm run build)

backend_binary="backend/better-monitor-backend-${target_arch}"
build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
commit="$(git rev-parse --short HEAD 2>/dev/null || printf unknown)"
(
  cd backend
  go build -ldflags="-w -s \
    -X github.com/user/server-ops-backend/pkg/version.Version=${dashboard_version} \
    -X github.com/user/server-ops-backend/pkg/version.Commit=${commit} \
    -X github.com/user/server-ops-backend/pkg/version.BuildDate=${build_date}" \
    -o "better-monitor-backend-${target_arch}" main.go
)

echo "all-in-one build inputs ready for ${target_arch}"

#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "::error::$1" >&2
  exit 1
}

[[ $# -eq 1 ]] || fail "usage: validate-agent-version.sh <version>"
version="$1"

semver_re='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
[[ "${version}" =~ ${semver_re} ]] || fail "AGENT_VERSION must be strict SemVer without a v prefix"

without_build="${version%%+*}"
prerelease=""
if [[ "${without_build}" == *-* ]]; then
  prerelease="${without_build#*-}"
  IFS='.' read -r -a identifiers <<< "${prerelease}"
  for identifier in "${identifiers[@]}"; do
    if [[ "${identifier}" =~ ^[0-9]+$ && ${#identifier} -gt 1 && "${identifier}" == 0* ]]; then
      fail "AGENT_VERSION prerelease numeric identifiers must not have leading zeroes"
    fi
  done
fi

lower_prerelease="$(printf '%s' "${prerelease}" | tr '[:upper:]' '[:lower:]')"
if [[ "${lower_prerelease}" == *nightly* ]]; then
  nightly_re='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-nightly\.[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*$'
  [[ "${version}" =~ ${nightly_re} ]] || fail "Nightly AGENT_VERSION must match x.y.z-nightly.<build>"
fi

if [[ -n "${prerelease}" ]]; then
  echo "true"
else
  echo "false"
fi

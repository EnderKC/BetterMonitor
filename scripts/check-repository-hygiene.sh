#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root_dir"

for path in .env backend/.env; do
  test ! -e "$path"
  git check-ignore -q "$path"
done

test -f .env.example
test -f backend/.env.example
test ! -f transform.py

echo "repository hygiene contract passed"

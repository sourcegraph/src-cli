#!/usr/bin/env bash

set -euo pipefail

unset CDPATH
cd "$(dirname "${BASH_SOURCE[0]}")/.."

SRC_CMD=(go run ./cmd/src)

run() {
  echo
  echo "+ $*"
  "$@"
}

# This script intentionally relies on the currently exported SRC_* envvars.

echo "Running src-cli smoke checks with current SRC_* environment"

run "${SRC_CMD[@]}" version
run "${SRC_CMD[@]}" api -query 'query { site { buildVersion } }'
run "${SRC_CMD[@]}" api -query 'query { currentUser { username } }'
run "${SRC_CMD[@]}" users list -first=1 -f '{{.Username}}'
run "${SRC_CMD[@]}" orgs list -first=1 -f '{{.Name}}'
run "${SRC_CMD[@]}" repos list -first=1 -f '{{.Name}}'
run "${SRC_CMD[@]}" search -less=false -json 'type:repo count:1'

echo
echo "Smoke checks completed successfully."

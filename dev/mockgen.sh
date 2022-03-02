#!/usr/bin/env bash

set -euf -o pipefail

BASE="$(realpath "$(dirname "${BASH_SOURCE[0]}")/..")"

export GOBIN
GOBIN="$BASE/.bin"
export PATH="$GOBIN:$PATH"
export GO111MODULE=on

# Get the required version from go.mod.
REQUIRED_VERSION=$(grep go-mockgen "$BASE/go.mod" | awk '{ print $2 }')

set +o pipefail
INSTALLED_VERSION="$(go-mockgen --version || :)"
set -o pipefail

if [[ "${INSTALLED_VERSION}" != "${REQUIRED_VERSION}" ]]; then
  echo "Updating local installation of go-mockgen"

  go install "github.com/derision-test/go-mockgen/cmd/go-mockgen@${REQUIRED_VERSION}"
  go install "golang.org/x/tools/cmd/goimports"
fi

go-mockgen -f "$@"

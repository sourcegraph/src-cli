#!/bin/sh

set -e
set -x

exec docker build -t sourcegraph/src-campaign-workspace - <"$(dirname "$0")/Dockerfile"

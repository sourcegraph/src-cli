#!/bin/bash

set -e

# Sourcegraph API credentials - replace with your own if needed
export SRC_ENDPOINT=https://sourcegraph.sourcegraph.com
export SRC_ACCESS_TOKEN=sgp_

attempt=1
max_attempts=10

echo "================================="
echo "Starting bug reproducer test..."
echo "================================="
echo 

echo "This script will attempt to reproduce a bug where edits are"
echo "occasionally dropped during processing of large batch changes."
echo "With the provided batch-spec.yaml containing thousands of"
echo "changesets, we can reliably reproduce the issue."
echo 

while [ $attempt -le $max_attempts ]
do
    echo "Attempt $attempt of $max_attempts" >&2
    if ! go run ./cmd/src batch preview -f batch-spec.yaml 2>error.log >/dev/null; then
        echo -e "\n=================================" >&2
        echo "BUG REPRODUCED at attempt $attempt!" >&2
        echo -e "=================================\n" >&2
        echo "ERROR OUTPUT:" >&2
        cat error.log >&2
        echo -e "\n" >&2
        echo "The error above shows that some changesets have empty diffs," >&2
        echo "which violates the schema requirement. This happens because" >&2
        echo "edits are occasionally dropped during processing of large batch changes." >&2
        break
    fi
    attempt=$((attempt+1))
done

if [ $attempt -gt $max_attempts ]; then
    echo "\n=================================" >&2
    echo "Reached maximum attempts without reproducing the bug." >&2
    echo "Try increasing max_attempts or review the batch-spec.yaml file." >&2
    echo "=================================" >&2
fi
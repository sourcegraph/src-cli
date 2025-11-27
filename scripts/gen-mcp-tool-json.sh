#!/usr/bin/env bash

DST=$1

if [[ -z "${SRC_ACCESS_TOKEN}" ]]; then
  echo "SRC_ACCESS_TOKEN is not set. Please set a access token for S2 (sourcegraph.sourcegraph.com)"
  exit 1
fi

if [[ -z "$DST" ]]; then
  echo "Usage: $0 <filename.json>"
  exit 1
fi

curl \
  -H "Content-Type: application/json" \
  -H "Authorization: token ${SRC_ACCESS_TOKEN}" \
  -X POST \
  -d '{ "jsonrpc": "2.0", "id": 1, "method": "tools/list", "params": {}}' \
   https://sourcegraph.sourcegraph.com/.api/mcp/v1 | grep 'data:' | cut -b 6- | jq '.result' > ${DST}


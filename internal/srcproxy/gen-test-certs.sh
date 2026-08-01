#!/usr/bin/env bash
# gen-test-certs.sh — generate certs for testing `src proxy` mTLS mode
#
# Usage: ./gen-test-certs.sh [email] [output-dir]
#   email:      email SAN to embed in client cert (default: alice@example.com)
#   output-dir: where to write files (default: ./test-certs)
set -euo pipefail

EMAIL="${1:-alice@example.com}"
DIR="${2:-./test-certs}"
mkdir -p "$DIR"

echo "==> Generating certs in $DIR (email: $EMAIL)"

# ── 1. CA ────────────────────────────────────────────────────────────────────
openssl genrsa -out "$DIR/ca.key" 2048 2>/dev/null

openssl req -new -x509 -days 1 \
  -key "$DIR/ca.key" \
  -out "$DIR/ca.pem" \
  -subj "/CN=Test Client CA" 2>/dev/null

echo "    ca.pem / ca.key"

# ── 2. Server cert (so you can pass it to the proxy and trust it in curl) ────
openssl genrsa -out "$DIR/server.key" 2048 2>/dev/null

openssl req -new \
  -key "$DIR/server.key" \
  -out "$DIR/server.csr" \
  -subj "/CN=localhost" 2>/dev/null

openssl x509 -req -days 1 \
  -in "$DIR/server.csr" \
  -signkey "$DIR/server.key" \
  -out "$DIR/server.pem" \
  -extfile <(printf 'subjectAltName=DNS:localhost,IP:127.0.0.1') 2>/dev/null

echo "    server.pem / server.key"

# ── 3. Client cert with email SAN signed by the CA ───────────────────────────
openssl genrsa -out "$DIR/client.key" 2048 2>/dev/null

openssl req -new \
  -key "$DIR/client.key" \
  -out "$DIR/client.csr" \
  -subj "/CN=test-client" 2>/dev/null

openssl x509 -req -days 1 \
  -in "$DIR/client.csr" \
  -CA "$DIR/ca.pem" \
  -CAkey "$DIR/ca.key" \
  -CAcreateserial \
  -out "$DIR/client.pem" \
  -extfile <(printf "subjectAltName=email:%s" "$EMAIL") 2>/dev/null

echo "    client.pem / client.key  (email SAN: $EMAIL)"

# Confirm the SAN is present
echo ""
echo "==> Verifying email SAN in client cert:"
openssl x509 -in "$DIR/client.pem" -noout -text \
  | grep -A1 "Subject Alternative Name"

echo ""
echo "==> Done. Next steps:"
echo ""
echo "  # 1. Start the proxy (in another terminal):"
echo "  export SRC_ENDPOINT=https://sourcegraph.example.com"
echo "  export SRC_ACCESS_TOKEN=<site-admin-sudo-token>"
echo "  go run ./cmd/src proxy \\"
echo "    -server-cert $DIR/server.pem \\"
echo "    -server-key  $DIR/server.key \\"
echo "    $DIR/ca.pem"
echo ""
echo "  # 2. Send a request via curl using the client cert:"
echo "  curl --cacert $DIR/server.pem \\"
echo "       --cert   $DIR/client.pem \\"
echo "       --key    $DIR/client.key \\"
echo "       https://localhost:7777/.api/graphql \\"
echo "       -d '{\"query\":\"{ currentUser { username } }\"}'"
echo ""
echo "  # Or skip server cert verification with -k:"
echo "  curl -k --cert $DIR/client.pem --key $DIR/client.key \\"
echo "       https://localhost:7777/.api/graphql \\"
echo "       -d '{\"query\":\"{ currentUser { username } }\"}'"

# srcproxy

`src proxy` is a local reverse proxy for Sourcegraph with two auth modes.

## Auth Modes

- Default mode (no CA arg): forwards requests with `Authorization: token <SRC_ACCESS_TOKEN>`.
- mTLS sudo mode (with CA arg): requires client certificate, extracts first email SAN, resolves Sourcegraph user by email, then forwards with `token-sudo`.

## Run

```bash
# default mode
src proxy

# mTLS sudo mode
src -v proxy \
  -server-cert ./internal/srcproxy/test-certs/server.pem \
  -server-key  ./internal/srcproxy/test-certs/server.key \
  ./internal/srcproxy/test-certs/ca.pem
```

## Logging

- `-v` enables request-level debug logging.
- `-log-file <path>` writes logs to a file.
- Without `-log-file`, logs go to stderr.

Example:

```bash
src -v proxy -log-file ./proxy.log ./internal/srcproxy/test-certs/ca.pem
```

## Request Format

GraphQL requests should use JSON:

```bash
curl -k \
  -H 'Content-Type: application/json' \
  --cert ./internal/srcproxy/test-certs/client.pem \
  --key  ./internal/srcproxy/test-certs/client.key \
  https://localhost:7777/.api/graphql \
  -d '{"query":"{ currentUser { username } }"}'
```

If `Content-Type` is omitted with `curl -d`, curl sends `application/x-www-form-urlencoded`, which Sourcegraph GraphQL rejects.

## Important Routing Behavior

The proxy rewrites upstream `Host` to `SRC_ENDPOINT` host.

This is required for name-based routing (for example Caddy virtual hosts). If `Host` is forwarded as `localhost:<proxy-port>`, some upstream setups return `200` with empty body from a default vhost instead of Sourcegraph GraphQL.

## mTLS Certificate Requirements

- Client cert must chain to the CA file passed as positional arg.
- Client cert must include an email SAN.
- The email SAN must map to an existing Sourcegraph user.

## Troubleshooting

- `HTTP 200` with empty body:
  upstream host routing mismatch. Confirm proxy is current and `Host` rewrite is in place.
- `no client certificate presented`:
  client did not send cert/key or CA trust does not match.
- `client certificate does not contain an email SAN`:
  regenerate client cert with email SAN.
- `no Sourcegraph user found for certificate email`:
  cert email is not a Sourcegraph user email.

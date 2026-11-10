# Since we're going to provide images based on Alpine, we also want to build on
# Alpine, rather than relying on the ./src in the surrounding environment to be
# sane.
#
# Nothing fancy here: we copy in the source code and build on the Alpine Go
# image. Refer to .dockerignore to get a sense of what we're not going to copy.
FROM golang:1.27.4-alpine3.24@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS builder

COPY . /src
WORKDIR /src
RUN go build ./cmd/src

# This stage should be kept in sync with Dockerfile.release.
FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# needed for `src code-intel upload` and `src actions exec`
# ca-certificates-bundle >= 20261020-r0 drops the eMudhra roots (CVE-2026-111943).
RUN apk add --no-cache git && apk upgrade --no-cache ca-certificates-bundle

COPY --from=builder /src/src /usr/bin/
ENTRYPOINT ["/usr/bin/src"]

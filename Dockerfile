# Since we're going to provide images based on Alpine, we also want to build on
# Alpine, rather than relying on the ./src in the surrounding environment to be
# sane.
#
# Nothing fancy here: we copy in the source code and build on the Alpine Go
# image. Refer to .dockerignore to get a sense of what we're not going to copy.
FROM golang:1.27.3-alpine3.24@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS builder

COPY . /src
WORKDIR /src
RUN go build ./cmd/src

# This stage should be kept in sync with Dockerfile.release.
FROM alpine:3.24@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63

# needed for `src code-intel upload` and `src actions exec`
# ca-certificates-bundle >= 20261020-r0 drops the eMudhra roots (CVE-2026-111943).
RUN apk add --no-cache git && apk upgrade --no-cache ca-certificates-bundle

COPY --from=builder /src/src /usr/bin/
ENTRYPOINT ["/usr/bin/src"]

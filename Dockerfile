# Since we're going to provide images based on Alpine, we also want to build on
# Alpine, rather than relying on the ./src in the surrounding environment to be
# sane.
#
# Nothing fancy here: we copy in the source code and build on the Alpine Go
# image. Refer to .dockerignore to get a sense of what we're not going to copy.
FROM golang:1.15-alpine@sha256:86a9ac87a3317d2635b817493a9df24ad6fff38b8bcee4a72ce511305b897646 as builder

COPY . /src
WORKDIR /src
RUN go build ./cmd/src

# This stage should be kept in sync with Dockerfile.release.
FROM sourcegraph/alpine:3.12@sha256:133a0a767b836cf86a011101995641cf1b5cbefb3dd212d78d7be145adde636d

# needed for `src lsif upload` and `src actions exec`
RUN apk add --no-cache git

COPY --from=builder /src/src /usr/bin/
ENTRYPOINT ["/usr/bin/src"]

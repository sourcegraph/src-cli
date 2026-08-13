# Since we're going to provide images based on Alpine, we also want to build on
# Alpine, rather than relying on the ./src in the surrounding environment to be
# sane.
#
# Nothing fancy here: we copy in the source code and build on the Alpine Go
# image. Refer to .dockerignore to get a sense of what we're not going to copy.
FROM golang:1.26.5-alpine3.23@sha256:622e56dbc11a8cfe87cafa2331e9a201877271cbff918af53d3be315f3da88cc AS builder

COPY . /src
WORKDIR /src
RUN go build ./cmd/src

# This stage should be kept in sync with Dockerfile.release.
FROM alpine:3.22@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412

# needed for `src code-intel upload` and `src actions exec`
RUN apk add --no-cache git

COPY --from=builder /src/src /usr/bin/
ENTRYPOINT ["/usr/bin/src"]

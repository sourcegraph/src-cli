# This Dockerfile builds the sourcegraph/src-batch-change-volume-workspace
# image that we use to run curl, git, and unzip against a Docker volume when
# using the volume workspace.

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

# ca-certificates-bundle >= 20261020-r0 drops the eMudhra roots (CVE-2026-111943).
RUN apk add --update git unzip && apk upgrade --no-cache ca-certificates-bundle

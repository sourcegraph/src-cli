# This Dockerfile builds the sourcegraph/src-batch-change-volume-workspace
# image that we use to run curl, git, and unzip against a Docker volume when
# using the volume workspace.

FROM alpine:3.24@sha256:ff7f0c727dd796aa36787bfcd9a64bf35a6ce72a73451b3359fde849b6703d63

# ca-certificates-bundle >= 20261020-r0 drops the eMudhra roots (CVE-2026-111943).
RUN apk add --update git unzip && apk upgrade --no-cache ca-certificates-bundle

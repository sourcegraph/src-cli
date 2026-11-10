# This Dockerfile builds the sourcegraph/src-batch-change-volume-workspace
# image that we use to run curl, git, and unzip against a Docker volume when
# using the volume workspace.

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# ca-certificates-bundle >= 20261020-r0 drops the eMudhra roots (CVE-2026-111943).
RUN apk add --update git unzip && apk upgrade --no-cache ca-certificates-bundle

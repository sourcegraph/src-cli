# This Dockerfile builds the sourcegraph/src-batch-change-volume-workspace
# image that we use to run curl, git, and unzip against a Docker volume when
# using the volume workspace.

FROM alpine:3.21.2@sha256:56fa17d2a7e7f168a043a2712e63aed1f8543aeafdcee47c58dcffe38ed51099

RUN apk add --update git unzip

#!/bin/sh

set -eux

dir="$(mktemp -d)"
trap 'rm -rf "$dir"' EXIT

go build -o "$dir/protoc-gen-go" google.golang.org/protobuf/cmd/protoc-gen-go
go build -o "$dir/protoc-gen-go-grpc" google.golang.org/grpc/cmd/protoc-gen-go-grpc

PATH="$dir:$PATH"
export PATH
protoc \
    --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    marionette.proto

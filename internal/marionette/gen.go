package marionette

//go:generate ./gen.sh

// This keeps protoc-gen-go-grpc in the go.mod in spite of a go mod tidy.
import _ "google.golang.org/grpc"

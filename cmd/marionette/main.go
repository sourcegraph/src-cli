package main

import (
	"flag"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/sourcegraph/src-cli/internal/marionette"
)

var (
	address   string
	network   string
	workspace string
)

func init() {
	flag.StringVar(&address, "address", ":50051", "address to bind to")
	flag.StringVar(&network, "network", "tcp", "network to listen on")
	flag.StringVar(&workspace, "workspace", "", "base path of the workspace")
}

func main() {
	flag.Parse()

	l, err := net.Listen(network, address)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer(logInterceptor())
	marionette.RegisterMarionetteServer(s, &server{workspace: workspace})
	reflection.Register(s)

	log.Printf("listening on %s %s", network, address)
	if err := s.Serve(l); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

package main

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
)

func logInterceptor() grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		before := time.Now()
		resp, err := handler(ctx, req)
		after := time.Now()

		status := "OK"
		if err != nil {
			status = "ERROR"
		}
		log.Printf("%s (%v) %s", info.FullMethod, after.Sub(before), status)

		return resp, err
	})
}

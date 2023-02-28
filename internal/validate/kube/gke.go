package kube

import (
	"context"
	"log"

	"google.golang.org/api/gkehub/v1"
)

func Gke(ctx context.Context) Option {
    gkeClient, err := gkehub.NewService(ctx)
    if err != nil {
        log.Println("error while loading config: ", err)
    }

    return func(config *Config) {
        config.gke = true
        config.gkeClient = gkeClient
    }
}

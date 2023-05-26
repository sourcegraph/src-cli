package advise

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
)

func Docker(ctx context.Context, client client.Client, opts ...Option) error {
	cfg := &Config{
		docker:       true,
		pod:          "",
		container:    "",
		dockerClient: &client,
	}
    
    for _, opt := range opts {
        opt(cfg)
    }

    fmt.Println("Docker function needs code")
    return nil
}

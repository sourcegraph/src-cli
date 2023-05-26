package advise

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
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

    containers, err := client.ContainerList(ctx, types.ContainerListOptions{})
    if err != nil {
        return errors.Wrap(err, "could not get list of containers")
    }

    PrintContainers(containers)
    return nil
}

func PrintContainers(containers []types.Container) {
    for _, c := range containers {
        fmt.Println(c.Image)
    }
}

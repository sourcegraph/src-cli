package usage

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func Docker(ctx context.Context, client client.Client, opts ...Option) error {
	cfg := &Config{
		namespace:     "default",
		docker:        true,
		pod:           "",
		container:     "",
		spy:           false,
		restConfig:    nil,
		k8sClient:     nil,
		dockerClient:  &client,
		metricsClient: nil,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	containers, err := cfg.dockerClient.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return errors.Wrap(err, "could not get list of containers")
	}

	printContainerImages(containers)
	return nil
}

func printContainerImages(containers []types.Container) {
	for _, container := range containers {
		fmt.Println(container.Image)
	}
}

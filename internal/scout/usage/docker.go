package usage

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/jasonhawkharris/dockerstats"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/scout/style"
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

	return renderDockerUsageTable(ctx, cfg, containers)
}

func renderDockerUsageTable(ctx context.Context, cfg *Config, containers []types.Container) error {
	stats, err := dockerstats.Current()
	if err != nil {
		return errors.Wrap(err, "could not get docker stats")
	}

	columns := []table.Column{
		{Title: "Container", Width: 20},
		{Title: "Cores", Width: 10},
		{Title: "Usage", Width: 10},
		{Title: "Memory", Width: 10},
		{Title: "Usage", Width: 10},
	}
	rows := []table.Row{}

	for _, container := range containers {
		containerInfo, err := cfg.dockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			return errors.Wrap(err, "could not get container info")
		}

		for _, s := range stats {
			if s.Container == container.ID[0:12] {
				row := table.Row{
					containerInfo.Name,
					fmt.Sprintf("%v", containerInfo.HostConfig.NanoCPUs/1000000000),
					fmt.Sprintf("%v", s.CPU),
					fmt.Sprintf("%vG", containerInfo.HostConfig.Memory/1000000000),
					fmt.Sprintf("%v", s.Memory.Percent), // arbitrary number
				}

				rows = append(rows, row)
				break
			}
		}
	}

	style.ResourceTable(columns, rows)
	return nil
}

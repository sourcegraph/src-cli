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
		namespace:    "default",
		docker:       true,
		pod:          "",
		container:    "",
		spy:          false,
		dockerClient: &client,
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

// renderDockerUsageTable generates a table displaying CPU and memory usage for Docker containers.
// It gets a list of all running containers from the Docker API. For each container, it finds the
// corresponding stats from the dockerstats library. It then constructs table rows displaying the
// container name, number of CPU cores, CPU usage, memory limit, and memory usage. The table is
// rendered using the charmbracelet/bubbles table library.
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
				row := makeDockerUsageRow(containerInfo, s)
				rows = append(rows, row)
				break
			}
		}
	}

	style.ResourceTable(columns, rows)
	return nil
}

// makeDockerUsageRow generates a table row displaying CPU and memory usage for a Docker container.
// It takes a ContainerJSON struct containing info about the container and a Stats struct containing usage stats.
// It calculates the number of CPU cores, CPU usage percentage, memory limit in GB, and memory usage percentage.
// It then returns a table.Row containing this info, to be displayed in the usage table.
func makeDockerUsageRow(containerInfo types.ContainerJSON, usage dockerstats.Stats) table.Row {
	cpuCores := containerInfo.HostConfig.NanoCPUs / 1_000_000_000
	memory := containerInfo.HostConfig.Memory / 1_000_000_000
	cpuUsage := usage.CPU
	memoryUsage := usage.Memory.Percent
	return table.Row{
		containerInfo.Name,
		fmt.Sprintf("%.2f", float64(cpuCores)),
		fmt.Sprintf("%v", cpuUsage),
		fmt.Sprintf("%.2fG", float64(memory)),
		fmt.Sprintf("%v", memoryUsage), // arbitrary number
	}
}

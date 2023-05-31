package advise

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/scout"
)

func Docker(ctx context.Context, client client.Client, opts ...Option) error {
	cfg := &scout.Config{
		Namespace:    "default",
		Docker:       true,
		Pod:          "",
		Container:    "",
		Output:       "",
		Spy:          false,
		DockerClient: &client,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	containers, err := client.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return errors.Wrap(err, "could not get list of containers")
	}

	AdviseDocker(ctx, cfg, containers)
	return nil
}

func AdviseDocker(ctx context.Context, cfg *scout.Config, containers []types.Container) error {
	for _, container := range containers {
		containerInfo, err := cfg.DockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			return errors.Wrap(err, "failed to get container info")
		}

		if cfg.Container != "" {
			if containerInfo.Name == cfg.Container {
				adviseContainer(ctx, cfg, containerInfo)
				break
			} else {
				continue
			}
		}
		adviseContainer(ctx, cfg, containerInfo)
	}
	return nil
}

func adviseContainer(ctx context.Context, cfg *scout.Config, container types.ContainerJSON) error {
	var advice []string
	stats, err := cfg.DockerClient.ContainerStats(ctx, container.ID, false)
	if err != nil {
		return errors.Wrap(err, "error while getting container stats")
	}
	defer func() { _ = stats.Body.Close() }()

	var usage types.StatsJSON
	if err := json.NewDecoder(stats.Body).Decode(&usage); err != nil {
		return errors.Wrap(err, "could not get container usage stats")
	}

	cpuCores := float64(container.HostConfig.NanoCPUs)
	cpuUsage := float64(usage.CPUStats.CPUUsage.TotalUsage)
	cpuPercent := scout.GetPercentage(cpuUsage, cpuCores)

	memory := float64(container.HostConfig.Memory)
	memoryUsage := float64(usage.MemoryStats.Usage)
	memPercent := scout.GetPercentage(memoryUsage, memory)

	cpuAdvice := CheckUsage(cpuPercent, "CPU", container.Name)
	advice = append(advice, cpuAdvice)

	memoryAdvice := CheckUsage(memPercent, "memory", container.Name)
	advice = append(advice, memoryAdvice)

	if cfg.Output != "" {
		OutputToFile(ctx, cfg, container.Name, advice)
	} else {
		for _, msg := range advice {
			fmt.Println(msg)
		}
	}

	return nil
}

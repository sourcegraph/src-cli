package usage

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/scout"
	"github.com/sourcegraph/src-cli/internal/scout/kube"
	"github.com/sourcegraph/src-cli/internal/scout/style"

	"gopkg.in/inf.v0"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	metav1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type ContainerMetrics struct {
	podName string
	limits  map[string]Resources
}

type Resources struct {
	cpu     *resource.Quantity
	memory  *resource.Quantity
	storage *resource.Quantity
}

type UsageStats struct {
	containerName string
	cpuCores      *resource.Quantity
	memory        *resource.Quantity
	storage       *resource.Quantity
	cpuUsage      float64
	memoryUsage   float64
	storageUsage  float64
}

const (
	ABillion float64 = 1000000000
)

func K8s(
	ctx context.Context,
	clientSet *kubernetes.Clientset,
	metricsClient *metricsv.Clientset,
	restConfig *restclient.Config,
	opts ...Option,
) error {
	cfg := &scout.Config{
		Namespace:     "default",
		Docker:        false,
		Pod:           "",
		Container:     "",
		Spy:           false,
		RestConfig:    restConfig,
		K8sClient:     clientSet,
		DockerClient:  nil,
		MetricsClient: metricsClient,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	pods, err := kube.GetPods(ctx, cfg)
	if err != nil {
		return errors.Wrap(err, "could not get list of pods")
	}

	if cfg.Pod != "" {
		return renderSinglePodUsageTable(ctx, cfg, pods)
	}

	return renderUsageTable(ctx, cfg, pods)
}

// renderSinglePodUsageStats prints resource usage statistics for a single pod.
func renderSinglePodUsageTable(ctx context.Context, cfg *scout.Config, pods []corev1.Pod) error {
	pod, err := kube.GetPod(cfg.Pod, pods)
	if err != nil {
		return errors.Wrapf(err, "could not get pod with name %s", cfg.Pod)
	}

	podMetrics, err := kube.GetPodMetrics(ctx, cfg, pod)
	if err != nil {
		return errors.Wrap(err, "while attempting to fetch pod metrics")
	}

	containerMetrics := &ContainerMetrics{cfg.Pod, map[string]Resources{}}
	if err = getLimits(ctx, cfg, &pod, containerMetrics); err != nil {
		return errors.Wrap(err, "failed to get get container metrics")
	}

	columns := []table.Column{
		{Title: "Container", Width: 20},
		{Title: "Cores", Width: 10},
		{Title: "Usage(%)", Width: 10},
		{Title: "Memory", Width: 10},
		{Title: "Usage(%)", Width: 10},
		{Title: "Storage", Width: 10},
		{Title: "Usage(%)", Width: 10},
	}
	var rows []table.Row

	for _, container := range podMetrics.Containers {
		stats, err := getContainerUsageStats(ctx, cfg, *containerMetrics, pod, container)
		if err != nil {
			return errors.Wrapf(err, "could not compile usage data for row: %s\n", container.Name)
		}

		row := makeRow(stats)
		rows = append(rows, row)
	}

	style.ResourceTable(columns, rows)
	return nil
}

// getLimits extracts resource limits for containers in a pod and stores
// them in a ContainerMetrics struct.
//
// It populates the containerMetrics struct with:
// - The name of each container
// - The CPU, memory, and storage resource limits for each container
// - A print method to print the resource limits for each container
func getLimits(ctx context.Context, cfg *scout.Config, pod *corev1.Pod, containerMetrics *ContainerMetrics) error {
	for _, container := range pod.Spec.Containers {
		containerName := container.Name
		capacity, err := kube.GetPvcCapacity(ctx, cfg, container, pod)
		if err != nil {
			return errors.Wrap(err, "while getting storage capacity of PV")
		}

		rsrcs := Resources{
			cpu:     container.Resources.Limits.Cpu().ToDec(),
			memory:  container.Resources.Limits.Memory().ToDec(),
			storage: capacity,
		}
		containerMetrics.limits[containerName] = rsrcs
	}

	return nil
}

// renderUsageTable renders a table displaying resource usage for pods.

// It returns:
// - Any error that occurred while rendering the table
func renderUsageTable(ctx context.Context, cfg *scout.Config, pods []corev1.Pod) error {
	columns := []table.Column{
		{Title: "Container", Width: 20},
		{Title: "Cores", Width: 10},
		{Title: "Usage(%)", Width: 10},
		{Title: "Memory", Width: 10},
		{Title: "Usage(%)", Width: 10},
		{Title: "Storage", Width: 10},
		{Title: "Usage(%)", Width: 10},
	}
	var rows []table.Row

	for _, pod := range pods {
		containerMetrics := &ContainerMetrics{pod.Name, map[string]Resources{}}
		podMetrics, err := kube.GetPodMetrics(ctx, cfg, pod)
		if err != nil {
			return errors.Wrap(err, "while attempting to fetch pod metrics")
		}

		if err = getLimits(ctx, cfg, &pod, containerMetrics); err != nil {
			return errors.Wrap(err, "failed to get get container metrics")
		}

		for _, container := range podMetrics.Containers {
			stats, err := getContainerUsageStats(ctx, cfg, *containerMetrics, pod, container)
			if err != nil {
				return errors.Wrapf(err, "could not compile usage data for row %s\n", container.Name)
			}

			row := makeRow(stats)
			rows = append(rows, row)
		}
	}

	style.UsageTable(columns, rows)
	return nil
}

// makeRow generates a table row containing resource usage data for a container.
// It returns:
// - A table.Row containing the resource usage information
// - An error if there was an issue generating the row
func makeRow(usageStats UsageStats) table.Row {
	if usageStats.storage == nil {
		return table.Row{
			usageStats.containerName,
			usageStats.cpuCores.String(),
			fmt.Sprintf("%.2f%%", usageStats.cpuUsage),
			usageStats.memory.String(),
			fmt.Sprintf("%.2f%%", usageStats.memoryUsage),
			"-",
			"-",
		}
	}

	return table.Row{
		usageStats.containerName,
		usageStats.cpuCores.String(),
		fmt.Sprintf("%.2f%%", usageStats.cpuUsage),
		usageStats.memory.String(),
		fmt.Sprintf("%.2f%%", usageStats.memoryUsage),
		usageStats.storage.String(),
		fmt.Sprintf("%.2f%%", usageStats.storageUsage),
	}
}

// makeUsageRow generates a table row containing resource usage data for a container.
//
// It returns:
// - A table.Row containing the resource usage information
// - An error if there was an issue generating the row
func getContainerUsageStats(
	ctx context.Context,
	cfg *scout.Config,
	metrics ContainerMetrics,
	pod corev1.Pod,
	container metav1beta1.ContainerMetrics,
) (UsageStats, error) {
	var usageStats UsageStats
	usageStats.containerName = container.Name

	cpuUsage, err := kube.GetRawUsage(container.Usage, "cpu")
	if err != nil {
		return UsageStats{}, errors.Wrap(err, "failed to get raw CPU usage")
	}

	memUsage, err := kube.GetRawUsage(container.Usage, "memory")
	if err != nil {
		return UsageStats{}, errors.Wrap(err, "failed to get raw memory usage")
	}

	var storageCapacity float64
	var storageUsage float64
	stateless := []string{
		"cadvisor",
		"pgsql-exporter",
		"executor",
		"dind",
		"github-proxy",
		"jaeger",
		"node-exporter",
		"otel-agent",
		"otel-collector",
		"precise-code-intel-worker",
		"redis-exporter",
		"repo-updater",
		"frontend",
		"syntect-server",
		"worker",
	}

	if contains(stateless, container.Name) {
		storageUsage = 0
		storageCapacity = 0
	} else {
		storageCapacity, storageUsage, err = kube.GetStorageUsage(ctx, cfg, pod.Name, container.Name)
		if err != nil {
			return UsageStats{}, errors.Wrap(err, "failed to get storage usage")
		}
	}

	limits := metrics.limits[container.Name]

	usageStats.cpuCores = limits.cpu
	usageStats.cpuUsage = getPercentage(
		cpuUsage,
		limits.cpu.AsApproximateFloat64()*ABillion,
	)

	usageStats.memory = limits.memory
	usageStats.memoryUsage = getPercentage(
		memUsage,
		limits.memory.AsApproximateFloat64(),
	)

	if limits.storage == nil {
		storageDec := *inf.NewDec(0, 0)
		usageStats.storage = resource.NewDecimalQuantity(storageDec, resource.Format("DecimalSI"))
	} else {
		usageStats.storage = limits.storage
	}

	usageStats.storageUsage = getPercentage(
		storageUsage,
		storageCapacity,
	)

	if metrics.limits[container.Name].storage == nil {
		usageStats.storage = nil
	}

	return usageStats, nil
}

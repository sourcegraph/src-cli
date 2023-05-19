package usage

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/scout/style"

	"gopkg.in/inf.v0"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type ContainerMetrics struct {
	pod    string
	limits map[string]Resources
}

type Resources struct {
	cpu     *resource.Quantity
	memory  *resource.Quantity
	storage *resource.Quantity
}

const (
	ABillion float64 = 1000000000
)

func K8s(
	ctx context.Context,
	clientSet *kubernetes.Clientset,
	metricsClient *metricsv.Clientset,
	rest *rest.Config,
	opts ...Option,
) error {
	cfg := &Config{
		namespace:     "default",
		docker:        false,
		pod:           "",
		container:     "",
		spy:           false,
		k8sClient:     clientSet,
		dockerClient:  nil,
		metricsClient: metricsClient,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return renderUsageTable(ctx, cfg)
}

func renderUsageTable(ctx context.Context, cfg *Config) error {
	podInterface := cfg.k8sClient.CoreV1().Pods(cfg.namespace)
	podList, err := podInterface.List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "error listing pods: ")
	}

	pods := podList.Items
	if len(pods) == 0 {
		msg := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
		fmt.Println(msg.Render(`
            No pods exist in this namespace.
            Did you mean to use the --namespace flag?

            If you are attemptying to check
            resources for a docker deployment, you
            must use the --docker flag.
            See --help for more info.
            `))
		os.Exit(1)
	}

	columns := []table.Column{
		{Title: "Container", Width: 20},
		{Title: "CPU Limits", Width: 10},
		{Title: "Usage(%)", Width: 13},
		{Title: "MEM Limits", Width: 10},
		{Title: "Usage(%)", Width: 13},
		{Title: "Capacity", Width: 13},
		{Title: "Usage(%)", Width: 16},
	}
	var rows []table.Row

	for _, pod := range pods {
		podName := pod.Name
		containerMetrics := &ContainerMetrics{podName, map[string]Resources{}}
		podMetrics, err := getPodMetrics(ctx, cfg, pod)
		if err != nil {
			return errors.Wrap(err, "error while getting pod metrics: ")
		}

		err = getLimits(ctx, cfg, &pod, containerMetrics)
		if err != nil {
			return errors.Wrap(err, "error while getting container limits: ")
		}

		for _, container := range podMetrics.Containers {
			limits := containerMetrics.limits[container.Name]

			cpuUsage, err := getRawUsage(container.Usage, "cpu")
			if err != nil {
				return errors.Wrap(err, "error while getting raw cpu usage: ")
			}

			memUsage, err := getRawUsage(container.Usage, "memory")
			if err != nil {
				return errors.Wrap(err, "error while getting raw memory usage: ")
			}

			cpuUsagePercent := getPercentage(
				cpuUsage,
				limits.cpu.AsApproximateFloat64()*ABillion,
			)

			memUsagePercent := getPercentage(
				memUsage,
				limits.memory.AsApproximateFloat64(),
			)

			storageVal := limits.storage.String()
			if containerMetrics.limits[container.Name].storage == nil {
				storageVal = "-"
			}

			row := table.Row{
				container.Name,
				containerMetrics.limits[container.Name].cpu.String(),
				fmt.Sprintf("%.2f%%", cpuUsagePercent),
				containerMetrics.limits[container.Name].memory.String(),
				fmt.Sprintf("%.2f%%", memUsagePercent),
				storageVal,
				":-)",
			}
			rows = append(rows, row)
		}
	}

	style.ResourceTable(columns, rows)
	return nil
}

// getPodMetrics retrieves metrics for a given pod.
// It accepts:
// - ctx: The context for the request
// - cfg: A config struct containing:
//   - metricsClient: A metrics clientset for accessing the Kubernetes Metrics API
//   - namespace: The namespace of the pod
// - pod: The pod object for which metrics should be retrieved
// It returns:
// - podMetrics: The PodMetrics object containing metrics for the pod
// - err: Any error that occurred while getting the pod metrics
func getPodMetrics(ctx context.Context, cfg *Config, pod v1.Pod) (*v1beta1.PodMetrics, error) {
	podMetrics, err := cfg.metricsClient.
		MetricsV1beta1().
		PodMetricses(cfg.namespace).
		Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "error while getting pod metrics: ")
	}

	return podMetrics, nil
}

// getLimits extracts resource limits for containers in a pod and stores
// them in a ContainerMetrics struct.
//
// It accepts:
// - pod: A Kubernetes pod object
// - containerMetrics: A ContainerMetrics struct to store the resource limits
//
// It populates the containerMetrics struct with:
// - The name of each container
// - The CPU, memory, ephemeral storage, and storage resource limits for each container
// - A print method to print the resource limits for each container
func getLimits(ctx context.Context, cfg *Config, pod *v1.Pod, containerMetrics *ContainerMetrics) error {
	for _, container := range pod.Spec.Containers {
		containerName := container.Name
		capacity, err := getPvcCapacity(ctx, cfg, container, pod)
		if err != nil {
			return errors.Wrap(err, "error while getting storage capacity of PV: ")
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

// getRawUsage converts a Kubernetes ResourceList (map[ResourceName]Quantity)
// into a raw float64 usage value for a given resource.
//
// It accepts:
// - usages: The ResourceList containing usage values
// - targetKey: The key for the target resource in the ResourceList (e.g. "cpu" or "memory")
//
// It returns:
// - The raw float64 usage value for the target resource
// - Any error that occurred during conversion
func getRawUsage(usages v1.ResourceList, targetKey string) (float64, error) {
	var usage *inf.Dec

	for key, val := range usages {
		if key.String() == targetKey {
			usage = val.AsDec().SetScale(0)
		}
	}

	toFloat, err := strconv.ParseFloat(usage.String(), 64)
	if err != nil {
		return 0, errors.Wrap(err, "error while converting inf.Dec to float")
	}

	return toFloat, nil
}

// getPvcCapacity retrieves the storage capacity of a PersistentVolumeClaim
// mounted as a volume by a container.
//
// It accepts:
// - ctx: The context for the request
// - cfg: A config struct containing:
//   - k8sClient: A Kubernetes clientset for accessing the API server
//   - namespace: The namespace of the pod
// - container: The container object which may have PVC mounts
// - pod: The pod object which contains the container
//
// It returns:
// - The capacity Quantity of the PVC if a matching PVC mount is found
// - nil if no PVC mount is found
// - Any error that occurred while getting the PVC
func getPvcCapacity(ctx context.Context, cfg *Config, container v1.Container, pod *v1.Pod) (*resource.Quantity, error) {
	for _, volumeMount := range container.VolumeMounts {
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == volumeMount.Name && volume.PersistentVolumeClaim != nil {
				pvc, err := cfg.k8sClient.CoreV1().PersistentVolumeClaims(cfg.namespace).Get(
					ctx,
					volume.PersistentVolumeClaim.ClaimName,
					metav1.GetOptions{},
				)
				if err != nil {
					return nil, errors.Wrapf(err, "error getting PVC %s", volume.PersistentVolumeClaim.ClaimName)
				}
				return pvc.Status.Capacity.Storage().ToDec(), nil
			}
		}
	}
	return nil, nil
}

// getPercentage calculates the percentage of x out of y.
//
// It accepts:
// - x: The numerator value
// - y: The denominator value
//
// It returns:
// - The percentage of x out of y, rounded to 2 decimal places.
// - 0 if either x or y are 0 to avoid division by 0.
func getPercentage(x, y float64) float64 {
	if x == 0 {
		return 0
	}

	if y == 0 {
		return 0
	}

	return x * 100 / y
}

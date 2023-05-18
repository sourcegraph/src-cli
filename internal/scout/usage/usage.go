package usage

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/scout/style"

	"gopkg.in/inf.v0"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Option = func(config *Config)
type Config struct {
	namespace     string
	pod           string
	container     string
	spy           bool
	docker        bool
	k8sClient     *kubernetes.Clientset
	dockerClient  *client.Client
	metricsClient *metricsv.Clientset
}

func WithNamespace(namespace string) Option {
	return func(config *Config) {
		config.namespace = namespace
	}
}

func WithPod(podname string) Option {
	return func(config *Config) {
		config.pod = podname
	}
}

func WithContainer(containerName string) Option {
	return func(config *Config) {
		config.container = containerName
	}
}

func WithSpy(spy bool) Option {
	return func(config *Config) {
		config.spy = true
	}
}

const (
	Billion = 1000000000
	Million = 1000000
)

func K8s(ctx context.Context, clientSet *kubernetes.Clientset, metricsClient *metricsv.Clientset, client *rest.Config, opts ...Option) error {
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

	return listPodUsage(ctx, cfg)
}

func listPodUsage(ctx context.Context, cfg *Config) error {
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
		{Title: "CPU Usage(%)", Width: 13},
		{Title: "MEM Limits", Width: 10},
		{Title: "MEM Usage(%)", Width: 13},
		{Title: "Disk Space", Width: 13},
		{Title: "Disk Used(%)", Width: 16},
	}

	var rows []table.Row
	for _, pod := range pods {
		podMetrics, err := cfg.metricsClient.
			MetricsV1beta1().
			PodMetricses(cfg.namespace).
			Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrap(err, "error while getting pod metrics: ")
		}

		var cpuLimits *resource.Quantity
		var memLimits *resource.Quantity
		var storageCapacity float64

		if pod.GetNamespace() == cfg.namespace {
			for _, container := range pod.Spec.Containers {
                if (container.Name == "psql-exporter") { 
                    continue
                }
				cpuLimits = container.Resources.Limits.Cpu()
				memLimits = container.Resources.Limits.Memory()

				storageCapacity, err = getPvcCapacity(ctx, cfg, container, pod)
				if err != nil {
					errors.Wrap(err, "error while getting storage capacity: ")
				}
			}

			for _, container := range podMetrics.Containers {
				cpuUsage, err := getRawUsage(container.Usage, "cpu")
				if err != nil {
					return errors.Wrap(err, "error while getting raw cpu usage: ")
				}

				memUsage, err := getRawUsage(container.Usage, "memory")
				if err != nil {
					return errors.Wrap(err, "error while getting raw memory usage: ")
				}

				diskUsage := "22%"

				fmt.Println(container.Name)
				cpuUsagePercent := getPercentage(
					cpuUsage,
					cpuLimits.AsApproximateFloat64()*Billion,
				)
				memUsagePercent := getPercentage(
					memUsage,
					memLimits.AsApproximateFloat64(),
				)

				// TODO convert to percentages before adding to the row
				row := table.Row{
					container.Name,
					fmt.Sprintf("%v", cpuLimits),
					fmt.Sprintf("%.2f%%", cpuUsagePercent),
					fmt.Sprintf("%v", memLimits),
					fmt.Sprintf("%.2f%%", memUsagePercent),
					fmt.Sprintf("%.2f", storageCapacity),
					diskUsage,
				}

				rows = append(rows, row)
			}
		}
	}

	style.ResourceTable(columns, rows)
	return nil
}

// getRawUsage returns the raw usage value for a given resource key in a ResourceList.
//
// usages is the ResourceList containing resource usages.
// targetKey is the key of the resource to get the usage for.
//
// The function returns the usage value for the given key and a nil error if found.
// If the key does not exist in the ResourceList, an error is returned.
//
// The usage value is returned as an int64. If the usage value in the ResourceList
// is a decimal, it is rounded down to an int64.
func getRawUsage(usages v1.ResourceList, targetKey string) (float64, error) {
	var usage *inf.Dec

	for key, val := range usages {
		if key.String() == targetKey {
			usage = val.AsDec().SetScale(0)
		}
	}

    toFloat, err := strconv.ParseFloat(usage.String(), 64)
    if err != nil {
        return 0, errors.Wrap(err, "error while convering inf.Dec to float")
    }
    
    return toFloat, nil
}

// getPvcCapacity returns the capacity in GiB of the PersistentVolumeClaim
// associated with the given volumeMount in the container. If no PVC is found,
// -1 and no error is returned.
func getPvcCapacity(ctx context.Context, cfg *Config, container v1.Container, pod v1.Pod) (float64, error) {
	for _, volumeMount := range container.VolumeMounts {
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == volumeMount.Name && volume.PersistentVolumeClaim != nil {
				pvc, err := cfg.k8sClient.CoreV1().PersistentVolumeClaims(cfg.namespace).Get(
					ctx,
					volume.PersistentVolumeClaim.ClaimName,
					metav1.GetOptions{},
				)
				if err != nil {
					return -1, errors.Wrapf(err, "error getting PVC %s", volume.PersistentVolumeClaim.ClaimName)
				}
				return pvc.Status.Capacity.Storage().AsApproximateFloat64(), nil
			}
		}
	}
	return -1, nil
}

func iterateOverResourceList(rl v1.ResourceList) {
	for k, v := range rl {
		fmt.Printf("%v: %v\n", k, v)
	}
	fmt.Printf("\n")
}

// Calculates the percentage of x out of y.
//
// Returns the percentage of x out of y. If x is 0, returns 0. If y is 0, returns -1.
// Otherwise, returns x * 100 / y.
//
// Prints the values of x and y before calculating the percentage.
func getPercentage(x, y float64) float64 {
	if x == 0 {
		return 0
	}
    fmt.Printf("\tx: %v\n", x)
    fmt.Printf("\ty: %v\n", y)

	if y == 0 {
		return 0
	}

	return x * 100 / y
}

func Docker(ctx context.Context, client client.Client, opts ...Option) error {
	cfg := &Config{
		namespace:     "default",
		docker:        true,
		pod:           "",
		container:     "",
		spy:           false,
		k8sClient:     nil,
		dockerClient:  &client,
		metricsClient: nil,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	fmt.Println("docker works!")
	fmt.Printf("config: %v", &cfg)
	return nil
}

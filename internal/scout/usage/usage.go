package usage

import (
	"context"
	"fmt"
	"os"

	// "os"

	// "github.com/charmbracelet/bubbles/table"
	// "github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/scout/style"

	// "github.com/sourcegraph/src-cli/internal/scout/style"
	"k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/api/resource"
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
		{Title: "CPU Usage", Width: 12},
		{Title: "MEM Limits", Width: 10},
		{Title: "MEM Usage", Width: 12},
		{Title: "Storage Used", Width: 8},
	}

	var rows []table.Row
	for _, pod := range pods {
		podMetrics, err := cfg.metricsClient.
			MetricsV1beta1().
			PodMetricses(cfg.namespace).
			Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrap(err, "error while getting pod metrics")
		}

		var availableDiskSpace string
		var cpuLimits *resource.Quantity
		var memLimits *resource.Quantity
		var cpuUsage resource.Quantity
		var memUsage resource.Quantity

		if pod.GetNamespace() == cfg.namespace {
			for _, container := range pod.Spec.Containers {
				cpuLimits = container.Resources.Limits.Cpu()
				memLimits = container.Resources.Limits.Memory()
			}

			for _, container := range podMetrics.Containers {
				cpuUsage = container.Usage[v1.ResourceCPU]
				memUsage = container.Usage[v1.ResourceMemory]
                // FIXME this isn't returning anything.
				availableBytes := container.Usage[v1.ResourceStorage]
				availableDiskSpace = availableBytes.String()

				// TODO convert to percentages before adding to the row
				row := table.Row{
					container.Name,
					cpuLimits.String(),
					cpuUsage.String(),
					memLimits.String(),
					memUsage.String(),
					availableDiskSpace,
				}

				rows = append(rows, row)
			}

		}
	}

	style.ResourceTable(columns, rows)
	return nil
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

func getPercentage(x, y float64) (float64, error) {
	if x == 0 {
		return 0, nil
	}

	if y == 0 {
		return -1, errors.New("cannot divide by 0")
	}

	return x * 100 / y, nil
}

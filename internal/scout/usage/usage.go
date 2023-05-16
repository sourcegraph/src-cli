package usage

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	"github.com/sourcegraph/src-cli/internal/scout/style"
	"k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/api/resource"
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

func K8s(ctx context.Context, clientSet *kubernetes.Clientset, client *rest.Config, opts ...Option) error {
	cfg := &Config{
		namespace:     "default",
		docker:        false,
		pod:           "",
		container:     "",
		spy:           false,
		k8sClient:     clientSet,
		dockerClient:  nil,
		metricsClient: &metricsv.Clientset{},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return listPodUsage(ctx, cfg)
}

func listPodUsage(ctx context.Context, cfg *Config) error {
	podInterface := cfg.k8sClient.CoreV1().Pods(cfg.namespace)
	pods, err := podInterface.List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "error listing pods: ")
	}

	columns := []table.Column{
		{Title: "Container", Width: 20},
		{Title: "CPU Limits", Width: 10},
		{Title: "CPU Usage", Width: 12},
		{Title: "MEM Limits", Width: 10},
		{Title: "MEM Usage", Width: 12},
		{Title: "Storage Used", Width: 8},
	}

	if len(pods.Items) == 0 {
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

	var rows []table.Row
	for _, pod := range pods.Items {
		rawMetrics := cfg.metricsClient.MetricsV1beta1().PodMetricses(cfg.namespace) //.Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return errors.Wrap(err, "error while getting pod metrics")
		}
        fmt.Println(pod.Name, rawMetrics) 
		/* for _, container := range rawMetrics.Containers {
			cpuUsage := container.Usage[v1.ResourceCPU]
			memUsage := container.Usage[v1.ResourceMemory]

			var cpuLimit resource.Quantity
			var memLimit resource.Quantity
			var availableDiskSpace string

			for _, podContainer := range pod.Spec.Containers {
				if podContainer.Name == container.Name {
					cpuLimit = podContainer.Resources.Limits[v1.ResourceCPU]
					memLimit = podContainer.Resources.Limits[v1.ResourceMemory]
					availableBytes := container.Usage[v1.ResourceStorage]
					availableDiskSpace = availableBytes.String()
				}
			}

			cpuUsageFraction := float64(cpuUsage.MilliValue()) / float64(cpuLimit.MilliValue())
			memUsageFraction := float64(memUsage.Value()) / float64(memLimit.Value())

			row := table.Row{
				pod.Name,
				cpuLimit.String(),
				fmt.Sprintf("%.2f", cpuUsageFraction),
				memLimit.String(),
				fmt.Sprintf("%.2f", memUsageFraction),
				availableDiskSpace,
			}

			rows = append(rows, row)
		} */
	}

	style.ResourceTable(columns, rows)
	return nil
}

func getAvailableDiskSpace(ctx context.Context, clientSet kubernetes.Clientset, metricsClient metricsv.Clientset, namespace string, pod v1.Pod) (string, error) {
	// Retrieve available disk space metric for the pod
	metricsClientset := metricsClient.MetricsV1beta1().PodMetricses(namespace)
	podMetrics, err := metricsClientset.Get(context.TODO(), pod.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error retrieving pod metrics: %v", err)
	}

	// Find available disk space metric for the pod
	var availableDiskSpace string
	for _, container := range podMetrics.Containers {
		if container.Name == pod.Name {
			// Retrieve the available disk space metric for the container
			availableBytes := container.Usage[v1.ResourceEphemeralStorage]
			availableDiskSpace = availableBytes.String()
			break
		}
	}

	return availableDiskSpace, nil
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

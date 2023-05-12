package resource

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sourcegraph/sourcegraph/lib/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Option = func(config *Config)

type Config struct {
	namespace    string
	docker       bool
	k8sClient    *kubernetes.Clientset
	dockerClient *client.Client
}

func WithNamespace(namespace string) Option {
	return func(config *Config) {
		config.namespace = namespace
	}
}

// K8s prints the CPU and memory resource limits and requests for all pods in the given namespace.
func K8s(ctx context.Context, clientSet *kubernetes.Clientset, restConfig *rest.Config, opts ...Option) error {
	cfg := &Config{
		namespace:    "default",
		docker:       false,
		k8sClient:    clientSet,
		dockerClient: nil,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return listPodResources(ctx, cfg)
}

func listPodResources(ctx context.Context, cfg *Config) error {
	podInterface := cfg.k8sClient.CoreV1().Pods(cfg.namespace)
	podList, err := podInterface.List(ctx, metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "error listing pods: ")
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() {
		_ = w.Flush()
	}()

	fmt.Fprintln(w, "CONTAINER\tCPU LIMITS\tCPU REQUESTS\tMEM LIMITS\tMEM REQUESTS\tCAPACITY")

	for _, pod := range podList.Items {
		if pod.GetNamespace() == cfg.namespace {
			for _, container := range pod.Spec.Containers {
				cpuLimits := container.Resources.Limits.Cpu()
				cpuRequests := container.Resources.Requests.Cpu()
				memLimits := container.Resources.Limits.Memory()
				memRequests := container.Resources.Requests.Memory()

				capacity, err := getPVCCapacity(ctx, cfg, container, pod)
				if err != nil {
					return err
				}

				fmt.Fprintf(
					w,
					"%s\t%s\t%s\t%s\t%s\t%s\t\n",
					container.Name,
					cpuLimits,
					cpuRequests,
					memLimits,
					memRequests,
					capacity,
				)
			}
		}
	}

	return nil
}

func getPVCCapacity(ctx context.Context, cfg *Config, container v1.Container, pod v1.Pod) (string, error) {
	for _, volumeMount := range container.VolumeMounts {
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == volumeMount.Name && volume.PersistentVolumeClaim != nil {
				pvc, err := cfg.k8sClient.CoreV1().PersistentVolumeClaims(cfg.namespace).Get(
					ctx,
					volume.PersistentVolumeClaim.ClaimName,
					metav1.GetOptions{},
				)
				if err != nil {
					return "", errors.Wrapf(err, "error getting PVC %s", volume.PersistentVolumeClaim.ClaimName)
				}
				return pvc.Status.Capacity.Storage().String(), nil
			}
		}
	}
	return "", nil
}

// Docker prints the CPU and memory resource limits and requests for running Docker containers.
func Docker(ctx context.Context, dockerClient client.Client) error {
	containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{})
	if err != nil {
		return fmt.Errorf("error listing docker containers: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer func() {
		_ = w.Flush()
	}()

	fmt.Fprintln(w, "Container\tCPU Cores\tCPU Shares\tMem Limits\tMem Reservations")

	for _, container := range containers {
		containerInfo, err := dockerClient.ContainerInspect(ctx, container.ID)
		if err != nil {
			return fmt.Errorf("error inspecting container %s: %v", container.ID, err)
		}

		getResourceInfo(&containerInfo, w)
	}

	return nil
}

// getMemUnits converts a byte value to the appropriate memory unit.
func getMemUnits(valToConvert int64) (string, int64, error) {
	if valToConvert < 0 {
		return "", valToConvert, fmt.Errorf("invalid memory value: %d", valToConvert)
	}

	var memUnit string
	switch {
	case valToConvert < 1000000:
		memUnit = "KB"
	case valToConvert < 1000000000:
		memUnit = "MB"
		valToConvert = valToConvert / 1000000
	default:
		memUnit = "GB"
		valToConvert = valToConvert / 1000000000
	}

	return memUnit, valToConvert, nil
}

func getResourceInfo(container *types.ContainerJSON, w *tabwriter.Writer) error {
	cpuCores := container.HostConfig.NanoCPUs
	cpuShares := container.HostConfig.CPUShares
	memLimits := container.HostConfig.Memory
	memReservations := container.HostConfig.MemoryReservation

	limUnit, limVal, err := getMemUnits(memLimits)
	if err != nil {
		return errors.Wrap(err, "error while getting limit units")
	}

	reqUnit, reqVal, err := getMemUnits(memReservations)
	if err != nil {
		return errors.Wrap(err, "error while getting request units")
	}

	fmt.Fprintf(
		w,
		"%s\t%d\t%v\t%v\t%v\n",
		container.Name,
		cpuCores/1e9,
		cpuShares,
		fmt.Sprintf("%d %s", limVal, limUnit),
		fmt.Sprintf("%d %s", reqVal, reqUnit),
	)
	return nil
}
